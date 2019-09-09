package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/lorawan"
)

func (ts *StorageTestSuite) TestDownlinkFrames() {
	downlinkFrames := []gw.DownlinkFrame{
		{
			Token:      10,
			PhyPayload: []byte{1, 2, 3, 4},
		},
		{
			Token:      10,
			PhyPayload: []byte{5, 6, 7, 8},
		},
	}

	devEUI := lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}
	ctx := context.Background()

	ts.T().Run("Save", func(t *testing.T) {
		assert := require.New(t)
		assert.NoError(SaveDownlinkFrames(ctx, ts.RedisPool(), devEUI, downlinkFrames))

		t.Run("Pop", func(t *testing.T) {
			assert := require.New(t)

			d, frame, err := PopDownlinkFrame(context.Background(), ts.RedisPool(), 10)
			assert.NoError(err)
			assert.Equal(downlinkFrames[0], frame)
			assert.Equal(devEUI, d)

			d, frame, err = PopDownlinkFrame(context.Background(), ts.RedisPool(), 10)
			assert.NoError(err)
			assert.Equal(downlinkFrames[1], frame)
			assert.Equal(devEUI, d)

			_, _, err = PopDownlinkFrame(context.Background(), ts.RedisPool(), 10)
			assert.Equal(ErrDoesNotExist, err)
		})
	})
}
