package rejoin

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/internal/config"
	joindown "github.com/brocaar/loraserver/internal/downlink/join"
	"github.com/brocaar/loraserver/internal/framelog"
	"github.com/brocaar/loraserver/internal/helpers"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
	"github.com/brocaar/lorawan/band"
)

var tasks = []func(*context) error{
	setContextFromRejoinRequestPHY,
	logRejoinRequestFramesCollected,
	getDeviceAndProfiles,
	forRejoinType([]lorawan.JoinType{lorawan.RejoinRequestType0, lorawan.RejoinRequestType2},
		getDeviceSession,
		validateRejoinCounter0,
		validateMIC,
		getRandomDevAddr,
		getRejoinAcceptFromJS,
	),
	forRejoinType([]lorawan.JoinType{lorawan.RejoinRequestType0},
		setRejoin0PendingDeviceSession,
	),
	forRejoinType([]lorawan.JoinType{lorawan.RejoinRequestType2},
		setRejoin2PendingDeviceSession,
	),
	createDeviceActivation,
	sendJoinAcceptDownlink,
}

type context struct {
	RXPacket   models.RXPacket
	RejoinType lorawan.JoinType
	RJCount    uint16

	NetID   lorawan.NetID
	DevEUI  lorawan.EUI64
	JoinEUI lorawan.EUI64

	Device         storage.Device
	ServiceProfile storage.ServiceProfile
	DeviceProfile  storage.DeviceProfile
	DeviceSession  storage.DeviceSession

	DevAddr lorawan.DevAddr

	RejoinAnsPayload backend.RejoinAnsPayload
}

// Handle handles a rejoin-request.
func Handle(rxPacket models.RXPacket) error {
	ctx := context{
		RXPacket: rxPacket,
	}

	for _, t := range tasks {
		if err := t(&ctx); err != nil {
			return err
		}
	}

	return nil
}

func forRejoinType(types []lorawan.JoinType, tasks ...func(*context) error) func(*context) error {
	return func(ctx *context) error {
		for _, t := range types {
			if ctx.RejoinType == t {
				for _, f := range tasks {
					if err := f(ctx); err != nil {
						return err
					}
				}
			}
		}

		return nil
	}
}

func setContextFromRejoinRequestPHY(ctx *context) error {
	switch v := ctx.RXPacket.PHYPayload.MACPayload.(type) {
	case *lorawan.RejoinRequestType02Payload:
		ctx.NetID = v.NetID
		ctx.DevEUI = v.DevEUI
		ctx.RJCount = v.RJCount0
		ctx.RejoinType = v.RejoinType
	case *lorawan.RejoinRequestType1Payload:
		ctx.JoinEUI = v.JoinEUI
		ctx.DevEUI = v.DevEUI
		ctx.RJCount = v.RJCount1
		ctx.RejoinType = v.RejoinType
	default:
		return fmt.Errorf("expected *lorawan.RejoinRequestType02Payload or *lorawan.RejoinRequestType1Payload, got: %T", ctx.RXPacket.PHYPayload.MACPayload)
	}

	return nil
}

func logRejoinRequestFramesCollected(ctx *context) error {
	var gatewayIDs []string
	for _, p := range ctx.RXPacket.RXInfoSet {
		gatewayIDs = append(gatewayIDs, helpers.GetGatewayID(p).String())
	}

	uplinkFrameSet, err := framelog.CreateUplinkFrameSet(ctx.RXPacket)
	if err != nil {
		return errors.Wrap(err, "create uplink frame-set error")
	}

	if err := framelog.LogUplinkFrameForDevEUI(ctx.DevEUI, uplinkFrameSet); err != nil {
		log.WithError(err).Error("log uplink frame for device error")
	}

	log.WithFields(log.Fields{
		"dev_eui":     ctx.DevEUI,
		"gw_count":    len(gatewayIDs),
		"gw_ids":      strings.Join(gatewayIDs, ", "),
		"mtype":       ctx.RXPacket.PHYPayload.MHDR.MType,
		"rejoin_type": ctx.RejoinType,
	}).Info("packet(s) collected")

	return nil
}

func getDeviceAndProfiles(ctx *context) error {
	var err error

	ctx.Device, err = storage.GetDevice(config.C.PostgreSQL.DB, ctx.DevEUI)
	if err != nil {
		return errors.Wrap(err, "get device error")
	}

	ctx.DeviceProfile, err = storage.GetDeviceProfile(config.C.PostgreSQL.DB, ctx.Device.DeviceProfileID)
	if err != nil {
		return errors.Wrap(err, "get device-profile error")
	}

	ctx.ServiceProfile, err = storage.GetServiceProfile(config.C.PostgreSQL.DB, ctx.Device.ServiceProfileID)
	if err != nil {
		return errors.Wrap(err, "get service-profile error")
	}

	if !ctx.DeviceProfile.SupportsJoin {
		return errors.New("device does not support join")
	}

	return nil
}

