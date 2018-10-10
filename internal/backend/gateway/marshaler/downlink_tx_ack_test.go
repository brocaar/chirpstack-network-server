package marshaler

import (
	"encoding/json"
	"testing"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/lorawan"
)

func TestUnmarshalDownlinkTXAck(t *testing.T) {
	t.Run("V2 JSON", func(t *testing.T) {
		assert := require.New(t)

		in := gw.TXAck{
			MAC:   lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
			Token: 12345,
			Error: "Boom!",
		}
		b, err := json.Marshal(in)
		assert.NoError(err)

		var out gw.DownlinkTXAck
		typ, err := UnmarshalDownlinkTXAck(b, &out)
		assert.NoError(err)
		assert.Equal(V2JSON, typ)
		assert.Equal(gw.DownlinkTXAck{
			GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
			Token:     12345,
			Error:     "Boom!",
		}, out)
	})

	t.Run("JSON", func(t *testing.T) {
		assert := require.New(t)

		in := gw.DownlinkTXAck{
			GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
			Token:     12345,
			Error:     "Boom!",
		}
		m := jsonpb.Marshaler{}
		str, err := m.MarshalToString(&in)
		assert.NoError(err)

		var out gw.DownlinkTXAck
		typ, err := UnmarshalDownlinkTXAck([]byte(str), &out)
		assert.NoError(err)
		assert.Equal(JSON, typ)
		assert.True(proto.Equal(&in, &out))
	})

	t.Run("Protobuf", func(t *testing.T) {
		assert := require.New(t)

		in := gw.DownlinkTXAck{
			GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
			Token:     12345,
			Error:     "Boom!",
		}
		b, err := proto.Marshal(&in)
		assert.NoError(err)

		var out gw.DownlinkTXAck
		typ, err := UnmarshalDownlinkTXAck(b, &out)
		assert.NoError(err)
		assert.Equal(Protobuf, typ)
		assert.True(proto.Equal(&in, &out))
	})
}
