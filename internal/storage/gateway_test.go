package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/brocaar/lorawan"
)

func (ts *StorageTestSuite) TestGateway() {
	assert := require.New(ts.T())
	rp := RoutingProfile{
		ASID: "localhost:1234",
	}
	assert.NoError(CreateRoutingProfile(context.Background(), ts.Tx(), &rp))

	sp := ServiceProfile{
		GwsPrivate: true,
	}
	assert.NoError(CreateServiceProfile(context.Background(), ts.Tx(), &sp))

	ts.T().Run("Create", func(t *testing.T) {
		assert := require.New(t)

		fpgaID := lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}
		aesKey := lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8}

		gw := Gateway{
			GatewayID:        lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
			RoutingProfileID: rp.ID,
			ServiceProfileID: &sp.ID,
			Location: GPSPoint{
				Latitude:  1.123,
				Longitude: 2.123,
			},
			Boards: []GatewayBoard{
				{
					FPGAID: &fpgaID,
				},
				{
					FineTimestampKey: &aesKey,
				},
			},
			TLSCert: []byte{1, 2, 3},
		}
		assert.NoError(CreateGateway(context.Background(), ts.Tx(), &gw))

		gw.CreatedAt = gw.CreatedAt.Round(time.Millisecond).UTC()
		gw.UpdatedAt = gw.UpdatedAt.Round(time.Millisecond).UTC()

		t.Run("Get", func(t *testing.T) {
			assert := require.New(t)

			gwGet, err := GetGateway(context.Background(), ts.Tx(), gw.GatewayID)
			assert.NoError(err)

			gwGet.CreatedAt = gwGet.CreatedAt.Round(time.Millisecond).UTC()
			gwGet.UpdatedAt = gwGet.UpdatedAt.Round(time.Millisecond).UTC()

			assert.Equal(gw, gwGet)
		})

		t.Run("GetMeta", func(t *testing.T) {
			assert := require.New(t)

			gwGet, err := GetGatewayMeta(context.Background(), ts.Tx(), gw.GatewayID)
			assert.NoError(err)
			assert.Equal(GatewayMeta{
				GatewayID:        gw.GatewayID,
				Location:         gw.Location,
				Altitude:         gw.Altitude,
				RoutingProfileID: rp.ID,
				ServiceProfileID: &sp.ID,
				IsPrivate:        true,
				Boards: []GatewayBoard{
					{
						FPGAID: &fpgaID,
					},
					{
						FineTimestampKey: &aesKey,
					},
				},
			}, gwGet)
		})

		t.Run("Test cache", func(t *testing.T) {
			gwGet, err := GetAndCacheGatewayMeta(context.Background(), ts.Tx(), gw.GatewayID)
			assert.NoError(err)
			assert.Equal(gw.GatewayID, gwGet.GatewayID)

			gwGet, err = GetGatewayMetaCache(context.Background(), gw.GatewayID)
			assert.NoError(err)
			assert.Equal(gw.GatewayID, gwGet.GatewayID)

			assert.NoError(FlushGatewayMetaCache(context.Background(), gw.GatewayID))
			_, err = GetGatewayMetaCache(context.Background(), gw.GatewayID)
			assert.Equal(ErrDoesNotExist, err)
		})

		t.Run("Update", func(t *testing.T) {
			assert := require.New(t)
			now := time.Now().Round(time.Millisecond).UTC()

			gp := GatewayProfile{
				Channels: []int64{0, 1, 2},
			}
			assert.NoError(CreateGatewayProfile(context.Background(), ts.Tx(), &gp))

			gw.GatewayProfileID = &gp.ID
			gw.ServiceProfileID = nil
			gw.FirstSeenAt = &now
			gw.LastSeenAt = &now
			gw.Location = GPSPoint{
				Latitude:  2.123,
				Longitude: 3.123,
			}
			gw.Altitude = 100.5
			gw.Boards = []GatewayBoard{
				{
					FineTimestampKey: &aesKey,
				},
				{
					FPGAID: &fpgaID,
				},
			}
			gw.TLSCert = []byte{4, 5, 6}

			assert.NoError(UpdateGateway(context.Background(), ts.Tx(), &gw))
			gw.UpdatedAt = gw.UpdatedAt.Round(time.Millisecond).UTC()

			gwGet, err := GetGateway(context.Background(), ts.Tx(), gw.GatewayID)
			assert.NoError(err)

			gwGet.CreatedAt = gwGet.CreatedAt.Round(time.Millisecond).UTC()
			gwGet.UpdatedAt = gwGet.UpdatedAt.Round(time.Millisecond).UTC()

			assert.True(gwGet.FirstSeenAt.Round(time.Microsecond).Equal(now))
			assert.True(gwGet.LastSeenAt.Round(time.Microsecond).Equal(now))
			gwGet.FirstSeenAt = &now
			gwGet.LastSeenAt = &now

			assert.Equal(gw, gwGet)
		})

		t.Run("UpdateState", func(t *testing.T) {
			assert := require.New(t)

			assert.NoError(UpdateGatewayState(context.Background(), ts.Tx(), gw.GatewayID, 1.111, 2.222, 3.333))
			gwGet, err := GetGateway(context.Background(), ts.Tx(), gw.GatewayID)
			assert.NoError(err)

			assert.Equal(GPSPoint{
				Latitude:  1.111,
				Longitude: 2.222,
			}, gwGet.Location)
			assert.Equal(3.333, gwGet.Altitude)
		})

		t.Run("Delete", func(t *testing.T) {
			assert := require.New(t)
			assert.NoError(DeleteGateway(context.Background(), ts.Tx(), gw.GatewayID))
			_, err := GetGateway(context.Background(), ts.Tx(), gw.GatewayID)
			assert.Equal(ErrDoesNotExist, err)
		})
	})
}
