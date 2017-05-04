package maccommand

import (
	"fmt"

	log "github.com/Sirupsen/logrus"
	"github.com/pkg/errors"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/session"
	"github.com/brocaar/lorawan"
)

// Handle handles a MACCommand sent by a node.
func Handle(ctx common.Context, ns *session.NodeSession, cmd lorawan.MACCommand, rxInfoSet models.RXInfoSet) error {
	var err error
	switch cmd.CID {
	case lorawan.LinkADRAns:
		err = handleLinkADRAns(ctx, ns, cmd.Payload)
	case lorawan.LinkCheckReq:
		err = handleLinkCheckReq(ctx, ns, rxInfoSet)
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
		return ErrDoesNotExist
	}

	if err = DeletePending(ctx.RedisPool, ns.DevEUI, lorawan.LinkADRReq); err != nil {
		return errors.Wrap(err, "delete pending mac-command block error")
	}

	var linkADRPayloads []lorawan.LinkADRReqPayload
	for i := range block.MACCommands {
		linkADRPayloads = append(linkADRPayloads, *block.MACCommands[i].Payload.(*lorawan.LinkADRReqPayload))
	}

	// as we're sending the same txpower and nbrep for each channel we
	// take the last one
	adrReq := linkADRPayloads[len(linkADRPayloads)-1]

	if adrAns.ChannelMaskACK && adrAns.DataRateACK && adrAns.PowerACK {
		chans, err := common.Band.GetEnabledChannelsForLinkADRReqPayloads(ns.EnabledChannels, linkADRPayloads)
		if err != nil {
			return errors.Wrap(err, "get enalbed channels for link_adr_req payloads error")
		}

		ns.TXPower = common.Band.TXPower[adrReq.TXPower]
		ns.NbTrans = adrReq.Redundancy.NbRep
		ns.EnabledChannels = chans

		log.WithFields(log.Fields{
			"dev_eui":          ns.DevEUI,
			"tx_power":         ns.TXPower,
			"dr":               adrReq.DataRate,
			"nb_trans":         adrReq.Redundancy.NbRep,
			"enabled_channels": chans,
		}).Info("link_adr request acknowledged")
	} else {
		log.WithFields(log.Fields{
			"dev_eui":          ns.DevEUI,
			"channel_mask_ack": adrAns.ChannelMaskACK,
			"data_rate_ack":    adrAns.DataRateACK,
			"power_ack":        adrAns.PowerACK,
		}).Warning("link_adr request not acknowledged")
	}

	return nil
}

func handleLinkCheckReq(ctx common.Context, ns *session.NodeSession, rxInfoSet models.RXInfoSet) error {
	if len(rxInfoSet) == 0 {
		return errors.New("rx info-set contains zero items")
	}

	requiredSNR, ok := common.SpreadFactorToRequiredSNRTable[rxInfoSet[0].DataRate.SpreadFactor]
	if !ok {
		return fmt.Errorf("sf %d not in sf to required snr table", rxInfoSet[0].DataRate.SpreadFactor)
	}

	margin := rxInfoSet[0].LoRaSNR - requiredSNR
	if margin < 0 {
		margin = 0
	}

	block := Block{
		CID: lorawan.LinkCheckAns,
		MACCommands: MACCommands{
			{
				CID: lorawan.LinkCheckAns,
				Payload: &lorawan.LinkCheckAnsPayload{
					Margin: uint8(margin),
					GwCnt:  uint8(len(rxInfoSet)),
				},
			},
		},
	}

	if err := AddToQueue(ctx.RedisPool, ns.DevEUI, block); err != nil {
		return errors.Wrap(err, "add mac-command block to queue error")
	}
	return nil
}
