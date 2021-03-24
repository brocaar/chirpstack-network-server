package data

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-api/go/v3/as"
	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-api/go/v3/nc"
	"github.com/brocaar/chirpstack-network-server/internal/backend/applicationserver"
	"github.com/brocaar/chirpstack-network-server/internal/backend/controller"
	"github.com/brocaar/chirpstack-network-server/internal/band"
	"github.com/brocaar/chirpstack-network-server/internal/config"
	datadown "github.com/brocaar/chirpstack-network-server/internal/downlink/data"
	"github.com/brocaar/chirpstack-network-server/internal/framelog"
	"github.com/brocaar/chirpstack-network-server/internal/helpers"
	"github.com/brocaar/chirpstack-network-server/internal/logging"
	"github.com/brocaar/chirpstack-network-server/internal/maccommand"
	"github.com/brocaar/chirpstack-network-server/internal/models"
	"github.com/brocaar/chirpstack-network-server/internal/roaming"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
)

const applicationClientTimeout = time.Second

// ErrAbort is used to abort the flow without error
var ErrAbort = errors.New("nothing to do")

var tasks = []func(*dataContext) error{
	setContextFromDataPHYPayload,
	handlePassiveRoamingDevice,
	getDeviceSessionForPHYPayload,
	abortOnDeviceIsDisabled,
	getDeviceProfile,
	getServiceProfile,
	filterRxInfoByServiceProfile,
	decryptFOptsMACCommands,
	decryptFRMPayloadMACCommands,
	logUplinkFrame,
	getApplicationServerClientForDataUp,
	setADR,
	setUplinkDataRate,
	setBeaconLocked,
	sendUplinkMetaDataToNetworkController,
	handleFOptsMACCommands,
	handleFRMPayloadMACCommands,
	storeDeviceGatewayRXInfoSet,
	appendMetaDataToUplinkHistory,
	sendFRMPayloadToApplicationServer,
	syncUplinkFCnt,
	saveDeviceSession,
	handleUplinkACK,
	handleDownlink,
}

var (
	getDownlinkDataDelay time.Duration
	disableMACCommands   bool
)

// Setup configures the package.
func Setup(conf config.Config) error {
	getDownlinkDataDelay = conf.NetworkServer.GetDownlinkDataDelay
	disableMACCommands = conf.NetworkServer.NetworkSettings.DisableMACCommands

	return nil
}

type dataContext struct {
	ctx context.Context

	RXPacket                models.RXPacket
	MACPayload              *lorawan.MACPayload
	DeviceSession           storage.DeviceSession
	DeviceProfile           storage.DeviceProfile
	ServiceProfile          storage.ServiceProfile
	ApplicationServerClient as.ApplicationServerServiceClient
	MACCommandResponses     []storage.MACCommandBlock
	MustSendDownlink        bool
}

