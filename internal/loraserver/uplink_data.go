package loraserver

import (
	"errors"
	"fmt"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
)

func validateAndCollectDataUpRXPacket(ctx Context, rxPacket models.RXPacket) error {
	// MACPayload must be of type *lorawan.MACPayload
	macPL, ok := rxPacket.PHYPayload.MACPayload.(*lorawan.MACPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.MACPayload, got: %T", rxPacket.PHYPayload.MACPayload)
	}

	// get the session data
	ns, err := storage.GetNodeSession(ctx.RedisPool, macPL.FHDR.DevAddr)
	if err != nil {
		return err
	}

	// validate and get the full int32 FCnt
	fullFCnt, ok := storage.ValidateAndGetFullFCntUp(ns, macPL.FHDR.FCnt)
	if !ok {
		log.WithFields(log.Fields{
			"dev_addr":    macPL.FHDR.DevAddr,
			"dev_eui":     ns.DevEUI,
			"packet_fcnt": macPL.FHDR.FCnt,
			"server_fcnt": ns.FCntUp,
		}).Warning("invalid FCnt")
		return errors.New("invalid FCnt or too many dropped frames")
	}
	macPL.FHDR.FCnt = fullFCnt

	// validate MIC
	micOK, err := rxPacket.PHYPayload.ValidateMIC(ns.NwkSKey)
	if err != nil {
		return fmt.Errorf("validate MIC error: %s", err)
	}
	if !micOK {
		return errors.New("invalid MIC")
	}

	if macPL.FPort != nil {
		if *macPL.FPort == 0 {
			// decrypt FRMPayload with NwkSKey when FPort == 0
			if err := rxPacket.PHYPayload.DecryptFRMPayload(ns.NwkSKey); err != nil {
				return fmt.Errorf("decrypt FRMPayload error: %s", err)
			}
		} else {
			if err := rxPacket.PHYPayload.DecryptFRMPayload(ns.AppSKey); err != nil {
				return fmt.Errorf("decrypt FRMPayload error: %s", err)
			}
		}
	}

	return collectAndCallOnce(ctx.RedisPool, rxPacket, func(rxPackets RXPackets) error {
		return handleCollectedDataUpPackets(ctx, rxPackets)
	})
}

func handleCollectedDataUpPackets(ctx Context, rxPackets RXPackets) error {
	if len(rxPackets) == 0 {
		return errors.New("packet collector returned 0 packets")
	}
	rxPacket := rxPackets[0]

	var macs []string
	for _, p := range rxPackets {
		macs = append(macs, p.RXInfo.MAC.String())
	}

	macPL, ok := rxPacket.PHYPayload.MACPayload.(*lorawan.MACPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.MACPayload, got: %T", rxPacket.PHYPayload.MACPayload)
	}

	ns, err := storage.GetNodeSession(ctx.RedisPool, macPL.FHDR.DevAddr)
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"dev_eui":  ns.DevEUI,
		"gw_count": len(rxPackets),
		"gw_macs":  strings.Join(macs, ", "),
		"mtype":    rxPackets[0].PHYPayload.MHDR.MType,
	}).Info("packet(s) collected")

	// send rx info notification to be used by the network-controller
	if err = sendRXInfoPayload(ctx, ns.AppEUI, ns.DevEUI, rxPackets); err != nil {
		return fmt.Errorf("send rx info notification error: %s", err)
	}

	// handle FOpts mac commands (if any)
	if len(macPL.FHDR.FOpts) > 0 {
		if err := handleUplinkMACCommands(ctx, ns.AppEUI, ns.DevEUI, false, macPL.FHDR.FOpts); err != nil {
			return fmt.Errorf("handle FOpts mac commands error: %s", err)
		}
	}

	if macPL.FPort != nil {
		if *macPL.FPort == 0 {
			if len(macPL.FRMPayload) == 0 {
				return errors.New("expected mac commands, but FRMPayload is empty (FPort=0)")
			}

			// since the PHYPayload has been marshaled / unmarshaled when
			// storing it into and retrieving it from the database, we need
			// to decode the MAC commands from the FRMPayload.
			if err = rxPacket.PHYPayload.DecodeFRMPayloadToMACCommands(); err != nil {
				return fmt.Errorf("decode FRMPayload field to MACCommand items error: %s", err)
			}

			var commands []lorawan.MACCommand
			for _, pl := range macPL.FRMPayload {
				cmd, ok := pl.(*lorawan.MACCommand)
				if !ok {
					return fmt.Errorf("expected MACPayload, but got %T", macPL.FRMPayload)
				}
				commands = append(commands, *cmd)
			}
			if err := handleUplinkMACCommands(ctx, ns.AppEUI, ns.DevEUI, true, commands); err != nil {
				return fmt.Errorf("handle FRMPayload mac commands error: %s", err)
			}
		} else {
			var data []byte

			// it is possible that the FRMPayload is empty, in this case only
			// the FPort will be send
			if len(macPL.FRMPayload) == 1 {
				dataPL, ok := macPL.FRMPayload[0].(*lorawan.DataPayload)
				if !ok {
					return errors.New("FRMPayload must be of type *lorawan.DataPayload")
				}
				data = dataPL.Bytes
			}

			err = ctx.Application.SendRXPayload(ns.AppEUI, ns.DevEUI, models.RXPayload{
				DevEUI:       ns.DevEUI,
				Time:         rxPacket.RXInfo.Time,
				GatewayCount: len(rxPackets),
				FPort:        *macPL.FPort,
				RSSI:         rxPacket.RXInfo.RSSI,
				Data:         data,
			})
			if err != nil {
				return fmt.Errorf("send rx payload to application error: %s", err)
			}
		}
	}

	// sync counter with that of the device
	ns.FCntUp = macPL.FHDR.FCnt
	if err := storage.SaveNodeSession(ctx.RedisPool, ns); err != nil {
		return err
	}

	// handle uplink ACK
	if macPL.FHDR.FCtrl.ACK {
		if err := handleUplinkACK(ctx, ns); err != nil {
			return fmt.Errorf("handle uplink ack error: %s", err)
		}
	}

	// handle downlink (ACK)
	time.Sleep(CollectDataDownWait)
	if err := handleDataDownReply(ctx, rxPacket, ns); err != nil {
		return fmt.Errorf("handling downlink data for node %s failed: %s", ns.DevEUI, err)
	}

	return nil
}

func handleUplinkMACCommands(ctx Context, appEUI, devEUI lorawan.EUI64, frmPayload bool, commands []lorawan.MACCommand) error {
	for _, cmd := range commands {
		log.WithFields(log.Fields{
			"dev_eui":     devEUI,
			"cid":         cmd.CID,
			"frm_payload": frmPayload,
		}).Info("mac command received")

		b, err := cmd.MarshalBinary()
		if err != nil {
			return fmt.Errorf("binary marshal mac command error: %s", err)
		}

		err = ctx.Controller.SendMACPayload(appEUI, devEUI, models.MACPayload{
			DevEUI:     devEUI,
			FRMPayload: frmPayload,
			MACCommand: b,
		})
		if err != nil {
			return fmt.Errorf("send mac payload error: %s", err)
		}
	}
	return nil
}

func handleUplinkACK(ctx Context, ns models.NodeSession) error {
	txPayload, err := storage.ClearInProcessTXPayload(ctx.RedisPool, ns.DevEUI)
	if err != nil {
		return err
	}
	ns.FCntDown++
	if err = storage.SaveNodeSession(ctx.RedisPool, ns); err != nil {
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
	return nil
}
