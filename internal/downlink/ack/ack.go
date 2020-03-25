package ack

import (
	"context"
	"encoding/binary"

	"github.com/brocaar/lorawan"
	"github.com/gofrs/uuid"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-api/go/v3/as"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-api/go/v3/nc"
	"github.com/brocaar/chirpstack-network-server/internal/backend/controller"
	"github.com/brocaar/chirpstack-network-server/internal/backend/gateway"
	"github.com/brocaar/chirpstack-network-server/internal/framelog"
	"github.com/brocaar/chirpstack-network-server/internal/helpers"
	"github.com/brocaar/chirpstack-network-server/internal/logging"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
)

var (
	errAbort = errors.New("abort")
)

var handleDownlinkTXAckTasks = []func(*ackContext) error{
	getToken,
	getDownlinkFrames,
	decodePHYPayload,
	onError(
		sendErrorToApplicationServerOnLastFrame,
		sendDownlinkFrame,
		saveDownlinkFrames,
	),
	onNoError(
		sendDownlinkMetaDataToNetworkController,
		sendTxAckToApplicationServer,
		logDownlinkFrame,
	),
}

type ackContext struct {
	ctx context.Context

	Token          uint16
	DownlinkTXAck  gw.DownlinkTXAck
	DownlinkFrames storage.DownlinkFrames
	MHDR           lorawan.MHDR
	MACPayload     *lorawan.MACPayload
}

// HandleDownlinkTXAck handles the given downlink TX acknowledgement.
func HandleDownlinkTXAck(ctx context.Context, downlinkTXAck gw.DownlinkTXAck) error {
	actx := ackContext{
		ctx:           ctx,
		DownlinkTXAck: downlinkTXAck,
	}

	for _, t := range handleDownlinkTXAckTasks {
		if err := t(&actx); err != nil {
			if err == errAbort {
				return nil
			}
			return err
		}
	}

	return nil
}

func onError(funcs ...func(*ackContext) error) func(*ackContext) error {
	return func(ctx *ackContext) error {
		if ctx.DownlinkTXAck.Error == "" {
			return nil
		}

		for _, f := range funcs {
			if err := f(ctx); err != nil {
				return err
			}
		}

		return nil
	}
}

func onNoError(funcs ...func(*ackContext) error) func(*ackContext) error {
	return func(ctx *ackContext) error {
		if ctx.DownlinkTXAck.Error != "" {
			return nil
		}

		for _, f := range funcs {
			if err := f(ctx); err != nil {
				return err
			}
		}

		return nil
	}
}

func getToken(ctx *ackContext) error {
	if ctx.DownlinkTXAck.Token != 0 {
		ctx.Token = uint16(ctx.DownlinkTXAck.Token)
	} else if len(ctx.DownlinkTXAck.DownlinkId) == 16 {
		ctx.Token = binary.BigEndian.Uint16(ctx.DownlinkTXAck.DownlinkId[0:2])
	}
	return nil
}

func getDownlinkFrames(ctx *ackContext) error {
	var err error
	ctx.DownlinkFrames, err = storage.GetDownlinkFrames(ctx.ctx, ctx.Token)
	if err != nil {
		return errors.Wrap(err, "get downlink-frames error")
	}

	return nil
}

func decodePHYPayload(ctx *ackContext) error {
	if len(ctx.DownlinkFrames.DownlinkFrames) == 0 {
		return errors.New("downlink-frames is empty")
	}

	downlinkFrame := ctx.DownlinkFrames.DownlinkFrames[0]

	var phy lorawan.PHYPayload
	if err := phy.UnmarshalBinary(downlinkFrame.PhyPayload); err != nil {
		log.WithError(err).WithFields(log.Fields{
			"ctx_id": ctx.ctx.Value(logging.ContextIDKey),
		}).Error("unmarshal phypayload error")
	}

	ctx.MHDR = phy.MHDR
	if macPL, ok := phy.MACPayload.(*lorawan.MACPayload); ok {
		ctx.MACPayload = macPL
	}

	return nil
}

