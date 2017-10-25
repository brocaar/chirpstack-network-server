package uplink

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/api/nc"
	"github.com/brocaar/loraserver/internal/adr"
	"github.com/brocaar/loraserver/internal/channels"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/downlink"
	"github.com/brocaar/loraserver/internal/gateway"
	"github.com/brocaar/loraserver/internal/maccommand"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
)

func setContextFromDataPHYPayload(ctx *DataUpContext) error {
	macPL, ok := ctx.RXPacket.PHYPayload.MACPayload.(*lorawan.MACPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.MACPayload, got: %T", ctx.RXPacket.PHYPayload.MACPayload)
	}
	ctx.MACPayload = macPL
	return nil
}

func getNodeSessionForDataUp(ctx *DataUpContext) error {
	ds, err := storage.GetDeviceSessionForPHYPayload(common.RedisPool, ctx.RXPacket.PHYPayload)
	if err != nil {
		return errors.Wrap(err, "get device-session error")
	}
	ctx.DeviceSession = ds

	return nil
}

func logDataFramesCollected(ctx *DataUpContext) error {
	var macs []string
	for _, p := range ctx.RXPacket.RXInfoSet {
		macs = append(macs, p.MAC.String())
	}

	log.WithFields(log.Fields{
		"dev_eui":  ctx.DeviceSession.DevEUI,
		"gw_count": len(macs),
		"gw_macs":  strings.Join(macs, ", "),
		"mtype":    ctx.RXPacket.PHYPayload.MHDR.MType,
	}).Info("packet(s) collected")

	logUplink(common.DB, ctx.DeviceSession.DevEUI, ctx.RXPacket)

	return nil
}

func getApplicationServerClientForDataUp(ctx *DataUpContext) error {
	rp, err := storage.GetRoutingProfile(common.DB, ctx.DeviceSession.RoutingProfileID)
	if err != nil {
		return errors.Wrap(err, "get routing-profile error")
	}

	asClient, err := common.ApplicationServerPool.Get(rp.ASID)
	if err != nil {
		return errors.Wrap(err, "get application-server client error")
	}

	ctx.ApplicationServerClient = asClient

	return nil
}

func decryptFRMPayloadMACCommands(ctx *DataUpContext) error {
	// only decrypt when FPort is equal to 0
	if ctx.MACPayload.FPort != nil && *ctx.MACPayload.FPort == 0 {
		if err := ctx.RXPacket.PHYPayload.DecryptFRMPayload(ctx.DeviceSession.NwkSKey); err != nil {
			return errors.Wrap(err, "decrypt FRMPayload error")
		}
	}

	return nil
}

func sendRXInfoToNetworkController(ctx *DataUpContext) error {
	// TODO: change so that errors get logged but not returned
	if err := sendRXInfoPayload(ctx.DeviceSession, ctx.RXPacket); err != nil {
		return errors.Wrap(err, "send rx-info to network-controller error")
	}

	return nil
}

func handleFOptsMACCommands(ctx *DataUpContext) error {
	if len(ctx.MACPayload.FHDR.FOpts) > 0 {
		if err := handleUplinkMACCommands(&ctx.DeviceSession, false, ctx.MACPayload.FHDR.FOpts, ctx.RXPacket.RXInfoSet); err != nil {
			log.WithFields(log.Fields{
				"dev_eui": ctx.DeviceSession.DevEUI,
				"fopts":   ctx.MACPayload.FHDR.FOpts,
			}).Errorf("handle FOpts mac commands error: %s", err)
		}
	}

	return nil
}

func handleFRMPayloadMACCommands(ctx *DataUpContext) error {
	if ctx.MACPayload.FPort != nil && *ctx.MACPayload.FPort == 0 {
		if len(ctx.MACPayload.FRMPayload) == 0 {
			return errors.New("expected mac commands, but FRMPayload is empty (FPort=0)")
		}

		var commands []lorawan.MACCommand
		for _, pl := range ctx.MACPayload.FRMPayload {
			cmd, ok := pl.(*lorawan.MACCommand)
			if !ok {
				return fmt.Errorf("expected MACPayload, but got %T", ctx.MACPayload.FRMPayload)
			}
			commands = append(commands, *cmd)
		}
		if err := handleUplinkMACCommands(&ctx.DeviceSession, true, commands, ctx.RXPacket.RXInfoSet); err != nil {
			log.WithFields(log.Fields{
				"dev_eui":  ctx.DeviceSession.DevEUI,
				"commands": commands,
			}).Errorf("handle FRMPayload mac commands error: %s", err)
		}
	}

	return nil
}

func sendFRMPayloadToApplicationServer(ctx *DataUpContext) error {
	if ctx.MACPayload.FPort != nil && *ctx.MACPayload.FPort > 0 {
		return publishDataUp(ctx.ApplicationServerClient, ctx.DeviceSession, ctx.RXPacket, *ctx.MACPayload)
	}

	return nil
}

func handleChannelReconfiguration(ctx *DataUpContext) error {
	// handle channel configuration
	// note that this must come before ADR!
	if err := channels.HandleChannelReconfigure(ctx.DeviceSession, ctx.RXPacket); err != nil {
		log.WithFields(log.Fields{
			"dev_eui": ctx.DeviceSession.DevEUI,
		}).Warningf("handle channel reconfigure error: %s", err)
	}

	return nil
}

