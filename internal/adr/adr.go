package adr

import (
	"fmt"

	log "github.com/Sirupsen/logrus"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/queue"
	"github.com/brocaar/loraserver/internal/session"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
)

var pktLossRateTable = [][3]uint8{
	{1, 1, 2},
	{1, 2, 3},
	{2, 3, 3},
	{3, 3, 3},
}

// TODO: move this to lorawan/band package in the future?
var requiredSNRTable = []float64{
	-20,
	-17.5,
	-15,
	-12.5,
	-10,
	-7.5,
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
	if !macPL.FHDR.FCtrl.ADR || fullFCnt%ns.ADRInterval > 0 {
		return nil
	}

	if common.BandName != band.EU_863_870 {
		log.WithFields(log.Fields{
			"dev_eui": ns.DevEUI,
		}).Info("ADR support is only available for EU_863_870 band currently")
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

	if currentDR > 5 {
		log.WithFields(log.Fields{
			"dr":      currentDR,
			"dev_eui": ns.DevEUI,
		}).Info("ADR is only supported up to DR5")
		return nil
	}

	snrMargin := snrM - requiredSNRTable[currentDR] - ns.InstallationMargin
	nStep := int(snrMargin / 3)

	currentTXPower := getCurrentTXPower(ns)
	currentTXPowerIndex := getTXPowerIndex(currentTXPower)
	idealTXPower, idealDR := getIdealTXPowerAndDR(nStep, currentTXPower, currentDR)
	idealTXPowerIndex := getTXPowerIndex(idealTXPower)
	idealNbRep := getNbRep(ns.NbTrans, ns.GetPacketLossPercentage())

	// there is nothing to adjust
	if currentTXPowerIndex == idealTXPowerIndex && currentDR == idealDR {
		return nil
	}

	var chMask lorawan.ChMask
	for i := 0; i < len(common.Band.DownlinkChannels); i++ {
		chMask[i] = true
	}

	for i := 0; ns.CFList != nil && i < len(ns.CFList); i++ {
		if ns.CFList[i] == 0 {
			continue
		}
		chMask[i+len(common.Band.DownlinkChannels)] = true
	}

	mac := lorawan.MACCommand{
		CID: lorawan.LinkADRReq,
		Payload: &lorawan.LinkADRReqPayload{
			DataRate: uint8(idealDR),
			TXPower:  uint8(idealTXPowerIndex),
			ChMask:   chMask,
			Redundancy: lorawan.Redundancy{
				ChMaskCntl: 0, // first block of 16 channels
				NbRep:      uint8(idealNbRep),
			},
		},
	}
	b, err := mac.MarshalBinary()
	if err != nil {
		return fmt.Errorf("marshal mac command error: %s", err)
	}

	err = queue.AddMACPayloadToTXQueue(ctx.RedisPool, queue.MACPayload{
		DevEUI: ns.DevEUI,
		Data:   b,
	})
	if err != nil {
		return fmt.Errorf("add mac-payload to tx-queue error: %s", err)
	}

	ns.TXPower = idealTXPower
	ns.NbTrans = idealNbRep

	log.WithFields(log.Fields{
		"dev_eui":            ns.DevEUI,
		"current_dr":         currentDR,
		"current_tx_power":   currentTXPower,
		"requested_dr":       idealDR,
		"requested_tx_power": common.Band.TXPower[idealTXPowerIndex],
	}).Info("adr is requesting a data-rate or tx-power change")

	return nil
}

func getCurrentTXPower(ns *session.NodeSession) int {
	if ns.TXPower > 0 {
		return ns.TXPower
	}
	return common.Band.DefaultTXPower
}

func getMaxTXPower() int {
	return common.Band.TXPower[0]
}

func getMinTXPower() int {
	var minTX int
	for _, p := range common.Band.TXPower {
		// make sure we never use 0 and disable the device
		if p > 0 {
			minTX = p
		}
	}
	return minTX
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

func getIdealTXPowerAndDR(nStep int, txPower int, dr int) (int, int) {
	if nStep == 0 {
		return txPower, dr
	}

	if nStep > 0 {
		if dr < 5 {
			dr++
		} else {
			txPower -= 3
		}
		nStep--
		if txPower <= getMinTXPower() {
			return txPower, dr
		}
	} else {
		if txPower < getMaxTXPower() {
			txPower += 3
			nStep++
		} else {
			return txPower, dr
		}
	}

	return getIdealTXPowerAndDR(nStep, txPower, dr)
}
