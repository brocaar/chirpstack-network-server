package marshaler

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"

	"github.com/brocaar/loraserver/api/gw"
)

// UnmarshalDownlinkTXAck unmarshals a DownlinkTXAck.
func UnmarshalDownlinkTXAck(b []byte, ack *gw.DownlinkTXAck) (Type, error) {
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
		return t, proto.Unmarshal(b, ack)
	case JSON:
		m := jsonpb.Unmarshaler{
			AllowUnknownFields: true,
		}
		return t, m.Unmarshal(bytes.NewReader(b), ack)
	case V2JSON:
		var txACK gw.TXAck
		err := json.Unmarshal(b, &txACK)
		if err != nil {
			return t, err
		}

		ack.GatewayId = txACK.MAC[:]
		ack.Token = uint32(txACK.Token)
		ack.Error = txACK.Error
	}

	return t, nil
}