func sendDownlinkMetaDataToNetworkController(ctx *ackContext) error {
	if len(ctx.DownlinkFrames.DownlinkFrames) == 0 {
		return errors.New("downlink-frames is empty")
	}

	downlinkFrame := ctx.DownlinkFrames.DownlinkFrames[0]

	req := nc.HandleDownlinkMetaDataRequest{
		DevEui:              ctx.DownlinkFrames.DevEui,
		MulticastGroupId:    ctx.DownlinkFrames.MulticastGroupId,
		TxInfo:              downlinkFrame.TxInfo,
		PhyPayloadByteCount: uint32(len(downlinkFrame.PhyPayload)),
	}

	// message type
	switch ctx.MHDR.MType {
	case lorawan.JoinAccept:
		req.MessageType = nc.MType_JOIN_ACCEPT
	case lorawan.UnconfirmedDataDown:
		req.MessageType = nc.MType_UNCONFIRMED_DATA_DOWN
	case lorawan.ConfirmedDataDown:
		req.MessageType = nc.MType_CONFIRMED_DATA_DOWN
	}

	if ctx.MACPayload != nil {
		for _, pl := range ctx.MACPayload.FRMPayload {
			if b, err := pl.MarshalBinary(); err == nil {
				if ctx.MACPayload.FPort != nil && *ctx.MACPayload.FPort != 0 {
					req.ApplicationPayloadByteCount += uint32(len(b))
				} else {
					req.MacCommandByteCount += uint32(len(b))
				}
			}
		}

		for _, m := range ctx.MACPayload.FHDR.FOpts {
			if b, err := m.MarshalBinary(); err == nil {
				req.MacCommandByteCount += uint32(len(b))
			}
		}
	}

	// send async to controller
	go func() {
		_, err := controller.Client().HandleDownlinkMetaData(ctx.ctx, &req)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"ctx_id": ctx.ctx.Value(logging.ContextIDKey),
			}).Error("sent downlink meta-data to network-controller error")
			return
		}

		log.WithFields(log.Fields{
			"ctx_id": ctx.ctx.Value(logging.ContextIDKey),
		}).Info("sent downlink meta-data to network-controller")
	}()

	return nil
}

func sendTxAckToApplicationServer(ctx *ackContext) error {
	// We do not want to inform the AS on non-application TX events.
	if len(ctx.DownlinkFrames.DevEui) == 0 || ctx.MACPayload == nil || ctx.MACPayload.FPort == nil || *ctx.MACPayload.FPort == 0 {
		return nil
	}

	var rpID uuid.UUID
	copy(rpID[:], ctx.DownlinkFrames.RoutingProfileId)

	asClient, err := helpers.GetASClientForRoutingProfileID(ctx.ctx, rpID)
	if err != nil {
		return errors.Wrap(err, "get application-server client for routing-profile id error")
	}

	// send async to as
	go func(ctx *ackContext, asClient as.ApplicationServerServiceClient) {
		_, err := asClient.HandleTxAck(ctx.ctx, &as.HandleTxAckRequest{
			DevEui: ctx.DownlinkFrames.DevEui,
			FCnt:   ctx.MACPayload.FHDR.FCnt,
		})
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"ctx_id": ctx.ctx.Value(logging.ContextIDKey),
			}).Error("send tx ack to application-server error")
			return
		}

		log.WithFields(log.Fields{
			"ctx_id": ctx.ctx.Value(logging.ContextIDKey),
		}).Info("sent tx ack to application-server")
	}(ctx, asClient)

	return nil
}

