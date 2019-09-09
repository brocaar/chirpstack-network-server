package maccommand

import (
	"context"
	"fmt"

	"github.com/brocaar/loraserver/internal/logging"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// RequestRejoinParamSetup modifies the rejoin-request interval parameters.
func RequestRejoinParamSetup(maxTimeN, maxCountN int) storage.MACCommandBlock {
	return storage.MACCommandBlock{
		CID: lorawan.RejoinParamSetupReq,
		MACCommands: []lorawan.MACCommand{
			{
				CID: lorawan.RejoinParamSetupReq,
				Payload: &lorawan.RejoinParamSetupReqPayload{
					MaxTimeN:  uint8(maxTimeN),
					MaxCountN: uint8(maxCountN),
				},
			},
		},
	}
}

func handleRejoinParamSetupAns(ctx context.Context, ds *storage.DeviceSession, block storage.MACCommandBlock, pendingBlock *storage.MACCommandBlock) ([]storage.MACCommandBlock, error) {
	if len(block.MACCommands) != 1 {
		return nil, fmt.Errorf("exactly one mac-command expected, got: %d", len(block.MACCommands))
	}

	if pendingBlock == nil || len(pendingBlock.MACCommands) == 0 {
		return nil, errors.New("expected pending mac-command")
	}
	req := pendingBlock.MACCommands[0].Payload.(*lorawan.RejoinParamSetupReqPayload)

	pl, ok := block.MACCommands[0].Payload.(*lorawan.RejoinParamSetupAnsPayload)
	if !ok {
		return nil, fmt.Errorf("expected *lorawan.RejoinParamSetupAnsPayload, got %T", block.MACCommands[0].Payload)
	}

	ds.RejoinRequestEnabled = true
	ds.RejoinRequestMaxCountN = int(req.MaxCountN)
	ds.RejoinRequestMaxTimeN = int(req.MaxTimeN)

	if pl.TimeOK {
		log.WithFields(log.Fields{
			"dev_eui": ds.DevEUI,
			"time_ok": pl.TimeOK,
			"ctx_id":  ctx.Value(logging.ContextIDKey),
		}).Info("rejoin_param_setup request acknowledged")
	} else {
		log.WithFields(log.Fields{
			"dev_eui": ds.DevEUI,
			"time_ok": pl.TimeOK,
			"ctx_id":  ctx.Value(logging.ContextIDKey),
		}).Warning("rejoin_param_setup request acknowledged")
	}

	return nil, nil
}
