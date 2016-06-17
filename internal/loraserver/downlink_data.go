package loraserver

import (
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
)

type dataDownProperties struct {
	rx1Channel int
	rx1DR      int
	rxDelay    time.Duration
}

func getDataDownProperties(rxInfo models.RXInfo, ns models.NodeSession) (dataDownProperties, error) {
	var err error
	var prop dataDownProperties

	// get TX DR
	uplinkDR, err := Band.GetDataRate(rxInfo.DataRate)
	if err != nil {
		return prop, err
	}

	// get TX channel
	uplinkChannel, err := Band.GetChannel(rxInfo.Frequency, uplinkDR)
	if err != nil {
		return prop, err
	}

	// get RX1 channel
	prop.rx1Channel = Band.GetRX1Channel(uplinkChannel)

	// get RX1 DR
	prop.rx1DR, err = Band.GetRX1DataRateForOffset(uplinkDR, int(ns.RX1DROffset))
	if err != nil {
		return prop, err
	}

	// get rx delay
	prop.rxDelay = Band.ReceiveDelay1
	if ns.RXDelay > 0 {
		prop.rxDelay = time.Duration(ns.RXDelay) * time.Second
	}

	return prop, nil
}

// getNextValidTXPayloadForDRFromQueue returns the next valid TXPayload from the
// queue for the given data-rate. When it exceeds the max size, the payload will
// be discarded and a notification will be sent to the application.
func getNextValidTXPayloadForDRFromQueue(ctx Context, ns models.NodeSession, dataRate int) (*models.TXPayload, error) {
	for {
		txPayload, err := getTXPayloadFromQueue(ctx.RedisPool, ns.DevEUI)
		if err != nil {
			if err == errEmptyQueue {
				return nil, nil
			}
			return nil, err
		}

		if len(txPayload.Data) <= Band.MaxPayloadSize[dataRate].N {
			return &txPayload, nil
		}

		// the payload exceeded the max payload size for the current data-rate
		// we'll remove the payload from the queue and notify the application
		if _, err = clearInProcessTXPayload(ctx.RedisPool, ns.DevEUI); err != nil {
			return nil, err
		}

		// log a warning
		log.WithFields(log.Fields{
			"dev_eui":             ns.DevEUI,
			"data_rate":           dataRate,
			"frmpayload_size":     len(txPayload.Data),
			"max_frmpayload_size": Band.MaxPayloadSize[dataRate].N,
			"reference":           txPayload.Reference,
		}).Warning("downlink payload max size exceeded")

		// notify the application
		err = ctx.Application.SendNotification(ns.AppEUI, ns.DevEUI, models.ErrorNotificationType, models.ErrorPayload{
			Reference: txPayload.Reference,
			DevEUI:    ns.DevEUI,
			Message:   fmt.Sprintf("downlink payload max size exceeded (dr: %d, allowed: %d, got: %d)", dataRate, Band.MaxPayloadSize[dataRate].N, len(txPayload.Data)),
		})
		if err != nil {
			return nil, err
		}
	}
}

