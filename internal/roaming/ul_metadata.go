package roaming

import (
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/pkg/errors"

	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-network-server/v3/internal/band"
	"github.com/brocaar/chirpstack-network-server/v3/internal/helpers"
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

		if gwInfo.FineRecvTime != nil {
			ts := time.Time(ulMetaData.RecvTime)
			ts = ts.Round(time.Second)
			ts = ts.Add(time.Duration(*gwInfo.FineRecvTime) * time.Nanosecond)
			tsProto, err := ptypes.TimestampProto(ts)
			if err != nil {
				return nil, errors.Wrap(err, "timestamp proto error")
			}

			rxInfo.FineTimestampType = gw.FineTimestampType_PLAIN
			rxInfo.FineTimestamp = &gw.UplinkRXInfo_PlainFineTimestamp{
				PlainFineTimestamp: &gw.PlainFineTimestamp{
					Time: tsProto,
				},
			}
		}

		out = append(out, &rxInfo)
	}

	return out, nil
}

func RecvTimeFromRXInfo(rxInfo []*gw.UplinkRXInfo) backend.ISO8601Time {
	for _, r := range rxInfo {
		if r.Time != nil {
			t, err := ptypes.Timestamp(r.Time)
			if err != nil {
				continue
			}

			return backend.ISO8601Time(t.UTC())
		}
	}

	return backend.ISO8601Time(time.Now().UTC())
}

func RXInfoToGWInfo(rxInfo []*gw.UplinkRXInfo) ([]backend.GWInfoElement, error) {
	var out []backend.GWInfoElement
	for i := range rxInfo {
		rssi := int(rxInfo[i].Rssi)
		var lat, lon *float64
		var fineRecvTime *int

		if loc := rxInfo[i].Location; loc != nil {
			lat = &loc.Latitude
			lon = &loc.Longitude
		}

		if fineTS := rxInfo[i].GetPlainFineTimestamp(); fineTS != nil {
			nanos := int(fineTS.GetTime().GetNanos())
			fineRecvTime = &nanos
		}

		b, err := proto.Marshal(rxInfo[i])
		if err != nil {
			return nil, errors.Wrap(err, "marshal rxinfo error")
		}

		id := backend.HEXBytes(rxInfo[i].GatewayId)
		if len(id) == 8 {
			id = id[4:]
		}

		e := backend.GWInfoElement{
			ID:           id,
			FineRecvTime: fineRecvTime,
			RSSI:         &rssi,
			SNR:          &rxInfo[i].LoraSnr,
			Lat:          lat,
			Lon:          lon,
			ULToken:      backend.HEXBytes(b),
			DLAllowed:    true,
		}

		out = append(out, e)
	}

	return out, nil
}
