package ack

import (
	"github.com/brocaar/lorawan"
	"github.com/pkg/errors"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/backend/gateway"
	"github.com/brocaar/loraserver/internal/storage"
)

var (
	errAbort = errors.New("abort")
)

var handleDownlinkTXAckTasks = []func(*ackContext) error{
	abortOnNoError,
	getDownlinkFrame,
	sendDownlinkFrame,
}

type ackContext struct {
	DevEUI        lorawan.EUI64
	DownlinkTXAck gw.DownlinkTXAck
	DownlinkFrame gw.DownlinkFrame
}

// HandleDownlinkTXAck handles the given downlink TX acknowledgement.
func HandleDownlinkTXAck(downlinkTXAck gw.DownlinkTXAck) error {
	ctx := ackContext{
		DownlinkTXAck: downlinkTXAck,
	}

	for _, t := range handleDownlinkTXAckTasks {
		if err := t(&ctx); err != nil {
			if err == errAbort {
				return nil
			}
			return err
		}
	}

	return nil
}

func abortOnNoError(ctx *ackContext) error {
	if ctx.DownlinkTXAck.Error == "" {
		// no error, nothing to do
		return errAbort
	}
	return nil
}

func getDownlinkFrame(ctx *ackContext) error {
	var err error
	ctx.DevEUI, ctx.DownlinkFrame, err = storage.PopDownlinkFrame(storage.RedisPool(), ctx.DownlinkTXAck.Token)
	if err != nil {
		if err == storage.ErrDoesNotExist {
			// no retry is possible, abort
			return errAbort
		}
		return errors.Wrap(err, "pop downlink-frame error")
	}
	return nil
}

func sendDownlinkFrame(ctx *ackContext) error {
	if err := gateway.Backend().SendTXPacket(ctx.DownlinkFrame); err != nil {
		return errors.Wrap(err, "send downlink-frame to gateway error")
	}
	return nil
}
