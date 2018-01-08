package data

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/api/nc"
	"github.com/brocaar/loraserver/internal/config"
	datadown "github.com/brocaar/loraserver/internal/downlink/data"
	"github.com/brocaar/loraserver/internal/downlink/data/classb"
	"github.com/brocaar/loraserver/internal/framelog"
	"github.com/brocaar/loraserver/internal/gateway"
	"github.com/brocaar/loraserver/internal/maccommand"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
)

var tasks = []func(*dataContext) error{
	setContextFromDataPHYPayload,
	getDeviceSessionForPHYPayload,
	logUplinkFrame,
	getDeviceProfile,
	getServiceProfile,
	setADR,
	setUplinkDataRate,
	appendMetaDataToUplinkHistory,
	getApplicationServerClientForDataUp,
	decryptFRMPayloadMACCommands,
	setBeaconLocked,
	sendRXInfoToNetworkController,
	handleFOptsMACCommands,
	handleFRMPayloadMACCommands,
	sendFRMPayloadToApplicationServer,
	setLastRXInfoSet,
	syncUplinkFCnt,
	saveNodeSession,
	handleUplinkACK,
	handleDownlink,
}

type dataContext struct {
	RXPacket                models.RXPacket
	MACPayload              *lorawan.MACPayload
	DeviceSession           storage.DeviceSession
	DeviceProfile           storage.DeviceProfile
	ServiceProfile          storage.ServiceProfile
	ApplicationServerClient as.ApplicationServerClient
	MACCommandResponses     []storage.MACCommandBlock
}

// Handle handles an uplink data frame
func Handle(rxPacket models.RXPacket) error {
	ctx := dataContext{
		RXPacket: rxPacket,
	}

	for _, t := range tasks {
		if err := t(&ctx); err != nil {
			return err
		}
	}

	return nil
}

func setContextFromDataPHYPayload(ctx *dataContext) error {
	macPL, ok := ctx.RXPacket.PHYPayload.MACPayload.(*lorawan.MACPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.MACPayload, got: %T", ctx.RXPacket.PHYPayload.MACPayload)
	}
	ctx.MACPayload = macPL
	return nil
}

func getDeviceSessionForPHYPayload(ctx *dataContext) error {
	ds, err := storage.GetDeviceSessionForPHYPayload(config.C.Redis.Pool, ctx.RXPacket.PHYPayload)
	if err != nil {
		return errors.Wrap(err, "get device-session error")
	}
	ctx.DeviceSession = ds

	return nil
}

func logUplinkFrame(ctx *dataContext) error {
	if err := framelog.LogUplinkFrameForDevEUI(ctx.DeviceSession.DevEUI, ctx.RXPacket); err != nil {
		log.WithError(err).Error("log uplink frame for device error")
	}

	return nil
}

func getDeviceProfile(ctx *dataContext) error {
	dp, err := storage.GetAndCacheDeviceProfile(config.C.PostgreSQL.DB, config.C.Redis.Pool, ctx.DeviceSession.DeviceProfileID)
	if err != nil {
		return errors.Wrap(err, "get device-profile error")
	}
	ctx.DeviceProfile = dp

	return nil
}

func getServiceProfile(ctx *dataContext) error {
	sp, err := storage.GetAndCacheServiceProfile(config.C.PostgreSQL.DB, config.C.Redis.Pool, ctx.DeviceSession.ServiceProfileID)
	if err != nil {
		return errors.Wrap(err, "get service-profile error")
	}
	ctx.ServiceProfile = sp

	return nil
}

func setADR(ctx *dataContext) error {
	ctx.DeviceSession.ADR = ctx.MACPayload.FHDR.FCtrl.ADR
	return nil
}

func setUplinkDataRate(ctx *dataContext) error {
	currentDR, err := config.C.NetworkServer.Band.Band.GetDataRate(ctx.RXPacket.TXInfo.DataRate)
	if err != nil {
		return errors.Wrap(err, "get data-rate error")
	}

	// The node changed its data-rate. Possibly the node did also reset its
	// tx-power to max power. Because of this, we need to reset the tx-power
	// at the network-server side too.
	if ctx.DeviceSession.DR != currentDR {
		ctx.DeviceSession.TXPowerIndex = 0
	}
	ctx.DeviceSession.DR = currentDR

	return nil
}

