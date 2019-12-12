package join

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
	loraband "github.com/brocaar/lorawan/band"
	"github.com/brocaar/chirpstack-api/go/v3/nc"
	"github.com/brocaar/chirpstack-network-server/internal/backend/controller"
	"github.com/brocaar/chirpstack-network-server/internal/backend/joinserver"
	"github.com/brocaar/chirpstack-network-server/internal/band"
	"github.com/brocaar/chirpstack-network-server/internal/config"
	joindown "github.com/brocaar/chirpstack-network-server/internal/downlink/join"
	"github.com/brocaar/chirpstack-network-server/internal/framelog"
	"github.com/brocaar/chirpstack-network-server/internal/logging"
	"github.com/brocaar/chirpstack-network-server/internal/models"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
)

var tasks = []func(*joinContext) error{
	setContextFromJoinRequestPHYPayload,
	logJoinRequestFramesCollected,
	getDeviceAndDeviceProfile,
	validateNonce,
	getRandomDevAddr,
	getJoinAcceptFromAS,
	sendUplinkMetaDataToNetworkController,
	flushDeviceQueue,
	createDeviceSession,
	createDeviceActivation,
	setDeviceMode,
	sendJoinAcceptDownlink,
}

type joinContext struct {
	ctx context.Context

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

var (
	netID       lorawan.NetID
	rx2DR       int
	rx1DROffset int
	rx1Delay    int
	keks        map[string][]byte
)

// Setup configures the package.
func Setup(conf config.Config) error {
	keks = make(map[string][]byte)

	netID = conf.NetworkServer.NetID
	rx2DR = conf.NetworkServer.NetworkSettings.RX2DR
	rx1DROffset = conf.NetworkServer.NetworkSettings.RX1DROffset
	rx1Delay = conf.NetworkServer.NetworkSettings.RX1Delay

	for _, k := range conf.JoinServer.KEK.Set {
		kek, err := hex.DecodeString(k.KEK)
		if err != nil {
			return errors.Wrap(err, "decode kek error")
		}

		keks[k.Label] = kek
	}

	return nil
}

// Handle handles a join-request
func Handle(ctx context.Context, rxPacket models.RXPacket) error {
	jctx := joinContext{
		ctx:      ctx,
		RXPacket: rxPacket,
	}

	for _, t := range tasks {
		if err := t(&jctx); err != nil {
			return err
		}
	}

	return nil
}

func setContextFromJoinRequestPHYPayload(ctx *joinContext) error {
	jrPL, ok := ctx.RXPacket.PHYPayload.MACPayload.(*lorawan.JoinRequestPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.JoinRequestPayload, got: %T", ctx.RXPacket.PHYPayload.MACPayload)
	}
	ctx.JoinRequestPayload = jrPL

	return nil
}

func logJoinRequestFramesCollected(ctx *joinContext) error {
	uplinkFrameSet, err := framelog.CreateUplinkFrameSet(ctx.RXPacket)
	if err != nil {
		return errors.Wrap(err, "create uplink frame-set error")
	}

	if err := framelog.LogUplinkFrameForDevEUI(ctx.ctx, storage.RedisPool(), ctx.JoinRequestPayload.DevEUI, uplinkFrameSet); err != nil {
		log.WithFields(log.Fields{
			"ctx_id": ctx.ctx.Value("join_ctx"),
		}).WithError(err).Error("log uplink frame for device error")
	}

	return nil
}

func getDeviceAndDeviceProfile(ctx *joinContext) error {
	var err error

	ctx.Device, err = storage.GetDevice(ctx.ctx, storage.DB(), ctx.JoinRequestPayload.DevEUI)
	if err != nil {
		return errors.Wrap(err, "get device error")
	}

	ctx.DeviceProfile, err = storage.GetDeviceProfile(ctx.ctx, storage.DB(), ctx.Device.DeviceProfileID)
	if err != nil {
		return errors.Wrap(err, "get device-profile error")
	}

	ctx.ServiceProfile, err = storage.GetServiceProfile(ctx.ctx, storage.DB(), ctx.Device.ServiceProfileID)
	if err != nil {
		return errors.Wrap(err, "get service-profile error")
	}

	if !ctx.DeviceProfile.SupportsJoin {
		return errors.New("device does not support join")
	}

	return nil
}

func validateNonce(ctx *joinContext) error {
	// validate that the nonce has not been used yet
	err := storage.ValidateDevNonce(
		ctx.ctx,
		storage.DB(),
		ctx.JoinRequestPayload.JoinEUI,
		ctx.JoinRequestPayload.DevEUI,
		ctx.JoinRequestPayload.DevNonce,
		lorawan.JoinRequestType,
	)
	if err != nil {
		return errors.Wrap(err, "validate dev-nonce error")
	}

	return nil
}

func getRandomDevAddr(ctx *joinContext) error {
	devAddr, err := storage.GetRandomDevAddr(netID)
	if err != nil {
		return errors.Wrap(err, "get random DevAddr error")
	}
	ctx.DevAddr = devAddr

	return nil
}

func getJoinAcceptFromAS(ctx *joinContext) error {
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
	cFList := band.Band().GetCFList(ctx.DeviceProfile.MACVersion)
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
			SenderID:        netID.String(),
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
			RX2DataRate: uint8(rx2DR),
			RX1DROffset: uint8(rx1DROffset),
		},
		RxDelay: rx1Delay,
		CFList:  backend.HEXBytes(cFListB),
	}

	jsClient, err := joinserver.GetPool().Get(ctx.JoinRequestPayload.JoinEUI)
	if err != nil {
		return errors.Wrap(err, "get join-server client error")
	}

	ctx.JoinAnsPayload, err = jsClient.JoinReq(ctx.ctx, joinReqPL)
	if err != nil {
		return errors.Wrap(err, "join-request to join-server error")
	}

	return nil
}

