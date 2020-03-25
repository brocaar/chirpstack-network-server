package storage

import (
	"context"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
)

func (ts *StorageTestSuite) TestDownlinkFrames() {

	df := DownlinkFrames{
		DevEui: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
		Token:  1234,
	}

	ts.T().Run("Does not exist", func(t *testing.T) {
		assert := require.New(t)

		_, err := GetDownlinkFrames(context.Background(), 1234)
		assert.Equal(ErrDoesNotExist, err)
	})

	ts.T().Run("Save", func(t *testing.T) {
		assert := require.New(t)
		assert.NoError(SaveDownlinkFrames(context.Background(), df))

		t.Run("Get", func(t *testing.T) {
			assert := require.New(t)

			dfGet, err := GetDownlinkFrames(context.Background(), 1234)
			assert.NoError(err)
			assert.True(proto.Equal(&df, &dfGet))
		})
	})
}
