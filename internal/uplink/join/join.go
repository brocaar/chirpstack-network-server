package join

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-api/go/v3/as"
	"github.com/brocaar/chirpstack-api/go/v3/nc"
	"github.com/brocaar/chirpstack-network-server/v3/internal/backend/controller"
	"github.com/brocaar/chirpstack-network-server/v3/internal/backend/joinserver"
	"github.com/brocaar/chirpstack-network-server/v3/internal/band"
	"github.com/brocaar/chirpstack-network-server/v3/internal/config"
	"github.com/brocaar/chirpstack-network-server/v3/internal/downlink/join"
	"github.com/brocaar/chirpstack-network-server/v3/internal/framelog"
	"github.com/brocaar/chirpstack-network-server/v3/internal/helpers"
	"github.com/brocaar/chirpstack-network-server/v3/internal/logging"
	"github.com/brocaar/chirpstack-network-server/v3/internal/models"
	"github.com/brocaar/chirpstack-network-server/v3/internal/roaming"
	"github.com/brocaar/chirpstack-network-server/v3/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
	loraband "github.com/brocaar/lorawan/band"
)

// ErrAbort is used to abort the flow without error
var ErrAbort = errors.New("nothing to do")

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

	PRStartReqPayload *backend.PRStartReqPayload
	PRStartAnsPayload *backend.PRStartAnsPayload
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

	for _, f := range []func() error{
		jctx.setContextFromJoinRequestPHYPayload,
		jctx.getDeviceOrTryRoaming,
		jctx.getDeviceProfile,
		jctx.getServiceProfile,
		jctx.filterRxInfoByServiceProfile,
		jctx.logJoinRequestFramesCollected,
		jctx.abortOnDeviceIsDisabled,
		jctx.validateNonce,
		jctx.getRandomDevAddr,
		jctx.getJoinAcceptFromAS,
		jctx.sendUplinkMetaDataToNetworkController,
		jctx.flushDeviceQueue,
		jctx.createDeviceSession,
		jctx.createDeviceActivation,
		jctx.setDeviceMode,
		jctx.sendJoinAcceptDownlink,
	} {
		if err := f(); err != nil {
			if err == ErrAbort {
				return nil
			}

			return err
		}
	}

	return nil
}

func (ctx *joinContext) setContextFromJoinRequestPHYPayload() error {
	jrPL, ok := ctx.RXPacket.PHYPayload.MACPayload.(*lorawan.JoinRequestPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.JoinRequestPayload, got: %T", ctx.RXPacket.PHYPayload.MACPayload)
	}
	ctx.JoinRequestPayload = jrPL

	return nil
}

func (ctx *joinContext) logJoinRequestFramesCollected() error {
	uplinkFrameLog, err := framelog.CreateUplinkFrameLog(ctx.RXPacket)
	if err != nil {
		return errors.Wrap(err, "create uplink frame-set error")
	}

	uplinkFrameLog.DevEui = ctx.JoinRequestPayload.DevEUI[:]

	if err := framelog.LogUplinkFrameForDevEUI(ctx.ctx, ctx.JoinRequestPayload.DevEUI, uplinkFrameLog); err != nil {
		log.WithFields(log.Fields{
			"ctx_id": ctx.ctx.Value("join_ctx"),
		}).WithError(err).Error("log uplink frame for device error")
	}

	return nil
}

func (ctx *joinContext) getDeviceOrTryRoaming() error {
	var err error
	ctx.Device, err = storage.GetDevice(ctx.ctx, storage.DB(), ctx.JoinRequestPayload.DevEUI, false)
	if err != nil {
		if errors.Cause(err) == storage.ErrDoesNotExist && roaming.IsRoamingEnabled() {
			log.WithFields(log.Fields{
				"ctx_id":   ctx.ctx.Value(logging.ContextIDKey),
				"dev_eui":  ctx.JoinRequestPayload.DevEUI,
				"join_eui": ctx.JoinRequestPayload.JoinEUI,
			}).Info("uplink/join: unknown device, try passive-roaming activation")

			if err := StartPRFNS(ctx.ctx, ctx.RXPacket, ctx.JoinRequestPayload); err != nil {
				return err
			}

			return ErrAbort
		}
		return errors.Wrap(err, "get device error")
	}
	return nil
}

func (ctx *joinContext) getDeviceProfile() error {
	var err error
	ctx.DeviceProfile, err = storage.GetDeviceProfile(ctx.ctx, storage.DB(), ctx.Device.DeviceProfileID)
	if err != nil {
		return errors.Wrap(err, "get device-profile error")
	}

	if !ctx.DeviceProfile.SupportsJoin {
		return errors.New("device does not support join")
	}

	return nil
}

