package api

import (
	"context"
	"testing"
	"time"

	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/brocaar/loraserver/api/ns"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/downlink/data/classb"
	"github.com/brocaar/loraserver/internal/gps"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
)

type NetworkServerAPITestSuite struct {
	suite.Suite
	test.DatabaseTestSuiteBase

	api ns.NetworkServerServiceServer
}

func (ts *NetworkServerAPITestSuite) SetupSuite() {
	ts.DatabaseTestSuiteBase.SetupSuite()
	ts.api = NewNetworkServerAPI()
}

func (ts *NetworkServerAPITestSuite) TestMulticastGroup() {
	assert := require.New(ts.T())

	var rp storage.RoutingProfile
	var sp storage.ServiceProfile

	assert.NoError(storage.CreateRoutingProfile(ts.DB(), &rp))
	assert.NoError(storage.CreateServiceProfile(ts.DB(), &sp))

	mg := ns.MulticastGroup{
		McAddr:           []byte{1, 2, 3, 4},
		McNwkSKey:        []byte{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
		FCnt:             10,
		GroupType:        ns.MulticastGroupType_CLASS_B,
		Dr:               5,
		Frequency:        868300000,
		PingSlotPeriod:   16,
		RoutingProfileId: rp.ID[:],
		ServiceProfileId: sp.ID[:],
	}

	ts.T().Run("Create", func(t *testing.T) {
		assert := require.New(t)
		createResp, err := ts.api.CreateMulticastGroup(context.Background(), &ns.CreateMulticastGroupRequest{
			MulticastGroup: &mg,
		})
		assert.Nil(err)
		assert.Len(createResp.Id, 16)
		assert.NotEqual(uuid.Nil.Bytes(), createResp.Id)
		mg.Id = createResp.Id

		t.Run("Get", func(t *testing.T) {
			assert := require.New(t)
			getResp, err := ts.api.GetMulticastGroup(context.Background(), &ns.GetMulticastGroupRequest{
				Id: createResp.Id,
			})
			assert.Nil(err)
			assert.NotNil(getResp.MulticastGroup)
			assert.NotNil(getResp.CreatedAt)
			assert.NotNil(getResp.UpdatedAt)
			assert.Equal(&mg, getResp.MulticastGroup)
		})

		t.Run("Update", func(t *testing.T) {
			assert := require.New(t)

			mgUpdated := ns.MulticastGroup{
				Id:               createResp.Id,
				McAddr:           []byte{4, 3, 2, 1},
				McNwkSKey:        []byte{8, 7, 6, 5, 4, 3, 2, 1, 8, 7, 6, 5, 4, 3, 2, 1},
				FCnt:             20,
				GroupType:        ns.MulticastGroupType_CLASS_C,
				Dr:               3,
				Frequency:        868100000,
				PingSlotPeriod:   32,
				RoutingProfileId: rp.ID[:],
				ServiceProfileId: sp.ID[:],
			}

			_, err := ts.api.UpdateMulticastGroup(context.Background(), &ns.UpdateMulticastGroupRequest{
				MulticastGroup: &mgUpdated,
			})
			assert.Nil(err)

			getResp, err := ts.api.GetMulticastGroup(context.Background(), &ns.GetMulticastGroupRequest{
				Id: createResp.Id,
			})
			assert.Nil(err)
			assert.Equal(&mgUpdated, getResp.MulticastGroup)
		})

		t.Run("Delete", func(t *testing.T) {
			assert := require.New(t)

			_, err := ts.api.DeleteMulticastGroup(context.Background(), &ns.DeleteMulticastGroupRequest{
				Id: createResp.Id,
			})
			assert.Nil(err)

			_, err = ts.api.DeleteMulticastGroup(context.Background(), &ns.DeleteMulticastGroupRequest{
				Id: createResp.Id,
			})
			assert.NotNil(err)
			assert.Equal(codes.NotFound, grpc.Code(err))

			_, err = ts.api.GetMulticastGroup(context.Background(), &ns.GetMulticastGroupRequest{
				Id: createResp.Id,
			})
			assert.NotNil(err)
			assert.Equal(codes.NotFound, grpc.Code(err))
		})
	})
}

func (ts *NetworkServerAPITestSuite) TestMulticastQueue() {
	assert := require.New(ts.T())

	gateways := []storage.Gateway{
		{
			GatewayID: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
		},
		{
			GatewayID: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 2},
		},
	}
	for i := range gateways {
		assert.NoError(storage.CreateGateway(ts.DB(), &gateways[i]))
	}

	var rp storage.RoutingProfile
	var sp storage.ServiceProfile
	var dp storage.DeviceProfile

	assert.NoError(storage.CreateRoutingProfile(ts.DB(), &rp))
	assert.NoError(storage.CreateServiceProfile(ts.DB(), &sp))
	assert.NoError(storage.CreateDeviceProfile(ts.DB(), &dp))

	devices := []storage.Device{
		{
			DevEUI:           lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 1},
			RoutingProfileID: rp.ID,
			ServiceProfileID: sp.ID,
			DeviceProfileID:  dp.ID,
		},
		{
			DevEUI:           lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
			RoutingProfileID: rp.ID,
			ServiceProfileID: sp.ID,
			DeviceProfileID:  dp.ID,
		},
	}
	for i := range devices {
		assert.NoError(storage.CreateDevice(ts.DB(), &devices[i]))
		assert.NoError(storage.SaveDeviceGatewayRXInfoSet(ts.RedisPool(), storage.DeviceGatewayRXInfoSet{
			DevEUI: devices[i].DevEUI,
			DR:     3,
			Items: []storage.DeviceGatewayRXInfo{
				{
					GatewayID: gateways[i].GatewayID,
					RSSI:      50,
					LoRaSNR:   5,
				},
			},
		}))
	}

	ts.T().Run("Class-B", func(t *testing.T) {
		assert := require.New(t)

		mg := storage.MulticastGroup{
			GroupType:        storage.MulticastGroupB,
			MCAddr:           lorawan.DevAddr{1, 2, 3, 4},
			PingSlotPeriod:   32 * 128, // every 128 seconds
			ServiceProfileID: sp.ID,
			RoutingProfileID: rp.ID,
		}
		assert.NoError(storage.CreateMulticastGroup(ts.DB(), &mg))

		for _, d := range devices {
			assert.NoError(storage.AddDeviceToMulticastGroup(ts.DB(), d.DevEUI, mg.ID))
		}

		ts.T().Run("Create", func(t *testing.T) {
			assert := require.New(t)

			qi1 := ns.MulticastQueueItem{
				MulticastGroupId: mg.ID.Bytes(),
				FCnt:             10,
				FPort:            20,
				FrmPayload:       []byte{1, 2, 3, 4},
			}
			qi2 := ns.MulticastQueueItem{
				MulticastGroupId: mg.ID.Bytes(),
				FCnt:             11,
				FPort:            20,
				FrmPayload:       []byte{1, 2, 3, 4},
			}

			_, err := ts.api.EnqueueMulticastQueueItem(context.Background(), &ns.EnqueueMulticastQueueItemRequest{
				MulticastQueueItem: &qi1,
			})
			assert.NoError(err)
			_, err = ts.api.EnqueueMulticastQueueItem(context.Background(), &ns.EnqueueMulticastQueueItemRequest{
				MulticastQueueItem: &qi2,
			})
			assert.NoError(err)

			t.Run("List", func(t *testing.T) {
				assert := require.New(t)

				listResp, err := ts.api.GetMulticastQueueItemsForMulticastGroup(context.Background(), &ns.GetMulticastQueueItemsForMulticastGroupRequest{
					MulticastGroupId: mg.ID.Bytes(),
				})
				assert.NoError(err)
				assert.Len(listResp.MulticastQueueItems, 2)

				for i, exp := range []struct {
					FCnt uint32
				}{
					{10}, {11},
				} {
					assert.Equal(exp.FCnt, listResp.MulticastQueueItems[i].FCnt)
				}
			})

			t.Run("Test emit and schedule at", func(t *testing.T) {
				assert := require.New(t)

				items, err := storage.GetMulticastQueueItemsForMulticastGroup(ts.DB(), mg.ID)
				assert.NoError(err)
				assert.Len(items, 4)

				for _, item := range items {
					assert.NotNil(item.EmitAtTimeSinceGPSEpoch)
				}

				emitAt := *items[0].EmitAtTimeSinceGPSEpoch

				// iterate over the enqueued items and based on the first item
				// calculate the next ping-slot and validate if this is used
				// for the next queue-item.
				for i := range items {
					if i == 0 {
						continue
					}
					var err error
					emitAt, err = classb.GetNextPingSlotAfter(emitAt, mg.MCAddr, (1<<12)/mg.PingSlotPeriod)
					assert.NoError(err)
					assert.Equal(emitAt, *items[i].EmitAtTimeSinceGPSEpoch, "queue item %d", i)

					scheduleAt := time.Time(gps.NewFromTimeSinceGPSEpoch(emitAt)).Add(-2 * config.SchedulerInterval)
					assert.EqualValues(scheduleAt.UTC(), items[i].ScheduleAt.UTC())
				}
			})
		})
	})

	ts.T().Run("Class-C", func(t *testing.T) {
		assert := require.New(t)

		mg := storage.MulticastGroup{
			GroupType:        storage.MulticastGroupC,
			ServiceProfileID: sp.ID,
			RoutingProfileID: rp.ID,
		}
		assert.NoError(storage.CreateMulticastGroup(ts.DB(), &mg))

		for _, d := range devices {
			assert.NoError(storage.AddDeviceToMulticastGroup(ts.DB(), d.DevEUI, mg.ID))
		}

		ts.T().Run("Create", func(t *testing.T) {
			assert := require.New(t)

			qi1 := ns.MulticastQueueItem{
				MulticastGroupId: mg.ID.Bytes(),
				FCnt:             10,
				FPort:            20,
				FrmPayload:       []byte{1, 2, 3, 4},
			}
			qi2 := ns.MulticastQueueItem{
				MulticastGroupId: mg.ID.Bytes(),
				FCnt:             11,
				FPort:            20,
				FrmPayload:       []byte{1, 2, 3, 4},
			}

			_, err := ts.api.EnqueueMulticastQueueItem(context.Background(), &ns.EnqueueMulticastQueueItemRequest{
				MulticastQueueItem: &qi1,
			})
			assert.NoError(err)
			_, err = ts.api.EnqueueMulticastQueueItem(context.Background(), &ns.EnqueueMulticastQueueItemRequest{
				MulticastQueueItem: &qi2,
			})
			assert.NoError(err)

			t.Run("List", func(t *testing.T) {
				assert := require.New(t)

				listResp, err := ts.api.GetMulticastQueueItemsForMulticastGroup(context.Background(), &ns.GetMulticastQueueItemsForMulticastGroupRequest{
					MulticastGroupId: mg.ID.Bytes(),
				})
				assert.NoError(err)
				assert.Len(listResp.MulticastQueueItems, 2)

				for i, exp := range []struct {
					FCnt uint32
				}{
					{10}, {11},
				} {
					assert.Equal(exp.FCnt, listResp.MulticastQueueItems[i].FCnt)
				}
			})

			t.Run("Test emit and schedule at", func(t *testing.T) {
				assert := require.New(t)

				items, err := storage.GetMulticastQueueItemsForMulticastGroup(ts.DB(), mg.ID)
				assert.NoError(err)
				assert.Len(items, 4)

				for _, item := range items {
					assert.Nil(item.EmitAtTimeSinceGPSEpoch)
				}

				scheduleAt := items[0].ScheduleAt

				for i := range items {
					if i == 0 {
						continue
					}

					assert.Equal(scheduleAt, items[i].ScheduleAt.Add(-config.ClassCDownlinkLockDuration))
					scheduleAt = items[i].ScheduleAt
				}
			})
		})
	})

}

