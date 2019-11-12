package maccommand

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-api/go/as"
	"github.com/brocaar/chirpstack-network-server/internal/logging"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
)

// RequestDevStatus returns a mac-command block for requesting the device-status.
func RequestDevStatus(ctx context.Context, ds *storage.DeviceSession) storage.MACCommandBlock {
	block := storage.MACCommandBlock{
		CID: lorawan.DevStatusReq,
		MACCommands: []lorawan.MACCommand{
			{
				CID: lorawan.DevStatusReq,
			},
		},
	}
	ds.LastDevStatusRequested = time.Now()
	log.WithFields(log.Fields{
		"dev_eui": ds.DevEUI,
		"ctx_id":  ctx.Value(logging.ContextIDKey),
	}).Info("requesting device-status")
	return block
}

func handleDevStatusAns(ctx context.Context, ds *storage.DeviceSession, sp storage.ServiceProfile, asClient as.ApplicationServerServiceClient, block storage.MACCommandBlock) ([]storage.MACCommandBlock, error) {
	if len(block.MACCommands) != 1 {
		return nil, fmt.Errorf("exactly one mac-command expected, got %d", len(block.MACCommands))
	}

	pl, ok := block.MACCommands[0].Payload.(*lorawan.DevStatusAnsPayload)
	if !ok {
		return nil, fmt.Errorf("expected *lorawan.DevStatusAnsPayload, got %T", block.MACCommands[0].Payload)
	}

	log.WithFields(log.Fields{
		"dev_eui": ds.DevEUI,
		"battery": pl.Battery,
		"margin":  pl.Margin,
		"ctx_id":  ctx.Value(logging.ContextIDKey),
	}).Info("dev_status_ans answer received")

	if !sp.ReportDevStatusBattery && !sp.ReportDevStatusMargin {
		log.WithFields(log.Fields{
			"dev_eui": ds.DevEUI,
			"ctx_id":  ctx.Value(logging.ContextIDKey),
		}).Warning("reporting device-status has been disabled in service-profile")
		return nil, nil
	}

	go func() {
		req := as.SetDeviceStatusRequest{
			DevEui: ds.DevEUI[:],
		}

		if sp.ReportDevStatusBattery {
			req.Battery = uint32(pl.Battery)

			switch pl.Battery {
			case 255:
				req.BatteryLevelUnavailable = true
			case 0:
				req.ExternalPowerSource = true
			default:
				req.BatteryLevel = float32(pl.Battery) / 254 * 100
			}

			if pl.Battery == 255 {
				req.BatteryLevelUnavailable = true
			}
		} else {
			req.BatteryLevelUnavailable = true
		}

		if sp.ReportDevStatusMargin {
			req.Margin = int32(pl.Margin)
		}

		_, err := asClient.SetDeviceStatus(ctx, &req)
		if err != nil {
			log.WithFields(log.Fields{
				"dev_eui": ds.DevEUI,
				"ctx_id":  ctx.Value(logging.ContextIDKey),
			}).WithError(err).Error("as.SetDeviceStatus error")
		}
	}()

	return nil, nil
}
