package data

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/api/common"
	"github.com/brocaar/loraserver/api/nc"
	"github.com/brocaar/loraserver/internal/config"
	datadown "github.com/brocaar/loraserver/internal/downlink/data"
	"github.com/brocaar/loraserver/internal/downlink/data/classb"
	"github.com/brocaar/loraserver/internal/framelog"
	"github.com/brocaar/loraserver/internal/helpers"
	"github.com/brocaar/loraserver/internal/maccommand"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
)

const applicationClientTimeout = time.Second

var tasks = []func(*dataContext) error{
	setContextFromDataPHYPayload,
	getDeviceSessionForPHYPayload,
	decryptFOptsMACCommands,
	decryptFRMPayloadMACCommands,
	logUplinkFrame,
	getDeviceProfile,
	getServiceProfile,
	setADR,
	setUplinkDataRate,
	getApplicationServerClientForDataUp,
	setBeaconLocked,
	sendRXInfoToNetworkController,
	handleFOptsMACCommands,
	handleFRMPayloadMACCommands,
	storeDeviceGatewayRXInfoSet,
	appendMetaDataToUplinkHistory,
	sendFRMPayloadToApplicationServer,
	setLastRXInfoSet,
	syncUplinkFCnt,
	saveDeviceSession,
	handleUplinkACK,
	handleDownlink,
}

type dataContext struct {
	RXPacket                models.RXPacket
	MACPayload              *lorawan.MACPayload
	DeviceSession           storage.DeviceSession
	DeviceProfile           storage.DeviceProfile
	ServiceProfile          storage.ServiceProfile
	ApplicationServerClient as.ApplicationServerServiceClient
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
	txDR, err := helpers.GetDataRateIndex(true, ctx.RXPacket.TXInfo, config.C.NetworkServer.Band.Band)
	if err != nil {
		return errors.Wrap(err, "get data-rate index error")
	}

	var txCh int
	for _, defaultChannel := range []bool{true, false} {
		i, err := config.C.NetworkServer.Band.Band.GetUplinkChannelIndex(int(ctx.RXPacket.TXInfo.Frequency), defaultChannel)
		if err != nil {
			continue
		}

		c, err := config.C.NetworkServer.Band.Band.GetUplinkChannel(i)
		if err != nil {
			return errors.Wrap(err, "get channel error")
		}

		// there could be multiple channels using the same frequency, but with different data-rates.
		// eg EU868:
		//  channel 1 (868.3 DR 0-5)
		//  channel x (868.3 DR 6)
		if c.MinDR <= txDR && c.MaxDR >= txDR {
			txCh = i
		}
	}

	ds, err := storage.GetDeviceSessionForPHYPayload(config.C.Redis.Pool, ctx.RXPacket.PHYPayload, txDR, txCh)
	if err != nil {
		return errors.Wrap(err, "get device-session error")
	}
	ctx.DeviceSession = ds

	return nil
}

