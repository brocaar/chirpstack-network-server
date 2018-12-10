package storage

import (
	"testing"
	"time"

	"github.com/brocaar/loraserver/internal/helpers"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/stretchr/testify/require"
)

func (ts *StorageTestSuite) TestGatewayState() {
	ts.T().Run("Save", func(t *testing.T) {
		assert := require.New(t)
		now := time.Now()

		state := GatewayState{
			GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
		}
		state.LastSeenAt, _ = ptypes.TimestampProto(now)

		assert.NoError(SaveGatewayState(ts.RedisPool(), state))

		t.Run("Get", func(t *testing.T) {
			assert := require.New(t)

			getState, err := GetGatewayState(ts.RedisPool(), helpers.GetGatewayID(&state))
			assert.NoError(err)
			if !proto.Equal(&state, &getState) {
				assert.Equal(state, getState)
			}
		})

		t.Run("Delete", func(t *testing.T) {
			assert := require.New(t)

			assert.NoError(DeleteGatewayState(ts.RedisPool(), helpers.GetGatewayID(&state)))
			assert.Equal(ErrDoesNotExist, DeleteGatewayState(ts.RedisPool(), helpers.GetGatewayID(&state)))
			_, err := GetGatewayState(ts.RedisPool(), helpers.GetGatewayID(&state))
			assert.Equal(ErrDoesNotExist, err)
		})
	})
}
