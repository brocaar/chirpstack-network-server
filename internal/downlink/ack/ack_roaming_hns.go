package ack

import (
	"context"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-network-server/v3/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
)

type RoamingAckPayload struct {
	XmitDataAns         backend.XmitDataAnsPayload
	Token               uint32
	DownlinkTXAck       *gw.DownlinkTXAck
	DownlinkTXAckStatus gw.TxAckStatus
	DownlinkFrame       *storage.DownlinkFrame
	DeviceSession       storage.DeviceSession
	DeviceProfile       storage.DeviceProfile
	DeviceQueueItem     storage.DeviceQueueItem
	MHDR                lorawan.MHDR
	MACPayload          *lorawan.MACPayload
}

// HandleDownlinkXmitDataAns handles an ack as hNS.
func HandleDownlinkXmitDataAns(ctx context.Context, pl RoamingAckPayload) error {
	actx := ackContext{
		ctx:                 ctx,
		DB:                  storage.DB(),
		DownlinkTXAck:       pl.DownlinkTXAck,
		DownlinkTXAckStatus: pl.DownlinkTXAckStatus,
	}

	for _, t := range handleDownlinkTXAckTasks {
		if err := t(&actx); err != nil {
			return err
		}
	}

	return nil
}