func appendMetaDataToUplinkHistory(ctx *dataContext) error {
	var maxSNR float64
	for i, rxInfo := range ctx.RXPacket.RXInfoSet {
		// as the default value is 0 and the LoRaSNR can be negative, we always
		// set it when i == 0 (the first item from the slice)
		if i == 0 || rxInfo.LoRaSNR > maxSNR {
			maxSNR = rxInfo.LoRaSNR
		}
	}

	ctx.DeviceSession.AppendUplinkHistory(storage.UplinkHistory{
		FCnt:         ctx.MACPayload.FHDR.FCnt,
		GatewayCount: len(ctx.RXPacket.RXInfoSet),
		MaxSNR:       maxSNR,
	})

	return nil
}

func getApplicationServerClientForDataUp(ctx *dataContext) error {
	rp, err := storage.GetRoutingProfile(config.C.PostgreSQL.DB, ctx.DeviceSession.RoutingProfileID)
	if err != nil {
		return errors.Wrap(err, "get routing-profile error")
	}

	asClient, err := config.C.ApplicationServer.Pool.Get(rp.ASID, []byte(rp.CACert), []byte(rp.TLSCert), []byte(rp.TLSKey))
	if err != nil {
		return errors.Wrap(err, "get application-server client error")
	}

	ctx.ApplicationServerClient = asClient

	return nil
}

func decryptFRMPayloadMACCommands(ctx *dataContext) error {
	// only decrypt when FPort is equal to 0
	if ctx.MACPayload.FPort != nil && *ctx.MACPayload.FPort == 0 {
		if err := ctx.RXPacket.PHYPayload.DecryptFRMPayload(ctx.DeviceSession.NwkSKey); err != nil {
			return errors.Wrap(err, "decrypt FRMPayload error")
		}
	}

	return nil
}

func setBeaconLocked(ctx *dataContext) error {
	// set the Class-B beacon locked
	if ctx.DeviceSession.BeaconLocked == ctx.MACPayload.FHDR.FCtrl.ClassB {
		// no state change
		return nil
	}

	ctx.DeviceSession.BeaconLocked = ctx.MACPayload.FHDR.FCtrl.ClassB
	if ctx.DeviceSession.BeaconLocked {
		if err := classb.ScheduleDeviceQueueToPingSlotsForDevEUI(config.C.PostgreSQL.DB, ctx.DeviceProfile, ctx.DeviceSession); err != nil {
			return errors.Wrap(err, "schedule device-queue to ping-slots error")
		}

		log.WithFields(log.Fields{
			"dev_eui": ctx.DeviceSession.DevEUI,
		}).Info("class-b beacon locked")
	}

	if !ctx.DeviceSession.BeaconLocked {
		log.WithFields(log.Fields{
			"dev_eui": ctx.DeviceSession.DevEUI,
		}).Info("class-b beacon lost")
	}

	return nil
}

func sendRXInfoToNetworkController(ctx *dataContext) error {
	// TODO: change so that errors get logged but not returned
	if err := sendRXInfoPayload(ctx.DeviceSession, ctx.RXPacket); err != nil {
		return errors.Wrap(err, "send rx-info to network-controller error")
	}

	return nil
}

func handleFOptsMACCommands(ctx *dataContext) error {
	if len(ctx.MACPayload.FHDR.FOpts) > 0 {
		blocks, err := handleUplinkMACCommands(&ctx.DeviceSession, ctx.MACPayload.FHDR.FOpts, ctx.RXPacket)
		if err != nil {
			log.WithFields(log.Fields{
				"dev_eui": ctx.DeviceSession.DevEUI,
				"fopts":   ctx.MACPayload.FHDR.FOpts,
			}).Errorf("handle FOpts mac commands error: %s", err)
		} else {
			ctx.MACCommandResponses = append(ctx.MACCommandResponses, blocks...)
		}
	}

	return nil
}

func handleFRMPayloadMACCommands(ctx *dataContext) error {
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
		blocks, err := handleUplinkMACCommands(&ctx.DeviceSession, commands, ctx.RXPacket)
		if err != nil {
			log.WithFields(log.Fields{
				"dev_eui":  ctx.DeviceSession.DevEUI,
				"commands": commands,
			}).Errorf("handle FRMPayload mac commands error: %s", err)
		} else {
			ctx.MACCommandResponses = append(ctx.MACCommandResponses, blocks...)
		}
	}

	return nil
}

func sendFRMPayloadToApplicationServer(ctx *dataContext) error {
	if ctx.MACPayload.FPort != nil && *ctx.MACPayload.FPort > 0 {
		return publishDataUp(ctx.ApplicationServerClient, ctx.DeviceSession, ctx.ServiceProfile, ctx.RXPacket, *ctx.MACPayload)
	}

	return nil
}

func setLastRXInfoSet(ctx *dataContext) error {
	// update the RXInfoSet
	ctx.DeviceSession.LastRXInfoSet = ctx.RXPacket.RXInfoSet
	return nil
}