func (ctx *joinContext) getServiceProfile() error {
	var err error
	ctx.ServiceProfile, err = storage.GetServiceProfile(ctx.ctx, storage.DB(), ctx.Device.ServiceProfileID)
	if err != nil {
		return errors.Wrap(err, "get service-profile error")
	}
	return nil
}

func (ctx *joinContext) filterRxInfoByServiceProfile() error {
	err := helpers.FilterRxInfoByServiceProfileID(ctx.Device.ServiceProfileID, &ctx.RXPacket)
	if err != nil {
		if err == helpers.ErrNoElements {
			log.WithFields(log.Fields{
				"dev_eui": ctx.Device.DevEUI,
				"ctx_id":  ctx.ctx.Value(logging.ContextIDKey),
			}).Warning("uplink/join: none of the receiving gateways are public or have the same service-profile")
			return ErrAbort
		}
		return err
	}

	return nil
}

func (ctx *joinContext) abortOnDeviceIsDisabled() error {
	if ctx.Device.IsDisabled {
		return ErrAbort
	}
	return nil
}

func (ctx *joinContext) validateNonce() error {
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
		returnErr := errors.Wrap(err, "validate dev-nonce error")
		asClient, err := helpers.GetASClientForRoutingProfileID(ctx.ctx, ctx.Device.RoutingProfileID)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"ctx_id":  ctx.ctx.Value(logging.ContextIDKey),
				"dev_eui": ctx.Device.DevEUI,
			}).Error("uplink/join: get as client for routing-profile id error")
		} else {
			_, err := asClient.HandleError(ctx.ctx, &as.HandleErrorRequest{
				DevEui: ctx.Device.DevEUI[:],
				Type:   as.ErrorType_OTAA,
				Error:  "validate dev-nonce error",
			})
			if err != nil {
				log.WithError(err).WithFields(log.Fields{
					"ctx_id":  ctx.ctx.Value(logging.ContextIDKey),
					"dev_eui": ctx.Device.DevEUI,
				}).Error("uplink/join: as.HandleError error")
			}
		}

		return returnErr
	}

	return nil
}

func (ctx *joinContext) getRandomDevAddr() error {
	devAddr, err := storage.GetRandomDevAddr(netID)
	if err != nil {
		return errors.Wrap(err, "get random DevAddr error")
	}
	ctx.DevAddr = devAddr

	return nil
}

func (ctx *joinContext) getJoinAcceptFromAS() error {
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
	// the join-server and device how to derive the session-keys and how to
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

	jsClient, err := joinserver.GetClientForJoinEUI(ctx.JoinRequestPayload.JoinEUI)
	if err != nil {
		return errors.Wrap(err, "get join-server client error")
	}

	ctx.JoinAnsPayload, err = jsClient.JoinReq(ctx.ctx, joinReqPL)
	if err != nil {
		returnErr := errors.Wrap(err, "join-request to join-server error")
		req := as.HandleErrorRequest{
			DevEui: ctx.Device.DevEUI[:],
			Type:   as.ErrorType_OTAA,
			Error:  "join-server returned error: " + err.Error(),
		}

		asClient, err := helpers.GetASClientForRoutingProfileID(ctx.ctx, ctx.Device.RoutingProfileID)
		if err != nil {

			log.WithError(err).WithFields(log.Fields{
				"ctx_id":  ctx.ctx.Value(logging.ContextIDKey),
				"dev_eui": ctx.Device.DevEUI,
			}).Error("uplink/join: get as client for routing-profile id error")
		} else {
			_, err := asClient.HandleError(ctx.ctx, &req)
			if err != nil {
				log.WithError(err).WithFields(log.Fields{
					"ctx_id":  ctx.ctx.Value(logging.ContextIDKey),
					"dev_eui": ctx.Device.DevEUI,
				}).Error("uplink/join: as.HandleError error")
			}
		}

		return returnErr
	}

	return nil
}