func logUplinkFrame(ctx *dataContext) error {
	uplinkFrameSet, err := framelog.CreateUplinkFrameSet(ctx.RXPacket)
	if err != nil {
		return errors.Wrap(err, "create uplink frame-log error")
	}

	if err := framelog.LogUplinkFrameForDevEUI(ctx.DeviceSession.DevEUI, uplinkFrameSet); err != nil {
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
	currentDR, err := helpers.GetDataRateIndex(true, ctx.RXPacket.TXInfo, config.C.NetworkServer.Band.Band)
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

// appendMetaDataToUplinkHistory appends uplink related meta-data to the
// uplink history in the device-session.
// As this also stores the TXPower, this function must be called after
// processing the mac-commands (we might have asked the device to change
// its TXPower and if one of the mac-commands contains a LinkADRReq ACK
// this will update the TXPowerIndex on the device-session).
func appendMetaDataToUplinkHistory(ctx *dataContext) error {
	var maxSNR float64
	for i, rxInfo := range ctx.RXPacket.RXInfoSet {
		// as the default value is 0 and the LoRaSNR can be negative, we always
		// set it when i == 0 (the first item from the slice)
		if i == 0 || rxInfo.LoraSnr > maxSNR {
			maxSNR = rxInfo.LoraSnr
		}
	}

	ctx.DeviceSession.AppendUplinkHistory(storage.UplinkHistory{
		FCnt:         ctx.MACPayload.FHDR.FCnt,
		GatewayCount: len(ctx.RXPacket.RXInfoSet),
		MaxSNR:       maxSNR,
		TXPowerIndex: ctx.DeviceSession.TXPowerIndex,
	})

	return nil
}

func storeDeviceGatewayRXInfoSet(ctx *dataContext) error {
	dr, err := helpers.GetDataRateIndex(true, ctx.RXPacket.TXInfo, config.C.NetworkServer.Band.Band)
	if err != nil {
		errors.Wrap(err, "get data-rate error")
	}

	rxInfoSet := storage.DeviceGatewayRXInfoSet{
		DevEUI: ctx.DeviceSession.DevEUI,
		DR:     dr,
	}

	for i := range ctx.RXPacket.RXInfoSet {
		rxInfoSet.Items = append(rxInfoSet.Items, storage.DeviceGatewayRXInfo{
			GatewayID: helpers.GetGatewayID(ctx.RXPacket.RXInfoSet[i]),
			RSSI:      int(ctx.RXPacket.RXInfoSet[i].Rssi),
			LoRaSNR:   ctx.RXPacket.RXInfoSet[i].LoraSnr,
		})
	}

	err = storage.SaveDeviceGatewayRXInfoSet(config.C.Redis.Pool, rxInfoSet)
	if err != nil {
		return errors.Wrap(err, "save device gateway rx-info set error")
	}

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

func decryptFOptsMACCommands(ctx *dataContext) error {
	if ctx.DeviceSession.GetMACVersion() == lorawan.LoRaWAN1_0 {
		if err := ctx.RXPacket.PHYPayload.DecodeFOptsToMACCommands(); err != nil {
			return errors.Wrap(err, "decode fOpts to mac-commands error")
		}
	} else {
		if err := ctx.RXPacket.PHYPayload.DecryptFOpts(ctx.DeviceSession.NwkSEncKey); err != nil {
			return errors.Wrap(err, "decrypt fOpts mac-commands error")
		}
	}
	return nil
}

func decryptFRMPayloadMACCommands(ctx *dataContext) error {
	// only decrypt when FPort is equal to 0
	if ctx.MACPayload.FPort != nil && *ctx.MACPayload.FPort == 0 {
		if err := ctx.RXPacket.PHYPayload.DecryptFRMPayload(ctx.DeviceSession.NwkSEncKey); err != nil {
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
	if config.C.NetworkServer.NetworkSettings.DisableMACCommands {
		return nil
	}

	if len(ctx.MACPayload.FHDR.FOpts) > 0 {
		blocks, err := handleUplinkMACCommands(&ctx.DeviceSession, ctx.DeviceProfile, ctx.ServiceProfile, ctx.ApplicationServerClient, ctx.MACPayload.FHDR.FOpts, ctx.RXPacket)
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
	if config.C.NetworkServer.NetworkSettings.DisableMACCommands {
		return nil
	}

	if ctx.MACPayload.FPort != nil && *ctx.MACPayload.FPort == 0 {
		if len(ctx.MACPayload.FRMPayload) == 0 {
			return errors.New("expected mac commands, but FRMPayload is empty (FPort=0)")
		}

		blocks, err := handleUplinkMACCommands(&ctx.DeviceSession, ctx.DeviceProfile, ctx.ServiceProfile, ctx.ApplicationServerClient, ctx.MACPayload.FRMPayload, ctx.RXPacket)
		if err != nil {
			log.WithFields(log.Fields{
				"dev_eui":  ctx.DeviceSession.DevEUI,
				"commands": ctx.MACPayload.FRMPayload,
			}).Errorf("handle FRMPayload mac commands error: %s", err)
		} else {
			ctx.MACCommandResponses = append(ctx.MACCommandResponses, blocks...)
		}
	}

	return nil
}

func sendFRMPayloadToApplicationServer(ctx *dataContext) error {
	if ctx.MACPayload.FPort == nil || (ctx.MACPayload.FPort != nil && *ctx.MACPayload.FPort == 0) {
		return nil
	}

	publishDataUpReq := as.HandleUplinkDataRequest{
		DevEui:  ctx.DeviceSession.DevEUI[:],
		JoinEui: ctx.DeviceSession.JoinEUI[:],
		FCnt:    ctx.MACPayload.FHDR.FCnt,
		Adr:     ctx.MACPayload.FHDR.FCtrl.ADR,
		TxInfo:  ctx.RXPacket.TXInfo,
	}

	dr, err := helpers.GetDataRateIndex(true, ctx.RXPacket.TXInfo, config.C.NetworkServer.Band.Band)
	if err != nil {
		errors.Wrap(err, "get data-rate error")
	}
	publishDataUpReq.Dr = uint32(dr)

	if ctx.DeviceSession.AppSKeyEvelope != nil {
		publishDataUpReq.DeviceActivationContext = &as.DeviceActivationContext{
			DevAddr: ctx.DeviceSession.DevAddr[:],
			AppSKey: &common.KeyEnvelope{
				KekLabel: ctx.DeviceSession.AppSKeyEvelope.KEKLabel,
				AesKey:   ctx.DeviceSession.AppSKeyEvelope.AESKey,
			},
		}

		ctx.DeviceSession.AppSKeyEvelope = nil
	}

	if ctx.ServiceProfile.AddGWMetadata {
		publishDataUpReq.RxInfo = ctx.RXPacket.RXInfoSet
	}

	if ctx.MACPayload.FPort != nil {
		publishDataUpReq.FPort = uint32(*ctx.MACPayload.FPort)
	}

	if len(ctx.MACPayload.FRMPayload) == 1 {
		dataPL, ok := ctx.MACPayload.FRMPayload[0].(*lorawan.DataPayload)
		if !ok {
			return fmt.Errorf("expected type *lorawan.DataPayload, got %T", ctx.MACPayload.FRMPayload[0])
		}
		publishDataUpReq.Data = dataPL.Bytes

	}

	go func(asClient as.ApplicationServerServiceClient, publishDataUpReq as.HandleUplinkDataRequest) {
		ctx := context.Background()
		ctxTimeout, cancel := context.WithTimeout(ctx, applicationClientTimeout)
		defer cancel()

		if _, err := asClient.HandleUplinkData(ctxTimeout, &publishDataUpReq); err != nil {
			log.WithError(err).Error("publish uplink data to application-server error")
		}
	}(ctx.ApplicationServerClient, publishDataUpReq)

	return nil
}

func setLastRXInfoSet(ctx *dataContext) error {
	if len(ctx.RXPacket.RXInfoSet) != 0 {
		gatewayID := helpers.GetGatewayID(ctx.RXPacket.RXInfoSet[0])
		ctx.DeviceSession.UplinkGatewayHistory = map[lorawan.EUI64]storage.UplinkGatewayHistory{
			gatewayID: storage.UplinkGatewayHistory{},
		}
	}
	return nil
}

func syncUplinkFCnt(ctx *dataContext) error {
	// sync counter with that of the device + 1
	ctx.DeviceSession.FCntUp = ctx.MACPayload.FHDR.FCnt + 1
	return nil
}

func saveDeviceSession(ctx *dataContext) error {
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
	if qi.FCnt != ctx.DeviceSession.NFCntDown-1 {
		log.WithFields(log.Fields{
			"dev_eui":                  ctx.DeviceSession.DevEUI,
			"device_queue_item_fcnt":   qi.FCnt,
			"device_session_fcnt_down": ctx.DeviceSession.NFCntDown,
		}).Error("frame-counter of device-queue item out of sync with device-session")
		return nil
	}

	if err := storage.DeleteDeviceQueueItem(config.C.PostgreSQL.DB, qi.ID); err != nil {
		return errors.Wrap(err, "delete device-queue item error")
	}

	_, err = ctx.ApplicationServerClient.HandleDownlinkACK(context.Background(), &as.HandleDownlinkACKRequest{
		DevEui:       ctx.DeviceSession.DevEUI[:],
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
	rxInfoReq := nc.HandleUplinkMetaDataRequest{
		DevEui: ds.DevEUI[:],
		TxInfo: rxPacket.TXInfo,
		RxInfo: rxPacket.RXInfoSet,
	}

	_, err := config.C.NetworkController.Client.HandleUplinkMetaData(context.Background(), &rxInfoReq)
	if err != nil {
		return fmt.Errorf("publish rxinfo to network-controller error: %s", err)
	}
	log.WithFields(log.Fields{
		"dev_eui": ds.DevEUI,
	}).Info("rx info sent to network-controller")
	return nil
}

func handleUplinkMACCommands(ds *storage.DeviceSession, dp storage.DeviceProfile, sp storage.ServiceProfile, asClient as.ApplicationServerServiceClient, commands []lorawan.Payload, rxPacket models.RXPacket) ([]storage.MACCommandBlock, error) {
	var cids []lorawan.CID
	var out []storage.MACCommandBlock
	blocks := make(map[lorawan.CID]storage.MACCommandBlock)

	// group mac-commands by CID
	for _, pl := range commands {
		cmd, ok := pl.(*lorawan.MACCommand)
		if !ok {
			return nil, fmt.Errorf("expected *lorawan.MACCommand, got %T", pl)
		}
		if cmd == nil {
			return nil, errors.New("*lorawan.MACCommand must not be nil")
		}

		block, ok := blocks[cmd.CID]
		if !ok {
			block = storage.MACCommandBlock{
				CID: cmd.CID,
			}
			cids = append(cids, cmd.CID)
		}
		block.MACCommands = append(block.MACCommands, *cmd)
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
			responseBlocks, err := maccommand.Handle(ds, dp, sp, asClient, block, pending, rxPacket)
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
			_, err = config.C.NetworkController.Client.HandleUplinkMACCommand(context.Background(), &nc.HandleUplinkMACCommandRequest{
				DevEui:   ds.DevEUI[:],
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
