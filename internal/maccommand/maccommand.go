package maccommand

import (
	"errors"
	"fmt"

	log "github.com/Sirupsen/logrus"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/session"
	"github.com/brocaar/lorawan"
)

// Handle handles a MACCommand sent by a node.
func Handle(ctx common.Context, ns *session.NodeSession, cmd lorawan.MACCommand) error {
	var err error
	switch cmd.CID {
	case lorawan.LinkADRAns:
		err = handleLinkADRAns(ctx, ns, cmd.Payload)
	default:
		err = fmt.Errorf("undefined CID %d", cmd.CID)

	}
	return err
}

// handleLinkADRAns handles the ack of an ADR request
func handleLinkADRAns(ctx common.Context, ns *session.NodeSession, pl lorawan.MACCommandPayload) error {
	adrAns, ok := pl.(*lorawan.LinkADRAnsPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.LinkADRAnsPayload, got %T", pl)
	}

	block, err := ReadPending(ctx.RedisPool, ns.DevEUI, lorawan.LinkADRReq)
	if err != nil {
		return fmt.Errorf("read pending mac-commands error: %s", err)
	}
	if block == nil || len(block.MACCommands) == 0 {
		return errors.New("no pending adr requests found")
	}

	// as we're sending the same txpower and nbrep for each channel we
	// just take the first item from the slice and use its values
	adrReq, ok := block.MACCommands[0].Payload.(*lorawan.LinkADRReqPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.LinkADRReqPayload, got %T", block.MACCommands[0].Payload)
	}

	if adrAns.ChannelMaskACK && adrAns.DataRateACK && adrAns.PowerACK {
		ns.TXPower = common.Band.TXPower[adrReq.TXPower]
		ns.NbTrans = adrReq.Redundancy.NbRep

		log.WithFields(log.Fields{
			"dev_eui":  ns.DevEUI,
			"tx_power": ns.TXPower,
			"dr":       adrReq.DataRate,
			"nb_trans": adrReq.Redundancy.NbRep,
		}).Info("adr request acknowledged")
	} else {
		log.WithFields(log.Fields{
			"dev_eui":          ns.DevEUI,
			"channel_mask_ack": adrAns.ChannelMaskACK,
			"data_rate_ack":    adrAns.DataRateACK,
			"power_ack":        adrAns.PowerACK,
		}).Warning("adr request not acknowledged")
	}

	return nil
}
