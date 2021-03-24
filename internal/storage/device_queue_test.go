package storage

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"

	"github.com/brocaar/chirpstack-network-server/internal/backend/applicationserver"
	"github.com/brocaar/chirpstack-network-server/internal/gps"
	"github.com/brocaar/chirpstack-network-server/internal/test"
	"github.com/brocaar/lorawan"
)

func TestDeviceQueueItemValidate(t *testing.T) {
	Convey("Given a test-set", t, func() {
		tests := []struct {
			Item          DeviceQueueItem
			ExpectedError error
		}{
			{
				Item: DeviceQueueItem{
					FPort: 0,
				},
				ExpectedError: ErrInvalidFPort,
			},
			{
				Item: DeviceQueueItem{
					FPort: 1,
				},
				ExpectedError: nil,
			},
		}

		for _, test := range tests {
			So(test.Item.Validate(), ShouldResemble, test.ExpectedError)
		}
	})
}

func (ts *StorageTestSuite) TestDeviceQueue() {
	assert := require.New(ts.T())

	asClient := test.NewApplicationClient()
	applicationserver.SetPool(test.NewApplicationServerPool(asClient))

	sp := ServiceProfile{}
	dp := DeviceProfile{}
	rp := RoutingProfile{}
	ds := DeviceSession{}

	assert.Nil(CreateServiceProfile(context.Background(), ts.Tx(), &sp))
	assert.Nil(CreateDeviceProfile(context.Background(), ts.Tx(), &dp))
	assert.Nil(CreateRoutingProfile(context.Background(), ts.Tx(), &rp))

	d := Device{
		DevEUI:           lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
		ServiceProfileID: sp.ID,
		DeviceProfileID:  dp.ID,
		RoutingProfileID: rp.ID,
	}
	assert.NoError(CreateDevice(context.Background(), ts.Tx(), &d))

	ts.T().Run("Create", func(t *testing.T) {
		assert := require.New(t)

		gpsEpochTS1 := time.Second * 30
		gpsEpochTS2 := time.Second * 40
		inOneHour := time.Now().Add(time.Hour).UTC().Truncate(time.Millisecond)

		items := []DeviceQueueItem{
			{
				DevAddr:    lorawan.DevAddr{1, 2, 3, 4},
				DevEUI:     d.DevEUI,
				FRMPayload: []byte{1, 2, 3},
				FCnt:       1,
				FPort:      10,
				Confirmed:  true,
			},
			{
				DevAddr:                 lorawan.DevAddr{1, 2, 3, 4},
				DevEUI:                  d.DevEUI,
				FRMPayload:              []byte{4, 5, 6},
				FCnt:                    3,
				FPort:                   11,
				EmitAtTimeSinceGPSEpoch: &gpsEpochTS1,
			},
			{
				DevAddr:                 lorawan.DevAddr{1, 2, 3, 4},
				DevEUI:                  d.DevEUI,
				FRMPayload:              []byte{7, 8, 9},
				FCnt:                    2,
				FPort:                   12,
				EmitAtTimeSinceGPSEpoch: &gpsEpochTS2,
			},
		}
		for i := range items {
			assert.NoError(CreateDeviceQueueItem(context.Background(), ts.Tx(), &items[i], dp, ds))
			items[i].CreatedAt = items[i].UpdatedAt.UTC().Round(time.Millisecond)
			items[i].UpdatedAt = items[i].UpdatedAt.UTC().Round(time.Millisecond)
		}

		t.Run("Get count", func(t *testing.T) {
			assert := require.New(t)

			count, err := GetDeviceQueueItemCountForDevEUI(context.Background(), ts.Tx(), d.DevEUI)
			assert.NoError(err)
			assert.Equal(3, count)
		})

		t.Run("GetMaxEmitAtTimeSinceGPSEpochForDevEUI", func(t *testing.T) {
			assert := require.New(t)
			d, err := GetMaxEmitAtTimeSinceGPSEpochForDevEUI(context.Background(), ts.Tx(), d.DevEUI)
			assert.NoError(err)
			assert.Equal(gpsEpochTS2, d)
		})

		t.Run("Get queue item", func(t *testing.T) {
			assert := require.New(t)

			qi, err := GetDeviceQueueItem(context.Background(), ts.Tx(), items[0].ID)
			assert.NoError(err)
			qi.CreatedAt = qi.CreatedAt.UTC().Round(time.Millisecond)
			qi.UpdatedAt = qi.UpdatedAt.UTC().Round(time.Millisecond)
			assert.Equal(items[0], qi)
		})

		t.Run("Get for DevEUI", func(t *testing.T) {
			assert := require.New(t)

			queueItems, err := GetDeviceQueueItemsForDevEUI(context.Background(), ts.Tx(), d.DevEUI)
			assert.NoError(err)
			assert.Len(queueItems, len(items))
			assert.EqualValues(1, queueItems[0].FCnt)
			assert.EqualValues(2, queueItems[1].FCnt)
			assert.EqualValues(3, queueItems[2].FCnt)
		})

		t.Run("GetNextDeviceQueueItemForDevEUI", func(t *testing.T) {
			assert := require.New(t)

			qi, more, err := GetNextDeviceQueueItemForDevEUI(context.Background(), ts.Tx(), d.DevEUI)
			assert.NoError(err)
			assert.EqualValues(1, qi.FCnt)
			assert.True(more)
		})

		t.Run("First item in queue is pending and timeout in future", func(t *testing.T) {
			assert := require.New(t)

			tss := time.Now().Add(time.Minute)
			items[0].IsPending = true
			items[0].TimeoutAfter = &tss
			assert.NoError(UpdateDeviceQueueItem(context.Background(), ts.Tx(), &items[0]))

			_, _, err := GetNextDeviceQueueItemForDevEUI(context.Background(), ts.Tx(), d.DevEUI)
			assert.Equal(err, ErrDoesNotExist)
		})

		t.Run("Update", func(t *testing.T) {
			assert := require.New(t)

			items[0].IsPending = true
			items[0].TimeoutAfter = &inOneHour
			items[0].RetryAfter = &inOneHour
			assert.NoError(UpdateDeviceQueueItem(context.Background(), ts.Tx(), &items[0]))
			items[0].UpdatedAt = items[0].UpdatedAt.UTC().Round(time.Millisecond)

			qi, err := GetDeviceQueueItem(context.Background(), ts.Tx(), items[0].ID)
			assert.NoError(err)
			qi.CreatedAt = qi.CreatedAt.UTC().Round(time.Millisecond)
			qi.UpdatedAt = qi.UpdatedAt.UTC().Round(time.Millisecond)
			assert.True(qi.TimeoutAfter.Equal(inOneHour))
			qi.TimeoutAfter = &inOneHour
			assert.True(qi.RetryAfter.Equal(inOneHour))
			qi.RetryAfter = &inOneHour
			assert.Equal(items[0], qi)
		})

		t.Run("Delete item", func(t *testing.T) {
			assert := require.New(t)

			assert.NoError(DeleteDeviceQueueItem(context.Background(), ts.Tx(), items[0].ID))
			items, err := GetDeviceQueueItemsForDevEUI(context.Background(), ts.Tx(), d.DevEUI)
			assert.NoError(err)
			assert.Len(items, 2)
		})

		t.Run("Flush all", func(t *testing.T) {
			assert := require.New(t)

			assert.NoError(FlushDeviceQueueForDevEUI(context.Background(), ts.Tx(), d.DevEUI))
			items, err := GetDeviceQueueItemsForDevEUI(context.Background(), ts.Tx(), d.DevEUI)
			assert.NoError(err)
			assert.Len(items, 0)
		})

		t.Run("Create for Class-B", func(t *testing.T) {
			assert := require.New(t)

			dp.SupportsClassB = true
			ds.BeaconLocked = true
			ds.PingSlotNb = 1

			qi := DeviceQueueItem{
				DevAddr:    lorawan.DevAddr{1, 2, 3, 4},
				DevEUI:     d.DevEUI,
				FRMPayload: []byte{1, 2, 3},
				FCnt:       1,
				FPort:      10,
			}
			assert.NoError(CreateDeviceQueueItem(context.Background(), ts.Tx(), &qi, dp, ds))

			qiGet, more, err := GetNextDeviceQueueItemForDevEUI(context.Background(), ts.Tx(), d.DevEUI)
			assert.NoError(err)
			assert.False(more)

			// For class-b, the EmitAtTimeSinceGPSEpoch must be set.
			assert.NotNil(qiGet.EmitAtTimeSinceGPSEpoch)
		})

	})
}

