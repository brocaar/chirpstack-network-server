package marshaler

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"

	"github.com/brocaar/loraserver/api/common"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/lorawan/band"
)

// UnmarshalUplinkFrame unmarshals an UplinkFrame.
func UnmarshalUplinkFrame(b []byte, uf *gw.UplinkFrame) (Type, error) {
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
		return t, proto.Unmarshal(b, uf)
	case JSON:
		m := jsonpb.Unmarshaler{}
		return t, m.Unmarshal(bytes.NewReader(b), uf)
	case V2JSON:
		var rxPacket gw.RXPacketBytes
		err := json.Unmarshal(b, &rxPacket)
		if err != nil {
			return t, err
		}

		uf.PhyPayload = rxPacket.PHYPayload
		uf.TxInfo = &gw.UplinkTXInfo{
			Frequency: uint32(rxPacket.RXInfo.Frequency),
		}

		switch rxPacket.RXInfo.DataRate.Modulation {
		case band.LoRaModulation:
			uf.TxInfo.Modulation = common.Modulation_LORA
			uf.TxInfo.ModulationInfo = &gw.UplinkTXInfo_LoraModulationInfo{
				LoraModulationInfo: &gw.LoRaModulationInfo{
					Bandwidth:       uint32(rxPacket.RXInfo.DataRate.Bandwidth),
					SpreadingFactor: uint32(rxPacket.RXInfo.DataRate.SpreadFactor),
					CodeRate:        rxPacket.RXInfo.CodeRate,
				},
			}
		case band.FSKModulation:
			uf.TxInfo.Modulation = common.Modulation_FSK
			uf.TxInfo.ModulationInfo = &gw.UplinkTXInfo_FskModulationInfo{
				FskModulationInfo: &gw.FSKModulationInfo{
					Bandwidth: uint32(rxPacket.RXInfo.DataRate.Bandwidth),
					Bitrate:   uint32(rxPacket.RXInfo.DataRate.BitRate),
				},
			}
		}

		uf.RxInfo = &gw.UplinkRXInfo{
			GatewayId: rxPacket.RXInfo.MAC[:],
			Timestamp: rxPacket.RXInfo.Timestamp,
			Rssi:      int32(rxPacket.RXInfo.RSSI),
			LoraSnr:   float64(rxPacket.RXInfo.LoRaSNR),
			Channel:   uint32(rxPacket.RXInfo.Channel),
			RfChain:   uint32(rxPacket.RXInfo.RFChain),
			Board:     uint32(rxPacket.RXInfo.Board),
			Antenna:   uint32(rxPacket.RXInfo.Antenna),
		}

		if rxPacket.RXInfo.Time != nil {
			ts, err := ptypes.TimestampProto(*rxPacket.RXInfo.Time)
			if err != nil {
				return t, err
			}
			uf.RxInfo.Time = ts
		}

		if rxPacket.RXInfo.TimeSinceGPSEpoch != nil {
			dr := ptypes.DurationProto(time.Duration(*rxPacket.RXInfo.TimeSinceGPSEpoch))
			uf.RxInfo.TimeSinceGpsEpoch = dr
		}
	}

	return t, nil
}
