package adr

import (
	"github.com/brocaar/chirpstack-network-server/v3/adr"
	"github.com/brocaar/chirpstack-network-server/v3/internal/band"
	loraband "github.com/brocaar/lorawan/band"
)

// DefaultHandler implements the default ADR handler.
type DefaultHandler struct{}

// ID returns the default ID.
func (h *DefaultHandler) ID() (string, error) {
	return "default", nil
}

// Name returns the default name.
func (h *DefaultHandler) Name() (string, error) {
	return "Default ADR algorithm (LoRa only)", nil
}

// Handle handles the ADR request.
func (h *DefaultHandler) Handle(req adr.HandleRequest) (adr.HandleResponse, error) {
	// This defines the default response, which is equal to the current device
	// state.
	resp := adr.HandleResponse{
		DR:           req.DR,
		TxPowerIndex: req.TxPowerIndex,
		NbTrans:      req.NbTrans,
	}

	// If ADR is disabled, return with current values.
	if !req.ADR {
		return resp, nil
	}

	// The max DR might be configured to a non LoRa (125kHz) data-rate.
	// As this algorithm works on LoRa (125kHz) data-rates only, we need to
	// find the max LoRa (125 kHz) data-rate.
	maxDR := req.MaxDR
	maxLoRaDR := 0
	enabledDRs := band.Band().GetEnabledUplinkDataRates()
	for _, i := range enabledDRs {
		dr, err := band.Band().GetDataRate(i)
		if err != nil {
			return resp, err
		}

		if dr.Modulation == loraband.LoRaModulation && dr.Bandwidth == 125 {
			maxLoRaDR = i
		}
	}

	// Reduce to max LoRa DR.
	if maxDR > maxLoRaDR {
		maxDR = maxLoRaDR
	}

	// Lower the DR only if it exceeds the max. allowed DR.
	if req.DR > maxDR {
		resp.DR = maxDR
	}

	// Set the new NbTrans.
	resp.NbTrans = h.getNbTrans(req.NbTrans, h.getPacketLossPercentage(req))

	// Calculate the number of 'steps'.
	snrM := h.getMaxSNR(req)
	snrMargin := snrM - req.RequiredSNRForDR - req.InstallationMargin
	nStep := int(snrMargin / 3)

	// In case of negative steps the ADR algorithm will increase the TxPower
	// if possible. To avoid up / down / up / down TxPower changes, wait until
	// we have at least the required number of uplink history elements.
	if nStep < 0 && h.getHistoryCount(req) != h.requiredHistoryCount() {
		return resp, nil
	}

	resp.TxPowerIndex, resp.DR = h.getIdealTxPowerIndexAndDR(nStep, resp.TxPowerIndex, resp.DR, req.MaxTxPowerIndex, maxDR)

	return resp, nil
}

func (h *DefaultHandler) pktLossRateTable() [][3]int {
	return [][3]int{
		{1, 1, 2},
		{1, 2, 3},
		{2, 3, 3},
		{3, 3, 3},
	}
}

func (h *DefaultHandler) getMaxSNR(req adr.HandleRequest) float32 {
	var snrM float32 = -999
	for _, m := range req.UplinkHistory {
		if m.MaxSNR > snrM {
			snrM = m.MaxSNR
		}
	}
	return snrM
}

// getHistoryCount returns the history count with equal TxPowerIndex.
func (h *DefaultHandler) getHistoryCount(req adr.HandleRequest) int {
	var count int
	for _, uh := range req.UplinkHistory {
		if req.TxPowerIndex == uh.TXPowerIndex {
			count++
		}
	}
	return count
}

func (h *DefaultHandler) requiredHistoryCount() int {
	return 20
}

func (h *DefaultHandler) getIdealTxPowerIndexAndDR(nStep, txPowerIndex, dr, maxTxPowerIndex, maxDR int) (int, int) {
	if nStep == 0 {
		return txPowerIndex, dr
	}

	if nStep > 0 {
		if dr < maxDR {
			// Increase the DR.
			dr++
		} else if txPowerIndex < maxTxPowerIndex {
			// Decrease the TxPower.
			txPowerIndex++
		}
		nStep--
	} else {
		if txPowerIndex > 0 {
			// Increase TxPower.
			txPowerIndex--
		}
		nStep++
	}

	return h.getIdealTxPowerIndexAndDR(nStep, txPowerIndex, dr, maxTxPowerIndex, maxDR)
}

func (h *DefaultHandler) getNbTrans(currentNbTrans int, pktLossRate float32) int {
	if currentNbTrans < 1 {
		currentNbTrans = 1
	}

	if currentNbTrans > 3 {
		currentNbTrans = 3
	}

	if pktLossRate < 5 {
		return h.pktLossRateTable()[0][currentNbTrans-1]
	} else if pktLossRate < 10 {
		return h.pktLossRateTable()[1][currentNbTrans-1]
	} else if pktLossRate < 30 {
		return h.pktLossRateTable()[2][currentNbTrans-1]
	}

	return h.pktLossRateTable()[3][currentNbTrans-1]
}

func (h *DefaultHandler) getPacketLossPercentage(req adr.HandleRequest) float32 {
	if len(req.UplinkHistory) < h.requiredHistoryCount() {
		return 0
	}

	var lostPackets uint32
	var previousFCnt uint32

	for i, m := range req.UplinkHistory {
		if i == 0 {
			previousFCnt = m.FCnt
			continue
		}

		lostPackets += m.FCnt - previousFCnt - 1 // there is always an expected difference of 1
		previousFCnt = m.FCnt
	}

	return float32(lostPackets) / float32(len(req.UplinkHistory)) * 100
}
