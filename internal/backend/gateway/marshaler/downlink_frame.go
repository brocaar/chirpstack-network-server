package marshaler

import (
	"encoding/json"

	"github.com/brocaar/lorawan/band"

	"github.com/brocaar/loraserver/api/common"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/pkg/errors"
)

// MarshalDownlinkFrame marshals the given DownlinkFrame.
func MarshalDownlinkFrame(t Type, df gw.DownlinkFrame) ([]byte, error) {
	var b []byte
	var err error

	switch t {
	case Protobuf:
		b, err = proto.Marshal(&df)
	case JSON:
		var str string
		m := &jsonpb.Marshaler{
			EmitDefaults: true,
		}
		str, err = m.MarshalToString(&df)
		b = []byte(str)
	case V2JSON:
		txPacket := gw.TXPacketBytes{
			Token: uint16(df.Token),
			TXInfo: gw.TXInfo{
				Immediately: df.TxInfo.Immediately,
				Frequency:   int(df.TxInfo.Frequency),
				Power:       int(df.TxInfo.Power),
				Board:       int(df.TxInfo.Board),
				Antenna:     int(df.TxInfo.Antenna),
			},
			PHYPayload: df.PhyPayload,
		}

		copy(txPacket.TXInfo.MAC[:], df.TxInfo.GatewayId)

		if df.TxInfo.TimeSinceGpsEpoch != nil {
			if dur, err := ptypes.Duration(df.TxInfo.TimeSinceGpsEpoch); err == nil {
				gwDur := gw.Duration(dur)
				txPacket.TXInfo.TimeSinceGPSEpoch = &gwDur
			}
		}

		if df.TxInfo.TimeSinceGpsEpoch == nil && !df.TxInfo.Immediately {
			txPacket.TXInfo.Timestamp = &df.TxInfo.Timestamp
		}

		switch df.TxInfo.Modulation {
		case common.Modulation_LORA:
			modInfo := df.TxInfo.GetLoraModulationInfo()
			if modInfo == nil {
				return nil, errors.Wrap(err, "lora_modulation_info must not be nil")
			}

			txPacket.TXInfo.DataRate = band.DataRate{
				Modulation:   band.LoRaModulation,
				SpreadFactor: int(modInfo.SpreadingFactor),
				Bandwidth:    int(modInfo.Bandwidth),
			}
			txPacket.TXInfo.CodeRate = modInfo.CodeRate
			txPacket.TXInfo.IPol = &modInfo.PolarizationInversion
		case common.Modulation_FSK:
			modInfo := df.TxInfo.GetFskModulationInfo()
			if modInfo == nil {
				return nil, errors.Wrap(err, "fsk_modulation_info must not be nil")
			}

			txPacket.TXInfo.DataRate = band.DataRate{
				Modulation: band.FSKModulation,
				BitRate:    int(modInfo.Bitrate),
				Bandwidth:  int(modInfo.Bandwidth),
			}
		}

		b, err = json.Marshal(txPacket)
	}

	return b, err
}