func getDeviceSession(ctx *context) error {
	var err error
	ctx.DeviceSession, err = storage.GetDeviceSession(config.C.Redis.Pool, ctx.DevEUI)
	if err != nil {
		return errors.Wrap(err, "get device-session error")
	}
	return nil
}

func validateRejoinCounter0(ctx *context) error {
	// RejoinCount0 contains the next expected value
	// This assumes that 0 is the first counter values that will occur
	if ctx.RJCount >= ctx.DeviceSession.RejoinCount0 {
		// Increment it so that it can't be re-used
		ctx.DeviceSession.RejoinCount0 = ctx.RJCount + 1

		if ctx.DeviceSession.RejoinCount0 == 0 {
			return errors.New("RJcount0 rollover detected")
		}
		return nil
	}

	return errors.New("invalid RJcount0")
}

func validateMIC(ctx *context) error {
	ok, err := ctx.RXPacket.PHYPayload.ValidateUplinkJoinMIC(ctx.DeviceSession.SNwkSIntKey)
	if err != nil {
		return err
	}

	if ok {
		return nil
	}

	return errors.New("invalid MIC")
}

func getRandomDevAddr(ctx *context) error {
	devAddr, err := storage.GetRandomDevAddr(config.C.Redis.Pool, config.C.NetworkServer.NetID)
	if err != nil {
		return errors.Wrap(err, "get random DevAddr error")
	}
	ctx.DevAddr = devAddr
	return nil
}

func getRejoinAcceptFromJS(ctx *context) error {
	b, err := ctx.RXPacket.PHYPayload.MarshalBinary()
	if err != nil {
		return errors.Wrap(err, "PHYPayload marshal binary error")
	}

	randomBytes := make([]byte, 4)
	_, err = rand.Read(randomBytes)
	if err != nil {
		return errors.Wrap(err, "read random bytes error")
	}
	transactionID := binary.LittleEndian.Uint32(randomBytes)

	rejoinReqPL := backend.RejoinReqPayload{
		BasePayload: backend.BasePayload{
			ProtocolVersion: backend.ProtocolVersion1_0,
			SenderID:        config.C.NetworkServer.NetID.String(),
			ReceiverID:      ctx.DeviceSession.JoinEUI.String(),
			TransactionID:   transactionID,
			MessageType:     backend.RejoinReq,
		},
		MACVersion: ctx.DeviceProfile.MACVersion,
		PHYPayload: backend.HEXBytes(b),
		DevEUI:     ctx.DevEUI,
		DevAddr:    ctx.DevAddr,
		DLSettings: lorawan.DLSettings{
			OptNeg:      !strings.HasPrefix(ctx.DeviceProfile.MACVersion, "1.0"),
			RX2DataRate: uint8(config.C.NetworkServer.NetworkSettings.RX2DR),
			RX1DROffset: uint8(config.C.NetworkServer.NetworkSettings.RX1DROffset),
		},
		RxDelay: config.C.NetworkServer.NetworkSettings.RX1Delay,
	}

	// 0: Used to reset a device context including all radio parameters.
	// 1: Exactly equivalent to the initial Join-Request message but may
	//    be transmitted on top of normal applicative traffic without
	//    disconnecting the device.
	// 2: Used to rekey a device or change its DevAddr (DevAddr, session keys,
	//    frame counters). Radio parameters are kept unchanged.
	if ctx.RejoinType == lorawan.RejoinRequestType0 || ctx.RejoinType == lorawan.RejoinRequestType1 {
		cFList := config.C.NetworkServer.Band.Band.GetCFList(ctx.DeviceSession.MACVersion)
		if cFList != nil {
			cFListB, err := cFList.MarshalBinary()
			if err != nil {
				return errors.Wrap(err, "marshal cflist error")
			}
			rejoinReqPL.CFList = backend.HEXBytes(cFListB)
		}
	}

	jsClient, err := config.C.JoinServer.Pool.Get(ctx.DeviceSession.JoinEUI)
	if err != nil {
		return errors.Wrap(err, "get join-server client error")
	}

	ctx.RejoinAnsPayload, err = jsClient.RejoinReq(rejoinReqPL)
	if err != nil {
		return errors.Wrap(err, "rejoin-request to join-server error")
	}

	return nil
}

