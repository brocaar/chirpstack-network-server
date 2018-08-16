package helpers

import (
	"fmt"

	"github.com/brocaar/lorawan"

	"github.com/brocaar/loraserver/api/common"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/lorawan/band"
	"github.com/pkg/errors"
)

const defaultCodeRate = "4/5"

// GatewayIDGetter provides a GatewayId getter interface.
type GatewayIDGetter interface {
	GetGatewayId() []byte
}

// DataRateGetter provides an interface for getting the data-rate.
type DataRateGetter interface {
	GetModulation() common.Modulation
	GetLoraModulationInfo() *gw.LoRaModulationInfo
	GetFskModulationInfo() *gw.FSKModulationInfo
}

// SetDownlinkTXInfoDataRate sets the DownlinkTXInfo data-rate.
func SetDownlinkTXInfoDataRate(txInfo *gw.DownlinkTXInfo, dr int, b band.Band) error {
	dataRate, err := b.GetDataRate(dr)
	if err != nil {
		return errors.Wrap(err, "get data-rate error")
	}

	switch dataRate.Modulation {
	case band.LoRaModulation:
		txInfo.Modulation = common.Modulation_LORA
		txInfo.ModulationInfo = &gw.DownlinkTXInfo_LoraModulationInfo{
			LoraModulationInfo: &gw.LoRaModulationInfo{
				SpreadingFactor:       uint32(dataRate.SpreadFactor),
				Bandwidth:             uint32(dataRate.Bandwidth),
				CodeRate:              defaultCodeRate,
				PolarizationInversion: true,
			},
		}
	case band.FSKModulation:
		txInfo.Modulation = common.Modulation_FSK
		txInfo.ModulationInfo = &gw.DownlinkTXInfo_FskModulationInfo{
			FskModulationInfo: &gw.FSKModulationInfo{
				Bitrate:   uint32(dataRate.BitRate),
				Bandwidth: uint32(dataRate.Bandwidth),
			},
		}
	default:
		return fmt.Errorf("unknown modulation: %s", dataRate.Modulation)
	}

	return nil
}

// SetUplinkTXInfoDataRate sets the UplinkTXInfo data-rate.
func SetUplinkTXInfoDataRate(txInfo *gw.UplinkTXInfo, dr int, b band.Band) error {
	dataRate, err := b.GetDataRate(dr)
	if err != nil {
		return errors.Wrap(err, "get data-rate error")
	}

	switch dataRate.Modulation {
	case band.LoRaModulation:
		txInfo.Modulation = common.Modulation_LORA
		txInfo.ModulationInfo = &gw.UplinkTXInfo_LoraModulationInfo{
			LoraModulationInfo: &gw.LoRaModulationInfo{
				SpreadingFactor:       uint32(dataRate.SpreadFactor),
				Bandwidth:             uint32(dataRate.Bandwidth),
				CodeRate:              defaultCodeRate,
				PolarizationInversion: true,
			},
		}
	case band.FSKModulation:
		txInfo.Modulation = common.Modulation_FSK
		txInfo.ModulationInfo = &gw.UplinkTXInfo_FskModulationInfo{
			FskModulationInfo: &gw.FSKModulationInfo{
				Bitrate:   uint32(dataRate.BitRate),
				Bandwidth: uint32(dataRate.Bandwidth),
			},
		}
	default:
		return fmt.Errorf("unknown modulation: %s", dataRate.Modulation)
	}

	return nil
}

// GetGatewayID returns the typed gateway ID.
func GetGatewayID(v GatewayIDGetter) lorawan.EUI64 {
	var gatewayID lorawan.EUI64
	copy(gatewayID[:], v.GetGatewayId())
	return gatewayID
}

// GetDataRateIndex returns the data-rate index.
func GetDataRateIndex(uplink bool, v DataRateGetter, b band.Band) (int, error) {
	var dr band.DataRate

	switch v.GetModulation() {
	case common.Modulation_LORA:
		modInfo := v.GetLoraModulationInfo()
		if modInfo == nil {
			return 0, errors.New("lora_modulation_info must not be nil")
		}
		dr.Modulation = band.LoRaModulation
		dr.SpreadFactor = int(modInfo.SpreadingFactor)
		dr.Bandwidth = int(modInfo.Bandwidth)
	case common.Modulation_FSK:
		modInfo := v.GetFskModulationInfo()
		if modInfo == nil {
			return 0, errors.New("fsk_modulation_info must not be nil")
		}
		dr.Modulation = band.FSKModulation
		dr.Bandwidth = int(modInfo.Bandwidth)
		dr.BitRate = int(modInfo.Bitrate)
	default:
		return 0, fmt.Errorf("unknown modulation: %s", v.GetModulation())
	}

	return b.GetDataRateIndex(uplink, dr)
}