func handleDataDownReply(ctx Context, rxPacket models.RXPacket, ns models.NodeSession) error {
	macPL, ok := rxPacket.PHYPayload.MACPayload.(*lorawan.MACPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.MACPayload, got: %T", rxPacket.PHYPayload.MACPayload)
	}

	// get data down properies
	properties, err := getDataDownProperties(rxPacket.RXInfo, ns)
	if err != nil {
		return fmt.Errorf("get data down properties error: %s", err)
	}

	var frmMACCommands bool
	var macPayloads []models.MACPayload
	allMACPayloads, err := readMACPayloadTXQueue(ctx.RedisPool, ns.DevAddr)
	if err != nil {
		return fmt.Errorf("read mac-payload tx queue error: %s", err)
	}

	if len(allMACPayloads) > 0 {
		if allMACPayloads[0].FRMPayload {
			// the first mac-commands must be sent as FRMPayload, filter the rest
			// of the MACPayload items with the same property, respecting the
			// max FRMPayload size for the data-rate.
			frmMACCommands = true
			macPayloads = filterMACPayloads(allMACPayloads, true, Band.MaxPayloadSize[properties.rx1DR].N)
		} else {
			// the first mac-command must be sent as FOpts, filter the rest of
			// the MACPayload items with the same property, respecting the
			// max FOpts size of 15.
			macPayloads = filterMACPayloads(allMACPayloads, false, 15)
		}
	}

	// if the MACCommands (if any) are not sent as FRMPayload, check if there
	// is a tx-payload in the queue and validate if the FOpts + FRMPayload
	// does not exceed the max payload size.
	var txPayload *models.TXPayload
	if !frmMACCommands {
		// check if there are payloads pending in the queue
		txPayload, err = getNextValidTXPayloadForDRFromQueue(ctx, ns, properties.rx1DR)
		if err != nil {
			return fmt.Errorf("get next valid tx-payload error: %s", err)
		}

		var macByteCount int
		for _, mac := range macPayloads {
			macByteCount += len(mac.MACCommand)
		}

		if txPayload != nil && len(txPayload.Data)+macByteCount > Band.MaxPayloadSize[properties.rx1DR].N {
			log.WithFields(log.Fields{
				"data_rate": properties.rx1DR,
				"dev_eui":   ns.DevEUI,
				"reference": txPayload.Reference,
			}).Info("scheduling tx-payload for next downlink, mac-commands + payload exceeds max size")
			txPayload = nil
		}
	}

	// convert the MACPayload items into MACCommand items
	var macCommmands []lorawan.MACCommand
	for _, pl := range macPayloads {
		var mac lorawan.MACCommand
		if err := mac.UnmarshalBinary(false, pl.MACCommand); err != nil {
			// in case the mac commands can't be unmarshaled, the payload
			// is ignored and an error sent to the network-controller
			errStr := fmt.Sprintf("unmarshal mac command error: %s", err)
			log.WithFields(log.Fields{
				"dev_eui":   ns.DevEUI,
				"reference": pl.Reference,
			}).Warning(errStr)
			err = ctx.Controller.SendErrorPayload(ns.AppEUI, ns.DevEUI, models.ErrorPayload{
				Reference: pl.Reference,
				DevEUI:    ns.DevEUI,
				Message:   errStr,
			})
			if err != nil {
				return fmt.Errorf("send error payload to network-controller error: %s", err)
			}
			continue
		}
		macCommmands = append(macCommmands, mac)
	}

	// uplink was unconfirmed and no downlink data in queue and no mac commands to send
	if txPayload == nil && rxPacket.PHYPayload.MHDR.MType == lorawan.UnconfirmedDataUp && len(macCommmands) == 0 {
		return nil
	}

	// get the queue size (the size includes the current payload)
	queueSize, err := getTXPayloadQueueSize(ctx.RedisPool, ns.DevEUI)
	if err != nil {
		return err
	}
	if txPayload != nil {
		queueSize-- // substract the current tx-payload from the queue-size
	}

	phy := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: lorawan.UnconfirmedDataDown,
			Major: lorawan.LoRaWANR1,
		},
	}
	macPL = &lorawan.MACPayload{
		FHDR: lorawan.FHDR{
			DevAddr: ns.DevAddr,
			FCtrl: lorawan.FCtrl{
				ACK:      rxPacket.PHYPayload.MHDR.MType == lorawan.ConfirmedDataUp, // set ACK when uplink packet was of type ConfirmedDataUp
				FPending: queueSize > 0 || len(allMACPayloads) != len(macPayloads),  // items in the queue or not all mac commands being sent
			},
			FCnt: ns.FCntDown,
		},
	}
	phy.MACPayload = macPL

	if len(macCommmands) > 0 {
		if frmMACCommands {
			var fPort uint8 // 0
			var frmPayload []lorawan.Payload
			for _, pl := range macCommmands {
				frmPayload = append(frmPayload, &pl)
			}
			macPL.FPort = &fPort
			macPL.FRMPayload = frmPayload
		} else {
			macPL.FHDR.FOpts = macCommmands
		}
	}

	// add the payload to FRMPayload field
	// note that txPayload is by definition nil when there are mac commands
	// to send in the FRMPayload field.
	if txPayload != nil {
		if txPayload.Confirmed {
			phy.MHDR.MType = lorawan.ConfirmedDataDown
		}

		macPL.FPort = &txPayload.FPort
		macPL.FRMPayload = []lorawan.Payload{
			&lorawan.DataPayload{Bytes: txPayload.Data},
		}
	}

	// if there is no payload set, encrypt will just do nothing
	if err := phy.EncryptFRMPayload(ns.AppSKey); err != nil {
		return fmt.Errorf("encrypt FRMPayload error: %s", err)
	}

	if err := phy.SetMIC(ns.NwkSKey); err != nil {
		return fmt.Errorf("set MIC error: %s", err)
	}

	txPacket := models.TXPacket{
		TXInfo: models.TXInfo{
			MAC:       rxPacket.RXInfo.MAC,
			Timestamp: rxPacket.RXInfo.Timestamp + uint32(properties.rxDelay/time.Microsecond),
			Frequency: Band.DownlinkChannels[properties.rx1Channel].Frequency,
			Power:     Band.DefaultTXPower,
			DataRate:  Band.DataRates[properties.rx1DR],
			CodeRate:  rxPacket.RXInfo.CodeRate,
		},
		PHYPayload: phy,
	}

	// window 1
	if err := ctx.Gateway.SendTXPacket(txPacket); err != nil {
		return fmt.Errorf("send tx packet (rx window 1) to gateway error: %s", err)
	}

	// increment the FCntDown when MType != ConfirmedDataDown and clear
	// in-process queue. In case of ConfirmedDataDown we increment on ACK.
	if phy.MHDR.MType != lorawan.ConfirmedDataDown {
		ns.FCntDown++
		if err = saveNodeSession(ctx.RedisPool, ns); err != nil {
			return err
		}

		if _, err = clearInProcessTXPayload(ctx.RedisPool, ns.DevEUI); err != nil {
			return err
		}
	}

	// remove the mac commands from the queue
	for _, pl := range macPayloads {
		if err = deleteMACPayloadFromTXQueue(ctx.RedisPool, ns.DevAddr, pl); err != nil {
			return fmt.Errorf("delete mac-payload from tx queue error: %s", err)
		}
	}

	return nil
}
