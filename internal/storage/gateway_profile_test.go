package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func (ts *StorageTestSuite) TestGatewayProfile() {
	ts.T().Run("Create", func(t *testing.T) {
		assert := require.New(t)

		gp := GatewayProfile{
			Channels:      []int64{0, 1, 2},
			StatsInterval: time.Second * 30,
			ExtraChannels: []ExtraChannel{
				{
					Modulation:       ModulationLoRa,
					Frequency:        868700000,
					Bandwidth:        125,
					SpreadingFactors: []int64{10, 11, 12},
				},
				{
					Modulation: ModulationLoRa,
					Frequency:  868900000,
					Bandwidth:  125,
					Bitrate:    50000,
				},
			},
		}
		assert.NoError(CreateGatewayProfile(context.Background(), ts.Tx(), &gp))
		gp.CreatedAt = gp.CreatedAt.UTC().Truncate(time.Millisecond)
		gp.UpdatedAt = gp.UpdatedAt.UTC().Truncate(time.Millisecond)

		t.Run("Get", func(t *testing.T) {
			assert := require.New(t)

			gpGet, err := GetGatewayProfile(context.Background(), ts.Tx(), gp.ID)
			assert.NoError(err)

			gpGet.CreatedAt = gpGet.CreatedAt.UTC().Truncate(time.Millisecond)
			gpGet.UpdatedAt = gpGet.UpdatedAt.UTC().Truncate(time.Millisecond)
			assert.Equal(gp, gpGet)
		})

		t.Run("Update", func(t *testing.T) {
			assert := require.New(t)

			gp.Channels = []int64{0, 1}
			gp.StatsInterval = time.Minute * 30
			gp.ExtraChannels = []ExtraChannel{
				{
					Modulation: ModulationLoRa,
					Frequency:  868900000,
					Bandwidth:  125,
					Bitrate:    50000,
				},
				{
					Modulation:       ModulationLoRa,
					Frequency:        868700000,
					Bandwidth:        125,
					SpreadingFactors: []int64{10, 11, 12},
				},
			}

			assert.NoError(UpdateGatewayProfile(context.Background(), ts.Tx(), &gp))
			gp.UpdatedAt = gp.UpdatedAt.UTC().Truncate(time.Millisecond)

			gpGet, err := GetGatewayProfile(context.Background(), ts.Tx(), gp.ID)
			assert.NoError(err)

			gpGet.CreatedAt = gpGet.CreatedAt.UTC().Truncate(time.Millisecond)
			gpGet.UpdatedAt = gpGet.UpdatedAt.UTC().Truncate(time.Millisecond)
			assert.Equal(gp, gpGet)
		})

		t.Run("Delete", func(t *testing.T) {
			assert := require.New(t)

			assert.NoError(DeleteGatewayProfile(context.Background(), ts.Tx(), gp.ID))
			_, err := GetGatewayProfile(context.Background(), ts.Tx(), gp.ID)
			assert.Equal(ErrDoesNotExist, err)
		})
	})
}
