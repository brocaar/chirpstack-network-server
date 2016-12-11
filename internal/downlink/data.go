package downlink

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"

	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/api/nc"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/maccommand"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/session"
	"github.com/brocaar/lorawan"
)

func getDataDownTXInfoAndDR(ctx common.Context, ns session.NodeSession, rxInfo gw.RXInfo) (gw.TXInfo, int, error) {
	var dr int
	txInfo := gw.TXInfo{
		MAC:      rxInfo.MAC,
		CodeRate: rxInfo.CodeRate,
		Power:    common.Band.DefaultTXPower,
	}

	if ns.RXWindow == session.RX1 {
		uplinkDR, err := common.Band.GetDataRate(rxInfo.DataRate)
		if err != nil {
			return txInfo, dr, err
		}

		// get rx1 dr
		dr, err = common.Band.GetRX1DataRate(uplinkDR, int(ns.RX1DROffset))
		if err != nil {
			return txInfo, dr, err
		}
		txInfo.DataRate = common.Band.DataRates[dr]

		// get rx1 frequency
		txInfo.Frequency, err = common.Band.GetRX1Frequency(rxInfo.Frequency)
		if err != nil {
			return txInfo, dr, err
		}

		// get timestamp
		txInfo.Timestamp = rxInfo.Timestamp + uint32(common.Band.ReceiveDelay1/time.Microsecond)
		if ns.RXDelay > 0 {
			txInfo.Timestamp = rxInfo.Timestamp + uint32(time.Duration(ns.RXDelay)*time.Second/time.Microsecond)
		}
	} else if ns.RXWindow == session.RX2 {
		// rx2 dr
		dr = int(ns.RX2DR)
		if dr > len(common.Band.DataRates)-1 {
			return txInfo, 0, fmt.Errorf("invalid rx2 dr: %d (max dr: %d)", dr, len(common.Band.DataRates)-1)
		}
		txInfo.DataRate = common.Band.DataRates[dr]

		// rx2 frequency
		txInfo.Frequency = common.Band.RX2Frequency

		// rx2 timestamp (rx1 + 1 sec)
		txInfo.Timestamp = rxInfo.Timestamp + uint32(common.Band.ReceiveDelay1/time.Microsecond)
		if ns.RXDelay > 0 {
			txInfo.Timestamp = rxInfo.Timestamp + uint32(time.Duration(ns.RXDelay)*time.Second/time.Microsecond)
		}
		txInfo.Timestamp = txInfo.Timestamp + uint32(time.Second/time.Microsecond)
	} else {
		return txInfo, dr, fmt.Errorf("unknown RXWindow option %d", ns.RXWindow)
	}

	return txInfo, dr, nil
}

// getDataDownFromApplication gets the downlink data from the application
// (if any). On error the error is logged.
func getDataDownFromApplication(ctx common.Context, ns session.NodeSession, dr int) *as.GetDataDownResponse {
	resp, err := ctx.Application.GetDataDown(context.Background(), &as.GetDataDownRequest{
		AppEUI:         ns.AppEUI[:],
		DevEUI:         ns.DevEUI[:],
		MaxPayloadSize: uint32(common.Band.MaxPayloadSize[dr].N),
		FCnt:           ns.FCntDown,
	})
	if err != nil {
		log.WithFields(log.Fields{
			"dev_eui": ns.DevEUI,
			"fcnt":    ns.FCntDown,
		}).Errorf("get data down from application error: %s", err)
		return nil
	}

	if resp == nil || resp.FPort == 0 {
		return nil
	}

	if len(resp.Data) > common.Band.MaxPayloadSize[dr].N {
		log.WithFields(log.Fields{
			"dev_eui":          ns.DevEUI,
			"size":             len(resp.Data),
			"max_payload_size": common.Band.MaxPayloadSize[dr].N,
			"dr":               dr,
		}).Warning("data down from application exceeds max payload size")
		return nil
	}

	log.WithFields(log.Fields{
		"dev_eui":     ns.DevEUI,
		"fcnt":        ns.FCntDown,
		"data_base64": base64.StdEncoding.EncodeToString(resp.Data),
		"confirmed":   resp.Confirmed,
		"more_data":   resp.MoreData,
	}).Info("received data down from application")

	return resp
}

