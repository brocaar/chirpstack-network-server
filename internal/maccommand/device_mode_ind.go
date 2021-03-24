package maccommand

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
)

func handleDeviceModeInd(ctx context.Context, ds *storage.DeviceSession, block storage.MACCommandBlock) ([]storage.MACCommandBlock, error) {
	if len(block.MACCommands) != 1 {
		return nil, errors.New("exactly 1 mac-command is expected")
	}

	pl, ok := block.MACCommands[0].Payload.(*lorawan.DeviceModeIndPayload)
	if !ok {
		return nil, fmt.Errorf("expected *lorawan.DeviceModeIntPayload, got: %T", block.MACCommands[0].Payload)
	}

	d, err := storage.GetDevice(ctx, storage.DB(), ds.DevEUI, false)
	if err != nil {
		return nil, errors.Wrap(err, "get device error")
	}

	switch pl.Class {
	case lorawan.DeviceModeClassA:
		d.Mode = storage.DeviceModeA
	case lorawan.DeviceModeClassC:
		d.Mode = storage.DeviceModeC
	default:
		return nil, fmt.Errorf("unexpected device mode: %s", pl.Class)
	}

	if err := storage.UpdateDevice(ctx, storage.DB(), &d); err != nil {
		return nil, errors.Wrap(err, "update device error")
	}

	return []storage.MACCommandBlock{
		{
			CID: lorawan.DeviceModeConf,
			MACCommands: []lorawan.MACCommand{
				{
					CID: lorawan.DeviceModeConf,
					Payload: &lorawan.DeviceModeConfPayload{
						Class: pl.Class,
					},
				},
			},
		},
	}, nil
}
