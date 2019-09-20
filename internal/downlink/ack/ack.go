package ack

import (
	"context"
	"encoding/binary"

	"github.com/brocaar/lorawan"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/api/nc"
	"github.com/brocaar/loraserver/internal/backend/controller"
	"github.com/brocaar/loraserver/internal/backend/gateway"
	"github.com/brocaar/loraserver/internal/logging"
	"github.com/brocaar/loraserver/internal/storage"
)

var (
	errAbort = errors.New("abort")
)

var handleDownlinkTXAckTasks = []func(*ackContext) error{
	getToken,
	getDownlinkFrames,
	sendDownlinkMetaDataToNetworkControllerOnNoError,
	abortOnNoError,
	sendDownlinkFrame,
	saveDownlinkFrames,
}

type ackContext struct {
	ctx context.Context

	Token          uint16
	DevEUI         lorawan.EUI64
	DownlinkTXAck  gw.DownlinkTXAck
	DownlinkFrames storage.DownlinkFrames
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
	ctx.DownlinkFrames, err = storage.GetDownlinkFrames(ctx.ctx, storage.RedisPool(), ctx.Token)
	if err != nil {
		return errors.Wrap(err, "get downlink-frames error")
	}

	return nil
}

func sendDownlinkMetaDataToNetworkControllerOnNoError(ctx *ackContext) error {
	if ctx.DownlinkTXAck.Error != "" || controller.Client() == nil {
		return nil
	}

	if len(ctx.DownlinkFrames.DownlinkFrames) == 0 {
		return errors.New("downlink-frames is empty")
	}

	downlinkFrame := ctx.DownlinkFrames.DownlinkFrames[0]

	var phy lorawan.PHYPayload
	if err := phy.UnmarshalBinary(downlinkFrame.PhyPayload); err != nil {
		log.WithError(err).WithFields(log.Fields{
			"ctx_id": ctx.ctx.Value(logging.ContextIDKey),
		}).Error("unmarshal phypayload error")
		return nil
	}

	req := nc.HandleDownlinkMetaDataRequest{
		DevEui:              ctx.DownlinkFrames.DevEui,
		MulticastGroupId:    ctx.DownlinkFrames.MulticastGroupId,
		TxInfo:              downlinkFrame.TxInfo,
		PhyPayloadByteCount: uint32(len(downlinkFrame.PhyPayload)),
	}

	// message type
	switch phy.MHDR.MType {
	case lorawan.JoinAccept:
		req.MessageType = nc.MType_JOIN_ACCEPT
	case lorawan.UnconfirmedDataDown:
		req.MessageType = nc.MType_UNCONFIRMED_DATA_DOWN
	case lorawan.ConfirmedDataDown:
		req.MessageType = nc.MType_CONFIRMED_DATA_DOWN
	}

	if macPL, ok := phy.MACPayload.(*lorawan.MACPayload); ok {
		for _, pl := range macPL.FRMPayload {
			if b, err := pl.MarshalBinary(); err == nil {
				if macPL.FPort != nil && *macPL.FPort != 0 {
					req.ApplicationPayloadByteCount += uint32(len(b))
				} else {
					req.MacCommandByteCount += uint32(len(b))
				}
			}
		}

		for _, m := range macPL.FHDR.FOpts {
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

func abortOnNoError(ctx *ackContext) error {
	if ctx.DownlinkTXAck.Error == "" {
		// no error, nothing to do
		return errAbort
	}
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
	if err := storage.SaveDownlinkFrames(ctx.ctx, storage.RedisPool(), ctx.DownlinkFrames); err != nil {
		return errors.Wrap(err, "save downlink-frames error")
	}

	return nil
}
