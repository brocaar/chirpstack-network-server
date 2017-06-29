package adr

import (
	"fmt"

	log "github.com/Sirupsen/logrus"
	"github.com/pkg/errors"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/maccommand"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/session"
	"github.com/brocaar/lorawan"
)

var pktLossRateTable = [][3]uint8{
	{1, 1, 2},
	{1, 2, 3},
	{2, 3, 3},
	{3, 3, 3},
}

// HandleADR handles ADR in case requested by the node and configured
// in the node-session.
// Note that this function only implements ADR for the EU_863_870 band as
// ADR for other bands is still work in progress. When implementing ADR for
// other bands, maybe it is an idea to move ADR related constants to the
// lorawan/band package.
func HandleADR(ctx common.Context, ns *session.NodeSession, rxPacket models.RXPacket, fullFCnt uint32) error {
	var maxSNR float64
	for i, rxInfo := range rxPacket.RXInfoSet {
		// as the default value is 0 and the LoRaSNR can be negative, we always
		// set it when i == 0 (the first item from the slice)
		if i == 0 || rxInfo.LoRaSNR > maxSNR {
			maxSNR = rxInfo.LoRaSNR
		}
	}

	// append metadata to the UplinkHistory slice.
	ns.AppendUplinkHistory(session.UplinkHistory{
		FCnt:         fullFCnt,
		GatewayCount: len(rxPacket.RXInfoSet),
		MaxSNR:       maxSNR,
	})

	macPL, ok := rxPacket.PHYPayload.MACPayload.(*lorawan.MACPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.MACPayload, got: %T", rxPacket.PHYPayload.MACPayload)
	}

	// if the node has ADR disabled or the uplink framecounter does not meet
	// the configured ADR interval, there is nothing to do :-)
	if !macPL.FHDR.FCtrl.ADR || fullFCnt == 0 || ns.ADRInterval == 0 || fullFCnt%ns.ADRInterval > 0 {
		return nil
	}

	// get the max SNR from the UplinkHistory
	var snrM float64
	for i, uh := range ns.UplinkHistory {
		if i == 0 || uh.MaxSNR > snrM {
			snrM = uh.MaxSNR
		}
	}

	currentDR, err := common.Band.GetDataRate(rxPacket.RXInfoSet[0].DataRate)
	if err != nil {
		return fmt.Errorf("get data-rate error: %s", err)
	}

	if currentDR > getMaxAllowedDR() {
		log.WithFields(log.Fields{
			"dr":      currentDR,
			"dev_eui": ns.DevEUI,
		}).Infof("ADR is only supported up to DR%d", getMaxAllowedDR())
		return nil
	}

	requiredSNR, err := getRequiredSNRForSF(rxPacket.RXInfoSet[0].DataRate.SpreadFactor)
	if err != nil {
		return err
	}

	snrMargin := snrM - requiredSNR - ns.InstallationMargin
	nStep := int(snrMargin / 3)

	currentTXPowerOffset := getCurrentTXPowerOffset(ns)
	idealTXPowerOffset, idealDR := getIdealTXPowerOffsetAndDR(nStep, currentTXPowerOffset, currentDR)
	idealTXPowerIndex := getTXPowerIndexForOffset(idealTXPowerOffset)
	idealNbRep := getNbRep(ns.NbTrans, ns.GetPacketLossPercentage())

	// TODO: remove workaround once all nodes are implementing the
	// LoRaWAN Regional Parameters 1.0.2 spec.
	//
	// Never decrease the txpower by multiple steps.
	// This is a workaround to handle the changes introduced by the
	// LoRaWAN Regional Parameters 1.0.2 specification. This update
	// introduces new txpower indices for some regions. As we don't
	// know which version of these parameters are implemented by the
	// node, we can't make multiple steps as txpower n might be
	// supported, but txpower n+1 not.
	if idealTXPowerIndex > ns.TXPowerIndex {
		idealTXPowerIndex = ns.TXPowerIndex + 1
	}

	// there is nothing to adjust
	if ns.TXPowerIndex == idealTXPowerIndex && currentDR == idealDR {
		return nil
	}

	var block *maccommand.Block

	// see if there is already a LinkADRReq commands in the queue
	block, err = maccommand.GetQueueItemByCID(ctx.RedisPool, ns.DevEUI, lorawan.LinkADRReq)
	if err != nil {
		return errors.Wrap(err, "read pending error")
	}

	if block == nil || len(block.MACCommands) == 0 {
		// nothing is pending
		var chMask lorawan.ChMask
		chMaskCntl := -1
		for _, c := range ns.EnabledChannels {
			if chMaskCntl != c/16 {
				if chMaskCntl == -1 {
					// set the chMaskCntl
					chMaskCntl = c / 16
				} else {
					// break the loop as we only need to send one block of channels
					break
				}
			}
			chMask[c%16] = true
		}

		block = &maccommand.Block{
			CID: lorawan.LinkADRReq,
			MACCommands: []lorawan.MACCommand{
				{
					CID: lorawan.LinkADRReq,
					Payload: &lorawan.LinkADRReqPayload{
						DataRate: uint8(idealDR),
						TXPower:  uint8(idealTXPowerIndex),
						ChMask:   chMask,
						Redundancy: lorawan.Redundancy{
							ChMaskCntl: uint8(chMaskCntl),
							NbRep:      uint8(idealNbRep),
						},
					},
				},
			},
		}
	} else {
		// there is a pending block of commands in the queue, add the adr parameters
		// to the last mac-command (as in case there are multiple commands
		// the node will use the dr, tx power and nb-rep from the last command
		lastMAC := block.MACCommands[len(block.MACCommands)-1]
		lastMACPl, ok := lastMAC.Payload.(*lorawan.LinkADRReqPayload)
		if !ok {
			return fmt.Errorf("expected *lorawan.LinkADRReqPayload, got %T", lastMAC.Payload)
		}

		lastMACPl.DataRate = uint8(idealDR)
		lastMACPl.TXPower = uint8(idealTXPowerIndex)
		lastMACPl.Redundancy.NbRep = uint8(idealNbRep)
	}

	err = maccommand.AddQueueItem(ctx.RedisPool, ns.DevEUI, *block)
	if err != nil {
		return errors.Wrap(err, "add mac-command block to queue error")
	}

	log.WithFields(log.Fields{
		"dev_eui":          ns.DevEUI,
		"dr":               currentDR,
		"req_dr":           idealDR,
		"tx_power":         ns.TXPowerIndex,
		"req_tx_power_idx": idealTXPowerIndex,
		"nb_trans":         ns.NbTrans,
		"req_nb_trans":     idealNbRep,
	}).Info("adr request added to mac-command queue")

	return nil
}