// Handle handles an uplink data frame
func Handle(ctx context.Context, rxPacket models.RXPacket) error {
	dctx := dataContext{
		ctx:      ctx,
		RXPacket: rxPacket,
	}

	for _, t := range tasks {
		if err := t(&dctx); err != nil {
			if err == ErrAbort {
				return nil
			}

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

func handlePassiveRoamingDevice(ctx *dataContext) error {
	if roaming.IsRoamingDevAddr(ctx.MACPayload.FHDR.DevAddr) {
		log.WithFields(log.Fields{
			"dev_addr": ctx.MACPayload.FHDR.DevAddr,
			"ctx_id":   ctx.ctx.Value(logging.ContextIDKey),
		}).Info("uplink/data: devaddr does not match netid, assuming roaming device")

		if err := HandleRoamingFNS(ctx.ctx, ctx.RXPacket, ctx.MACPayload); err != nil {
			log.WithError(err).WithFields(log.Fields{
				"dev_addr": ctx.MACPayload.FHDR.DevAddr,
				"ctx_id":   ctx.ctx.Value(logging.ContextIDKey),
			}).Error("uplink/data: handling passive-roaming error")
		}

		// the flow stops here
		return ErrAbort
	}

	return nil
}

func getDeviceSessionForPHYPayload(ctx *dataContext) error {
	txCh, err := band.Band().GetUplinkChannelIndexForFrequencyDR(int(ctx.RXPacket.TXInfo.Frequency), ctx.RXPacket.DR)
	if err != nil {
		return errors.Wrap(err, "get channel error")
	}

	ds, err := storage.GetDeviceSessionForPHYPayload(ctx.ctx, ctx.RXPacket.PHYPayload, ctx.RXPacket.DR, txCh)
	if err != nil {
		returnErr := errors.Wrap(err, "get device-session error")

		// Forward error notification to the AS for debugging purpose.
		if err == storage.ErrFrameCounterReset || err == storage.ErrInvalidMIC || err == storage.ErrFrameCounterRetransmission {
			req := as.HandleErrorRequest{
				DevEui: ds.DevEUI[:],
				Error:  err.Error(),
				FCnt:   ctx.MACPayload.FHDR.FCnt,
			}

			switch err {
			case storage.ErrFrameCounterReset:
				req.Type = as.ErrorType_DATA_UP_FCNT_RESET
			case storage.ErrInvalidMIC:
				req.Type = as.ErrorType_DATA_UP_MIC
			case storage.ErrFrameCounterRetransmission:
				req.Type = as.ErrorType_DATA_UP_FCNT_RETRANSMISSION
			}

			asClient, err := helpers.GetASClientForRoutingProfileID(ctx.ctx, ds.RoutingProfileID)
			if err != nil {
				log.WithError(err).WithFields(log.Fields{
					"dev_eui": ds.DevEUI,
					"ctx_id":  ctx.ctx.Value(logging.ContextIDKey),
				}).Error("uplink/data: get as client for routing-profile id error")
			} else {
				_, err := asClient.HandleError(ctx.ctx, &req)
				if err != nil {
					log.WithError(err).WithFields(log.Fields{
						"dev_eui": ds.DevEUI,
						"ctx_id":  ctx.ctx.Value(logging.ContextIDKey),
					}).Error("uplink/data: as.HandleError error")
				}
			}
		}

		return returnErr
	}

	ctx.DeviceSession = ds
	return nil
}

func abortOnDeviceIsDisabled(ctx *dataContext) error {
	if ctx.DeviceSession.IsDisabled {
		return ErrAbort
	}
	return nil
}

func logUplinkFrame(ctx *dataContext) error {
	uplinkFrameLog, err := framelog.CreateUplinkFrameLog(ctx.RXPacket)
	if err != nil {
		return errors.Wrap(err, "create uplink frame-log error")
	}

	if err := framelog.LogUplinkFrameForDevEUI(ctx.ctx, ctx.DeviceSession.DevEUI, uplinkFrameLog); err != nil {
		log.WithError(err).Error("log uplink frame for device error")
	}

	return nil
}

func getDeviceProfile(ctx *dataContext) error {
	dp, err := storage.GetAndCacheDeviceProfile(ctx.ctx, storage.DB(), ctx.DeviceSession.DeviceProfileID)
	if err != nil {
		return errors.Wrap(err, "get device-profile error")
	}
	ctx.DeviceProfile = dp

	return nil
}

func getServiceProfile(ctx *dataContext) error {
	sp, err := storage.GetAndCacheServiceProfile(ctx.ctx, storage.DB(), ctx.DeviceSession.ServiceProfileID)
	if err != nil {
		return errors.Wrap(err, "get service-profile error")
	}
	ctx.ServiceProfile = sp

	return nil
}

func filterRxInfoByServiceProfile(ctx *dataContext) error {
	err := helpers.FilterRxInfoByServiceProfileID(ctx.DeviceSession.ServiceProfileID, &ctx.RXPacket)
	if err != nil {
		if err == helpers.ErrNoElements {
			log.WithFields(log.Fields{
				"dev_eui": ctx.DeviceSession.DevEUI,
				"ctx_id":  ctx.ctx.Value(logging.ContextIDKey),
			}).Warning("uplink/data: none of the receiving gateways are public or have the same service-profile")
			return ErrAbort
		}
		return err
	}

	return nil
}

func setADR(ctx *dataContext) error {
	ctx.DeviceSession.ADR = ctx.MACPayload.FHDR.FCtrl.ADR
	return nil
}

func setUplinkDataRate(ctx *dataContext) error {
	// The node changed its data-rate. Possibly the node did also reset its
	// tx-power to max power. Because of this, we need to reset the tx-power
	// and the uplink history at the network-server side too.
	if ctx.DeviceSession.DR != ctx.RXPacket.DR {
		ctx.DeviceSession.TXPowerIndex = 0
		ctx.DeviceSession.UplinkHistory = []storage.UplinkHistory{}
	}

	ctx.DeviceSession.DR = ctx.RXPacket.DR

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
	rxInfoSet := storage.DeviceGatewayRXInfoSet{
		DevEUI: ctx.DeviceSession.DevEUI,
		DR:     ctx.RXPacket.DR,
	}

	for i := range ctx.RXPacket.RXInfoSet {
		rxInfoSet.Items = append(rxInfoSet.Items, storage.DeviceGatewayRXInfo{
			GatewayID: helpers.GetGatewayID(ctx.RXPacket.RXInfoSet[i]),
			RSSI:      int(ctx.RXPacket.RXInfoSet[i].Rssi),
			LoRaSNR:   ctx.RXPacket.RXInfoSet[i].LoraSnr,
			Board:     ctx.RXPacket.RXInfoSet[i].Board,
			Antenna:   ctx.RXPacket.RXInfoSet[i].Antenna,
			Context:   ctx.RXPacket.RXInfoSet[i].Context,
		})
	}

	err := storage.SaveDeviceGatewayRXInfoSet(ctx.ctx, rxInfoSet)
	if err != nil {
		return errors.Wrap(err, "save device gateway rx-info set error")
	}

	return nil
}

func getApplicationServerClientForDataUp(ctx *dataContext) error {
	rp, err := storage.GetRoutingProfile(ctx.ctx, storage.DB(), ctx.DeviceSession.RoutingProfileID)
	if err != nil {
		return errors.Wrap(err, "get routing-profile error")
	}

	asClient, err := applicationserver.Pool().Get(rp.ASID, []byte(rp.CACert), []byte(rp.TLSCert), []byte(rp.TLSKey))
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
		d, err := storage.GetDevice(ctx.ctx, storage.DB(), ctx.DeviceSession.DevEUI, false)
		if err != nil {
			return errors.Wrap(err, "get device")
		}
		d.Mode = storage.DeviceModeB
		if err := storage.UpdateDevice(ctx.ctx, storage.DB(), &d); err != nil {
			return errors.Wrap(err, "update device error")
		}

		// Re-create device-queue items.
		// Note that the CreateDeviceQueueItem function will take care of setting
		// the Class-B ping-slot timing.
		if err := storage.Transaction(func(tx sqlx.Ext) error {
			items, err := storage.GetDeviceQueueItemsForDevEUI(ctx.ctx, tx, ctx.DeviceSession.DevEUI)
			if err != nil {
				return errors.Wrap(err, "get device-queue items error")
			}

			if err := storage.FlushDeviceQueueForDevEUI(ctx.ctx, tx, ctx.DeviceSession.DevEUI); err != nil {
				return errors.Wrap(err, "flush device-queue for deveui error")
			}

			for _, item := range items {
				if err := storage.CreateDeviceQueueItem(ctx.ctx, tx, &item, ctx.DeviceProfile, ctx.DeviceSession); err != nil {
					return errors.Wrap(err, "create device-queue item error")
				}
			}

			return nil
		}); err != nil {
			return errors.Wrap(err, "re-create device-queue items error")
		}

		log.WithFields(log.Fields{
			"dev_eui": ctx.DeviceSession.DevEUI,
			"mode":    storage.DeviceModeB,
			"ctx_id":  ctx.ctx.Value(logging.ContextIDKey),
		}).Info("device changed mode")
	} else {
		d, err := storage.GetDevice(ctx.ctx, storage.DB(), ctx.DeviceSession.DevEUI, false)
		if err != nil {
			return errors.Wrap(err, "get device")
		}
		d.Mode = storage.DeviceModeA
		if err := storage.UpdateDevice(ctx.ctx, storage.DB(), &d); err != nil {
			return errors.Wrap(err, "update device error")
		}

		log.WithFields(log.Fields{
			"dev_eui": ctx.DeviceSession.DevEUI,
			"mode":    storage.DeviceModeA,
			"ctx_id":  ctx.ctx.Value(logging.ContextIDKey),
		}).Info("device changed mode")
	}

	return nil
}

func sendUplinkMetaDataToNetworkController(ctx *dataContext) error {
	if controller.Client() == nil {
		return nil
	}

	req := nc.HandleUplinkMetaDataRequest{
		DevEui: ctx.DeviceSession.DevEUI[:],
		TxInfo: ctx.RXPacket.TXInfo,
		RxInfo: ctx.RXPacket.RXInfoSet,
	}

	// set message type
	switch ctx.RXPacket.PHYPayload.MHDR.MType {
	case lorawan.UnconfirmedDataUp:
		req.MessageType = nc.MType_UNCONFIRMED_DATA_UP
	case lorawan.ConfirmedDataUp:
		req.MessageType = nc.MType_CONFIRMED_DATA_UP
	}

	// set phypayload size
	if b, err := ctx.RXPacket.PHYPayload.MarshalBinary(); err == nil {
		req.PhyPayloadByteCount = uint32(len(b))
	}

	// set fopts size
	for _, m := range ctx.MACPayload.FHDR.FOpts {
		if b, err := m.MarshalBinary(); err == nil {
			req.MacCommandByteCount += uint32(len(b))
		}
	}

	// set frmpayload size
	for _, pl := range ctx.MACPayload.FRMPayload {
		if b, err := pl.MarshalBinary(); err == nil {
			if ctx.MACPayload.FPort != nil && *ctx.MACPayload.FPort != 0 {
				req.ApplicationPayloadByteCount += uint32(len(b))
			} else {
				req.MacCommandByteCount += uint32(len(b))
			}
		}
	}

	// send async to controller
	go func() {
		_, err := controller.Client().HandleUplinkMetaData(ctx.ctx, &req)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"dev_eui": ctx.DeviceSession.DevEUI,
				"ctx_id":  ctx.ctx.Value(logging.ContextIDKey),
			}).Error("sent uplink meta-data to network-controller error")
			return
		}

		log.WithFields(log.Fields{
			"dev_eui": ctx.DeviceSession.DevEUI,
			"ctx_id":  ctx.ctx.Value(logging.ContextIDKey),
		}).Info("sent uplink meta-data to network-controller")
	}()

	return nil
}

