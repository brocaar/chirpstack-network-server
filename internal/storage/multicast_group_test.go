package storage

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/brocaar/lorawan"
)

func (ts *StorageTestSuite) GetMulticastGroup() MulticastGroup {
	assert := require.New(ts.T())

	var sp ServiceProfile
	var rp RoutingProfile

	assert.NoError(CreateServiceProfile(context.Background(), ts.DB(), &sp))
	assert.NoError(CreateRoutingProfile(context.Background(), ts.DB(), &rp))

	return MulticastGroup{
		MCAddr:           lorawan.DevAddr{1, 2, 3, 4},
		MCNwkSKey:        lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
		FCnt:             10,
		GroupType:        MulticastGroupB,
		DR:               5,
		Frequency:        868300000,
		PingSlotPeriod:   16,
		ServiceProfileID: sp.ID,
		RoutingProfileID: rp.ID,
	}
}

func (ts *StorageTestSuite) TestMulticastGroup() {
	ts.T().Run("Create", func(t *testing.T) {
		assert := require.New(t)

		mc := ts.GetMulticastGroup()
		err := CreateMulticastGroup(context.Background(), ts.Tx(), &mc)
		assert.Nil(err)

		mc.CreatedAt = mc.CreatedAt.Round(time.Second).UTC()
		mc.UpdatedAt = mc.UpdatedAt.Round(time.Second).UTC()

		t.Run("Get", func(t *testing.T) {
			assert := require.New(t)
			mcGet, err := GetMulticastGroup(context.Background(), ts.Tx(), mc.ID, false)
			assert.Nil(err)

			mcGet.CreatedAt = mcGet.CreatedAt.Round(time.Second).UTC()
			mcGet.UpdatedAt = mcGet.UpdatedAt.Round(time.Second).UTC()

			assert.Equal(mc, mcGet)
		})

		t.Run("Update", func(t *testing.T) {
			assert := require.New(t)

			mc.MCAddr = lorawan.DevAddr{4, 3, 2, 1}
			mc.MCNwkSKey = lorawan.AES128Key{8, 7, 6, 5, 4, 3, 2, 1, 8, 7, 6, 5, 4, 3, 2, 1}
			mc.FCnt = 20
			mc.GroupType = MulticastGroupC
			mc.Frequency = 868100000
			mc.PingSlotPeriod = 32

			assert.Nil(UpdateMulticastGroup(context.Background(), ts.Tx(), &mc))

			mc.UpdatedAt = mc.UpdatedAt.Round(time.Second).UTC()

			mcGet, err := GetMulticastGroup(context.Background(), ts.Tx(), mc.ID, false)
			assert.Nil(err)

			mcGet.CreatedAt = mcGet.CreatedAt.Round(time.Second).UTC()
			mcGet.UpdatedAt = mcGet.UpdatedAt.Round(time.Second).UTC()

			assert.Equal(mc, mcGet)
		})

		t.Run("Delete", func(t *testing.T) {
			assert := require.New(t)

			assert.Nil(DeleteMulticastGroup(context.Background(), ts.Tx(), mc.ID))
			assert.Equal(ErrDoesNotExist, DeleteMulticastGroup(context.Background(), ts.Tx(), mc.ID))

			_, err := GetMulticastGroup(context.Background(), ts.Tx(), mc.ID, false)
			assert.Equal(ErrDoesNotExist, err)
		})
	})
}

