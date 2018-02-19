package maccommand

import (
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
)

// Handle handles a MACCommand sent by a node.
func Handle(ds *storage.DeviceSession, block storage.MACCommandBlock, pending *storage.MACCommandBlock, rxPacket models.RXPacket) ([]storage.MACCommandBlock, error) {
	switch block.CID {
	case lorawan.LinkADRAns:
		return handleLinkADRAns(ds, block, pending)
	case lorawan.LinkCheckReq:
		return handleLinkCheckReq(ds, rxPacket)
	case lorawan.DevStatusAns:
		return handleDevStatusAns(ds, block)
	default:
		return nil, fmt.Errorf("undefined CID %d", block.CID)

	}
}

// handleLinkADRAns handles the ack of an ADR request
func handleLinkADRAns(ds *storage.DeviceSession, block storage.MACCommandBlock, pendingBlock *storage.MACCommandBlock) ([]storage.MACCommandBlock, error) {
	if len(block.MACCommands) == 0 {
		return nil, errors.New("at least 1 mac-command expected, got none")
	}

	if pendingBlock == nil || len(pendingBlock.MACCommands) == 0 {
		return nil, errors.New("expected pending mac-command")
	}

	channelMaskACK := true
	dataRateACK := true
	powerACK := true

	for i := range block.MACCommands {
		pl, ok := block.MACCommands[i].Payload.(*lorawan.LinkADRAnsPayload)
		if !ok {
			return nil, fmt.Errorf("expected *lorawan.LinkADRAnsPayload, got %T", block.MACCommands[i].Payload)
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
		chans, err := config.C.NetworkServer.Band.Band.GetEnabledChannelsForLinkADRReqPayloads(ds.EnabledChannels, linkADRPayloads)
		if err != nil {
			return nil, errors.Wrap(err, "get enalbed channels for link_adr_req payloads error")
		}

		ds.TXPowerIndex = int(adrReq.TXPower)
		ds.DR = int(adrReq.DataRate)
		ds.NbTrans = adrReq.Redundancy.NbRep
		ds.EnabledChannels = chans

		log.WithFields(log.Fields{
			"dev_eui":          ds.DevEUI,
			"tx_power_idx":     ds.TXPowerIndex,
			"dr":               adrReq.DataRate,
			"nb_trans":         adrReq.Redundancy.NbRep,
			"enabled_channels": chans,
		}).Info("link_adr request acknowledged")

	} else {
		// TODO: remove workaround once all RN2483 nodes have the issue below
		// fixed.
		//
		// This is a workaround for the RN2483 firmware (1.0.3) which sends
		// a nACK on TXPower 0 (this is incorrect behaviour, following the
		// specs). It should ACK and operate at its maximum possible power
		// when TXPower 0 is not supported. See also section 5.2 in the
		// LoRaWAN specs.
		if !powerACK && adrReq.TXPower == 0 {
			ds.TXPowerIndex = 1
		}

		// It is possible that the node does not support all TXPower
		// indices. In this case we set the MaxSupportedTXPowerIndex
		// to the request - 1. If that index is not supported, it will
		// be lowered by 1 at the next nACK.
		if !powerACK && adrReq.TXPower > 0 {
			ds.MaxSupportedTXPowerIndex = int(adrReq.TXPower) - 1
		}

		// It is possible that the node does not support all data-rates.
		// In this case we set the MaxSupportedDR to the requested - 1.
		// If that DR is not supported, it will be lowered by 1 at the
		// next nACK.
		if !dataRateACK && adrReq.DataRate > 0 {
			ds.MaxSupportedDR = int(adrReq.DataRate) - 1
		}

		log.WithFields(log.Fields{
			"dev_eui":          ds.DevEUI,
			"channel_mask_ack": channelMaskACK,
			"data_rate_ack":    dataRateACK,
			"power_ack":        powerACK,
		}).Warning("link_adr request not acknowledged")
	}

	return nil, nil
}

func handleLinkCheckReq(ds *storage.DeviceSession, rxPacket models.RXPacket) ([]storage.MACCommandBlock, error) {
	if len(rxPacket.RXInfoSet) == 0 {
		return nil, errors.New("rx info-set contains zero items")
	}

	requiredSNR, ok := config.SpreadFactorToRequiredSNRTable[rxPacket.TXInfo.DataRate.SpreadFactor]
	if !ok {
		return nil, fmt.Errorf("sf %d not in sf to required snr table", rxPacket.TXInfo.DataRate.SpreadFactor)
	}

	margin := rxPacket.RXInfoSet[0].LoRaSNR - requiredSNR
	if margin < 0 {
		margin = 0
	}

	block := storage.MACCommandBlock{
		CID: lorawan.LinkCheckAns,
		MACCommands: storage.MACCommands{
			{
				CID: lorawan.LinkCheckAns,
				Payload: &lorawan.LinkCheckAnsPayload{
					Margin: uint8(margin),
					GwCnt:  uint8(len(rxPacket.RXInfoSet)),
				},
			},
		},
	}

	return []storage.MACCommandBlock{block}, nil
}