func (ts *NetworkServerAPITestSuite) TestDevice() {
	assert := require.New(ts.T())

	rp := storage.RoutingProfile{}
	assert.NoError(storage.CreateRoutingProfile(ts.DB(), &rp))

	sp := storage.ServiceProfile{}
	assert.NoError(storage.CreateServiceProfile(ts.DB(), &sp))

	dp := storage.DeviceProfile{}
	assert.NoError(storage.CreateDeviceProfile(ts.DB(), &dp))

	devEUI := lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}

	ts.T().Run("Create", func(t *testing.T) {
		assert := require.New(t)

		d := &ns.Device{
			DevEui:           devEUI[:],
			DeviceProfileId:  dp.ID.Bytes(),
			ServiceProfileId: sp.ID.Bytes(),
			RoutingProfileId: rp.ID.Bytes(),
			SkipFCntCheck:    true,
		}

		_, err := ts.api.CreateDevice(context.Background(), &ns.CreateDeviceRequest{
			Device: d,
		})
		assert.NoError(err)

		t.Run("Get", func(t *testing.T) {
			assert := require.New(t)

			getResp, err := ts.api.GetDevice(context.Background(), &ns.GetDeviceRequest{
				DevEui: devEUI[:],
			})
			assert.NoError(err)
			assert.Equal(d, getResp.Device)
		})

		t.Run("Update", func(t *testing.T) {
			assert := require.New(t)

			rp2 := storage.RoutingProfile{}
			assert.NoError(storage.CreateRoutingProfile(ts.DB(), &rp2))

			d.RoutingProfileId = rp2.ID.Bytes()
			_, err := ts.api.UpdateDevice(context.Background(), &ns.UpdateDeviceRequest{
				Device: d,
			})
			assert.NoError(err)

			getResp, err := ts.api.GetDevice(context.Background(), &ns.GetDeviceRequest{
				DevEui: devEUI[:],
			})
			assert.NoError(err)
			assert.Equal(d, getResp.Device)
		})

		t.Run("Multicast-groups", func(t *testing.T) {
			assert := require.New(t)

			mg1 := storage.MulticastGroup{
				RoutingProfileID: rp.ID,
				ServiceProfileID: sp.ID,
			}
			assert.NoError(storage.CreateMulticastGroup(ts.DB(), &mg1))

			t.Run("Add", func(t *testing.T) {
				_, err := ts.api.AddDeviceToMulticastGroup(context.Background(), &ns.AddDeviceToMulticastGroupRequest{
					DevEui:           devEUI[:],
					MulticastGroupId: mg1.ID.Bytes(),
				})
				assert.NoError(err)

				t.Run("Remove", func(t *testing.T) {
					assert := require.New(t)

					_, err := ts.api.RemoveDeviceFromMulticastGroup(context.Background(), &ns.RemoveDeviceFromMulticastGroupRequest{
						DevEui:           devEUI[:],
						MulticastGroupId: mg1.ID.Bytes(),
					})
					assert.NoError(err)

					_, err = ts.api.RemoveDeviceFromMulticastGroup(context.Background(), &ns.RemoveDeviceFromMulticastGroupRequest{
						DevEui:           devEUI[:],
						MulticastGroupId: mg1.ID.Bytes(),
					})
					assert.Error(err)
					assert.Equal(codes.NotFound, grpc.Code(err))
				})
			})
		})

		t.Run("Delete", func(t *testing.T) {
			assert := require.New(t)

			_, err := ts.api.DeleteDevice(context.Background(), &ns.DeleteDeviceRequest{
				DevEui: devEUI[:],
			})
			assert.NoError(err)

			_, err = ts.api.DeleteDevice(context.Background(), &ns.DeleteDeviceRequest{
				DevEui: devEUI[:],
			})
			assert.Error(err)
			assert.Equal(codes.NotFound, grpc.Code(err))
		})
	})
}

func TestNetworkServerAPINew(t *testing.T) {
	suite.Run(t, new(NetworkServerAPITestSuite))
}
