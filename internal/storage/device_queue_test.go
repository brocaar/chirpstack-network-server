package storage

import (
	"fmt"
	"testing"
	"time"

	"github.com/pkg/errors"

	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/gps"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
	. "github.com/smartystreets/goconvey/convey"
)

func TestDeviceQueue(t *testing.T) {
	conf := test.GetConfig()
	db, err := common.OpenDatabase(conf.PostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	config.C.PostgreSQL.DB = db

	Convey("Given a clean database", t, func() {
		test.MustResetDB(config.C.PostgreSQL.DB)
		asClient := test.NewApplicationClient()
		config.C.ApplicationServer.Pool = test.NewApplicationServerPool(asClient)

		Convey("Given a service, device and routing profile and device", func() {
			sp := ServiceProfile{}
			So(CreateServiceProfile(db, &sp), ShouldBeNil)

			dp := DeviceProfile{}
			So(CreateDeviceProfile(db, &dp), ShouldBeNil)

			rp := RoutingProfile{}
			So(CreateRoutingProfile(db, &rp), ShouldBeNil)

			d := Device{
				DevEUI:           lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
				ServiceProfileID: sp.ServiceProfile.ServiceProfileID,
				DeviceProfileID:  dp.DeviceProfile.DeviceProfileID,
				RoutingProfileID: rp.RoutingProfile.RoutingProfileID,
			}
			So(CreateDevice(db, &d), ShouldBeNil)

			Convey("Given a set of queue items", func() {
				gpsEpochTS1 := time.Second * 30
				gpsEpochTS2 := time.Second * 40
				inOneHour := time.Now().Add(time.Hour).UTC().Truncate(time.Millisecond)

				items := []DeviceQueueItem{
					{
						DevEUI:     d.DevEUI,
						FRMPayload: []byte{1, 2, 3},
						FCnt:       1,
						FPort:      10,
						Confirmed:  true,
					},
					{
						DevEUI:     d.DevEUI,
						FRMPayload: []byte{4, 5, 6},
						FCnt:       3,
						FPort:      11,
						EmitAtTimeSinceGPSEpoch: &gpsEpochTS1,
					},
					{
						DevEUI:     d.DevEUI,
						FRMPayload: []byte{7, 8, 9},
						FCnt:       2,
						FPort:      12,
						EmitAtTimeSinceGPSEpoch: &gpsEpochTS2,
					},
				}
				for i := range items {
					So(CreateDeviceQueueItem(db, &items[i]), ShouldBeNil)
					items[i].CreatedAt = items[i].UpdatedAt.UTC().Truncate(time.Millisecond)
					items[i].UpdatedAt = items[i].UpdatedAt.UTC().Truncate(time.Millisecond)
				}

				Convey("Then GetMaxEmlitAtTimeSinceGPSEpochForDevEUI returns the expected value", func() {
					d, err := GetMaxEmitAtTimeSinceGPSEpochForDevEUI(config.C.PostgreSQL.DB, d.DevEUI)
					So(err, ShouldBeNil)
					So(d, ShouldEqual, gpsEpochTS2)
				})

				Convey("Then GetDeviceQueueItem returns the requested item", func() {
					qi, err := GetDeviceQueueItem(db, items[0].ID)
					So(err, ShouldBeNil)
					qi.CreatedAt = qi.CreatedAt.UTC().Truncate(time.Millisecond)
					qi.UpdatedAt = qi.UpdatedAt.UTC().Truncate(time.Millisecond)
					So(qi, ShouldResemble, items[0])
				})

				Convey("Then UpdateDeviceQueueItem updates the queue item", func() {
					items[0].IsPending = true
					items[0].TimeoutAfter = &inOneHour
					So(UpdateDeviceQueueItem(db, &items[0]), ShouldBeNil)
					items[0].UpdatedAt = items[0].UpdatedAt.UTC().Truncate(time.Millisecond)

					qi, err := GetDeviceQueueItem(db, items[0].ID)
					So(err, ShouldBeNil)
					qi.CreatedAt = qi.CreatedAt.UTC().Truncate(time.Millisecond)
					qi.UpdatedAt = qi.UpdatedAt.UTC().Truncate(time.Millisecond)
					So(qi.TimeoutAfter.Equal(inOneHour), ShouldBeTrue)
					qi.TimeoutAfter = &inOneHour
					So(qi, ShouldResemble, items[0])
				})

				Convey("Then GetDeviceQueueItemsForDevEUI returns the expected items in the expected order", func() {
					queueItems, err := GetDeviceQueueItemsForDevEUI(db, d.DevEUI)
					So(err, ShouldBeNil)
					So(queueItems, ShouldHaveLength, len(items))
					So(queueItems[0].FCnt, ShouldEqual, 1)
					So(queueItems[1].FCnt, ShouldEqual, 2)
					So(queueItems[2].FCnt, ShouldEqual, 3)
				})

				Convey("Then GetNextDeviceQueueItemForDevEUI returns the first item that should be emitted", func() {
					qi, err := GetNextDeviceQueueItemForDevEUI(db, d.DevEUI)
					So(err, ShouldBeNil)
					So(qi.FCnt, ShouldEqual, 1)
				})

				Convey("Given the first item in the queue is pending and has a timeout in the future", func() {
					ts := time.Now().Add(time.Minute)
					items[0].IsPending = true
					items[0].TimeoutAfter = &ts
					So(UpdateDeviceQueueItem(config.C.PostgreSQL.DB, &items[0]), ShouldBeNil)

					Convey("Then GetNextDeviceQueueItemForDevEUI returns does not exist error", func() {
						_, err := GetNextDeviceQueueItemForDevEUI(config.C.PostgreSQL.DB, d.DevEUI)
						So(err, ShouldEqual, ErrDoesNotExist)
					})
				})

				Convey("Then FlushDeviceQueueForDevEUI flushes the queue", func() {
					So(FlushDeviceQueueForDevEUI(db, d.DevEUI), ShouldBeNil)
					items, err := GetDeviceQueueItemsForDevEUI(db, d.DevEUI)
					So(err, ShouldBeNil)
					So(items, ShouldHaveLength, 0)
				})

				Convey("Then DeleteDeviceQueueItem deletes a queue item", func() {
					So(DeleteDeviceQueueItem(db, items[0].ID), ShouldBeNil)
					items, err := GetDeviceQueueItemsForDevEUI(db, d.DevEUI)
					So(err, ShouldBeNil)
					So(items, ShouldHaveLength, 2)
				})
			})

			Convey("When testing GetNextDeviceQueueItemForDevEUIMaxPayloadSizeAndFCnt", func() {
				oneMinuteAgo := time.Now().Add(-time.Minute)

				items := []DeviceQueueItem{
					{
						DevEUI:       d.DevEUI,
						FCnt:         100,
						FPort:        1,
						FRMPayload:   []byte{1, 2, 3, 4, 5, 6, 7},
						IsPending:    true,
						TimeoutAfter: &oneMinuteAgo,
					},
					{
						DevEUI:     d.DevEUI,
						FCnt:       101,
						FPort:      1,
						FRMPayload: []byte{1, 2, 3, 4, 5, 6, 7},
					},
					{
						DevEUI:     d.DevEUI,
						FCnt:       102,
						FPort:      2,
						FRMPayload: []byte{1, 2, 3, 4, 5, 6},
					},
					{
						DevEUI:     d.DevEUI,
						FCnt:       103,
						FPort:      3,
						FRMPayload: []byte{1, 2, 3, 4, 5},
					},
					{
						DevEUI:     d.DevEUI,
						FCnt:       104,
						FPort:      4,
						FRMPayload: []byte{1, 2, 3, 4},
					},
				}
				for i := range items {
					So(CreateDeviceQueueItem(config.C.PostgreSQL.DB, &items[i]), ShouldBeNil)
				}

				tests := []struct {
					Name          string
					FCnt          uint32
					MaxFRMPayload int

					ExpectedDeviceQueueItemID *int64
					ExpectedHandleError       []as.HandleErrorRequest
					ExpectedHandleDownlinkACK []as.HandleDownlinkACKRequest
					ExpectedError             error
				}{
					{
						Name:                      "nACK + first item from the queue (timeout)",
						FCnt:                      100,
						MaxFRMPayload:             7,
						ExpectedDeviceQueueItemID: &items[1].ID,
						ExpectedHandleDownlinkACK: []as.HandleDownlinkACKRequest{
							{DevEUI: d.DevEUI[:], FCnt: items[0].FCnt, Acknowledged: false},
						},
					},
					{
						Name:                      "nACK + first item discarded (payload size)",
						FCnt:                      100,
						MaxFRMPayload:             6,
						ExpectedDeviceQueueItemID: &items[1].ID,
						ExpectedHandleError: []as.HandleErrorRequest{
							{DevEUI: d.DevEUI[:], Type: as.ErrorType_DEVICE_QUEUE_ITEM_SIZE, Error: "payload exceeds max payload size", FCnt: 101},
						},
						ExpectedHandleDownlinkACK: []as.HandleDownlinkACKRequest{
							{DevEUI: d.DevEUI[:], FCnt: items[0].FCnt, Acknowledged: false},
						},
					},
					{
						Name:                      "nACK + first two items discarded (payload size)",
						FCnt:                      100,
						MaxFRMPayload:             5,
						ExpectedDeviceQueueItemID: &items[1].ID,
						ExpectedHandleError: []as.HandleErrorRequest{
							{DevEUI: d.DevEUI[:], Type: as.ErrorType_DEVICE_QUEUE_ITEM_SIZE, Error: "payload exceeds max payload size", FCnt: 101},
							{DevEUI: d.DevEUI[:], Type: as.ErrorType_DEVICE_QUEUE_ITEM_SIZE, Error: "payload exceeds max payload size", FCnt: 102},
						},
						ExpectedHandleDownlinkACK: []as.HandleDownlinkACKRequest{
							{DevEUI: d.DevEUI[:], FCnt: items[0].FCnt, Acknowledged: false},
						},
					},
					{
						Name:          "nACK + all items discarded (payload size)",
						FCnt:          101,
						MaxFRMPayload: 3,
						ExpectedHandleError: []as.HandleErrorRequest{
							{DevEUI: d.DevEUI[:], Type: as.ErrorType_DEVICE_QUEUE_ITEM_SIZE, Error: "payload exceeds max payload size", FCnt: 101},
							{DevEUI: d.DevEUI[:], Type: as.ErrorType_DEVICE_QUEUE_ITEM_SIZE, Error: "payload exceeds max payload size", FCnt: 102},
							{DevEUI: d.DevEUI[:], Type: as.ErrorType_DEVICE_QUEUE_ITEM_SIZE, Error: "payload exceeds max payload size", FCnt: 103},
							{DevEUI: d.DevEUI[:], Type: as.ErrorType_DEVICE_QUEUE_ITEM_SIZE, Error: "payload exceeds max payload size", FCnt: 104},
						},
						ExpectedHandleDownlinkACK: []as.HandleDownlinkACKRequest{
							{DevEUI: d.DevEUI[:], FCnt: items[0].FCnt, Acknowledged: false},
						},
						ExpectedError: ErrDoesNotExist,
					},
					{
						Name:                      "nACK + first item discarded (fCnt)",
						FCnt:                      102,
						MaxFRMPayload:             7,
						ExpectedDeviceQueueItemID: &items[1].ID,
						ExpectedHandleError: []as.HandleErrorRequest{
							{DevEUI: d.DevEUI[:], Type: as.ErrorType_DEVICE_QUEUE_ITEM_FCNT, Error: "invalid frame-counter", FCnt: 101},
						},
						ExpectedHandleDownlinkACK: []as.HandleDownlinkACKRequest{
							{DevEUI: d.DevEUI[:], FCnt: items[0].FCnt, Acknowledged: false},
						},
					},
				}

				for i, test := range tests {
					Convey(fmt.Sprintf("Testing: %s [%d]", test.Name, i), func() {
						qi, err := GetNextDeviceQueueItemForDevEUIMaxPayloadSizeAndFCnt(config.C.PostgreSQL.DB, d.DevEUI, test.MaxFRMPayload, test.FCnt, rp.RoutingProfileID)
						if test.ExpectedHandleError == nil {
							So(*test.ExpectedDeviceQueueItemID, ShouldEqual, qi.ID)
							So(err, ShouldBeNil)
						} else {
							So(errors.Cause(err), ShouldEqual, test.ExpectedError)
						}

						So(asClient.HandleErrorChan, ShouldHaveLength, len(test.ExpectedHandleError))
						for _, err := range test.ExpectedHandleError {
							req := <-asClient.HandleErrorChan
							So(req, ShouldResemble, err)
						}

						So(asClient.HandleDownlinkACKChan, ShouldHaveLength, len(test.ExpectedHandleDownlinkACK))
						for _, ack := range test.ExpectedHandleDownlinkACK {
							req := <-asClient.HandleDownlinkACKChan
							So(req, ShouldResemble, ack)
						}
					})
				}
			})
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

func TestGetDevEUIsWithClassCDeviceQueueItems(t *testing.T) {
	conf := test.GetConfig()
	db, err := common.OpenDatabase(conf.PostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	config.C.PostgreSQL.DB = db

	Convey("Given a clean database", t, func() {
		test.MustResetDB(config.C.PostgreSQL.DB)

		Convey("Given a service-, class-b device- and routing-profile and two devices", func() {
			sp := ServiceProfile{}
			So(CreateServiceProfile(config.C.PostgreSQL.DB, &sp), ShouldBeNil)

			rp := RoutingProfile{}
			So(CreateRoutingProfile(config.C.PostgreSQL.DB, &rp), ShouldBeNil)

			dp := DeviceProfile{
				DeviceProfile: backend.DeviceProfile{
					SupportsClassB: true,
				},
			}
			So(CreateDeviceProfile(config.C.PostgreSQL.DB, &dp), ShouldBeNil)

			devices := []Device{
				{
					ServiceProfileID: sp.ServiceProfile.ServiceProfileID,
					DeviceProfileID:  dp.DeviceProfile.DeviceProfileID,
					RoutingProfileID: rp.RoutingProfile.RoutingProfileID,
					DevEUI:           lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
				},
				{
					ServiceProfileID: sp.ServiceProfile.ServiceProfileID,
					DeviceProfileID:  dp.DeviceProfile.DeviceProfileID,
					RoutingProfileID: rp.RoutingProfile.RoutingProfileID,
					DevEUI:           lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
				},
			}
			for i := range devices {
				So(CreateDevice(config.C.PostgreSQL.DB, &devices[i]), ShouldBeNil)
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
			So(CreateServiceProfile(config.C.PostgreSQL.DB, &sp), ShouldBeNil)

			rp := RoutingProfile{}
			So(CreateRoutingProfile(config.C.PostgreSQL.DB, &rp), ShouldBeNil)

			dp := DeviceProfile{
				DeviceProfile: backend.DeviceProfile{
					SupportsClassC: true,
				},
			}
			So(CreateDeviceProfile(config.C.PostgreSQL.DB, &dp), ShouldBeNil)

			devices := []Device{
				{
					ServiceProfileID: sp.ServiceProfile.ServiceProfileID,
					DeviceProfileID:  dp.DeviceProfile.DeviceProfileID,
					RoutingProfileID: rp.RoutingProfile.RoutingProfileID,
					DevEUI:           lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
				},
				{
					ServiceProfileID: sp.ServiceProfile.ServiceProfileID,
					DeviceProfileID:  dp.DeviceProfile.DeviceProfileID,
					RoutingProfileID: rp.RoutingProfile.RoutingProfileID,
					DevEUI:           lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
				},
			}
			for i := range devices {
				So(CreateDevice(config.C.PostgreSQL.DB, &devices[i]), ShouldBeNil)
			}

			inOneMinute := time.Now().Add(time.Minute)

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
			var transactions []*common.TxLogger
			var out [][]lorawan.EUI64

			defer func() {
				for i := range transactions {
					transactions[i].Rollback()
				}
			}()

			for i := range test.QueueItems {
				So(CreateDeviceQueueItem(config.C.PostgreSQL.DB, &test.QueueItems[i]), ShouldBeNil)
			}

			for i := 0; i < test.GetCallCount; i++ {
				tx, err := config.C.PostgreSQL.DB.Beginx()
				So(err, ShouldBeNil)
				transactions = append(transactions, tx)

				devs, err := GetDevicesWithClassBOrClassCDeviceQueueItems(tx, test.GetCount)
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
