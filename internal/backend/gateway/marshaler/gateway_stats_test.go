package marshaler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/stretchr/testify/require"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/lorawan"
)

func TestUnmarshalGatewayStats(t *testing.T) {
	t.Run("V2 JSON", func(t *testing.T) {
		assert := require.New(t)

		in := gw.GatewayStatsPacket{
			MAC:  lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
			Time: time.Now(),
		}
		b, err := json.Marshal(in)
		assert.NoError(err)

		nowPB, _ := ptypes.TimestampProto(in.Time)

		var out gw.GatewayStats
		typ, err := UnmarshalGatewayStats(b, &out)
		assert.NoError(err)
		assert.Equal(V2JSON, typ)
		assert.Equal(gw.GatewayStats{
			GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
			Time:      nowPB,
		}, out)
	})

	t.Run("JSON", func(t *testing.T) {
		assert := require.New(t)

		in := gw.GatewayStats{
			GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
		}
		m := jsonpb.Marshaler{}
		str, err := m.MarshalToString(&in)
		assert.NoError(err)

		var out gw.GatewayStats
		typ, err := UnmarshalGatewayStats([]byte(str), &out)
		assert.NoError(err)
		assert.Equal(JSON, typ)
		assert.True(proto.Equal(&in, &out))
	})

	t.Run("Protobuf", func(t *testing.T) {
		assert := require.New(t)

		in := gw.GatewayStats{
			GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
		}
		b, err := proto.Marshal(&in)
		assert.NoError(err)

		var out gw.GatewayStats
		typ, err := UnmarshalGatewayStats(b, &out)
		assert.NoError(err)
		assert.Equal(Protobuf, typ)
		assert.True(proto.Equal(&in, &out))
	})
}
