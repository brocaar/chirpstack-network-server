package maccommand

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
)

// RequestDevStatus returns a mac-command block for requesting the device-status.
func RequestDevStatus(ds *storage.DeviceSession) storage.MACCommandBlock {
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
	}).Info("requesting device-status")
	return block
}

func handleDevStatusAns(ds *storage.DeviceSession, block storage.MACCommandBlock) ([]storage.MACCommandBlock, error) {
	if len(block.MACCommands) != 1 {
		return nil, fmt.Errorf("exactly one mac-command expected, got %d", len(block.MACCommands))
	}

	pl, ok := block.MACCommands[0].Payload.(*lorawan.DevStatusAnsPayload)
	if !ok {
		return nil, fmt.Errorf("expected *lorawan.DevStatusAnsPayload, got %T", block.MACCommands[0].Payload)
	}

	ds.LastDevStatusBattery = pl.Battery
	ds.LastDevStatusMargin = pl.Margin

	log.WithFields(log.Fields{
		"dev_eui": ds.DevEUI,
		"battery": ds.LastDevStatusBattery,
		"margin":  ds.LastDevStatusMargin,
	}).Info("dev_status_ans answer received")

	return nil, nil
}