func flushDeviceQueue(ctx *context) error {
	if err := storage.FlushDeviceQueueForDevEUI(config.C.PostgreSQL.DB, ctx.DevEUI); err != nil {
		return errors.Wrap(err, "flush device-queue error")
	}
	return nil
}

func setRejoin0PendingDeviceSession(ctx *context) error {
	pendingDS := storage.DeviceSession{
		DeviceProfileID:  ctx.Device.DeviceProfileID,
		ServiceProfileID: ctx.Device.ServiceProfileID,
		RoutingProfileID: ctx.Device.RoutingProfileID,

		MACVersion:            ctx.DeviceProfile.MACVersion,
		DevAddr:               ctx.DevAddr,
		JoinEUI:               ctx.DeviceSession.JoinEUI,
		DevEUI:                ctx.DeviceSession.DevEUI,
		RXWindow:              storage.RX1,
		RXDelay:               uint8(config.C.NetworkServer.NetworkSettings.RX1Delay),
		RX1DROffset:           uint8(config.C.NetworkServer.NetworkSettings.RX1DROffset),
		RX2DR:                 uint8(config.C.NetworkServer.NetworkSettings.RX2DR),
		RX2Frequency:          config.C.NetworkServer.Band.Band.GetDefaults().RX2Frequency,
		EnabledUplinkChannels: config.C.NetworkServer.Band.Band.GetStandardUplinkChannelIndices(),
		ExtraUplinkChannels:   make(map[int]band.Channel),
		UplinkGatewayHistory:  map[lorawan.EUI64]storage.UplinkGatewayHistory{},
		MaxSupportedDR:        ctx.ServiceProfile.DRMax,
		SkipFCntValidation:    ctx.Device.SkipFCntCheck,
		PingSlotDR:            ctx.DeviceProfile.PingSlotDR,
		PingSlotFrequency:     int(ctx.DeviceProfile.PingSlotFreq),
		NbTrans:               1,
	}

	if ctx.RejoinAnsPayload.AppSKey != nil {
		pendingDS.AppSKeyEvelope = &storage.KeyEnvelope{
			KEKLabel: ctx.RejoinAnsPayload.AppSKey.KEKLabel,
			AESKey:   ctx.RejoinAnsPayload.AppSKey.AESKey,
		}
	}

	if ctx.RejoinAnsPayload.NwkSKey != nil {
		key, err := unwrapNSKeyEnvelope(ctx.RejoinAnsPayload.NwkSKey)
		if err != nil {
			return err
		}

		pendingDS.SNwkSIntKey = key
		pendingDS.FNwkSIntKey = key
		pendingDS.NwkSEncKey = key
	}

	if ctx.RejoinAnsPayload.SNwkSIntKey != nil {
		key, err := unwrapNSKeyEnvelope(ctx.RejoinAnsPayload.SNwkSIntKey)
		if err != nil {
			return err
		}

		pendingDS.SNwkSIntKey = key
	}

	if ctx.RejoinAnsPayload.FNwkSIntKey != nil {
		key, err := unwrapNSKeyEnvelope(ctx.RejoinAnsPayload.FNwkSIntKey)
		if err != nil {
			return err
		}

		pendingDS.FNwkSIntKey = key
	}

	if ctx.RejoinAnsPayload.NwkSEncKey != nil {
		key, err := unwrapNSKeyEnvelope(ctx.RejoinAnsPayload.NwkSEncKey)
		if err != nil {
			return err
		}

		pendingDS.NwkSEncKey = key
	}

	if cfList := config.C.NetworkServer.Band.Band.GetCFList(ctx.DeviceSession.MACVersion); cfList != nil && cfList.CFListType == lorawan.CFListChannel {
		channelPL, ok := cfList.Payload.(*lorawan.CFListChannelPayload)
		if !ok {
			return fmt.Errorf("expected *lorawan.CFListChannelPayload, got %T", cfList.Payload)
		}

		for _, f := range channelPL.Channels {
			if f == 0 {
				continue
			}

			i, err := config.C.NetworkServer.Band.Band.GetUplinkChannelIndex(int(f), false)
			if err != nil {
				// if this happens, something is really wrong
				log.WithError(err).WithFields(log.Fields{
					"frequency": f,
				}).Error("unknown fclist frequency")
				continue
			}

			// add extra channel to enabled channels
			pendingDS.EnabledUplinkChannels = append(pendingDS.EnabledUplinkChannels, i)

			// add extra channel to extra uplink channels, so that we can
			// keep track on frequency and data-rate changes
			c, err := config.C.NetworkServer.Band.Band.GetUplinkChannel(i)
			if err != nil {
				return errors.Wrap(err, "get uplink channel error")
			}
			pendingDS.ExtraUplinkChannels[i] = c
		}
	}

	if ctx.DeviceProfile.PingSlotPeriod != 0 {
		pendingDS.PingSlotNb = (1 << 12) / ctx.DeviceProfile.PingSlotPeriod
	}

	ctx.DeviceSession.PendingRejoinDeviceSession = &pendingDS

	if err := storage.SaveDeviceSession(config.C.Redis.Pool, ctx.DeviceSession); err != nil {
		return errors.Wrap(err, "save device-session error")
	}

	return nil
}

