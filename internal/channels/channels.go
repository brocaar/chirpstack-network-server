package channels

import (
	"fmt"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/maccommand"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/session"
	"github.com/brocaar/lorawan"
	"github.com/pkg/errors"
)

// HandleChannelReconfigure handles the reconfiguration of active channels
// on the node. This is needed in case only a sub-set of channels is used
// (e.g. for the US band) or when a reconfiguration of active channels
// happens.
func HandleChannelReconfigure(ctx common.Context, ns session.NodeSession, rxPacket models.RXPacket) error {
	payloads := common.Band.GetLinkADRReqPayloadsForEnabledChannels(ns.EnabledChannels)
	if len(payloads) == 0 {
		return nil
	}

	// set the current tx-power, data-rate and nbrep on the last payload
	currentDR, err := common.Band.GetDataRate(rxPacket.RXInfoSet[0].DataRate)
	if err != nil {
		return fmt.Errorf("get data-rate error: %s", err)
	}
	currentTXPower := getCurrentTXPower(ns)
	currentTXPowerIndex := getTXPowerIndex(currentTXPower)
	payloads[len(payloads)-1].TXPower = uint8(currentTXPowerIndex)
	payloads[len(payloads)-1].DataRate = uint8(currentDR)
	payloads[len(payloads)-1].Redundancy.NbRep = ns.NbTrans

	// when reconfiguring the channels requires more than 3 commands, we must
	// send these as FRMPayload as the FOpts has a max of 15 bytes and each
	// command requires 5 bytes.
	var frmPayload bool
	if len(payloads) > 3 {
		frmPayload = true
	}

	block := maccommand.Block{
		CID:        lorawan.LinkADRReq,
		FRMPayload: frmPayload,
	}
	for i := range payloads {
		block.MACCommands = append(block.MACCommands, lorawan.MACCommand{
			CID:     lorawan.LinkADRReq,
			Payload: &payloads[i],
		})
	}

	if err = maccommand.DeleteQueueItemByCID(ctx.RedisPool, ns.DevEUI, lorawan.LinkADRReq); err != nil {
		return errors.Wrap(err, "delete queue item by cid error")
	}

	if err = maccommand.AddToQueue(ctx.RedisPool, ns.DevEUI, block); err != nil {
		return errors.Wrap(err, "add mac-command block to queue error")
	}

	if err = maccommand.SetPending(ctx.RedisPool, ns.DevEUI, block); err != nil {
		return errors.Wrap(err, "set pending mac-command block error")
	}

	return nil
}

func getCurrentTXPower(ns session.NodeSession) int {
	if ns.TXPower > 0 {
		return ns.TXPower
	}
	return common.Band.DefaultTXPower
}

func getTXPowerIndex(txPower int) int {
	var idx int
	for i, p := range common.Band.TXPower {
		if p >= txPower {
			idx = i
		}
	}
	return idx
}
