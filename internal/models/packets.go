package models

import (
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/lorawan"
)

// maxSNRForSort defines the maximum SNR on which to sort. When both values
// are exceeding this values, the sorting will continue on RSSI.
const maxSNRForSort = 5.0

// RXPacket defines a received PHYPayload together with its rx metadata
// (rx information from all the receiving gateways).
type RXPacket struct {
	PHYPayload lorawan.PHYPayload
	RXInfoSet  RXInfoSet
}

// RXInfoSet implements a sortable slice of RXInfo elements.
// First it is sorted by LoRaSNR, within the sub-set where
// LoRaSNR > maxSNRForSort, it will sort by RSSI.
type RXInfoSet []gw.RXInfo

// Len implements sort.Interface.
func (s RXInfoSet) Len() int {
	return len(s)
}

// Swap implements sort.Interface.
func (s RXInfoSet) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Less implements sort.Interface.
func (s RXInfoSet) Less(i, j int) bool {
	// in case SNR is equal
	if s[i].LoRaSNR == s[j].LoRaSNR {
		return s[i].RSSI > s[j].RSSI
	}

	// in case the SNR > maxSNRForSort
	if s[i].LoRaSNR > maxSNRForSort && s[j].LoRaSNR > maxSNRForSort {
		return s[i].RSSI > s[j].RSSI
	}

	return s[i].LoRaSNR > s[j].LoRaSNR
}