func sendErrorToApplicationServerOnLastFrame(ctx *ackContext) error {
	// Only send an error to the AS on the last possible attempt.
	// We only want to send error for application payloads.
	if len(ctx.DownlinkFrames.DownlinkFrames) >= 2 || ctx.MACPayload == nil || ctx.MACPayload.FPort == nil || *ctx.MACPayload.FPort == 0 {
		return nil
	}

	var rpID uuid.UUID
	copy(rpID[:], ctx.DownlinkFrames.RoutingProfileId)

	asClient, err := helpers.GetASClientForRoutingProfileID(ctx.ctx, rpID)
	if err != nil {
		return errors.Wrap(err, "get application-server client for routing-profile id error")
	}

	// send async to as
	go func(ctx *ackContext, asClient as.ApplicationServerServiceClient) {
		_, err := asClient.HandleError(ctx.ctx, &as.HandleErrorRequest{
			DevEui: ctx.DownlinkFrames.DevEui,
			FCnt:   ctx.MACPayload.FHDR.FCnt,
			Type:   as.ErrorType_DATA_DOWN_GATEWAY,
			Error:  ctx.DownlinkTXAck.Error,
		})
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"ctx_id": ctx.ctx.Value(logging.ContextIDKey),
			}).Error("send error to application-server error")
			return
		}

		log.WithFields(log.Fields{
			"ctx_id": ctx.ctx.Value(logging.ContextIDKey),
		}).Info("sent error to application-server")
	}(ctx, asClient)

	return nil
}

func sendDownlinkFrame(ctx *ackContext) error {
	if len(ctx.DownlinkFrames.DownlinkFrames) < 2 {
		return nil
	}

	// send the next item
	if err := gateway.Backend().SendTXPacket(*ctx.DownlinkFrames.DownlinkFrames[1]); err != nil {
		return errors.Wrap(err, "send downlink-frame to gateway error")
	}
	return nil
}

func saveDownlinkFrames(ctx *ackContext) error {
	if len(ctx.DownlinkFrames.DownlinkFrames) < 2 {
		return nil
	}

	ctx.DownlinkFrames.DownlinkFrames = ctx.DownlinkFrames.DownlinkFrames[1:]
	if err := storage.SaveDownlinkFrames(ctx.ctx, ctx.DownlinkFrames); err != nil {
		return errors.Wrap(err, "save downlink-frames error")
	}

	return nil
}

func logDownlinkFrame(ctx *ackContext) error {
	if len(ctx.DownlinkFrames.DownlinkFrames) == 0 {
		return errors.New("downlink-frames is empty")
	}

	downlinkFrame := ctx.DownlinkFrames.DownlinkFrames[0]

	// log for gateway (with encrypted mac-commands)
	if err := framelog.LogDownlinkFrameForGateway(ctx.ctx, *downlinkFrame); err != nil {
		log.WithError(err).WithFields(log.Fields{
			"ctx_id": ctx.ctx.Value(logging.ContextIDKey),
		}).Error("log downlink frame for gateway error")
	}

	var devEUI lorawan.EUI64
	var nwkSEncKey lorawan.AES128Key
	copy(devEUI[:], ctx.DownlinkFrames.DevEui)
	copy(nwkSEncKey[:], ctx.DownlinkFrames.NwkSEncKey)

	// log for device (with decrypted mac-commands)
	var phy lorawan.PHYPayload
	if err := phy.UnmarshalBinary(downlinkFrame.PhyPayload); err != nil {
		return err
	}

	// decrypt FRMPayload mac-commands
	if ctx.MACPayload != nil && ctx.MACPayload.FPort != nil && *ctx.MACPayload.FPort == 0 {
		if err := phy.DecryptFRMPayload(nwkSEncKey); err != nil {
			return errors.Wrap(err, "decrypt frmpayload error")
		}
	}

	// decrypt FOpts mac-commands (LoRaWAN 1.1)
	if ctx.DownlinkFrames.EncryptedFopts {
		if err := phy.DecryptFOpts(nwkSEncKey); err != nil {
			return errors.Wrap(err, "decrypt FOpts error")
		}
	}

	phyB, err := phy.MarshalBinary()
	if err != nil {
		return err
	}

	if err := framelog.LogDownlinkFrameForDevEUI(ctx.ctx, devEUI, gw.DownlinkFrame{
		PhyPayload: phyB,
		TxInfo:     downlinkFrame.TxInfo,
		Token:      downlinkFrame.Token,
		DownlinkId: downlinkFrame.DownlinkId,
	}); err != nil {
		log.WithError(err).WithFields(log.Fields{
			"ctx_id": ctx.ctx.Value(logging.ContextIDKey),
		}).Error("log downlink frame for device error")
	}

	return nil
}
