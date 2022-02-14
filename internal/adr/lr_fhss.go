package adr

import (
	"math/rand"
	"sort"
	"time"

	"github.com/brocaar/chirpstack-network-server/v3/adr"
	"github.com/brocaar/chirpstack-network-server/v3/internal/band"
	loraband "github.com/brocaar/lorawan/band"
)

// LRFHSSHandler implements a LR-FHSS only ADR handler.
type LRFHSSHandler struct{}

// ID returns the ID.
func (h *LRFHSSHandler) ID() (string, error) {
	return "lr_fhss", nil
}

// Name returns the name.
func (h *LRFHSSHandler) Name() (string, error) {
	return "LR-FHSS only ADR algorithm", nil
}

// Handle handles the ADR request.
func (h *LRFHSSHandler) Handle(req adr.HandleRequest) (adr.HandleResponse, error) {
	resp := adr.HandleResponse{
		DR:           req.DR,
		TxPowerIndex: req.TxPowerIndex,
		NbTrans:      req.NbTrans,
	}

	if !req.ADR {
		return resp, nil
	}

	// Get the enabled uplink data-rates to find out which (if any) LR-FHSS data-rates
	// are enabled.
	band := band.Band()

	// Get current DR info.
	dr, err := band.GetDataRate(req.DR)
	if err != nil {
		return resp, err
	}

	// If we are already at the highest LR-FHSS data-rate, there is nothing to do.
	// Note that we only differentiate between coding-rate. The OCW doesn't change
	// the speed.
	if dr.Modulation == loraband.LRFHSSModulation && dr.CodingRate == "4/6" {
		return resp, nil
	}

	// Get median RSSI.
	medRSSI := getMedian(req.UplinkHistory)

	// If the median RSSI is below -130, coding-rate 1/3 is recommended,
	// if we are on this coding-rate already, there is nothing to do.
	if medRSSI < -130 && dr.Modulation == loraband.LRFHSSModulation && dr.CodingRate == "1/3" {
		return resp, nil
	}

	// Find out which LR-FHSS data-rates are enabled (note that not all
	// LR-FHSS data-rates might be configured in the channel-plan).
	enabledDRs := band.GetEnabledUplinkDataRates()
	lrFHSSDataRates := make(map[int]loraband.DataRate)
	for _, i := range enabledDRs {
		dr, err := band.GetDataRate(i)
		if err != nil {
			return resp, err
		}

		// Only get the LR-FHSS data-rates that <= MaxDR
		if i <= req.MaxDR && dr.Modulation == loraband.LRFHSSModulation {
			lrFHSSDataRates[i] = dr
		}
	}

	// There are no LR-FHSS data-rates enabled, so there is nothing to adjust.
	if len(lrFHSSDataRates) == 0 {
		return resp, nil
	}

	// Now we decide which DRs we can use.
	drs := make([]int, 0)

	// Select LR-FHSS data-rate with coding-rate 4/6 (if any available).
	// Note: that for RSSI (median) < -130, coding-rate 1/3 is recommended.
	// As the median is taken from the uplink history, make sure that we
	// take the median from a full history table.
	if medRSSI >= -130 && len(req.UplinkHistory) == 20 {
		for k, v := range lrFHSSDataRates {
			if v.CodingRate == "4/6" {
				drs = append(drs, k)
			}
		}

	}

	// This either means coding-rate 1/3 must be used, or no data-rate with
	// coding-rate 3/6 is enabled, and thus 1/3 is the only option.
	if len(drs) == 0 {
		for k, v := range lrFHSSDataRates {
			if v.CodingRate == "1/3" {
				drs = append(drs, k)
			}
		}
	}

	// Sanity check
	if len(drs) == 0 {
		return resp, nil
	}

	// Randomly select one of the available LR-FHSS data-rates.
	// In case there are multiple with the same coding-rate, we take
	// a random one.
	s := rand.NewSource(time.Now().Unix())
	r := rand.New(s) // initialize local pseudorandom generator
	resp.DR = drs[r.Intn(len(drs))]
	resp.NbTrans = 1      // 1 is the recommended value
	resp.TxPowerIndex = 0 // for now this ADR algorithm only controls the DR

	return resp, nil
}

func getMedian(upMetaData []adr.UplinkMetaData) int {
	// This should never occur.
	if len(upMetaData) == 0 {
		return 0
	}

	rssi := make([]int, 0, len(upMetaData))
	for _, up := range upMetaData {
		rssi = append(rssi, int(up.MaxRSSI))
	}
	sort.Ints(rssi)
	m := len(rssi) / 2

	// Odd
	if len(rssi)%2 != 0 {
		return rssi[m]
	}

	return (rssi[m-1] + rssi[m]) / 2
}