func sendUplinkMetaDataToNetworkController(ctx *joinContext) error {
	if controller.Client() == nil {
		return nil
	}

	req := nc.HandleUplinkMetaDataRequest{
		DevEui:      ctx.JoinRequestPayload.DevEUI[:],
		TxInfo:      ctx.RXPacket.TXInfo,
		RxInfo:      ctx.RXPacket.RXInfoSet,
		MessageType: nc.MType_JOIN_REQUEST,
	}

	// set phypayload size
	if b, err := ctx.RXPacket.PHYPayload.MarshalBinary(); err == nil {
		req.PhyPayloadByteCount = uint32(len(b))
	}

	// send async to controller
	go func() {
		_, err := controller.Client().HandleUplinkMetaData(ctx.ctx, &req)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"dev_eui": ctx.JoinRequestPayload.DevEUI,
				"ctx_id":  ctx.ctx.Value(logging.ContextIDKey),
			}).Error("sent uplink meta-data to network-controller error")
			return
		}

		log.WithFields(log.Fields{
			"dev_eui": ctx.JoinRequestPayload.DevEUI,
			"ctx_id":  ctx.ctx.Value(logging.ContextIDKey),
		}).Info("sent uplink meta-data to network-controller")
	}()

	return nil
}

func flushDeviceQueue(ctx *joinContext) error {
	if err := storage.FlushDeviceQueueForDevEUI(ctx.ctx, storage.DB(), ctx.Device.DevEUI); err != nil {
		return errors.Wrap(err, "flush device-queue error")
	}
	return nil
}

func createDeviceSession(ctx *joinContext) error {
	ds := storage.DeviceSession{
		DeviceProfileID:  ctx.Device.DeviceProfileID,
		ServiceProfileID: ctx.Device.ServiceProfileID,
		RoutingProfileID: ctx.Device.RoutingProfileID,

		MACVersion:            ctx.DeviceProfile.MACVersion,
		DevAddr:               ctx.DevAddr,
		JoinEUI:               ctx.JoinRequestPayload.JoinEUI,
		DevEUI:                ctx.JoinRequestPayload.DevEUI,
		RXWindow:              storage.RX1,
		RXDelay:               uint8(rx1Delay),
		RX1DROffset:           uint8(rx1DROffset),
		RX2DR:                 uint8(rx2DR),
		RX2Frequency:          band.Band().GetDefaults().RX2Frequency,
		EnabledUplinkChannels: band.Band().GetStandardUplinkChannelIndices(),
		ExtraUplinkChannels:   make(map[int]loraband.Channel),
		SkipFCntValidation:    ctx.Device.SkipFCntCheck,
		PingSlotDR:            ctx.DeviceProfile.PingSlotDR,
		PingSlotFrequency:     int(ctx.DeviceProfile.PingSlotFreq),
		NbTrans:               1,
		ReferenceAltitude:     ctx.Device.ReferenceAltitude,
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

	if cfList := band.Band().GetCFList(ctx.DeviceProfile.MACVersion); cfList != nil && cfList.CFListType == lorawan.CFListChannel {
		channelPL, ok := cfList.Payload.(*lorawan.CFListChannelPayload)
		if !ok {
			return fmt.Errorf("expected *lorawan.CFListChannelPayload, got %T", cfList.Payload)
		}

		for _, f := range channelPL.Channels {
			if f == 0 {
				continue
			}

			i, err := band.Band().GetUplinkChannelIndex(int(f), false)
			if err != nil {
				// if this happens, something is really wrong
				log.WithError(err).WithFields(log.Fields{
					"frequency": f,
				}).Error("unknown cflist frequency")
				continue
			}

			// add extra channel to enabled channels
			ds.EnabledUplinkChannels = append(ds.EnabledUplinkChannels, i)

			// add extra channel to extra uplink channels, so that we can
			// keep track on frequency and data-rate changes
			c, err := band.Band().GetUplinkChannel(i)
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

	if err := storage.SaveDeviceSession(ctx.ctx, storage.RedisPool(), ctx.DeviceSession); err != nil {
		return errors.Wrap(err, "save node-session error")
	}

	if err := storage.FlushMACCommandQueue(ctx.ctx, storage.RedisPool(), ctx.DeviceSession.DevEUI); err != nil {
		return fmt.Errorf("flush mac-command queue error: %s", err)
	}

	return nil
}

func createDeviceActivation(ctx *joinContext) error {
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

	if err := storage.CreateDeviceActivation(ctx.ctx, storage.DB(), &da); err != nil {
		return errors.Wrap(err, "create device-activation error")
	}

	return nil
}

func setDeviceMode(ctx *joinContext) error {
	// The device is never set to DeviceModeB because the device first needs to
	// aquire a Class-B beacon lock and will signal this to the network-server.
	if ctx.DeviceProfile.SupportsClassC {
		ctx.Device.Mode = storage.DeviceModeC
	} else {
		ctx.Device.Mode = storage.DeviceModeA
	}
	if err := storage.UpdateDevice(ctx.ctx, storage.DB(), &ctx.Device); err != nil {
		return errors.Wrap(err, "update device error")
	}
	return nil
}

func sendJoinAcceptDownlink(ctx *joinContext) error {
	var phy lorawan.PHYPayload
	if err := phy.UnmarshalBinary(ctx.JoinAnsPayload.PHYPayload[:]); err != nil {
		return errors.Wrap(err, "unmarshal downlink phypayload error")
	}

	if err := joindown.Handle(ctx.ctx, ctx.DeviceSession, ctx.RXPacket, phy); err != nil {
		return errors.Wrap(err, "run join-response flow error")
	}

	return nil
}
