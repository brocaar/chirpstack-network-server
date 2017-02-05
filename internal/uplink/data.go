package uplink

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"

	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/api/nc"
	"github.com/brocaar/loraserver/internal/adr"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/downlink"
	"github.com/brocaar/loraserver/internal/maccommand"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/session"
	"github.com/brocaar/lorawan"
)

func validateAndCollectDataUpRXPacket(ctx common.Context, rxPacket gw.RXPacket) error {
	ns, err := session.GetNodeSessionForPHYPayload(ctx.RedisPool, rxPacket.PHYPayload)
	if err != nil {
		return fmt.Errorf("get node-session error: %s", err)
	}

	// MACPayload must be of type *lorawan.MACPayload
	macPL, ok := rxPacket.PHYPayload.MACPayload.(*lorawan.MACPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.MACPayload, got: %T", rxPacket.PHYPayload.MACPayload)
	}

	if macPL.FPort != nil {
		if *macPL.FPort == 0 {
			// decrypt FRMPayload with NwkSKey when FPort == 0
			if err := rxPacket.PHYPayload.DecryptFRMPayload(ns.NwkSKey); err != nil {
				return fmt.Errorf("decrypt FRMPayload error: %s", err)
			}
		}
	}

	return collectAndCallOnce(ctx.RedisPool, rxPacket, func(rxPacket models.RXPacket) error {
		return handleCollectedDataUpPackets(ctx, rxPacket)
	})
}

func handleCollectedDataUpPackets(ctx common.Context, rxPacket models.RXPacket) error {
	var macs []string
	for _, p := range rxPacket.RXInfoSet {
		macs = append(macs, p.MAC.String())
	}

	macPL, ok := rxPacket.PHYPayload.MACPayload.(*lorawan.MACPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.MACPayload, got: %T", rxPacket.PHYPayload.MACPayload)
	}

	ns, err := session.GetNodeSession(ctx.RedisPool, macPL.FHDR.DevAddr)
	if err != nil {
		return err
	}

	// expand the FCnt, the value itself has already been validated during the
	// collection, so there is no need to handle the ok value
	macPL.FHDR.FCnt, _ = session.ValidateAndGetFullFCntUp(ns, macPL.FHDR.FCnt)

	log.WithFields(log.Fields{
		"dev_eui":  ns.DevEUI,
		"gw_count": len(macs),
		"gw_macs":  strings.Join(macs, ", "),
		"mtype":    rxPacket.PHYPayload.MHDR.MType,
	}).Info("packet(s) collected")

	// send rx info notification to be used by the network-controller
	if err = sendRXInfoPayload(ctx, ns, rxPacket); err != nil {
		log.WithFields(log.Fields{
			"dev_eui": ns.DevEUI,
		}).Errorf("send rx info to network-controller error: %s", err)
	}

	// handle FOpts mac commands (if any)
	if len(macPL.FHDR.FOpts) > 0 {
		if err := handleUplinkMACCommands(ctx, &ns, false, macPL.FHDR.FOpts); err != nil {
			log.WithFields(log.Fields{
				"dev_eui": ns.DevEUI,
				"fopts":   macPL.FHDR.FOpts,
			}).Errorf("handle FOpts mac commands error: %s", err)
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
			if err := handleUplinkMACCommands(ctx, &ns, true, commands); err != nil {
				log.WithFields(log.Fields{
					"dev_eui":  ns.DevEUI,
					"commands": commands,
				}).Errorf("handle FRMPayload mac commands error: %s", err)
			}
		} else {
			if err := publishDataUp(ctx, ns, rxPacket, *macPL); err != nil {
				return err
			}
		}
	}

	// handle ADR (should be executed before saving the node-session)
	if err := adr.HandleADR(ctx, &ns, rxPacket, macPL.FHDR.FCnt); err != nil {
		log.WithFields(log.Fields{
			"dev_eui": ns.DevEUI,
			"fcnt_up": macPL.FHDR.FCnt,
		}).Warningf("handle adr error: %s", err)
	}

	// update the RXInfoSet
	ns.LastRXInfoSet = rxPacket.RXInfoSet

	// sync counter with that of the device + 1
	ns.FCntUp = macPL.FHDR.FCnt + 1

	// save node-session
	if err := session.SaveNodeSession(ctx.RedisPool, ns); err != nil {
		return err
	}

	// handle uplink ACK
	if macPL.FHDR.FCtrl.ACK {
		if err := handleUplinkACK(ctx, &ns); err != nil {
			return fmt.Errorf("handle uplink ack error: %s", err)
		}
	}

	// handle downlink (ACK)
	time.Sleep(common.GetDownlinkDataDelay)
	if err := downlink.SendUplinkResponse(ctx, ns, rxPacket); err != nil {
		return fmt.Errorf("handling downlink data for node %s failed: %s", ns.DevEUI, err)
	}

	return nil
}

