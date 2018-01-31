package maccommand

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/Frankz/loraserver/internal/common"
	"github.com/Frankz/loraserver/internal/storage"
	"github.com/Frankz/lorawan"
)

// RequestDevStatus adds a dev-status request mac-command to the queue.
func RequestDevStatus(ds *storage.DeviceSession) error {
	block := Block{
		CID: lorawan.DevStatusReq,
		MACCommands: []lorawan.MACCommand{
			{
				CID: lorawan.DevStatusReq,
			},
		},
	}
	if err := AddQueueItem(common.RedisPool, ds.DevEUI, block); err != nil {
		return errors.Wrap(err, "add mac-command queue item error")
	}
	ds.LastDevStatusRequested = time.Now()
	return nil
}

func handleDevStatusAns(ds *storage.DeviceSession, block Block) error {
	if len(block.MACCommands) != 1 {
		return fmt.Errorf("exactly one mac-command expected, got %d", len(block.MACCommands))
	}

	pl, ok := block.MACCommands[0].Payload.(*lorawan.DevStatusAnsPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.DevStatusAnsPayload, got %T", block.MACCommands[0].Payload)
	}

	ds.LastDevStatusBattery = pl.Battery
	ds.LastDevStatusMargin = pl.Margin

	log.WithFields(log.Fields{
		"dev_eui": ds.DevEUI,
		"battery": ds.LastDevStatusBattery,
		"margin":  ds.LastDevStatusMargin,
	}).Info("dev_status_ans answer received")

	return nil
}
