package models

import (
	"time"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/lorawan"
)

// maxSNRForSort defines the maximum SNR on which to sort. When both values
// are exceeding this values, the sorting will continue on RSSI.
const maxSNRForSort = 5.0

// RXPacket contains a received PHYPayload together with its RX metadata.
type RXPacket struct {
	DR         int
	PHYPayload lorawan.PHYPayload
	TXInfo     *gw.UplinkTXInfo
	RXInfoSet  []*gw.UplinkRXInfo
}

// BySignalStrength implements sort.Interface for []gw.UplinkRXInfo
// based on signal strength.
type BySignalStrength []*gw.UplinkRXInfo

func (s BySignalStrength) Len() int {
	return len(s)
}

func (s BySignalStrength) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s BySignalStrength) Less(i, j int) bool {
	// in case SNR is equal
	if s[i].LoraSnr == s[j].LoraSnr {
		return s[i].Rssi > s[j].Rssi
	}

	// in case the SNR > maxSNRForSort
	if s[i].LoraSnr > maxSNRForSort && s[j].LoraSnr > maxSNRForSort {
		return s[i].Rssi > s[j].Rssi
	}

	return s[i].LoraSnr > s[j].LoraSnr
}

// RXInfoSet implements a sortable slice of RXInfo elements.
// First it is sorted by LoRaSNR, within the sub-set where
// LoRaSNR > maxSNRForSort, it will sort by RSSI.
type RXInfoSet []RXInfo

// RXInfo defines the RX related metadata (for each receiving gateway).
type RXInfo struct {
	MAC               lorawan.EUI64
	Time              *time.Time
	TimeSinceGPSEpoch *gw.Duration
	Timestamp         uint32
	RSSI              int
	LoRaSNR           float64
	Board             int
	Antenna           int
	RFChain           int
	Channel           int
}
