package adr

import (
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
)

var pktLossRateTable = [][3]uint8{
	{1, 1, 2},
	{1, 2, 3},
	{2, 3, 3},
	{3, 3, 3},
}

// HandleADR handles ADR in case requested by the node and configured
// in the device-session.
func HandleADR(ds storage.DeviceSession, linkADRReqBlock *storage.MACCommandBlock) ([]storage.MACCommandBlock, error) {

	var maxSNR float64
	for i, rxInfo := range ds.LastRXInfoSet {
		// as the default value is 0 and the LoRaSNR can be negative, we always
		// set it when i == 0 (the first item from the slice)
		if i == 0 || rxInfo.LoRaSNR > maxSNR {
			maxSNR = rxInfo.LoRaSNR
		}
	}

	// if the node has ADR disabled
	if !ds.ADR || config.C.NetworkServer.NetworkSettings.DisableADR {
		if linkADRReqBlock == nil {
			return nil, nil
		}
		return []storage.MACCommandBlock{*linkADRReqBlock}, nil
	}

	// get the max SNR from the UplinkHistory
	snrM := maxSNR
	for _, uh := range ds.UplinkHistory {
		if uh.MaxSNR > snrM && uh.TXPowerIndex == ds.TXPowerIndex {
			snrM = uh.MaxSNR
		}
	}

	if ds.DR > getMaxAllowedDR() {
		log.WithFields(log.Fields{
			"dr":      ds.DR,
			"dev_eui": ds.DevEUI,
		}).Infof("ADR is only supported up to DR%d", getMaxAllowedDR())
		return nil, nil
	}

	dr, err := config.C.NetworkServer.Band.Band.GetDataRate(ds.DR)
	if err != nil {
		return nil, errors.Wrap(err, "get data-rate error")
	}

	requiredSNR, err := getRequiredSNRForSF(dr.SpreadFactor)
	if err != nil {
		return nil, err
	}

	snrMargin := snrM - requiredSNR - config.C.NetworkServer.NetworkSettings.InstallationMargin
	nStep := int(snrMargin / 3)

	maxSupportedDR := getMaxSupportedDRForNode(ds)
	maxSupportedTXPowerOffsetIndex := getMaxSupportedTXPowerOffsetIndexForNode(ds)

	idealTXPowerIndex, idealDR := getIdealTXPowerOffsetAndDR(nStep, ds.TXPowerIndex, ds.DR, ds.MinSupportedTXPowerIndex, maxSupportedTXPowerOffsetIndex, maxSupportedDR)
	idealNbRep := getNbRep(ds.NbTrans, ds.GetPacketLossPercentage())

	// there is nothing to adjust
	if ds.TXPowerIndex == idealTXPowerIndex && ds.DR == idealDR && ds.NbTrans == idealNbRep {
		return nil, nil
	}

	if linkADRReqBlock == nil || len(linkADRReqBlock.MACCommands) == 0 {
		// nothing is pending
		var chMask lorawan.ChMask
		chMaskCntl := -1
		for _, c := range ds.EnabledUplinkChannels {
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

		linkADRReqBlock = &storage.MACCommandBlock{
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
		lastMAC := linkADRReqBlock.MACCommands[len(linkADRReqBlock.MACCommands)-1]
		lastMACPl, ok := lastMAC.Payload.(*lorawan.LinkADRReqPayload)
		if !ok {
			return nil, fmt.Errorf("expected *lorawan.LinkADRReqPayload, got %T", lastMAC.Payload)
		}

		lastMACPl.DataRate = uint8(idealDR)
		lastMACPl.TXPower = uint8(idealTXPowerIndex)
		lastMACPl.Redundancy.NbRep = uint8(idealNbRep)
	}

	log.WithFields(log.Fields{
		"dev_eui":          ds.DevEUI,
		"dr":               ds.DR,
		"req_dr":           idealDR,
		"tx_power":         ds.TXPowerIndex,
		"req_tx_power_idx": idealTXPowerIndex,
		"nb_trans":         ds.NbTrans,
		"req_nb_trans":     idealNbRep,
	}).Info("adr request added to mac-command queue")

	return []storage.MACCommandBlock{*linkADRReqBlock}, nil
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
	for i := 0; ; i++ {
		offset, err := config.C.NetworkServer.Band.Band.GetTXPowerOffset(i)
		if err != nil {
			break
		}
		if offset != 0 {
			idx = i
		}
	}

	return idx
}

func getMaxSupportedTXPowerOffsetIndexForNode(ds storage.DeviceSession) int {
	if ds.MaxSupportedTXPowerIndex != 0 {
		return ds.MaxSupportedTXPowerIndex
	}
	return getMaxTXPowerOffsetIndex()
}

func getIdealTXPowerOffsetAndDR(nStep, txPowerOffsetIndex, dr, minSupportedTXPowerOffsetIndex, maxSupportedTXPowerOffsetIndex, maxSupportedDR int) (int, int) {
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
		if txPowerOffsetIndex > minSupportedTXPowerOffsetIndex {
			txPowerOffsetIndex--
			nStep++
		} else if txPowerOffsetIndex <= minSupportedTXPowerOffsetIndex {
			return minSupportedTXPowerOffsetIndex, dr
		}
	}

	return getIdealTXPowerOffsetAndDR(nStep, txPowerOffsetIndex, dr, minSupportedTXPowerOffsetIndex, maxSupportedTXPowerOffsetIndex, maxSupportedDR)
}

func getRequiredSNRForSF(sf int) (float64, error) {
	snr, ok := config.SpreadFactorToRequiredSNRTable[sf]
	if !ok {
		return 0, fmt.Errorf("sf to required snr for does not exsists (sf: %d)", sf)
	}
	return snr, nil
}

func getMaxAllowedDR() int {
	var maxDR int
	stdChannels := config.C.NetworkServer.Band.Band.GetStandardUplinkChannelIndices()

	// we take the highest data-rate from the enabled standard uplink channels
	for _, i := range config.C.NetworkServer.Band.Band.GetEnabledUplinkChannelIndices() {
		for _, stdI := range stdChannels {
			if i == stdI {
				c, _ := config.C.NetworkServer.Band.Band.GetUplinkChannel(i)
				if c.MaxDR > maxDR {
					maxDR = c.MaxDR
				}
			}
		}
	}
	return maxDR
}

func getMaxSupportedDRForNode(ds storage.DeviceSession) int {
	if ds.MaxSupportedDR != 0 {
		return ds.MaxSupportedDR
	}
	return getMaxAllowedDR()
}
