package loraserver

import (
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
)

func handleDataDownReply(ctx Context, rxPacket models.RXPacket, ns models.NodeSession) error {
	macPL, ok := rxPacket.PHYPayload.MACPayload.(*lorawan.MACPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.MACPayload, got: %T", rxPacket.PHYPayload.MACPayload)
	}

	// the last payload was received by the node
	if macPL.FHDR.FCtrl.ACK {
		txPayload, err := clearInProcessTXPayload(ctx.RedisPool, ns.DevEUI)
		if err != nil {
			return err
		}
		ns.FCntDown++
		if err = saveNodeSession(ctx.RedisPool, ns); err != nil {
			return err
		}
		if txPayload != nil {
			err = ctx.Application.SendNotification(ns.AppEUI, ns.DevEUI, models.ACKNotificationType, models.ACKNotification{
				Reference: txPayload.Reference,
				DevEUI:    ns.DevEUI,
			})
			if err != nil {
				return err
			}
		}
	}

	// check if there are payloads pending in the queue
	txPayload, remaining, err := getTXPayloadAndRemainingFromQueue(ctx.RedisPool, ns.DevEUI)
	if err != nil {
		return err
	}

	// nothing pending in the queue and no need to ACK RXPacket
	if rxPacket.PHYPayload.MHDR.MType != lorawan.ConfirmedDataUp && txPayload == nil {
		return nil
	}

	// get TX DR
	uplinkDR, err := Band.GetDataRate(rxPacket.RXInfo.DataRate)
	if err != nil {
		return err
	}
	// get TX channel
	uplinkChannel, err := Band.GetChannel(rxPacket.RXInfo.Frequency, uplinkDR)
	if err != nil {
		return err
	}
	// get RX1 channel
	rx1Channel := Band.GetRX1Channel(uplinkChannel)
	// get RX1 DR
	rx1DR, err := Band.GetRX1DataRateForOffset(uplinkDR, int(ns.RX1DROffset))
	if err != nil {
		return err
	}
	// get rx delay
	rxDelay := Band.ReceiveDelay1
	if ns.RXDelay > 0 {
		rxDelay = time.Duration(ns.RXDelay) * time.Second
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
				ACK: rxPacket.PHYPayload.MHDR.MType == lorawan.ConfirmedDataUp, // set ACK to true when received packet needs an ACK
			},
			FCnt: ns.FCntDown,
		},
	}
	phy.MACPayload = macPL

	// add the payload from the queue
	if txPayload != nil {
		// validate the max payload size
		if len(txPayload.Data) > Band.MaxPayloadSize[rx1DR].N {
			// remove the payload from the queue regarding confirmed or not
			if _, err := clearInProcessTXPayload(ctx.RedisPool, ns.DevEUI); err != nil {
				return err
			}

			log.WithFields(log.Fields{
				"dev_eui":             ns.DevEUI,
				"data_rate":           rx1DR,
				"frmpayload_size":     len(txPayload.Data),
				"max_frmpayload_size": Band.MaxPayloadSize[rx1DR].N,
			}).Warning("downlink payload max size exceeded")
			err = ctx.Application.SendNotification(ns.AppEUI, ns.DevEUI, models.ErrorNotificationType, models.ErrorPayload{
				Reference: txPayload.Reference,
				DevEUI:    ns.DevEUI,
				Message:   fmt.Sprintf("downlink payload max size exceeded (dr: %d, allowed: %d, got: %d)", rx1DR, Band.MaxPayloadSize[rx1DR].N, len(txPayload.Data)),
			})
			if err != nil {
				return err
			}
		} else {
			// remove the payload from the queue when not confirmed
			if !txPayload.Confirmed {
				if _, err := clearInProcessTXPayload(ctx.RedisPool, ns.DevEUI); err != nil {
					return err
				}
			}

			macPL.FHDR.FCtrl.FPending = remaining
			if txPayload.Confirmed {
				phy.MHDR.MType = lorawan.ConfirmedDataDown
			}
			macPL.FPort = &txPayload.FPort
			macPL.FRMPayload = []lorawan.Payload{
				&lorawan.DataPayload{Bytes: txPayload.Data},
			}
		}
	}

	// when the payload did not pass the validation and there is no ACK set,
	// there is nothing to send
	if !macPL.FHDR.FCtrl.ACK && len(macPL.FRMPayload) == 0 {
		return nil
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
			Timestamp: rxPacket.RXInfo.Timestamp + uint32(rxDelay/time.Microsecond),
			Frequency: Band.DownlinkChannels[rx1Channel].Frequency,
			Power:     Band.DefaultTXPower,
			DataRate:  Band.DataRates[rx1DR],
			CodeRate:  rxPacket.RXInfo.CodeRate,
		},
		PHYPayload: phy,
	}

	// window 1
	if err := ctx.Gateway.SendTXPacket(txPacket); err != nil {
		return fmt.Errorf("send tx packet (rx window 1) to gateway error: %s", err)
	}

	// increment the FCntDown when MType != ConfirmedDataDown. In case of
	// ConfirmedDataDown we increment on ACK.
	if phy.MHDR.MType != lorawan.ConfirmedDataDown {
		ns.FCntDown++
		if err := saveNodeSession(ctx.RedisPool, ns); err != nil {
			return err
		}
	}

	return nil
}
