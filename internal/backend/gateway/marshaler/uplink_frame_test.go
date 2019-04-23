package marshaler

import (
	"testing"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"

	"github.com/brocaar/loraserver/api/gw"
)

func TestUnmarshalUplinkFrame(t *testing.T) {
	t.Run("JSON", func(t *testing.T) {
		assert := require.New(t)

		in := gw.UplinkFrame{
			TxInfo: &gw.UplinkTXInfo{
				Frequency: 868100000,
			},
			RxInfo: &gw.UplinkRXInfo{
				GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
			},
		}
		m := jsonpb.Marshaler{}
		str, err := m.MarshalToString(&in)
		assert.NoError(err)

		var out gw.UplinkFrame
		typ, err := UnmarshalUplinkFrame([]byte(str), &out)
		assert.NoError(err)
		assert.Equal(JSON, typ)
		assert.True(proto.Equal(&in, &out))
	})

	t.Run("Protobuf", func(t *testing.T) {
		assert := require.New(t)

		in := gw.UplinkFrame{
			TxInfo: &gw.UplinkTXInfo{
				Frequency: 868100000,
			},
			RxInfo: &gw.UplinkRXInfo{
				GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
			},
		}
		b, err := proto.Marshal(&in)
		assert.NoError(err)

		var out gw.UplinkFrame
		typ, err := UnmarshalUplinkFrame(b, &out)
		assert.NoError(err)
		assert.Equal(Protobuf, typ)
		assert.True(proto.Equal(&in, &out))
	})
}