func getNbRep(currentNbRep uint8, pktLossRate float64) uint8 {
	if currentNbRep < 1 {
		currentNbRep = 1
	}
	if currentNbRep > 3 {
		currentNbRep = 3
	}

	if pktLossRate < 5 {
		return pktLossRateTable[0][currentNbRep-1]
	} else if pktLossRate < 10 {
		return pktLossRateTable[1][currentNbRep-1]
	} else if pktLossRate < 30 {
		return pktLossRateTable[2][currentNbRep-1]
	}
	return pktLossRateTable[3][currentNbRep-1]
}

func getCurrentTXPowerOffset(ns *session.NodeSession) int {
	if ns.TXPowerIndex > len(common.Band.TXPowerOffset)+1 {
		return common.Band.TXPowerOffset[0]
	}
	return common.Band.TXPowerOffset[ns.TXPowerIndex]
}

func getMaxTXPowerOffset() int {
	var delta int
	for _, p := range common.Band.TXPowerOffset {
		if p < 0 {
			delta = p
		}
	}
	return delta
}

func getTXPowerIndexForOffset(txPowerOffset int) int {
	var idx int
	for i, d := range common.Band.TXPowerOffset {
		if txPowerOffset <= d {
			idx = i
		}
	}
	return idx
}

func getIdealTXPowerOffsetAndDR(nStep int, txPowerOffset int, dr int) (int, int) {
	if nStep == 0 {
		return txPowerOffset, dr
	}

	if nStep > 0 {
		if dr < getMaxAllowedDR() {
			dr++
		} else {
			txPowerOffset -= 3
		}
		nStep--
		if txPowerOffset <= getMaxTXPowerOffset() {
			return txPowerOffset, dr
		}
	} else {
		if txPowerOffset < 0 {
			txPowerOffset += 3
			nStep++
		} else {
			return txPowerOffset, dr
		}
	}

	return getIdealTXPowerOffsetAndDR(nStep, txPowerOffset, dr)
}

func getRequiredSNRForSF(sf int) (float64, error) {
	snr, ok := common.SpreadFactorToRequiredSNRTable[sf]
	if !ok {
		return 0, fmt.Errorf("sf to required snr for does not exsists (sf: %d)", sf)
	}
	return snr, nil
}

func getMaxAllowedDR() int {
	var maxDR int
	for _, c := range common.Band.GetEnabledUplinkChannels() {
		channel := common.Band.UplinkChannels[c]
		if len(channel.DataRates) > 1 && channel.DataRates[len(channel.DataRates)-1] > maxDR {
			maxDR = channel.DataRates[len(channel.DataRates)-1]
		}
	}
	return maxDR
}
