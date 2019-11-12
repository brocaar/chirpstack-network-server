package marshaler

import (
	"github.com/brocaar/chirpstack-api/go/gw"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
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
	}

	return b, err
}
