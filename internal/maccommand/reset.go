package maccommand

import (
	"context"
	"fmt"

	"github.com/brocaar/chirpstack-network-server/internal/logging"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
	log "github.com/sirupsen/logrus"
)

func handleResetInd(ctx context.Context, ds *storage.DeviceSession, dp storage.DeviceProfile, block storage.MACCommandBlock) ([]storage.MACCommandBlock, error) {
	if len(block.MACCommands) != 1 {
		return nil, fmt.Errorf("exactly one mac-command expected, got %d", len(block.MACCommands))
	}

	pl, ok := block.MACCommands[0].Payload.(*lorawan.ResetIndPayload)
	if !ok {
		return nil, fmt.Errorf("expected *lorawan.ResetIndPayload, got %T", block.MACCommands[0].Payload)
	}

	respPL := lorawan.ResetConfPayload{
		ServLoRaWANVersion: lorawan.Version{
			Minor: servLoRaWANVersionMinor,
		},
	}

	if servLoRaWANVersionMinor > pl.DevLoRaWANVersion.Minor {
		respPL.ServLoRaWANVersion.Minor = pl.DevLoRaWANVersion.Minor
	}

	log.WithFields(log.Fields{
		"dev_eui":                    ds.DevEUI,
		"dev_lorawan_version_minor":  pl.DevLoRaWANVersion.Minor,
		"serv_lorawan_version_minor": servLoRaWANVersionMinor,
		"ctx_id":                     ctx.Value(logging.ContextIDKey),
	}).Info("reset_ind received")

	ds.ResetToBootParameters(dp)

	return []storage.MACCommandBlock{
		{
			CID: lorawan.ResetConf,
			MACCommands: storage.MACCommands{
				{
					CID:     lorawan.ResetConf,
					Payload: &respPL,
				},
			},
		},
	}, nil
}
