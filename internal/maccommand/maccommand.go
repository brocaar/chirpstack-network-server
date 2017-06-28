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
func Handle(ctx common.Context, ns *session.NodeSession, block Block, pending *Block, rxInfoSet models.RXInfoSet) error {
	var err error
	switch block.CID {
	case lorawan.LinkADRAns:
		err = handleLinkADRAns(ctx, ns, block, pending)
	case lorawan.LinkCheckReq:
		err = handleLinkCheckReq(ctx, ns, rxInfoSet)
	default:
		err = fmt.Errorf("undefined CID %d", block.CID)

	}
	return err
}

// handleLinkADRAns handles the ack of an ADR request
func handleLinkADRAns(ctx common.Context, ns *session.NodeSession, block Block, pendingBlock *Block) error {
	if len(block.MACCommands) == 0 {
		return errors.New("at least 1 mac-command expected, got none")
	}

	if pendingBlock == nil || len(pendingBlock.MACCommands) == 0 {
		return ErrDoesNotExist
	}

	channelMaskACK := true
	dataRateACK := true
	powerACK := true

	for i := range block.MACCommands {
		pl, ok := block.MACCommands[i].Payload.(*lorawan.LinkADRAnsPayload)
		if !ok {
			return fmt.Errorf("expected *lorawan.LinkADRAnsPayload, got %T", pl)
		}

		if !pl.ChannelMaskACK {
			channelMaskACK = false
		}
		if !pl.DataRateACK {
			dataRateACK = false
		}
		if !pl.PowerACK {
			powerACK = false
		}
	}

	var linkADRPayloads []lorawan.LinkADRReqPayload
	for i := range pendingBlock.MACCommands {
		linkADRPayloads = append(linkADRPayloads, *pendingBlock.MACCommands[i].Payload.(*lorawan.LinkADRReqPayload))
	}

	// as we're sending the same txpower and nbrep for each channel we
	// take the last one
	adrReq := linkADRPayloads[len(linkADRPayloads)-1]

	if channelMaskACK && dataRateACK && powerACK {
		chans, err := common.Band.GetEnabledChannelsForLinkADRReqPayloads(ns.EnabledChannels, linkADRPayloads)
		if err != nil {
			return errors.Wrap(err, "get enalbed channels for link_adr_req payloads error")
		}

		ns.TXPowerIndex = int(adrReq.TXPower)
		ns.NbTrans = adrReq.Redundancy.NbRep
		ns.EnabledChannels = chans

		log.WithFields(log.Fields{
			"dev_eui":          ns.DevEUI,
			"tx_power_idx":     ns.TXPowerIndex,
			"dr":               adrReq.DataRate,
			"nb_trans":         adrReq.Redundancy.NbRep,
			"enabled_channels": chans,
		}).Info("link_adr request acknowledged")
	} else {
		// This is a workaround for the RN2483 firmware (1.0.3) which sends
		// a nACK on TXPower 0 (this is incorrect behaviour, following the
		// specs). It should ACK and operate at its maximum possible power
		// when TXPower 0 is not supported. See also section 5.2 in the
		// LoRaWAN specs.
		if channelMaskACK && dataRateACK && !powerACK && adrReq.TXPower == 0 {
			ns.TXPowerIndex = 1
		}

		log.WithFields(log.Fields{
			"dev_eui":          ns.DevEUI,
			"channel_mask_ack": channelMaskACK,
			"data_rate_ack":    dataRateACK,
			"power_ack":        powerACK,
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

	if err := AddQueueItem(ctx.RedisPool, ns.DevEUI, block); err != nil {
		return errors.Wrap(err, "add mac-command block to queue error")
	}
	return nil
}
