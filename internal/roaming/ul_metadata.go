package roaming

import (
	"github.com/pkg/errors"

	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-network-server/internal/band"
	"github.com/brocaar/chirpstack-network-server/internal/helpers"
	"github.com/brocaar/lorawan/backend"
)

func ULMetaDataToTXInfo(ulMetaData backend.ULMetaData) (*gw.UplinkTXInfo, error) {
	out := &gw.UplinkTXInfo{}

	// freq
	if ulMetaData.ULFreq != nil {
		out.Frequency = uint32(*ulMetaData.ULFreq * 1000000)
	}

	// data-rate
	if ulMetaData.DataRate != nil {
		if err := helpers.SetUplinkTXInfoDataRate(out, *ulMetaData.DataRate, band.Band()); err != nil {
			return nil, errors.Wrap(err, "set uplink txinfo data-rate error")
		}
	}

	return out, nil
}

func ULMetaDataToRXInfo(ulMetaData backend.ULMetaData) ([]*gw.UplinkRXInfo, error) {
	var out []*gw.UplinkRXInfo

	for i := range ulMetaData.GWInfo {
		gwInfo := ulMetaData.GWInfo[i]

		rxInfo := gw.UplinkRXInfo{
			GatewayId: gwInfo.ID[:],
			Context:   gwInfo.ULToken[:],
			CrcStatus: gw.CRCStatus_CRC_OK,
		}

		if gwInfo.RSSI != nil {
			rxInfo.Rssi = int32(*gwInfo.RSSI)
		}

		if gwInfo.SNR != nil {
			rxInfo.LoraSnr = *gwInfo.SNR
		}

		if gwInfo.Lat != nil && gwInfo.Lon != nil {
			rxInfo.Location = &common.Location{
				Latitude:  *gwInfo.Lat,
				Longitude: *gwInfo.Lon,
				Source:    common.LocationSource_UNKNOWN,
			}
		}

		out = append(out, &rxInfo)
	}

	return out, nil
}
