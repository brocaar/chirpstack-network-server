package helpers

import (
	"context"
	"fmt"

	"github.com/brocaar/chirpstack-network-server/internal/backend/applicationserver"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/gofrs/uuid"

	"github.com/brocaar/chirpstack-api/go/v3/as"
	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/lorawan/band"
	"github.com/pkg/errors"
)

const defaultCodeRate = "4/5"

// GatewayIDGetter provides a GatewayId getter interface.
type GatewayIDGetter interface {
	GetGatewayId() []byte
}

// DownlinkIDGetter provides a DownlinkId getter interface.
type DownlinkIDGetter interface {
	GetDownlinkId() []byte
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
				Datarate:           uint32(dataRate.BitRate),
				FrequencyDeviation: uint32(dataRate.BitRate / 2), // see: https://github.com/brocaar/chirpstack-gateway-bridge/issues/16
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
				Datarate: uint32(dataRate.BitRate),
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

// GetUplinkID returns the typed message ID.
func GetUplinkID(v *gw.UplinkRXInfo) uuid.UUID {
	var msgID uuid.UUID
	if v != nil {
		copy(msgID[:], v.GetUplinkId())
	}
	return msgID
}

// GetStatsID returns the typed stats ID.
func GetStatsID(v *gw.GatewayStats) uuid.UUID {
	var statsID uuid.UUID
	if v != nil {
		copy(statsID[:], v.GetStatsId())
	}
	return statsID
}

// GetDownlinkID returns the types downlink ID.
func GetDownlinkID(v DownlinkIDGetter) uuid.UUID {
	var downlinkID uuid.UUID
	if b := v.GetDownlinkId(); b != nil {
		copy(downlinkID[:], b)
	}
	return downlinkID
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
		dr.BitRate = int(modInfo.Datarate)
	default:
		return 0, fmt.Errorf("unknown modulation: %s", v.GetModulation())
	}

	return b.GetDataRateIndex(uplink, dr)
}

// GetASClientForRoutingProfileID returns the AS client given a Routing Profile ID.
func GetASClientForRoutingProfileID(ctx context.Context, id uuid.UUID) (as.ApplicationServerServiceClient, error) {
	rp, err := storage.GetRoutingProfile(ctx, storage.DB(), id)
	if err != nil {
		return nil, errors.Wrap(err, "get routing-profile error")
	}

	asClient, err := applicationserver.Pool().Get(rp.ASID, []byte(rp.CACert), []byte(rp.TLSCert), []byte(rp.TLSKey))
	if err != nil {
		return nil, errors.Wrap(err, "get application-server client error")
	}

	return asClient, nil
}