func (ctx *joinContext) sendUplinkMetaDataToNetworkController() error {
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

func (ctx *joinContext) flushDeviceQueue() error {
	if err := storage.FlushDeviceQueueForDevEUI(ctx.ctx, storage.DB(), ctx.Device.DevEUI); err != nil {
		return errors.Wrap(err, "flush device-queue error")
	}
	return nil
}

func (ctx *joinContext) createDeviceSession() error {
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
		PingSlotFrequency:     ctx.DeviceProfile.PingSlotFreq,
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

			i, err := band.Band().GetUplinkChannelIndex(f, false)
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

	if err := storage.SaveDeviceSession(ctx.ctx, ctx.DeviceSession); err != nil {
		return errors.Wrap(err, "save node-session error")
	}

	if err := storage.FlushMACCommandQueue(ctx.ctx, ctx.DeviceSession.DevEUI); err != nil {
		return fmt.Errorf("flush mac-command queue error: %s", err)
	}

	return nil
}

func (ctx *joinContext) createDeviceActivation() error {
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

func (ctx *joinContext) setDeviceMode() error {
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

func (ctx *joinContext) sendJoinAcceptDownlink() error {
	var phy lorawan.PHYPayload
	if err := phy.UnmarshalBinary(ctx.JoinAnsPayload.PHYPayload[:]); err != nil {
		return errors.Wrap(err, "unmarshal downlink phypayload error")
	}

	if err := join.Handle(ctx.ctx, ctx.DeviceSession, ctx.RXPacket, phy); err != nil {
		return errors.Wrap(err, "run join-response flow error")
	}

	return nil
}

func (ctx *joinContext) setPRStartAnsPayload() error {
	var netID lorawan.NetID
	err := netID.UnmarshalText([]byte(ctx.PRStartReqPayload.BasePayload.SenderID))
	if err != nil {
		return errors.Wrap(err, "decode netid error")
	}

	lifetime := int(roaming.GetPassiveRoamingLifetime(netID) / time.Second)
	fCntUp := uint32(0)

	// sess keys
	kekLabel := roaming.GetPassiveRoamingKEKLabel(netID)
	var kekKey []byte
	if kekLabel != "" {
		kekKey, err = roaming.GetKEKKey(kekLabel)
		if err != nil {
			return errors.Wrap(err, "get kek key error")
		}
	}
	var fNwkSIntKey *backend.KeyEnvelope
	var nwkSKey *backend.KeyEnvelope

	if ctx.DeviceSession.GetMACVersion() == lorawan.LoRaWAN1_0 {
		nwkSKey, err = backend.NewKeyEnvelope(kekLabel, kekKey, ctx.DeviceSession.NwkSEncKey)
		if err != nil {
			return errors.Wrap(err, "new key envelope error")
		}
	} else {
		fNwkSIntKey, err = backend.NewKeyEnvelope(kekLabel, kekKey, ctx.DeviceSession.FNwkSIntKey)
		if err != nil {
			return errors.Wrap(err, "new key envelope error")
		}
	}

	classA := "A"
	rxDelay1 := int(band.Band().GetDefaults().JoinAcceptDelay1 / time.Second)
	rx1DR, err := band.Band().GetRX1DataRateIndex(ctx.RXPacket.DR, 0)
	if err != nil {
		return errors.Wrap(err, "get rx1 data-rate error")
	}
	rx2DR := band.Band().GetDefaults().RX2DataRate
	dlFreq1, err := band.Band().GetRX1FrequencyForUplinkFrequency(ctx.RXPacket.TXInfo.Frequency)
	if err != nil {
		return errors.Wrap(err, "get rx1 frequency error")
	}
	dlFreq1Mhz := float64(dlFreq1) / 1000000
	dlFreq2Mhz := float64(band.Band().GetDefaults().RX2Frequency) / 1000000

	ctx.PRStartAnsPayload = &backend.PRStartAnsPayload{
		PHYPayload:  ctx.JoinAnsPayload.PHYPayload,
		DevEUI:      &ctx.Device.DevEUI,
		DevAddr:     &ctx.DevAddr,
		Lifetime:    &lifetime,
		FNwkSIntKey: fNwkSIntKey,
		NwkSKey:     nwkSKey,
		FCntUp:      &fCntUp,
		DLMetaData: &backend.DLMetaData{
			DevEUI:     &ctx.DeviceSession.DevEUI,
			DLFreq1:    &dlFreq1Mhz,
			DLFreq2:    &dlFreq2Mhz,
			RXDelay1:   &rxDelay1,
			ClassMode:  &classA,
			DataRate1:  &rx1DR,
			DataRate2:  &rx2DR,
			FNSULToken: ctx.PRStartReqPayload.ULMetaData.FNSULToken,
		},
	}

	for i := range ctx.PRStartReqPayload.ULMetaData.GWInfo {
		gwInfo := ctx.PRStartReqPayload.ULMetaData.GWInfo[i]
		ctx.PRStartAnsPayload.DLMetaData.GWInfo = append(ctx.PRStartAnsPayload.DLMetaData.GWInfo, backend.GWInfoElement{
			ULToken: gwInfo.ULToken,
		})
	}

	return nil
}
