package maccommand

import (
	"fmt"

	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/pkg/errors"
)

// RequestTXParamSetup modifies the uplink / downlink dwell time and uplink
// max. EIRP settings on the device.
func RequestTXParamSetup(uplinkDwellTime400ms, downlinkDwellTime400ms bool, maxEIRP uint8) storage.MACCommandBlock {
	uplinkDwellTime := lorawan.DwellTimeNoLimit
	downlinkDwellTime := lorawan.DwellTimeNoLimit
	if uplinkDwellTime400ms {
		uplinkDwellTime = lorawan.DwellTime400ms
	}
	if downlinkDwellTime400ms {
		downlinkDwellTime = lorawan.DwellTime400ms
	}

	return storage.MACCommandBlock{
		CID: lorawan.TXParamSetupReq,
		MACCommands: []lorawan.MACCommand{
			{
				CID: lorawan.TXParamSetupReq,
				Payload: &lorawan.TXParamSetupReqPayload{
					DownlinkDwelltime: downlinkDwellTime,
					UplinkDwellTime:   uplinkDwellTime,
					MaxEIRP:           maxEIRP,
				},
			},
		},
	}
}

func handleTXParamSetupAns(ds *storage.DeviceSession, block storage.MACCommandBlock, pendingBlock *storage.MACCommandBlock) ([]storage.MACCommandBlock, error) {
	if pendingBlock == nil || len(pendingBlock.MACCommands) == 0 {
		return nil, errors.New("expected pending mac-command")
	}

	txParamReqPL, ok := pendingBlock.MACCommands[0].Payload.(*lorawan.TXParamSetupReqPayload)
	if !ok {
		return nil, fmt.Errorf("expected *lorawan.TXParamSetupReqPayload, got %T", pendingBlock.MACCommands[0].Payload)
	}

	ds.UplinkDwellTime400ms = txParamReqPL.UplinkDwellTime == lorawan.DwellTime400ms
	ds.DownlinkDwellTime400ms = txParamReqPL.DownlinkDwelltime == lorawan.DwellTime400ms
	ds.UplinkMaxEIRPIndex = txParamReqPL.MaxEIRP

	return nil, nil
}
