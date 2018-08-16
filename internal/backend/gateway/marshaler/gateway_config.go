package marshaler

import (
	"encoding/json"

	"github.com/brocaar/loraserver/api/common"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/lorawan/band"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// MarshalGatewayConfiguration marshals the GatewayConfiguration.
func MarshalGatewayConfiguration(t Type, gc gw.GatewayConfiguration) ([]byte, error) {
	var b []byte
	var err error

	switch t {
	case Protobuf:
		b, err = proto.Marshal(&gc)
	case JSON:
		var str string
		m := &jsonpb.Marshaler{
			EmitDefaults: true,
		}
		str, err = m.MarshalToString(&gc)
		b = []byte(str)
	case V2JSON:
		configPacket := gw.GatewayConfigPacket{
			Version: gc.Version,
		}
		copy(configPacket.MAC[:], gc.GatewayId)

		for _, c := range gc.Channels {
			switch c.Modulation {
			case common.Modulation_LORA:
				modConfig := c.GetLoraModulationConfig()
				if modConfig == nil {
					return nil, errors.Wrap(err, "lora_modulation_config must not be nil")
				}

				var spreadingFactors []int
				for _, sf := range modConfig.SpreadingFactors {
					spreadingFactors = append(spreadingFactors, int(sf))
				}

				configPacket.Channels = append(configPacket.Channels, gw.Channel{
					Modulation:       band.LoRaModulation,
					Frequency:        int(c.Frequency),
					Bandwidth:        int(modConfig.Bandwidth),
					SpreadingFactors: spreadingFactors,
				})

			case common.Modulation_FSK:
				modConfig := c.GetFskModulationConfig()
				if modConfig == nil {
					return nil, errors.Wrap(err, "fsk_modulation_config must not be nil")
				}

				configPacket.Channels = append(configPacket.Channels, gw.Channel{
					Modulation: band.FSKModulation,
					Frequency:  int(c.Frequency),
					Bandwidth:  int(modConfig.Bandwidth),
					Bitrate:    int(modConfig.Bitrate),
				})
			}
		}

		b, err = json.Marshal(configPacket)
	}

	return b, err
}