type getDeviceQueueItemsTestCase struct {
	Name            string
	GetCallCount    int // the number of Get calls to make, each in a separate db transaction
	GetCount        int
	QueueItems      []DeviceQueueItem
	ExpectedDevEUIs [][]lorawan.EUI64 // slice of EUIs per database transaction
}

func TestGetDevEUIsWithClassBOrCDeviceQueueItems(t *testing.T) {
	conf := test.GetConfig()
	if err := Setup(conf); err != nil {
		t.Fatal(err)
	}

	Convey("Given a clean database", t, func() {
		So(MigrateDown(DB().DB), ShouldBeNil)
		So(MigrateUp(DB().DB), ShouldBeNil)

		Convey("Given a service-, class-b device- and routing-profile and two, SchedulerInterval setting", func() {
			sp := ServiceProfile{}
			So(CreateServiceProfile(context.Background(), DB(), &sp), ShouldBeNil)

			rp := RoutingProfile{}
			So(CreateRoutingProfile(context.Background(), DB(), &rp), ShouldBeNil)

			dp := DeviceProfile{
				SupportsClassB: true,
			}
			So(CreateDeviceProfile(context.Background(), DB(), &dp), ShouldBeNil)

			devices := []Device{
				{
					ServiceProfileID: sp.ID,
					DeviceProfileID:  dp.ID,
					RoutingProfileID: rp.ID,
					DevEUI:           lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
					Mode:             DeviceModeB,
				},
				{
					ServiceProfileID: sp.ID,
					DeviceProfileID:  dp.ID,
					RoutingProfileID: rp.ID,
					DevEUI:           lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
					Mode:             DeviceModeB,
				},
			}
			for i := range devices {
				So(CreateDevice(context.Background(), DB(), &devices[i]), ShouldBeNil)
			}

			inTwoSeconds := gps.Time(time.Now().Add(time.Second)).TimeSinceGPSEpoch()
			inFiveSeconds := gps.Time(time.Now().Add(5 * time.Second)).TimeSinceGPSEpoch()

			tests := []getDeviceQueueItemsTestCase{
				{
					Name:         "single pingslot queue item to be scheduled",
					GetCallCount: 2,
					GetCount:     1,
					QueueItems: []DeviceQueueItem{
						{DevEUI: devices[0].DevEUI, FCnt: 1, FPort: 1, FRMPayload: []byte{1, 2, 3}, EmitAtTimeSinceGPSEpoch: &inTwoSeconds},
					},
					ExpectedDevEUIs: [][]lorawan.EUI64{
						{devices[0].DevEUI},
						nil,
					},
				},
				{
					Name:         "single pingslot queue item, not yet to schedule",
					GetCallCount: 2,
					GetCount:     1,
					QueueItems: []DeviceQueueItem{
						{DevEUI: devices[0].DevEUI, FCnt: 1, FPort: 1, FRMPayload: []byte{1, 2, 3}, EmitAtTimeSinceGPSEpoch: &inFiveSeconds},
					},
					ExpectedDevEUIs: [][]lorawan.EUI64{
						nil,
						nil,
					},
				},
				{
					Name:         "two pingslot queue items for two devices (limit 1)",
					GetCallCount: 2,
					GetCount:     1,
					QueueItems: []DeviceQueueItem{
						{DevEUI: devices[0].DevEUI, FCnt: 1, FPort: 1, FRMPayload: []byte{1, 2, 3}, EmitAtTimeSinceGPSEpoch: &inTwoSeconds},
						{DevEUI: devices[1].DevEUI, FCnt: 1, FPort: 1, FRMPayload: []byte{1, 2, 3}, EmitAtTimeSinceGPSEpoch: &inTwoSeconds},
					},
					ExpectedDevEUIs: [][]lorawan.EUI64{
						{devices[0].DevEUI},
						{devices[1].DevEUI},
					},
				},
				{
					Name:         "two pingslot queue items for two devices (limit 2)",
					GetCallCount: 2,
					GetCount:     2,
					QueueItems: []DeviceQueueItem{
						{DevEUI: devices[0].DevEUI, FCnt: 1, FPort: 1, FRMPayload: []byte{1, 2, 3}, EmitAtTimeSinceGPSEpoch: &inTwoSeconds},
						{DevEUI: devices[1].DevEUI, FCnt: 1, FPort: 1, FRMPayload: []byte{1, 2, 3}, EmitAtTimeSinceGPSEpoch: &inTwoSeconds},
					},
					ExpectedDevEUIs: [][]lorawan.EUI64{
						{devices[0].DevEUI, devices[1].DevEUI},
						nil,
					},
				},
			}

			runGetDeviceQueueItemsTests(tests)
		})

		Convey("Given a service-, class-c device- and routing-profile and two devices", func() {
			sp := ServiceProfile{}
			So(CreateServiceProfile(context.Background(), DB(), &sp), ShouldBeNil)

			rp := RoutingProfile{}
			So(CreateRoutingProfile(context.Background(), DB(), &rp), ShouldBeNil)

			dp := DeviceProfile{
				SupportsClassC: true,
			}
			So(CreateDeviceProfile(context.Background(), DB(), &dp), ShouldBeNil)

			devices := []Device{
				{
					ServiceProfileID: sp.ID,
					DeviceProfileID:  dp.ID,
					RoutingProfileID: rp.ID,
					DevEUI:           lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
					Mode:             DeviceModeC,
				},
				{
					ServiceProfileID: sp.ID,
					DeviceProfileID:  dp.ID,
					RoutingProfileID: rp.ID,
					DevEUI:           lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
					Mode:             DeviceModeC,
				},
			}
			for i := range devices {
				So(CreateDevice(context.Background(), DB(), &devices[i]), ShouldBeNil)
			}

			inOneMinute := time.Now().Add(time.Minute)
			oneMinuteAgo := time.Now().Add(-time.Minute)

			tests := []getDeviceQueueItemsTestCase{
				{
					Name:         "single queue item",
					GetCallCount: 2,
					GetCount:     1,
					QueueItems: []DeviceQueueItem{
						{DevEUI: devices[0].DevEUI, FCnt: 1, FPort: 1, FRMPayload: []byte{1, 2, 3}},
					},
					ExpectedDevEUIs: [][]lorawan.EUI64{
						{devices[0].DevEUI},
						nil,
					},
				},
				{
					Name:         "single pending queue item with timeout in one minute",
					GetCallCount: 2,
					GetCount:     1,
					QueueItems: []DeviceQueueItem{
						{DevEUI: devices[0].DevEUI, FCnt: 1, FPort: 1, FRMPayload: []byte{1, 2, 3}, IsPending: true, TimeoutAfter: &inOneMinute},
					},
					ExpectedDevEUIs: [][]lorawan.EUI64{
						nil,
						nil,
					},
				},
				{
					Name:         "single queue item with retry_after in the future",
					GetCallCount: 2,
					GetCount:     1,
					QueueItems: []DeviceQueueItem{
						{DevEUI: devices[0].DevEUI, FCnt: 1, FPort: 1, FRMPayload: []byte{1, 2, 3}, RetryAfter: &inOneMinute},
					},
					ExpectedDevEUIs: [][]lorawan.EUI64{
						nil,
						nil,
					},
				},
				{
					Name:         "single queue item with retry_after in the past",
					GetCallCount: 2,
					GetCount:     1,
					QueueItems: []DeviceQueueItem{
						{DevEUI: devices[0].DevEUI, FCnt: 1, FPort: 1, FRMPayload: []byte{1, 2, 3}, RetryAfter: &oneMinuteAgo},
					},
					ExpectedDevEUIs: [][]lorawan.EUI64{
						{devices[0].DevEUI},
						nil,
					},
				},
				{
					Name:         "two queue items, first one pending and timeout in one minute",
					GetCallCount: 2,
					GetCount:     1,
					QueueItems: []DeviceQueueItem{
						{DevEUI: devices[0].DevEUI, FCnt: 1, FPort: 1, FRMPayload: []byte{1, 2, 3}, IsPending: true, TimeoutAfter: &inOneMinute},
						{DevEUI: devices[0].DevEUI, FCnt: 2, FPort: 1, FRMPayload: []byte{1, 2, 3}},
					},
					ExpectedDevEUIs: [][]lorawan.EUI64{
						nil,
						nil,
					},
				},
				{
					Name:         "two queue items for two devices (limit 1)",
					GetCallCount: 2,
					GetCount:     1,
					QueueItems: []DeviceQueueItem{
						{DevEUI: devices[0].DevEUI, FCnt: 1, FPort: 1, FRMPayload: []byte{1, 2, 3}},
						{DevEUI: devices[1].DevEUI, FCnt: 1, FPort: 1, FRMPayload: []byte{1, 2, 3}},
					},
					ExpectedDevEUIs: [][]lorawan.EUI64{
						{devices[0].DevEUI},
						{devices[1].DevEUI},
					},
				},
				{
					Name:         "two queue items for two devices (limit 2)",
					GetCallCount: 2,
					GetCount:     2,
					QueueItems: []DeviceQueueItem{
						{DevEUI: devices[0].DevEUI, FCnt: 1, FPort: 1, FRMPayload: []byte{1, 2, 3}},
						{DevEUI: devices[1].DevEUI, FCnt: 1, FPort: 1, FRMPayload: []byte{1, 2, 3}},
					},
					ExpectedDevEUIs: [][]lorawan.EUI64{
						{devices[0].DevEUI, devices[1].DevEUI},
						nil,
					},
				},
			}

			runGetDeviceQueueItemsTests(tests)
		})
	})
}

func runGetDeviceQueueItemsTests(tests []getDeviceQueueItemsTestCase) {
	for i, test := range tests {
		Convey(fmt.Sprintf("testing: %s [%d]", test.Name, i), func() {
			var transactions []*TxLogger
			var out [][]lorawan.EUI64

			defer func() {
				for i := range transactions {
					transactions[i].Rollback()
				}
			}()

			for i := range test.QueueItems {
				So(CreateDeviceQueueItem(context.Background(), DB(), &test.QueueItems[i], DeviceProfile{}, DeviceSession{}), ShouldBeNil)
			}

			for i := 0; i < test.GetCallCount; i++ {
				tx, err := DB().Beginx()
				So(err, ShouldBeNil)
				transactions = append(transactions, tx)

				devs, err := GetDevicesWithClassBOrClassCDeviceQueueItems(context.Background(), tx, test.GetCount)
				So(err, ShouldBeNil)

				var euis []lorawan.EUI64
				for i := range devs {
					euis = append(euis, devs[i].DevEUI)
				}
				out = append(out, euis)
			}

			So(out, ShouldResemble, test.ExpectedDevEUIs)
		})
	}
}