// sendRXInfoPayload sends the rx and tx meta-data to the network controller.
func sendRXInfoPayload(ctx common.Context, ns session.NodeSession, rxPacket models.RXPacket) error {
	macPL, ok := rxPacket.PHYPayload.MACPayload.(*lorawan.MACPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.MACPayload, got: %T", rxPacket.PHYPayload.MACPayload)
	}

	rxInfoReq := nc.HandleRXInfoRequest{
		AppEUI: ns.AppEUI[:],
		DevEUI: ns.DevEUI[:],
		TxInfo: &nc.TXInfo{
			Frequency: int64(rxPacket.RXInfoSet[0].Frequency),
			Adr:       macPL.FHDR.FCtrl.ADR,
			CodeRate:  rxPacket.RXInfoSet[0].CodeRate,
			DataRate: &nc.DataRate{
				Modulation:   string(rxPacket.RXInfoSet[0].DataRate.Modulation),
				BandWidth:    uint32(rxPacket.RXInfoSet[0].DataRate.Bandwidth),
				SpreadFactor: uint32(rxPacket.RXInfoSet[0].DataRate.SpreadFactor),
				Bitrate:      uint32(rxPacket.RXInfoSet[0].DataRate.BitRate),
			},
		},
	}

	for _, rxInfo := range rxPacket.RXInfoSet {
		// make sure we have a copy of the MAC byte slice, else every RxInfo
		// slice item will get the same Mac
		mac := make([]byte, 8)
		copy(mac, rxInfo.MAC[:])

		rxInfoReq.RxInfo = append(rxInfoReq.RxInfo, &nc.RXInfo{
			Mac:     mac,
			Time:    rxInfo.Time.Format(time.RFC3339Nano),
			Rssi:    int32(rxInfo.RSSI),
			LoRaSNR: rxInfo.LoRaSNR,
		})
	}

	_, err := ctx.Controller.HandleRXInfo(context.Background(), &rxInfoReq)
	if err != nil {
		return fmt.Errorf("publish rxinfo to network-controller error: %s", err)
	}
	log.WithFields(log.Fields{
		"dev_eui": ns.DevEUI,
	}).Info("rx info sent to network-controller")
	return nil
}

func publishDataUp(ctx common.Context, ns session.NodeSession, rxPacket models.RXPacket, macPL lorawan.MACPayload) error {
	publishDataUpReq := as.HandleDataUpRequest{
		AppEUI: ns.AppEUI[:],
		DevEUI: ns.DevEUI[:],
		FCnt:   macPL.FHDR.FCnt,
		TxInfo: &as.TXInfo{
			Frequency: int64(rxPacket.RXInfoSet[0].Frequency),
			Adr:       macPL.FHDR.FCtrl.ADR,
			CodeRate:  rxPacket.RXInfoSet[0].CodeRate,
			DataRate: &as.DataRate{
				Modulation:   string(rxPacket.RXInfoSet[0].DataRate.Modulation),
				BandWidth:    uint32(rxPacket.RXInfoSet[0].DataRate.Bandwidth),
				SpreadFactor: uint32(rxPacket.RXInfoSet[0].DataRate.SpreadFactor),
				Bitrate:      uint32(rxPacket.RXInfoSet[0].DataRate.BitRate),
			},
		},
	}

	for _, rxInfo := range rxPacket.RXInfoSet {
		// make sure we have a copy of the MAC byte slice, else every RxInfo
		// slice item will get the same Mac
		mac := make([]byte, 8)
		copy(mac, rxInfo.MAC[:])

		publishDataUpReq.RxInfo = append(publishDataUpReq.RxInfo, &as.RXInfo{
			Mac:     mac,
			Time:    rxInfo.Time.Format(time.RFC3339Nano),
			Rssi:    int32(rxInfo.RSSI),
			LoRaSNR: rxInfo.LoRaSNR,
		})
	}

	if macPL.FPort != nil {
		publishDataUpReq.FPort = uint32(*macPL.FPort)
	}

	if len(macPL.FRMPayload) == 1 {
		dataPL, ok := macPL.FRMPayload[0].(*lorawan.DataPayload)
		if !ok {
			return fmt.Errorf("expected type *lorawan.DataPayload, got %T", macPL.FRMPayload[0])
		}
		publishDataUpReq.Data = dataPL.Bytes

	}

	if _, err := ctx.Application.HandleDataUp(context.Background(), &publishDataUpReq); err != nil {
		return fmt.Errorf("publish data up to application-server error: %s", err)
	}
	return nil
}

func handleUplinkMACCommands(ctx common.Context, ns *session.NodeSession, frmPayload bool, commands []lorawan.MACCommand) error {
	for _, cmd := range commands {
		logFields := log.Fields{
			"dev_eui":     ns.DevEUI,
			"cid":         cmd.CID,
			"frm_payload": frmPayload,
		}

		// proprietary MAC commands
		if cmd.CID >= 0x80 {
			b, err := cmd.MarshalBinary()
			if err != nil {
				return fmt.Errorf("binary marshal mac command error: %s", err)
			}
			_, err = ctx.Controller.HandleDataUpMACCommand(context.Background(), &nc.HandleDataUpMACCommandRequest{
				AppEUI:     ns.AppEUI[:],
				DevEUI:     ns.DevEUI[:],
				FrmPayload: frmPayload,
				Data:       b,
			})
			if err != nil {
				log.WithFields(logFields).Errorf("send proprietary mac-command to network-controller error: %s", err)
			} else {
				log.WithFields(logFields).Info("proprietary mac-command sent to network-controller")
			}
		} else {
			if err := maccommand.Handle(ctx, ns, cmd); err != nil {
				log.WithFields(logFields).Errorf("handle mac-command error: %s", err)
			}
		}
	}
	return nil
}

func handleUplinkACK(ctx common.Context, ns *session.NodeSession) error {
	_, err := ctx.Application.HandleDataDownACK(context.Background(), &as.HandleDataDownACKRequest{
		AppEUI: ns.AppEUI[:],
		DevEUI: ns.DevEUI[:],
		FCnt:   ns.FCntDown,
	})
	if err != nil {
		return fmt.Errorf("error publish downlink data ack to application-server: %s", err)
	}
	ns.FCntDown++
	if err = session.SaveNodeSession(ctx.RedisPool, *ns); err != nil {
		return err
	}
	return nil
}