func handleFOptsMACCommands(ctx *dataContext) error {
	if len(ctx.MACPayload.FHDR.FOpts) == 0 {
		return nil
	}

	blocks, mustRespondWithDownlink, err := handleUplinkMACCommands(
		ctx.ctx,
		&ctx.DeviceSession,
		ctx.DeviceProfile,
		ctx.ServiceProfile,
		ctx.ApplicationServerClient,
		ctx.MACPayload.FHDR.FOpts,
		ctx.RXPacket,
	)
	if err != nil {
		log.WithFields(log.Fields{
			"dev_eui": ctx.DeviceSession.DevEUI,
			"fopts":   ctx.MACPayload.FHDR.FOpts,
			"ctx_id":  ctx.ctx.Value(logging.ContextIDKey),
		}).Errorf("handle FOpts mac commands error: %s", err)
		return nil
	}

	ctx.MACCommandResponses = append(ctx.MACCommandResponses, blocks...)
	if !ctx.MustSendDownlink {
		ctx.MustSendDownlink = mustRespondWithDownlink
	}

	return nil
}

func handleFRMPayloadMACCommands(ctx *dataContext) error {
	if ctx.MACPayload.FPort == nil || *ctx.MACPayload.FPort != 0 {
		return nil
	}

	if len(ctx.MACPayload.FRMPayload) == 0 {
		return errors.New("expected mac commands, but FRMPayload is empty (FPort=0)")
	}

	blocks, mustRespondWithDownlink, err := handleUplinkMACCommands(ctx.ctx, &ctx.DeviceSession, ctx.DeviceProfile, ctx.ServiceProfile, ctx.ApplicationServerClient, ctx.MACPayload.FRMPayload, ctx.RXPacket)
	if err != nil {
		log.WithFields(log.Fields{
			"dev_eui":  ctx.DeviceSession.DevEUI,
			"commands": ctx.MACPayload.FRMPayload,
			"ctx_id":   ctx.ctx.Value(logging.ContextIDKey),
		}).Errorf("handle FRMPayload mac commands error: %s", err)
		return nil
	}

	ctx.MACCommandResponses = append(ctx.MACCommandResponses, blocks...)
	if !ctx.MustSendDownlink {
		ctx.MustSendDownlink = mustRespondWithDownlink
	}

	return nil
}

