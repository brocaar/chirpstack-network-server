package storage

import (
	"context"
	"testing"

	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/require"

	"github.com/brocaar/lorawan"
)

func (ts *StorageTestSuite) TestDeviceMulticastGroup() {
	assert := require.New(ts.T())

	var sp ServiceProfile
	var rp RoutingProfile

	assert.NoError(CreateServiceProfile(context.Background(), ts.Tx(), &sp))
	assert.NoError(CreateRoutingProfile(context.Background(), ts.Tx(), &rp))

	mg := MulticastGroup{
		GroupType:        MulticastGroupB,
		ServiceProfileID: sp.ID,
		RoutingProfileID: rp.ID,
	}
	assert.Nil(CreateMulticastGroup(context.Background(), ts.Tx(), &mg))

	dp := DeviceProfile{}

	assert.Nil(CreateDeviceProfile(context.Background(), ts.Tx(), &dp))

	d := Device{
		DevEUI:           lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
		ServiceProfileID: sp.ID,
		DeviceProfileID:  dp.ID,
		RoutingProfileID: rp.ID,
	}
	assert.Nil(CreateDevice(context.Background(), ts.Tx(), &d))

	ts.T().Run("Add", func(t *testing.T) {
		assert := require.New(t)

		assert.Nil(AddDeviceToMulticastGroup(context.Background(), ts.Tx(), d.DevEUI, mg.ID))

		t.Run("Get multicast-groups for DevEUI", func(t *testing.T) {
			assert := require.New(t)

			groups, err := GetMulticastGroupsForDevEUI(context.Background(), ts.Tx(), d.DevEUI)
			assert.Nil(err)
			assert.Len(groups, 1)
			assert.Equal([]uuid.UUID{mg.ID}, groups)
		})

		t.Run("Get DevEUIs for multicast-group", func(t *testing.T) {
			assert := require.New(t)

			devEUIs, err := GetDevEUIsForMulticastGroup(context.Background(), ts.Tx(), mg.ID)
			assert.NoError(err)
			assert.Len(devEUIs, 1)
			assert.Equal(d.DevEUI, devEUIs[0])
		})

		t.Run("Remove", func(t *testing.T) {
			assert := require.New(t)

			assert.Nil(RemoveDeviceFromMulticastGroup(context.Background(), ts.Tx(), d.DevEUI, mg.ID))
			groups, err := GetMulticastGroupsForDevEUI(context.Background(), ts.Tx(), d.DevEUI)
			assert.Nil(err)
			assert.Len(groups, 0)
		})
	})
}
