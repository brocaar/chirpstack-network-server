package maccommand

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-network-server/internal/logging"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
)

func handlePingSlotInfoReq(ctx context.Context, ds *storage.DeviceSession, block storage.MACCommandBlock) ([]storage.MACCommandBlock, error) {
	if len(block.MACCommands) != 1 {
		return nil, fmt.Errorf("exactly one mac-command expected, got: %d", len(block.MACCommands))
	}

	pl, ok := block.MACCommands[0].Payload.(*lorawan.PingSlotInfoReqPayload)
	if !ok {
		return nil, fmt.Errorf("expected *lorawan.PingSlotInfoReqPayload, got: %T", block.MACCommands[0].Payload)
	}

	ds.PingSlotNb = 1 << (7 - pl.Periodicity)

	log.WithFields(log.Fields{
		"dev_eui":      ds.DevEUI,
		"periodicity":  pl.Periodicity,
		"ping_slot_nb": ds.PingSlotNb,
		"ctx_id":       ctx.Value(logging.ContextIDKey),
	}).Info("ping_slot_info_req request received")

	return []storage.MACCommandBlock{
		{
			CID: lorawan.PingSlotInfoAns,
			MACCommands: []lorawan.MACCommand{
				{CID: lorawan.PingSlotInfoAns},
			},
		},
	}, nil
}