func sendFRMPayloadToApplicationServer(ctx *dataContext) error {
	publishDataUpReq := as.HandleUplinkDataRequest{
		DevEui:          ctx.DeviceSession.DevEUI[:],
		JoinEui:         ctx.DeviceSession.JoinEUI[:],
		FCnt:            ctx.MACPayload.FHDR.FCnt,
		Adr:             ctx.MACPayload.FHDR.FCtrl.ADR,
		TxInfo:          ctx.RXPacket.TXInfo,
		ConfirmedUplink: ctx.RXPacket.PHYPayload.MHDR.MType == lorawan.ConfirmedDataUp,
	}

	publishDataUpReq.Dr = uint32(ctx.RXPacket.DR)

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

	// The DataPayload is only used for FPort != 0 (or nil)
	if ctx.MACPayload.FPort != nil && *ctx.MACPayload.FPort != 0 && len(ctx.MACPayload.FRMPayload) == 1 {
		dataPL, ok := ctx.MACPayload.FRMPayload[0].(*lorawan.DataPayload)
		if !ok {
			return fmt.Errorf("expected type *lorawan.DataPayload, got %T", ctx.MACPayload.FRMPayload[0])
		}
		publishDataUpReq.Data = dataPL.Bytes
	}

	go func(ctx context.Context, asClient as.ApplicationServerServiceClient, publishDataUpReq as.HandleUplinkDataRequest) {
		ctxTimeout, cancel := context.WithTimeout(ctx, applicationClientTimeout)
		defer cancel()

		if _, err := asClient.HandleUplinkData(ctxTimeout, &publishDataUpReq); err != nil {
			log.WithFields(log.Fields{
				"ctx_id": ctx.Value(logging.ContextIDKey),
			}).WithError(err).Error("publish uplink data to application-server error")
		}
	}(ctx.ctx, ctx.ApplicationServerClient, publishDataUpReq)

	return nil
}

