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
func HandleADR(ns *session.NodeSession, rxPacket models.RXPacket, fullFCnt uint32) error {
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

	currentDR, err := common.Band.GetDataRate(rxPacket.RXInfoSet[0].DataRate)
	if err != nil {
		return fmt.Errorf("get data-rate error: %s", err)
	}

	// The node changed its data-rate. Possibly the node did also reset its
	// tx-power to max power. Because of this, we need to reset the tx-power
	// at the network-server side too.
	if ns.DR != currentDR {
		ns.TXPowerIndex = 0
	}

	// keep track of the last-used data-rate
	ns.DR = currentDR

	// get the MACPayload
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

	maxSupportedDR := getMaxSupportedDRForNode(ns)
	maxSupportedTXPowerOffsetIndex := getMaxSupportedTXPowerOffsetIndexForNode(ns)

	idealTXPowerIndex, idealDR := getIdealTXPowerOffsetAndDR(nStep, ns.TXPowerIndex, currentDR, maxSupportedTXPowerOffsetIndex, maxSupportedDR)
	idealNbRep := getNbRep(ns.NbTrans, ns.GetPacketLossPercentage())

	// there is nothing to adjust
	if ns.TXPowerIndex == idealTXPowerIndex && currentDR == idealDR {
		return nil
	}

	var block *maccommand.Block

	// see if there is already a LinkADRReq commands in the queue
	block, err = maccommand.GetQueueItemByCID(common.RedisPool, ns.DevEUI, lorawan.LinkADRReq)
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

	err = maccommand.AddQueueItem(common.RedisPool, ns.DevEUI, *block)
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

func getMaxTXPowerOffsetIndex() int {
	var idx int
	for i, p := range common.Band.TXPowerOffset {
		if p < 0 {
			idx = i
		}
	}
	return idx
}

func getMaxSupportedTXPowerOffsetIndexForNode(ns *session.NodeSession) int {
	if ns.MaxSupportedTXPowerIndex != 0 {
		return ns.MaxSupportedTXPowerIndex
	}
	return getMaxTXPowerOffsetIndex()
}

func getIdealTXPowerOffsetAndDR(nStep, txPowerOffsetIndex, dr, maxSupportedTXPowerOffsetIndex, maxSupportedDR int) (int, int) {
	if nStep == 0 {
		return txPowerOffsetIndex, dr
	}

	if nStep > 0 {
		if dr < getMaxAllowedDR() && dr < maxSupportedDR {
			// maxSupportedDR is the max supported DR by the node. Depending the
			// Regional Parameters specification the node is implementing, this
			// might not be equal to the getMaxAllowedDR value.
			dr++

		} else if txPowerOffsetIndex < getMaxTXPowerOffsetIndex() && txPowerOffsetIndex < maxSupportedTXPowerOffsetIndex {
			// maxSupportedTXPowerOffsetIndex is the max supported TXPower
			// index by the node. Depending the Regional Parameters
			// specification the node is implementing, this might not be
			// equal to the getMaxTXPowerOffsetIndex value.
			txPowerOffsetIndex++

		}

		nStep--
		if txPowerOffsetIndex >= getMaxTXPowerOffsetIndex() {
			return getMaxTXPowerOffsetIndex(), dr
		}

	} else {
		if txPowerOffsetIndex > 0 {
			txPowerOffsetIndex--
			nStep++
		} else {
			return txPowerOffsetIndex, dr
		}
	}

	return getIdealTXPowerOffsetAndDR(nStep, txPowerOffsetIndex, dr, maxSupportedTXPowerOffsetIndex, maxSupportedDR)
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

func getMaxSupportedDRForNode(ns *session.NodeSession) int {
	if ns.MaxSupportedDR != 0 {
		return ns.MaxSupportedDR
	}
	return getMaxAllowedDR()
}