func (ts *StorageTestSuite) TestMulticastQueue() {
	assert := require.New(ts.T())

	mg := ts.GetMulticastGroup()
	assert.NoError(CreateMulticastGroup(context.Background(), ts.Tx(), &mg))

	rp := RoutingProfile{
		ASID: "localhost:1234",
	}
	assert.NoError(CreateRoutingProfile(context.Background(), ts.Tx(), &rp))

	gw := Gateway{
		GatewayID:        lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
		RoutingProfileID: rp.ID,
	}
	assert.NoError(CreateGateway(context.Background(), ts.Tx(), &gw))

	ts.T().Run("Create", func(t *testing.T) {
		assert := require.New(t)

		gps1 := 100 * time.Second
		gps2 := 110 * time.Second

		qi1 := MulticastQueueItem{
			MulticastGroupID:        mg.ID,
			GatewayID:               lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
			FCnt:                    10,
			FPort:                   20,
			FRMPayload:              []byte{1, 2, 3, 4},
			EmitAtTimeSinceGPSEpoch: &gps1,
		}

		qi2 := MulticastQueueItem{
			MulticastGroupID:        mg.ID,
			GatewayID:               lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
			FCnt:                    11,
			FPort:                   20,
			FRMPayload:              []byte{1, 2, 3, 4},
			EmitAtTimeSinceGPSEpoch: &gps2,
		}

		assert.NoError(CreateMulticastQueueItem(context.Background(), ts.Tx(), &qi1))
		assert.NoError(CreateMulticastQueueItem(context.Background(), ts.Tx(), &qi2))

		t.Run("List", func(t *testing.T) {
			assert := require.New(t)

			items, err := GetMulticastQueueItemsForMulticastGroup(context.Background(), ts.Tx(), mg.ID)
			assert.NoError(err)
			assert.Len(items, 2)

			assert.EqualValues(items[0].FCnt, 10)
			assert.EqualValues(items[1].FCnt, 11)
		})

		t.Run("Schedulable multicast queue-items", func(t *testing.T) {
			assert := require.New(t)

			items, err := GetSchedulableMulticastQueueItems(context.Background(), ts.Tx(), 1)
			assert.NoError(err)
			assert.Len(items, 1)

			assert.Equal(qi1.FCnt, items[0].FCnt)
		})

		t.Run("Max emit at", func(t *testing.T) {
			assert := require.New(t)

			d, err := GetMaxEmitAtTimeSinceGPSEpochForMulticastGroup(context.Background(), ts.Tx(), mg.ID)
			assert.NoError(err)
			assert.Equal(gps2, d)
		})

		t.Run("Update", func(t *testing.T) {
			assert := require.New(t)

			now := time.Now().Truncate(time.Millisecond)
			qi1.RetryAfter = &now
			assert.NoError(UpdateMulticastQueueItem(context.Background(), ts.Tx(), &qi1))
			qi1.CreatedAt = qi1.CreatedAt.UTC().Truncate(time.Millisecond)
			qi1.UpdatedAt = qi1.UpdatedAt.UTC().Truncate(time.Millisecond)

			qi, err := GetMulticastQueueItem(context.Background(), ts.Tx(), qi1.ID)
			assert.NoError(err)

			qi.CreatedAt = qi.CreatedAt.UTC().Truncate(time.Millisecond)
			qi.UpdatedAt = qi.UpdatedAt.UTC().Truncate(time.Millisecond)
			assert.True(qi.RetryAfter.Equal(now))
			qi.RetryAfter = &now

			assert.Equal(qi1, qi)
		})

		t.Run("Delete", func(t *testing.T) {
			assert := require.New(t)

			assert.NoError(DeleteMulticastQueueItem(context.Background(), ts.Tx(), qi1.ID))
			items, err := GetMulticastQueueItemsForMulticastGroup(context.Background(), ts.Tx(), mg.ID)
			assert.NoError(err)
			assert.Len(items, 1)
		})

		t.Run("Flush", func(t *testing.T) {
			assert := require.New(t)

			assert.NoError(FlushMulticastQueueForMulticastGroup(context.Background(), ts.Tx(), mg.ID))
			items, err := GetMulticastQueueItemsForMulticastGroup(context.Background(), ts.Tx(), mg.ID)
			assert.NoError(err)
			assert.Len(items, 0)
		})
	})
}

func (ts *StorageTestSuite) TestGetMulticastGroupsWithQueueItems() {
	assert := require.New(ts.T())

	mg1 := ts.GetMulticastGroup()
	mg1.GroupType = MulticastGroupC
	assert.NoError(CreateMulticastGroup(context.Background(), ts.DB(), &mg1))
	mg2 := ts.GetMulticastGroup()
	mg2.GroupType = MulticastGroupC
	assert.NoError(CreateMulticastGroup(context.Background(), ts.DB(), &mg2))

	rp := RoutingProfile{
		ASID: "localhost:1234",
	}
	assert.NoError(CreateRoutingProfile(context.Background(), ts.DB(), &rp))

	gw := Gateway{
		GatewayID:        lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 2},
		RoutingProfileID: rp.ID,
	}
	assert.NoError(CreateGateway(context.Background(), ts.DB(), &gw))

	qi1 := MulticastQueueItem{
		ScheduleAt:       time.Now(),
		MulticastGroupID: mg1.ID,
		GatewayID:        gw.GatewayID,
		FCnt:             10,
		FPort:            20,
		FRMPayload:       []byte{1, 2, 3, 4},
	}
	assert.NoError(CreateMulticastQueueItem(context.Background(), ts.DB(), &qi1))

	qi2 := MulticastQueueItem{
		ScheduleAt:       time.Now().Add(time.Second),
		MulticastGroupID: mg2.ID,
		GatewayID:        gw.GatewayID,
		FCnt:             10,
		FPort:            20,
		FRMPayload:       []byte{1, 2, 3, 4},
	}
	assert.NoError(CreateMulticastQueueItem(context.Background(), ts.DB(), &qi2))

	Transaction(func(tx sqlx.Ext) error {
		items, err := GetSchedulableMulticastQueueItems(context.Background(), tx, 10)
		assert.NoError(err)
		assert.Len(items, 1)
		assert.Equal(mg1.ID, items[0].MulticastGroupID)

		// new transaction must return 0 items as the first one did lock
		// the multicast-group
		Transaction(func(tx sqlx.Ext) error {
			items, err := GetSchedulableMulticastQueueItems(context.Background(), tx, 10)
			assert.NoError(err)
			assert.Len(items, 0)
			return nil
		})

		return nil
	})
}
