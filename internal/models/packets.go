package models

import (
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
