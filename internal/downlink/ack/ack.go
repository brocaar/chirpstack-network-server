package ack

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/brocaar/lorawan"
	"github.com/gofrs/uuid"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-api/go/v3/as"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-api/go/v3/nc"
	"github.com/brocaar/chirpstack-api/go/v3/ns"
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
	getDownlinkFrame,
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

	Token               uint16
	DownlinkTXAck       gw.DownlinkTXAck
	DownlinkTXAckStatus gw.TxAckStatus
	DownlinkFrame       storage.DownlinkFrame
	DownlinkFrameItem   *gw.DownlinkFrameItem
	MHDR                lorawan.MHDR
	MACPayload          *lorawan.MACPayload
}

// HandleDownlinkTXAck handles the given downlink TX acknowledgement.
func HandleDownlinkTXAck(ctx context.Context, downlinkTXAck gw.DownlinkTXAck) error {
	var ackStatus gw.TxAckStatus

	if len(downlinkTXAck.Items) == 0 {
		if downlinkTXAck.Error == "" {
			ackStatus = gw.TxAckStatus_OK
		} else {
			if val, ok := gw.TxAckStatus_value[downlinkTXAck.Error]; ok {
				ackStatus = gw.TxAckStatus(val)
			} else {
				return fmt.Errorf("invalid ack error: %s", downlinkTXAck.Error)
			}
		}
	} else {
		for i := range downlinkTXAck.Items {
			ackStatus = downlinkTXAck.Items[i].Status

			if ackStatus == gw.TxAckStatus_OK {
				break
			}
		}
	}

	actx := ackContext{
		ctx:                 ctx,
		DownlinkTXAck:       downlinkTXAck,
		DownlinkTXAckStatus: ackStatus,
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
		if ctx.DownlinkTXAckStatus == gw.TxAckStatus_OK {
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
		if ctx.DownlinkTXAckStatus != gw.TxAckStatus_OK {
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

func getDownlinkFrame(ctx *ackContext) error {
	var err error

	// get the downlink frame using the token
	ctx.DownlinkFrame, err = storage.GetDownlinkFrame(ctx.ctx, ctx.Token)
	if err != nil {
		// return errors.Wrap(err, "get downlink-frame error")
		return errAbort
	}

	// items defines the multiple downlink opportunities (e.g. rx1 and rx2)
	if len(ctx.DownlinkFrame.DownlinkFrame.Items) == 0 {
		return errors.New("downlink-frame has no items")
	}

	// TODO: remove len(Items) != 0 check at next major release
	if len(ctx.DownlinkTXAck.Items) != 0 && len(ctx.DownlinkTXAck.Items) != len(ctx.DownlinkFrame.DownlinkFrame.Items) {
		return errors.New("length of ack items is not equal to length of downlink items")
	}

	// for backwards compatibility
	// TODO: remove at next major release
	if len(ctx.DownlinkTXAck.Items) == 0 {
		ctx.DownlinkFrameItem = ctx.DownlinkFrame.DownlinkFrame.Items[0]
	} else {
		// find positive ack
		for i := range ctx.DownlinkTXAck.Items {
			if ctx.DownlinkTXAck.Items[i].Status == gw.TxAckStatus_OK {
				ctx.DownlinkFrameItem = ctx.DownlinkFrame.DownlinkFrame.Items[i]
				break
			}
		}

		// take last negative ack if there is no positive ack
		if ctx.DownlinkFrameItem == nil {
			ctx.DownlinkFrameItem = ctx.DownlinkFrame.DownlinkFrame.Items[len(ctx.DownlinkTXAck.Items)-1]
		}
	}

	return nil
}

func decodePHYPayload(ctx *ackContext) error {
	var phy lorawan.PHYPayload
	if err := phy.UnmarshalBinary(ctx.DownlinkFrameItem.PhyPayload); err != nil {
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
	req := nc.HandleDownlinkMetaDataRequest{
		GatewayId:           ctx.DownlinkFrame.DownlinkFrame.GatewayId,
		DevEui:              ctx.DownlinkFrame.DevEui,
		MulticastGroupId:    ctx.DownlinkFrame.MulticastGroupId,
		TxInfo:              ctx.DownlinkFrameItem.TxInfo,
		PhyPayloadByteCount: uint32(len(ctx.DownlinkFrameItem.PhyPayload)),
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
	if len(ctx.DownlinkFrame.DevEui) == 0 || ctx.MACPayload == nil || ctx.MACPayload.FPort == nil || *ctx.MACPayload.FPort == 0 {
		return nil
	}

	var rpID uuid.UUID
	copy(rpID[:], ctx.DownlinkFrame.RoutingProfileId)

	asClient, err := helpers.GetASClientForRoutingProfileID(ctx.ctx, rpID)
	if err != nil {
		return errors.Wrap(err, "get application-server client for routing-profile id error")
	}

	// send async to as
	go func(ctx *ackContext, asClient as.ApplicationServerServiceClient) {
		_, err := asClient.HandleTxAck(ctx.ctx, &as.HandleTxAckRequest{
			DevEui: ctx.DownlinkFrame.DevEui,
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
	if (len(ctx.DownlinkTXAck.Items) == 0 && len(ctx.DownlinkFrame.DownlinkFrame.Items) >= 2) || ctx.MACPayload == nil || ctx.MACPayload.FPort == nil || *ctx.MACPayload.FPort == 0 {
		return nil
	}

	var rpID uuid.UUID
	copy(rpID[:], ctx.DownlinkFrame.RoutingProfileId)

	asClient, err := helpers.GetASClientForRoutingProfileID(ctx.ctx, rpID)
	if err != nil {
		return errors.Wrap(err, "get application-server client for routing-profile id error")
	}

	// send async to as
	go func(ctx *ackContext, asClient as.ApplicationServerServiceClient) {
		_, err := asClient.HandleError(ctx.ctx, &as.HandleErrorRequest{
			DevEui: ctx.DownlinkFrame.DevEui,
			FCnt:   ctx.MACPayload.FHDR.FCnt,
			Type:   as.ErrorType_DATA_DOWN_GATEWAY,
			Error:  ctx.DownlinkTXAckStatus.String(),
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

// for backwards compatibility
// TODO: remove at next major release
func sendDownlinkFrame(ctx *ackContext) error {
	if len(ctx.DownlinkTXAck.Items) != 0 || len(ctx.DownlinkFrame.DownlinkFrame.Items) < 2 {
		return nil
	}

	// send the next item
	item := ctx.DownlinkFrame.DownlinkFrame.Items[1]
	if err := gateway.Backend().SendTXPacket(gw.DownlinkFrame{
		GatewayId:  ctx.DownlinkFrame.DownlinkFrame.GatewayId,
		Token:      ctx.DownlinkFrame.DownlinkFrame.Token,
		DownlinkId: ctx.DownlinkFrame.DownlinkFrame.DownlinkId,
		Items: []*gw.DownlinkFrameItem{
			item,
		},
	}); err != nil {
		return errors.Wrap(err, "send downlink-frame to gateway error")
	}
	return nil
}

// for backwards compatibility
// TODO: remove at next major release
func saveDownlinkFrames(ctx *ackContext) error {
	if len(ctx.DownlinkTXAck.Items) != 0 || len(ctx.DownlinkFrame.DownlinkFrame.Items) < 2 {
		return nil
	}

	ctx.DownlinkFrame.DownlinkFrame.Items = ctx.DownlinkFrame.DownlinkFrame.Items[1:]
	if err := storage.SaveDownlinkFrame(ctx.ctx, ctx.DownlinkFrame); err != nil {
		return errors.Wrap(err, "save downlink-frames error")
	}

	return nil
}

func logDownlinkFrame(ctx *ackContext) error {
	// log for gateway (with encrypted mac-commands)
	if err := framelog.LogDownlinkFrameForGateway(ctx.ctx, ns.DownlinkFrameLog{
		PhyPayload: ctx.DownlinkFrameItem.PhyPayload,
		TxInfo:     ctx.DownlinkFrameItem.TxInfo,
		Token:      ctx.DownlinkFrame.DownlinkFrame.Token,
		DownlinkId: ctx.DownlinkFrame.DownlinkFrame.DownlinkId,
		GatewayId:  ctx.DownlinkFrame.DownlinkFrame.GatewayId,
	}); err != nil {
		log.WithError(err).WithFields(log.Fields{
			"ctx_id": ctx.ctx.Value(logging.ContextIDKey),
		}).Error("log downlink frame for gateway error")
	}

	var devEUI lorawan.EUI64
	var nwkSEncKey lorawan.AES128Key
	copy(devEUI[:], ctx.DownlinkFrame.DevEui)
	copy(nwkSEncKey[:], ctx.DownlinkFrame.NwkSEncKey)

	// log for device (with decrypted mac-commands)
	var phy lorawan.PHYPayload
	if err := phy.UnmarshalBinary(ctx.DownlinkFrameItem.PhyPayload); err != nil {
		return err
	}

	// decrypt FRMPayload mac-commands
	if ctx.MACPayload != nil && ctx.MACPayload.FPort != nil && *ctx.MACPayload.FPort == 0 {
		if err := phy.DecryptFRMPayload(nwkSEncKey); err != nil {
			return errors.Wrap(err, "decrypt frmpayload error")
		}
	}

	// decrypt FOpts mac-commands (LoRaWAN 1.1)
	if ctx.DownlinkFrame.EncryptedFopts {
		if err := phy.DecryptFOpts(nwkSEncKey); err != nil {
			return errors.Wrap(err, "decrypt FOpts error")
		}
	}

	phyB, err := phy.MarshalBinary()
	if err != nil {
		return err
	}

	if err := framelog.LogDownlinkFrameForDevEUI(ctx.ctx, devEUI, ns.DownlinkFrameLog{
		PhyPayload: phyB,
		TxInfo:     ctx.DownlinkFrameItem.TxInfo,
		Token:      ctx.DownlinkFrame.DownlinkFrame.Token,
		DownlinkId: ctx.DownlinkFrame.DownlinkFrame.DownlinkId,
		GatewayId:  ctx.DownlinkFrame.DownlinkFrame.GatewayId,
	}); err != nil {
		log.WithError(err).WithFields(log.Fields{
			"ctx_id": ctx.ctx.Value(logging.ContextIDKey),
		}).Error("log downlink frame for device error")
	}

	return nil
}