func handleADR(ctx *DataUpContext) error {
	// handle ADR (should be executed before saving the node-session)
	if err := adr.HandleADR(&ctx.DeviceSession, ctx.RXPacket, ctx.MACPayload.FHDR.FCnt); err != nil {
		log.WithFields(log.Fields{
			"dev_eui": ctx.DeviceSession.DevEUI,
			"fcnt_up": ctx.MACPayload.FHDR.FCnt,
		}).Warningf("handle adr error: %s", err)
	}

	return nil
}

func setLastRXInfoSet(ctx *DataUpContext) error {
	// update the RXInfoSet
	ctx.DeviceSession.LastRXInfoSet = ctx.RXPacket.RXInfoSet
	return nil
}

func syncUplinkFCnt(ctx *DataUpContext) error {
	// sync counter with that of the device + 1
	ctx.DeviceSession.FCntUp = ctx.MACPayload.FHDR.FCnt + 1
	return nil
}

func saveNodeSession(ctx *DataUpContext) error {
	// save node-session
	return storage.SaveDeviceSession(common.RedisPool, ctx.DeviceSession)
}

func handleUplinkACK(ctx *DataUpContext) error {
	// TODO: only log in case of error?
	if !ctx.MACPayload.FHDR.FCtrl.ACK {
		return nil
	}

	_, err := ctx.ApplicationServerClient.HandleDataDownACK(context.Background(), &as.HandleDataDownACKRequest{
		AppEUI: ctx.DeviceSession.JoinEUI[:],
		DevEUI: ctx.DeviceSession.DevEUI[:],
		FCnt:   ctx.DeviceSession.FCntDown,
	})
	if err != nil {
		return errors.Wrap(err, "error publish downlink data ack to application-server")
	}

	return nil
}

func handleDownlink(ctx *DataUpContext) error {
	// handle downlink (ACK)
	time.Sleep(common.GetDownlinkDataDelay)
	if err := downlink.Flow.RunUplinkResponse(
		ctx.DeviceSession,
		ctx.MACPayload.FHDR.FCtrl.ADR,
		ctx.MACPayload.FHDR.FCtrl.ADRACKReq,
		ctx.RXPacket.PHYPayload.MHDR.MType == lorawan.ConfirmedDataUp,
	); err != nil {
		return errors.Wrap(err, "run uplink response flow error")
	}

	return nil
}

// sendRXInfoPayload sends the rx and tx meta-data to the network controller.
func sendRXInfoPayload(ds storage.DeviceSession, rxPacket models.RXPacket) error {
	macPL, ok := rxPacket.PHYPayload.MACPayload.(*lorawan.MACPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.MACPayload, got: %T", rxPacket.PHYPayload.MACPayload)
	}

	rxInfoReq := nc.HandleRXInfoRequest{
		DevEUI: ds.DevEUI[:],
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

	_, err := common.Controller.HandleRXInfo(context.Background(), &rxInfoReq)
	if err != nil {
		return fmt.Errorf("publish rxinfo to network-controller error: %s", err)
	}
	log.WithFields(log.Fields{
		"dev_eui": ds.DevEUI,
	}).Info("rx info sent to network-controller")
	return nil
}

func publishDataUp(asClient as.ApplicationServerClient, ds storage.DeviceSession, rxPacket models.RXPacket, macPL lorawan.MACPayload) error {
	publishDataUpReq := as.HandleDataUpRequest{
		AppEUI: ds.JoinEUI[:],
		DevEUI: ds.DevEUI[:],
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
	gws, err := gateway.GetGatewaysForMACs(common.DB, macs)
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
			asRxInfo.Latitude = gw.Location.Latitude
			asRxInfo.Longitude = gw.Location.Longitude
			asRxInfo.Altitude = gw.Altitude
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

	if _, err := asClient.HandleDataUp(context.Background(), &publishDataUpReq); err != nil {
		return fmt.Errorf("publish data up to application-server error: %s", err)
	}
	return nil
}

func handleUplinkMACCommands(ds *storage.DeviceSession, frmPayload bool, commands []lorawan.MACCommand, rxInfoSet models.RXInfoSet) error {
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
			"dev_eui":     ds.DevEUI,
			"cid":         block.CID,
			"frm_payload": block.FRMPayload,
		}

		// read pending mac-command block for CID. e.g. on case of an ack, the
		// pending mac-command block contains the request.
		// we need this pending mac-command block to find out if the command
		// was scheduled through the API (external).
		pending, err := maccommand.ReadPending(common.RedisPool, ds.DevEUI, block.CID)
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
			if err = maccommand.DeletePending(common.RedisPool, ds.DevEUI, block.CID); err != nil {
				log.WithFields(logFields).Errorf("delete pending mac-command error: %s", err)
			}
		}

		// CID >= 0x80 are proprietary mac-commands and are not handled by LoRa Server
		if block.CID < 0x80 {
			if err := maccommand.Handle(ds, block, pending, rxInfoSet); err != nil {
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
			_, err = common.Controller.HandleDataUpMACCommand(context.Background(), &nc.HandleDataUpMACCommandRequest{
				DevEUI:     ds.DevEUI[:],
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
