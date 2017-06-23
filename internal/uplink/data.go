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
	"github.com/brocaar/loraserver/internal/channels"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/downlink"
	"github.com/brocaar/loraserver/internal/gateway"
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
		rxPacket.DevEUI = ns.DevEUI
		logUplink(ctx.DB, ns.DevEUI, rxPacket)
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

	ns, err := session.GetNodeSession(ctx.RedisPool, rxPacket.DevEUI)
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
		if err := handleUplinkMACCommands(ctx, &ns, false, macPL.FHDR.FOpts, rxPacket.RXInfoSet); err != nil {
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
			if err := handleUplinkMACCommands(ctx, &ns, true, commands, rxPacket.RXInfoSet); err != nil {
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

	// handle channel configuration
	// note that this must come before ADR!
	if err := channels.HandleChannelReconfigure(ctx, ns, rxPacket); err != nil {
		log.WithFields(log.Fields{
			"dev_eui": ns.DevEUI,
		}).Warningf("handle channel reconfigure error: %s", err)
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

	var macs []lorawan.EUI64
	for i := range rxPacket.RXInfoSet {
		macs = append(macs, rxPacket.RXInfoSet[i].MAC)
	}

	// get gateway info
	gws, err := gateway.GetGatewaysForMACs(ctx.DB, macs)
	if err != nil {
		log.WithField("macs", macs).Warningf("get gateways for macs error: %s", err)
		gws = make(map[lorawan.EUI64]gateway.Gateway)
	}

	for _, rxInfo := range rxPacket.RXInfoSet {
		// make sure we have a copy of the MAC byte slice, else every RxInfo
		// slice item will get the same Mac
		mac := make([]byte, 8)
		copy(mac, rxInfo.MAC[:])

		asRxInfo := as.RXInfo{
			Mac:     mac,
			Time:    rxInfo.Time.Format(time.RFC3339Nano),
			Rssi:    int32(rxInfo.RSSI),
			LoRaSNR: rxInfo.LoRaSNR,
		}

		if gw, ok := gws[rxInfo.MAC]; ok {
			asRxInfo.Name = gw.Name

			if gw.Location != nil {
				asRxInfo.Latitude = gw.Location.Latitude
				asRxInfo.Longitude = gw.Location.Longitude
			}

			if gw.Altitude != nil {
				asRxInfo.Altitude = *gw.Altitude
			}
		}

		publishDataUpReq.RxInfo = append(publishDataUpReq.RxInfo, &asRxInfo)
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

func handleUplinkMACCommands(ctx common.Context, ns *session.NodeSession, frmPayload bool, commands []lorawan.MACCommand, rxInfoSet models.RXInfoSet) error {
	var cids []lorawan.CID
	blocks := make(map[lorawan.CID]maccommand.Block)

	// group mac-commands by CID
	for _, cmd := range commands {
		block, ok := blocks[cmd.CID]
		if !ok {
			block = maccommand.Block{
				CID:        cmd.CID,
				FRMPayload: frmPayload,
			}
			cids = append(cids, cmd.CID)
		}
		block.MACCommands = append(block.MACCommands, cmd)
		blocks[cmd.CID] = block
	}

	for _, cid := range cids {
		block := blocks[cid]

		logFields := log.Fields{
			"dev_eui":     ns.DevEUI,
			"cid":         block.CID,
			"frm_payload": block.FRMPayload,
		}

		// read pending mac-command block for CID. e.g. on case of an ack, the
		// pending mac-command block contains the request.
		// we need this pending mac-command block to find out if the command
		// was scheduled through the API (external).
		pending, err := maccommand.ReadPending(ctx.RedisPool, ns.DevEUI, block.CID)
		if err != nil {
			log.WithFields(logFields).Errorf("read pending mac-command error: %s", err)
			continue
		}
		var external bool
		if pending != nil {
			external = pending.External
		}

		// in case the node is requesting a mac-command, there is nothing pending
		if pending != nil {
			if err = maccommand.DeletePending(ctx.RedisPool, ns.DevEUI, block.CID); err != nil {
				log.WithFields(logFields).Errorf("delete pending mac-command error: %s", err)
			}
		}

		// CID >= 0x80 are proprietary mac-commands and are not handled by LoRa Server
		if block.CID < 0x80 {
			if err := maccommand.Handle(ctx, ns, block, pending, rxInfoSet); err != nil {
				log.WithFields(logFields).Errorf("handle mac-command block error: %s", err)
			}
		}

		// report to external controller in case of proprietary mac-commands or
		// in case when the request has been scheduled through the API.
		if block.CID >= 0x80 || external {
			var data [][]byte
			for _, cmd := range block.MACCommands {
				b, err := cmd.MarshalBinary()
				if err != nil {
					log.WithFields(logFields).Errorf("marshal mac-command to binary error: %s", err)
					continue
				}
				data = append(data, b)
			}
			_, err = ctx.Controller.HandleDataUpMACCommand(context.Background(), &nc.HandleDataUpMACCommandRequest{
				DevEUI:     ns.DevEUI[:],
				FrmPayload: block.FRMPayload,
				Cid:        uint32(block.CID),
				Commands:   data,
			})
			if err != nil {
				log.WithFields(logFields).Errorf("send mac-command to network-controller error: %s", err)
			} else {
				log.WithFields(logFields).Info("mac-command sent to network-controller")
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
