package join

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
	setContextFromJoinRequestPHYPayload,
	logJoinRequestFramesCollected,
	getDeviceAndDeviceProfile,
	validateNonce,
	getRandomDevAddr,
	getJoinAcceptFromAS,
	flushDeviceQueue,
	createDeviceSession,
	createDeviceActivation,
	sendJoinAcceptDownlink,
}

type context struct {
	RXPacket           models.RXPacket
	JoinRequestPayload *lorawan.JoinRequestPayload
	Device             storage.Device
	ServiceProfile     storage.ServiceProfile
	DeviceProfile      storage.DeviceProfile
	DevAddr            lorawan.DevAddr
	CFList             []uint32
	JoinAnsPayload     backend.JoinAnsPayload
	DeviceSession      storage.DeviceSession
}

// Handle handles a join-request
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

func setContextFromJoinRequestPHYPayload(ctx *context) error {
	jrPL, ok := ctx.RXPacket.PHYPayload.MACPayload.(*lorawan.JoinRequestPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.JoinRequestPayload, got: %T", ctx.RXPacket.PHYPayload.MACPayload)
	}
	ctx.JoinRequestPayload = jrPL

	return nil
}

func logJoinRequestFramesCollected(ctx *context) error {
	var gatewayIDs []string
	for _, p := range ctx.RXPacket.RXInfoSet {
		gatewayIDs = append(gatewayIDs, helpers.GetGatewayID(p).String())
	}

	uplinkFrameSet, err := framelog.CreateUplinkFrameSet(ctx.RXPacket)
	if err != nil {
		return errors.Wrap(err, "create uplink frame-set error")
	}

	if err := framelog.LogUplinkFrameForDevEUI(ctx.JoinRequestPayload.DevEUI, uplinkFrameSet); err != nil {
		log.WithError(err).Error("log uplink frame for device error")
	}

	log.WithFields(log.Fields{
		"dev_eui":  ctx.JoinRequestPayload.DevEUI,
		"gw_count": len(gatewayIDs),
		"gw_ids":   strings.Join(gatewayIDs, ", "),
		"mtype":    ctx.RXPacket.PHYPayload.MHDR.MType,
	}).Info("packet(s) collected")

	return nil
}

