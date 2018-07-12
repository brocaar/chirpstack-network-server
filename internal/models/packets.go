package models

import (
	"time"

	"github.com/golang/protobuf/ptypes"

	"github.com/brocaar/lorawan/band"

	"github.com/brocaar/loraserver/api/common"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/lorawan"
)

// maxSNRForSort defines the maximum SNR on which to sort. When both values
// are exceeding this values, the sorting will continue on RSSI.
const maxSNRForSort = 5.0

// RXPacket contains a received PHYPayload together with its RX metadata.
type RXPacket struct {
	PHYPayload lorawan.PHYPayload
	TXInfo     TXInfo
	RXInfoSet  RXInfoSet
}

// GetGWUplinkTXInfo returns the gw.UplinkTXInfo struct.
// TODO: replace original TXInfo with this gw.UplinkTXInfo.
func (r RXPacket) GetGWUplinkTXInfo() *gw.UplinkTXInfo {
	txInfo := gw.UplinkTXInfo{
		Frequency: uint32(r.TXInfo.Frequency),
	}

	switch r.TXInfo.DataRate.Modulation {
	case band.LoRaModulation:
		txInfo.Modulation = common.Modulation_LORA
		txInfo.ModulationInfo = &gw.UplinkTXInfo_LoraModulationInfo{
			LoraModulationInfo: &gw.LoRaModulationInfo{
				Bandwidth:       uint32(r.TXInfo.DataRate.Bandwidth),
				SpreadingFactor: uint32(r.TXInfo.DataRate.SpreadFactor),
				CodeRate:        r.TXInfo.CodeRate,
			},
		}
	case band.FSKModulation:
		txInfo.Modulation = common.Modulation_FSK
		txInfo.ModulationInfo = &gw.UplinkTXInfo_FskModulationInfo{
			FskModulationInfo: &gw.FSKModulationInfo{
				Bandwidth: uint32(r.TXInfo.DataRate.Bandwidth),
				Bitrate:   uint32(r.TXInfo.DataRate.BitRate),
			},
		}
	}

	return &txInfo
}

// GetGWUplinkRXInfoSet returns the gw.UplinkTXInfo set.
// TODO: replace original RXInfo with gw.UplinkTXInfo.
func (r RXPacket) GetGWUplinkRXInfoSet() []*gw.UplinkRXInfo {
	var out []*gw.UplinkRXInfo

	for i := range r.RXInfoSet {
		rxInfo := gw.UplinkRXInfo{
			GatewayId: r.RXInfoSet[i].MAC[:],
			Rssi:      int32(r.RXInfoSet[i].RSSI),
			LoraSnr:   r.RXInfoSet[i].LoRaSNR,
			Board:     uint32(r.RXInfoSet[i].Board),
			Antenna:   uint32(r.RXInfoSet[i].Antenna),
			RfChain:   uint32(r.RXInfoSet[i].RFChain),
			Channel:   uint32(r.RXInfoSet[i].Channel),
		}

		if r.RXInfoSet[i].Time != nil {
			rxInfo.Time, _ = ptypes.TimestampProto(*r.RXInfoSet[i].Time)
		}

		if r.RXInfoSet[i].TimeSinceGPSEpoch != nil {
			rxInfo.TimeSinceGpsEpoch = ptypes.DurationProto(time.Duration(*r.RXInfoSet[i].TimeSinceGPSEpoch))
		}

		out = append(out, &rxInfo)
	}

	return out
}

// TXInfo defines the metadata used for the transmission.
type TXInfo struct {
	Frequency int
	DataRate  band.DataRate
	CodeRate  string
}

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

// RXInfoSet implements a sortable slice of RXInfo elements.
// First it is sorted by LoRaSNR, within the sub-set where
// LoRaSNR > maxSNRForSort, it will sort by RSSI.
type RXInfoSet []RXInfo

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