func syncUplinkFCnt(ctx *dataContext) error {
	// sync counter with that of the device + 1
	ctx.DeviceSession.FCntUp = ctx.MACPayload.FHDR.FCnt + 1
	return nil
}

func saveDeviceSession(ctx *dataContext) error {
	// save node-session
	return storage.SaveDeviceSession(ctx.ctx, ctx.DeviceSession)
}

func handleUplinkACK(ctx *dataContext) error {
	if !ctx.MACPayload.FHDR.FCtrl.ACK {
		return nil
	}

	qi, err := storage.GetPendingDeviceQueueItemForDevEUI(ctx.ctx, storage.DB(), ctx.DeviceSession.DevEUI)
	if err != nil {
		log.WithFields(log.Fields{
			"dev_eui": ctx.DeviceSession.DevEUI,
			"ctx_id":  ctx.ctx.Value(logging.ContextIDKey),
		}).WithError(err).Error("get device-queue item error")
		return nil
	}
	if qi.FCnt != ctx.DeviceSession.NFCntDown-1 {
		log.WithFields(log.Fields{
			"dev_eui":                  ctx.DeviceSession.DevEUI,
			"device_queue_item_fcnt":   qi.FCnt,
			"device_session_fcnt_down": ctx.DeviceSession.NFCntDown,
			"ctx_id":                   ctx.ctx.Value(logging.ContextIDKey),
		}).Error("frame-counter of device-queue item out of sync with device-session")
		return nil
	}

	if err := storage.DeleteDeviceQueueItem(ctx.ctx, storage.DB(), qi.ID); err != nil {
		return errors.Wrap(err, "delete device-queue item error")
	}

	_, err = ctx.ApplicationServerClient.HandleDownlinkACK(ctx.ctx, &as.HandleDownlinkACKRequest{
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
	time.Sleep(getDownlinkDataDelay)
	if err := datadown.HandleResponse(
		ctx.ctx,
		ctx.RXPacket,
		ctx.ServiceProfile,
		ctx.DeviceSession,
		ctx.MACPayload.FHDR.FCtrl.ADR,
		ctx.MACPayload.FHDR.FCtrl.ADRACKReq || ctx.MustSendDownlink,
		ctx.RXPacket.PHYPayload.MHDR.MType == lorawan.ConfirmedDataUp,
		ctx.MACCommandResponses,
	); err != nil {
		return errors.Wrap(err, "run uplink response flow error")
	}

	return nil
}

// handleUplinkMACCommands handles the given uplink mac-commands.
// It returns the mac-commands to respond with + a bool indicating the a downlink MUST be send,
// this to make sure that a response has been received by the NS.
func handleUplinkMACCommands(ctx context.Context, ds *storage.DeviceSession, dp storage.DeviceProfile, sp storage.ServiceProfile, asClient as.ApplicationServerServiceClient, commands []lorawan.Payload, rxPacket models.RXPacket) ([]storage.MACCommandBlock, bool, error) {
	var cids []lorawan.CID
	var out []storage.MACCommandBlock
	var mustRespondWithDownlink bool
	blocks := make(map[lorawan.CID]storage.MACCommandBlock)

	// group mac-commands by CID
	for _, pl := range commands {
		cmd, ok := pl.(*lorawan.MACCommand)
		if !ok {
			return nil, false, fmt.Errorf("expected *lorawan.MACCommand, got %T", pl)
		}
		if cmd == nil {
			return nil, false, errors.New("*lorawan.MACCommand must not be nil")
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
		switch cid {
		case lorawan.RXTimingSetupAns, lorawan.RXParamSetupAns:
			// From the specs:
			// The RXTimingSetupAns command should be added in the FOpt field of all uplinks until a
			// class A downlink is received by the end-device.
			mustRespondWithDownlink = true
		default:
			// nothing to do
		}

		block := blocks[cid]

		logFields := log.Fields{
			"dev_eui": ds.DevEUI,
			"cid":     block.CID,
			"ctx_id":  ctx.Value(logging.ContextIDKey),
		}

		var external bool

		if !disableMACCommands {
			// read pending mac-command block for CID. e.g. on case of an ack, the
			// pending mac-command block contains the request.
			// we need this pending mac-command block to find out if the command
			// was scheduled through the API (external).
			pending, err := storage.GetPendingMACCommand(ctx, ds.DevEUI, block.CID)
			if err != nil {
				log.WithFields(logFields).Errorf("read pending mac-command error: %s", err)
				continue
			}
			if pending != nil {
				external = pending.External
			}

			// in case the node is requesting a mac-command, there is nothing pending
			if pending != nil {
				if err = storage.DeletePendingMACCommand(ctx, ds.DevEUI, block.CID); err != nil {
					log.WithFields(logFields).Errorf("delete pending mac-command error: %s", err)
				}
			}

			// CID >= 0x80 are proprietary mac-commands and are not handled by ChirpStack Network Server
			if block.CID < 0x80 {
				responseBlocks, err := maccommand.Handle(ctx, ds, dp, sp, asClient, block, pending, rxPacket)
				if err != nil {
					log.WithFields(logFields).Errorf("handle mac-command block error: %s", err)
				} else {
					out = append(out, responseBlocks...)
				}
			}
		}

		// Report to external controller:
		//  * in case of proprietary mac-commands
		//  * in case when the request has been scheduled through the API
		//  * in case mac-commands are disabled in the ChirpStack Network Server configuration
		if disableMACCommands || block.CID >= 0x80 || external {
			var data [][]byte
			for _, cmd := range block.MACCommands {
				b, err := cmd.MarshalBinary()
				if err != nil {
					log.WithFields(logFields).Errorf("marshal mac-command to binary error: %s", err)
					continue
				}
				data = append(data, b)
			}
			_, err := controller.Client().HandleUplinkMACCommand(context.Background(), &nc.HandleUplinkMACCommandRequest{
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

	return out, mustRespondWithDownlink, nil
}