// SendDataDownResponse sends the data-down response to an uplink packet.
// A downlink response happens when: there is data in the downlink queue,
// there are MAC commmands to send and / or when the uplink packet was of
// type ConfirmedDataUp, so an ACK response is needed.
func SendDataDownResponse(ctx common.Context, ns session.NodeSession, rxPacket models.RXPacket) error {
	macPL, ok := rxPacket.PHYPayload.MACPayload.(*lorawan.MACPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.MACPayload, got: %T", rxPacket.PHYPayload.MACPayload)
	}

	var frmMACCommands bool
	var macPayloads []maccommand.QueueItem

	// get data down tx properties
	txInfo, dr, err := getDataDownTXInfoAndDR(ctx, ns, rxPacket.RXInfoSet[0])
	if err != nil {
		return fmt.Errorf("get data down txinfo error: %s", err)
	}

	// get data down from application-server (if it has anything in its queue)
	txPayload := getDataDownFromApplication(ctx, ns, dr)

	// read the mac payload queue
	allMACPayloads, err := maccommand.ReadQueue(ctx.RedisPool, ns.DevAddr)
	if err != nil {
		return fmt.Errorf("read mac-payload tx queue error: %s", err)
	}

	// get the mac commands from the queue (if any) and filter them so that the
	// frmpayload + mac commands don't exceed the max macpayload size.
	if len(allMACPayloads) > 0 {
		if txPayload != nil {
			maxFOptsLen := common.Band.MaxPayloadSize[dr].N - len(txPayload.Data)
			if maxFOptsLen > 15 {
				maxFOptsLen = 15 // max FOpts len is 15
			}

			macPayloads = maccommand.FilterItems(allMACPayloads, false, maxFOptsLen)
		} else if allMACPayloads[0].FRMPayload {
			// the first mac-commands must be sent as FRMPayload, filter the rest
			// of the MACPayload items with the same property, respecting the
			// max FRMPayload size for the data-rate.
			frmMACCommands = true
			macPayloads = maccommand.FilterItems(allMACPayloads, true, common.Band.MaxPayloadSize[dr].N)
		} else {
			// the first mac-command must be sent as FOpts, filter the rest of
			// the MACPayload items with the same property, respecting the
			// max FOpts size of 15.
			maxFOptsLen := common.Band.MaxPayloadSize[dr].N
			if maxFOptsLen > 15 {
				maxFOptsLen = 15 // max FOpts len is 15
			}
			macPayloads = maccommand.FilterItems(allMACPayloads, false, maxFOptsLen)
		}
	}

	// convert the MACPayload items into MACCommand items
	var macCommands []lorawan.MACCommand
	for _, pl := range macPayloads {
		var mac lorawan.MACCommand
		if err := mac.UnmarshalBinary(false, pl.Data); err != nil {
			// in case the mac commands can't be unmarshaled, the payload
			// is ignored and an error sent to the network-controller
			errStr := fmt.Sprintf("unmarshal mac command error: %s", err)
			log.WithFields(log.Fields{
				"dev_eui": ns.DevEUI,
				"command": hex.EncodeToString(pl.Data),
			}).Warning(errStr)
			ctx.Controller.HandleError(context.Background(), &nc.HandleErrorRequest{
				AppEUI: ns.AppEUI[:],
				DevEUI: ns.DevEUI[:],
				Error:  errStr + fmt.Sprintf(" (command: %X)", pl.Data),
			})
			continue
		}
		macCommands = append(macCommands, mac)
	}

	// uplink was unconfirmed and no downlink data in queue and no mac commands to send
	if txPayload == nil && rxPacket.PHYPayload.MHDR.MType == lorawan.UnconfirmedDataUp && len(macCommands) == 0 {
		return nil
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
				ADR:      ns.ADRInterval != 0,
				ACK:      rxPacket.PHYPayload.MHDR.MType == lorawan.ConfirmedDataUp,                           // set ACK when uplink packet was of type ConfirmedDataUp
				FPending: (txPayload != nil && txPayload.MoreData) || len(allMACPayloads) != len(macPayloads), // items in the queue or not all mac commands being sent
			},
			FCnt: ns.FCntDown,
		},
	}
	phy.MACPayload = macPL

	if len(macCommands) > 0 {
		if frmMACCommands {
			var fPort uint8 // 0
			var frmPayload []lorawan.Payload
			for i := range macCommands {
				frmPayload = append(frmPayload, &macCommands[i])
			}
			macPL.FPort = &fPort
			macPL.FRMPayload = frmPayload

			// encrypt the FRMPayload with the NwkSKey
			if err := phy.EncryptFRMPayload(ns.NwkSKey); err != nil {
				return fmt.Errorf("encrypt FRMPayload error: %s", err)
			}
		} else {
			macPL.FHDR.FOpts = macCommands
		}
	}

	// add the payload to FRMPayload field
	// note that txPayload is by definition nil when there are mac commands
	// to send in the FRMPayload field.
	if txPayload != nil {
		if txPayload.Confirmed {
			phy.MHDR.MType = lorawan.ConfirmedDataDown
		}

		fPort := uint8(txPayload.FPort)
		macPL.FPort = &fPort
		macPL.FRMPayload = []lorawan.Payload{
			&lorawan.DataPayload{Bytes: txPayload.Data},
		}
	}

	if err := phy.SetMIC(ns.NwkSKey); err != nil {
		return fmt.Errorf("set MIC error: %s", err)
	}

	// send the TXPacket to the gateway
	if err := ctx.Gateway.SendTXPacket(gw.TXPacket{
		TXInfo:     txInfo,
		PHYPayload: phy,
	}); err != nil {
		return fmt.Errorf("send tx packet to gateway error: %s", err)
	}

	// increment the FCntDown when MType != ConfirmedDataDown.
	// In case of ConfirmedDataDown we increment on ACK (handled in uplink).
	if phy.MHDR.MType != lorawan.ConfirmedDataDown {
		// increment FCntDown
		ns.FCntDown++
		if err = session.SaveNodeSession(ctx.RedisPool, ns); err != nil {
			return err
		}
	}

	// remove the transmitted mac commands from the queue
	for _, pl := range macPayloads {
		if err = maccommand.DeleteQueueItem(ctx.RedisPool, ns.DevAddr, pl); err != nil {
			return fmt.Errorf("delete mac-payload from tx queue error: %s", err)
		}
	}

	return nil
}