func setRejoin2PendingDeviceSession(ctx *context) error {
	pendingDS := ctx.DeviceSession
	pendingDS.DevAddr = ctx.DevAddr
	pendingDS.FCntUp = 0
	pendingDS.NFCntDown = 0
	pendingDS.AFCntDown = 0
	pendingDS.RejoinCount0 = 0

	if ctx.RejoinAnsPayload.AppSKey != nil {
		pendingDS.AppSKeyEvelope = &storage.KeyEnvelope{
			KEKLabel: ctx.RejoinAnsPayload.AppSKey.KEKLabel,
			AESKey:   ctx.RejoinAnsPayload.AppSKey.AESKey,
		}
	}

	if ctx.RejoinAnsPayload.NwkSKey != nil {
		key, err := unwrapNSKeyEnvelope(ctx.RejoinAnsPayload.NwkSKey)
		if err != nil {
			return err
		}

		pendingDS.SNwkSIntKey = key
		pendingDS.FNwkSIntKey = key
		pendingDS.NwkSEncKey = key
	}

	if ctx.RejoinAnsPayload.SNwkSIntKey != nil {
		key, err := unwrapNSKeyEnvelope(ctx.RejoinAnsPayload.SNwkSIntKey)
		if err != nil {
			return err
		}

		pendingDS.SNwkSIntKey = key
	}

	if ctx.RejoinAnsPayload.FNwkSIntKey != nil {
		key, err := unwrapNSKeyEnvelope(ctx.RejoinAnsPayload.FNwkSIntKey)
		if err != nil {
			return err
		}

		pendingDS.FNwkSIntKey = key
	}

	if ctx.RejoinAnsPayload.NwkSEncKey != nil {
		key, err := unwrapNSKeyEnvelope(ctx.RejoinAnsPayload.NwkSEncKey)
		if err != nil {
			return err
		}

		pendingDS.NwkSEncKey = key
	}

	ctx.DeviceSession.PendingRejoinDeviceSession = &pendingDS

	if err := storage.SaveDeviceSession(config.C.Redis.Pool, ctx.DeviceSession); err != nil {
		return errors.Wrap(err, "save device-session error")
	}

	return nil
}

func createDeviceActivation(ctx *context) error {
	da := storage.DeviceActivation{
		DevEUI:      ctx.DeviceSession.PendingRejoinDeviceSession.DevEUI,
		JoinEUI:     ctx.DeviceSession.PendingRejoinDeviceSession.JoinEUI,
		DevAddr:     ctx.DeviceSession.PendingRejoinDeviceSession.DevAddr,
		SNwkSIntKey: ctx.DeviceSession.PendingRejoinDeviceSession.SNwkSIntKey,
		FNwkSIntKey: ctx.DeviceSession.PendingRejoinDeviceSession.FNwkSIntKey,
		NwkSEncKey:  ctx.DeviceSession.PendingRejoinDeviceSession.NwkSEncKey,
		DevNonce:    lorawan.DevNonce(ctx.RJCount),
		JoinReqType: lorawan.JoinType(ctx.RejoinType),
	}

	if err := storage.CreateDeviceActivation(config.C.PostgreSQL.DB, &da); err != nil {
		return errors.Wrap(err, "create device-activation error")
	}

	return nil
}

func sendJoinAcceptDownlink(ctx *context) error {
	var phy lorawan.PHYPayload
	if err := phy.UnmarshalBinary(ctx.RejoinAnsPayload.PHYPayload[:]); err != nil {
		return errors.Wrap(err, "unmarshal downlink phypayload error")
	}

	if err := joindown.Handle(ctx.DeviceSession, ctx.RXPacket, phy); err != nil {
		return errors.Wrap(err, "join join-response flow error")
	}

	return nil
}