func syncUplinkFCnt(ctx *dataContext) error {
	// sync counter with that of the device + 1
	ctx.DeviceSession.FCntUp = ctx.MACPayload.FHDR.FCnt + 1
	return nil
}

func saveNodeSession(ctx *dataContext) error {
	// save node-session
	return storage.SaveDeviceSession(config.C.Redis.Pool, ctx.DeviceSession)
}

func handleUplinkACK(ctx *dataContext) error {
	if !ctx.MACPayload.FHDR.FCtrl.ACK {
		return nil
	}

	qi, err := storage.GetPendingDeviceQueueItemForDevEUI(config.C.PostgreSQL.DB, ctx.DeviceSession.DevEUI)
	if err != nil {
		log.WithFields(log.Fields{
			"dev_eui": ctx.DeviceSession.DevEUI,
		}).WithError(err).Error("get device-queue item error")
		return nil
	}
	if qi.FCnt != ctx.DeviceSession.FCntDown-1 {
		log.WithFields(log.Fields{
			"dev_eui":                  ctx.DeviceSession.DevEUI,
			"device_queue_item_fcnt":   qi.FCnt,
			"device_session_fcnt_down": ctx.DeviceSession.FCntDown,
		}).Error("frame-counter of device-queue item out of sync with device-session")
		return nil
	}

	if err := storage.DeleteDeviceQueueItem(config.C.PostgreSQL.DB, qi.ID); err != nil {
		return errors.Wrap(err, "delete device-queue item error")
	}

	_, err = ctx.ApplicationServerClient.HandleDownlinkACK(context.Background(), &as.HandleDownlinkACKRequest{
		DevEUI:       ctx.DeviceSession.DevEUI[:],
		FCnt:         qi.FCnt,
		Acknowledged: true,
	})
	if err != nil {
		return errors.Wrap(err, "application-server client error")
	}

	return nil
}

func handleDownlink(ctx *dataContext) error {
	// handle downlink (ACK)
	time.Sleep(config.C.NetworkServer.GetDownlinkDataDelay)
	if err := datadown.HandleResponse(
		ctx.RXPacket,
		ctx.ServiceProfile,
		ctx.DeviceSession,
		ctx.MACPayload.FHDR.FCtrl.ADR,
		ctx.MACPayload.FHDR.FCtrl.ADRACKReq,
		ctx.RXPacket.PHYPayload.MHDR.MType == lorawan.ConfirmedDataUp,
		ctx.MACCommandResponses,
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

	dr := rxPacket.TXInfo.DataRate

	rxInfoReq := nc.HandleRXInfoRequest{
		DevEUI: ds.DevEUI[:],
		TxInfo: &nc.TXInfo{
			Frequency: int64(rxPacket.TXInfo.Frequency),
			Adr:       macPL.FHDR.FCtrl.ADR,
			CodeRate:  rxPacket.TXInfo.CodeRate,
			DataRate: &nc.DataRate{
				Modulation:   string(dr.Modulation),
				BandWidth:    uint32(dr.Bandwidth),
				SpreadFactor: uint32(dr.SpreadFactor),
				Bitrate:      uint32(dr.BitRate),
			},
		},
	}

	for _, rxInfo := range rxPacket.RXInfoSet {
		// make sure we have a copy of the MAC byte slice, else every RxInfo
		// slice item will get the same Mac
		mac := make([]byte, 8)
		copy(mac, rxInfo.MAC[:])

		rx := nc.RXInfo{
			Mac:     mac,
			Rssi:    int32(rxInfo.RSSI),
			LoRaSNR: rxInfo.LoRaSNR,
		}

		if rxInfo.Time != nil {
			rx.Time = rxInfo.Time.Format(time.RFC3339Nano)
		}

		rxInfoReq.RxInfo = append(rxInfoReq.RxInfo, &rx)

	}

	_, err := config.C.NetworkController.Client.HandleRXInfo(context.Background(), &rxInfoReq)
	if err != nil {
		return fmt.Errorf("publish rxinfo to network-controller error: %s", err)
	}
	log.WithFields(log.Fields{
		"dev_eui": ds.DevEUI,
	}).Info("rx info sent to network-controller")
	return nil
}

func publishDataUp(asClient as.ApplicationServerClient, ds storage.DeviceSession, sp storage.ServiceProfile, rxPacket models.RXPacket, macPL lorawan.MACPayload) error {
	dr := rxPacket.TXInfo.DataRate

	publishDataUpReq := as.HandleUplinkDataRequest{
		AppEUI: ds.JoinEUI[:],
		DevEUI: ds.DevEUI[:],
		FCnt:   macPL.FHDR.FCnt,
		TxInfo: &as.TXInfo{
			Frequency: int64(rxPacket.TXInfo.Frequency),
			Adr:       macPL.FHDR.FCtrl.ADR,
			CodeRate:  rxPacket.TXInfo.CodeRate,
			DataRate: &as.DataRate{
				Modulation:   string(dr.Modulation),
				BandWidth:    uint32(dr.Bandwidth),
				SpreadFactor: uint32(dr.SpreadFactor),
				Bitrate:      uint32(dr.BitRate),
			},
		},
		DeviceStatusBattery: 256,
		DeviceStatusMargin:  256,
	}

	if sp.ServiceProfile.DevStatusReqFreq != 0 && ds.LastDevStatusMargin != 127 {
		if sp.ServiceProfile.ReportDevStatusBattery {
			publishDataUpReq.DeviceStatusBattery = uint32(ds.LastDevStatusBattery)
		}
		if sp.ServiceProfile.ReportDevStatusMargin {
			publishDataUpReq.DeviceStatusMargin = int32(ds.LastDevStatusMargin)
		}
	}

	if sp.ServiceProfile.AddGWMetadata {
		var macs []lorawan.EUI64
		for i := range rxPacket.RXInfoSet {
			macs = append(macs, rxPacket.RXInfoSet[i].MAC)
		}

		// get gateway info
		gws, err := gateway.GetGatewaysForMACs(config.C.PostgreSQL.DB, macs)
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
				Rssi:    int32(rxInfo.RSSI),
				LoRaSNR: rxInfo.LoRaSNR,
			}

			if rxInfo.Time != nil {
				asRxInfo.Time = rxInfo.Time.Format(time.RFC3339Nano)
			}

			if gw, ok := gws[rxInfo.MAC]; ok {
				asRxInfo.Name = gw.Name
				asRxInfo.Latitude = gw.Location.Latitude
				asRxInfo.Longitude = gw.Location.Longitude
				asRxInfo.Altitude = gw.Altitude
			}

			publishDataUpReq.RxInfo = append(publishDataUpReq.RxInfo, &asRxInfo)
		}
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

	if _, err := asClient.HandleUplinkData(context.Background(), &publishDataUpReq); err != nil {
		return fmt.Errorf("publish data up to application-server error: %s", err)
	}
	return nil
}

