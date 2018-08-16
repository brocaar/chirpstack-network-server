package marshaler

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"

	"github.com/brocaar/loraserver/api/common"
	"github.com/brocaar/loraserver/api/gw"
)

// UnmarshalGatewayStats unmarshals an GatewayStats.
func UnmarshalGatewayStats(b []byte, stats *gw.GatewayStats) (Type, error) {
	var t Type

	if strings.Contains(string(b), `"mac"`) {
		t = V2JSON
	} else if strings.Contains(string(b), `"gatewayID"`) {
		t = JSON
	} else {
		t = Protobuf
	}

	switch t {
	case Protobuf:
		return t, proto.Unmarshal(b, stats)
	case JSON:
		m := jsonpb.Unmarshaler{}
		return t, m.Unmarshal(bytes.NewReader(b), stats)
	case V2JSON:
		var statsPacket gw.GatewayStatsPacket
		err := json.Unmarshal(b, &statsPacket)
		if err != nil {
			return t, err
		}

		stats.GatewayId = statsPacket.MAC[:]
		stats.Time, err = ptypes.TimestampProto(statsPacket.Time)
		if err != nil {
			return t, err
		}

		if statsPacket.Latitude != nil && statsPacket.Longitude != nil && statsPacket.Altitude != nil {
			stats.Location = &common.Location{
				Latitude:  *statsPacket.Latitude,
				Longitude: *statsPacket.Longitude,
				Altitude:  *statsPacket.Altitude,
				Source:    common.LocationSource_GPS,
			}
		}

		stats.ConfigVersion = statsPacket.ConfigVersion
		stats.RxPacketsReceived = uint32(statsPacket.RXPacketsReceived)
		stats.RxPacketsReceivedOk = uint32(statsPacket.RXPacketsReceivedOK)
		stats.TxPacketsEmitted = uint32(statsPacket.TXPacketsEmitted)
		stats.TxPacketsReceived = uint32(statsPacket.TXPacketsReceived)
	}

	return t, nil
}