func getDeviceAndDeviceProfile(ctx *context) error {
	var err error

	ctx.Device, err = storage.GetDevice(config.C.PostgreSQL.DB, ctx.JoinRequestPayload.DevEUI)
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

func validateNonce(ctx *context) error {
	// validate that the nonce has not been used yet
	err := storage.ValidateDevNonce(config.C.PostgreSQL.DB, ctx.JoinRequestPayload.JoinEUI, ctx.JoinRequestPayload.DevEUI, ctx.JoinRequestPayload.DevNonce, lorawan.JoinRequestType)
	if err != nil {
		return errors.Wrap(err, "validate dev-nonce error")
	}

	return nil
}

func getRandomDevAddr(ctx *context) error {
	devAddr, err := storage.GetRandomDevAddr(config.C.Redis.Pool, config.C.NetworkServer.NetID)
	if err != nil {
		return errors.Wrap(err, "get random DevAddr error")
	}
	ctx.DevAddr = devAddr

	return nil
}

func getJoinAcceptFromAS(ctx *context) error {
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

	var cFListB []byte
	cFList := config.C.NetworkServer.Band.Band.GetCFList(ctx.DeviceProfile.MACVersion)
	if cFList != nil {
		cFListB, err = cFList.MarshalBinary()
		if err != nil {
			return errors.Wrap(err, "marshal cflist error")
		}
	}

	// note about the OptNeg field:
	// it must only be set to true for devices != 1.0.x as it will indicate to
	// the join-server and device how to derrive the session-keys and how to
	// sign the join-accept message

	joinReqPL := backend.JoinReqPayload{
		BasePayload: backend.BasePayload{
			ProtocolVersion: backend.ProtocolVersion1_0,
			SenderID:        config.C.NetworkServer.NetID.String(),
			ReceiverID:      ctx.JoinRequestPayload.JoinEUI.String(),
			TransactionID:   transactionID,
			MessageType:     backend.JoinReq,
		},
		MACVersion: ctx.DeviceProfile.MACVersion,
		PHYPayload: backend.HEXBytes(b),
		DevEUI:     ctx.JoinRequestPayload.DevEUI,
		DevAddr:    ctx.DevAddr,
		DLSettings: lorawan.DLSettings{
			OptNeg:      !strings.HasPrefix(ctx.DeviceProfile.MACVersion, "1.0"), // must be set to true for != "1.0" devices
			RX2DataRate: uint8(config.C.NetworkServer.NetworkSettings.RX2DR),
			RX1DROffset: uint8(config.C.NetworkServer.NetworkSettings.RX1DROffset),
		},
		RxDelay: config.C.NetworkServer.NetworkSettings.RX1Delay,
		CFList:  backend.HEXBytes(cFListB),
	}

	jsClient, err := config.C.JoinServer.Pool.Get(ctx.JoinRequestPayload.JoinEUI)
	if err != nil {
		return errors.Wrap(err, "get join-server client error")
	}

	ctx.JoinAnsPayload, err = jsClient.JoinReq(joinReqPL)
	if err != nil {
		return errors.Wrap(err, "join-request to join-server error")
	}

	return nil
}

func flushDeviceQueue(ctx *context) error {
	if err := storage.FlushDeviceQueueForDevEUI(config.C.PostgreSQL.DB, ctx.Device.DevEUI); err != nil {
		return errors.Wrap(err, "flush device-queue error")
	}
	return nil
}

func createDeviceSession(ctx *context) error {
	ds := storage.DeviceSession{
		DeviceProfileID:  ctx.Device.DeviceProfileID,
		ServiceProfileID: ctx.Device.ServiceProfileID,
		RoutingProfileID: ctx.Device.RoutingProfileID,

		MACVersion:            ctx.DeviceProfile.MACVersion,
		DevAddr:               ctx.DevAddr,
		JoinEUI:               ctx.JoinRequestPayload.JoinEUI,
		DevEUI:                ctx.JoinRequestPayload.DevEUI,
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

	if ctx.JoinAnsPayload.AppSKey != nil {
		ds.AppSKeyEvelope = &storage.KeyEnvelope{
			KEKLabel: ctx.JoinAnsPayload.AppSKey.KEKLabel,
			AESKey:   ctx.JoinAnsPayload.AppSKey.AESKey,
		}
	}

	if ctx.JoinAnsPayload.NwkSKey != nil {
		key, err := unwrapNSKeyEnvelope(ctx.JoinAnsPayload.NwkSKey)
		if err != nil {
			return err
		}

		ds.SNwkSIntKey = key
		ds.FNwkSIntKey = key
		ds.NwkSEncKey = key
	}

	if ctx.JoinAnsPayload.SNwkSIntKey != nil {
		key, err := unwrapNSKeyEnvelope(ctx.JoinAnsPayload.SNwkSIntKey)
		if err != nil {
			return err
		}

		ds.SNwkSIntKey = key
	}

	if ctx.JoinAnsPayload.FNwkSIntKey != nil {
		key, err := unwrapNSKeyEnvelope(ctx.JoinAnsPayload.FNwkSIntKey)
		if err != nil {
			return err
		}

		ds.FNwkSIntKey = key
	}

	if ctx.JoinAnsPayload.NwkSEncKey != nil {
		key, err := unwrapNSKeyEnvelope(ctx.JoinAnsPayload.NwkSEncKey)
		if err != nil {
			return err
		}

		ds.NwkSEncKey = key
	}

	if cfList := config.C.NetworkServer.Band.Band.GetCFList(ctx.DeviceProfile.MACVersion); cfList != nil && cfList.CFListType == lorawan.CFListChannel {
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
			ds.EnabledUplinkChannels = append(ds.EnabledUplinkChannels, i)

			// add extra channel to extra uplink channels, so that we can
			// keep track on frequency and data-rate changes
			c, err := config.C.NetworkServer.Band.Band.GetUplinkChannel(i)
			if err != nil {
				return errors.Wrap(err, "get uplink channel error")
			}
			ds.ExtraUplinkChannels[i] = c
		}
	}

	if ctx.DeviceProfile.PingSlotPeriod != 0 {
		ds.PingSlotNb = (1 << 12) / ctx.DeviceProfile.PingSlotPeriod
	}

	ctx.DeviceSession = ds

	if err := storage.SaveDeviceSession(config.C.Redis.Pool, ctx.DeviceSession); err != nil {
		return errors.Wrap(err, "save node-session error")
	}

	if err := storage.FlushMACCommandQueue(config.C.Redis.Pool, ctx.DeviceSession.DevEUI); err != nil {
		return fmt.Errorf("flush mac-command queue error: %s", err)
	}

	return nil
}

func createDeviceActivation(ctx *context) error {
	da := storage.DeviceActivation{
		DevEUI:      ctx.DeviceSession.DevEUI,
		JoinEUI:     ctx.DeviceSession.JoinEUI,
		DevAddr:     ctx.DeviceSession.DevAddr,
		SNwkSIntKey: ctx.DeviceSession.SNwkSIntKey,
		FNwkSIntKey: ctx.DeviceSession.FNwkSIntKey,
		NwkSEncKey:  ctx.DeviceSession.NwkSEncKey,
		DevNonce:    ctx.JoinRequestPayload.DevNonce,
		JoinReqType: lorawan.JoinRequestType,
	}

	if err := storage.CreateDeviceActivation(config.C.PostgreSQL.DB, &da); err != nil {
		return errors.Wrap(err, "create device-activation error")
	}

	return nil
}

func sendJoinAcceptDownlink(ctx *context) error {
	var phy lorawan.PHYPayload
	if err := phy.UnmarshalBinary(ctx.JoinAnsPayload.PHYPayload[:]); err != nil {
		return errors.Wrap(err, "unmarshal downlink phypayload error")
	}

	if err := joindown.Handle(ctx.DeviceSession, ctx.RXPacket, phy); err != nil {
		return errors.Wrap(err, "run join-response flow error")
	}

	return nil
}