func handleUplinkMACCommands(ds *storage.DeviceSession, commands []lorawan.MACCommand, rxPacket models.RXPacket) ([]storage.MACCommandBlock, error) {
	var cids []lorawan.CID
	var out []storage.MACCommandBlock
	blocks := make(map[lorawan.CID]storage.MACCommandBlock)

	// group mac-commands by CID
	for _, cmd := range commands {
		block, ok := blocks[cmd.CID]
		if !ok {
			block = storage.MACCommandBlock{
				CID: cmd.CID,
			}
			cids = append(cids, cmd.CID)
		}
		block.MACCommands = append(block.MACCommands, cmd)
		blocks[cmd.CID] = block
	}

	for _, cid := range cids {
		block := blocks[cid]

		logFields := log.Fields{
			"dev_eui": ds.DevEUI,
			"cid":     block.CID,
		}

		// read pending mac-command block for CID. e.g. on case of an ack, the
		// pending mac-command block contains the request.
		// we need this pending mac-command block to find out if the command
		// was scheduled through the API (external).
		pending, err := storage.GetPendingMACCommand(config.C.Redis.Pool, ds.DevEUI, block.CID)
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
			if err = storage.DeletePendingMACCommand(config.C.Redis.Pool, ds.DevEUI, block.CID); err != nil {
				log.WithFields(logFields).Errorf("delete pending mac-command error: %s", err)
			}
		}

		// CID >= 0x80 are proprietary mac-commands and are not handled by LoRa Server
		if block.CID < 0x80 {
			responseBlocks, err := maccommand.Handle(ds, block, pending, rxPacket)
			if err != nil {
				log.WithFields(logFields).Errorf("handle mac-command block error: %s", err)
			} else {
				out = append(out, responseBlocks...)
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
			_, err = config.C.NetworkController.Client.HandleDataUpMACCommand(context.Background(), &nc.HandleDataUpMACCommandRequest{
				DevEUI:   ds.DevEUI[:],
				Cid:      uint32(block.CID),
				Commands: data,
			})
			if err != nil {
				log.WithFields(logFields).Errorf("send mac-command to network-controller error: %s", err)
			} else {
				log.WithFields(logFields).Info("mac-command sent to network-controller")
			}
		}
	}

	return out, nil
}
