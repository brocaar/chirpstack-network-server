package marshaler

import (
	"bytes"
	"strings"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"

	"github.com/brocaar/chirpstack-api/go/v3/gw"
)

// UnmarshalGatewayStats unmarshals an GatewayStats.
func UnmarshalGatewayStats(b []byte, stats *gw.GatewayStats) (Type, error) {
	var t Type

	if strings.Contains(string(b), `"gatewayID"`) {
		t = JSON
	} else {
		t = Protobuf
	}

	switch t {
	case Protobuf:
		return t, proto.Unmarshal(b, stats)
	case JSON:
		m := jsonpb.Unmarshaler{
			AllowUnknownFields: true,
		}
		return t, m.Unmarshal(bytes.NewReader(b), stats)
	}

	return t, nil
}
