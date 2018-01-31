package data

import (
	"fmt"
	"testing"

	"github.com/Frankz/loraserver/internal/common"
	"github.com/Frankz/loraserver/internal/storage"
	"github.com/Frankz/loraserver/internal/test"
	"github.com/Frankz/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

func TestGetNextDeviceQueueItem(t *testing.T) {
	conf := test.GetConfig()
	db, err := common.OpenDatabase(conf.PostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	common.DB = db

	Convey("Given a clean database", t, func() {
		test.MustResetDB(common.DB)

		asClient := test.NewApplicationClient()
		common.ApplicationServerPool = test.NewApplicationServerPool(asClient)

		Convey("Given a service, device and routing profile and device", func() {
			sp := storage.ServiceProfile{}
			So(storage.CreateServiceProfile(db, &sp), ShouldBeNil)

			dp := storage.DeviceProfile{}
			So(storage.CreateDeviceProfile(db, &dp), ShouldBeNil)

			rp := storage.RoutingProfile{}
			So(storage.CreateRoutingProfile(db, &rp), ShouldBeNil)

			d := storage.Device{
				DevEUI:           lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
				ServiceProfileID: sp.ServiceProfile.ServiceProfileID,
				DeviceProfileID:  dp.DeviceProfile.DeviceProfileID,
				RoutingProfileID: rp.RoutingProfile.RoutingProfileID,
			}
			So(storage.CreateDevice(db, &d), ShouldBeNil)

			ctx := dataContext{
				DeviceSession: storage.DeviceSession{
					RoutingProfileID: rp.RoutingProfile.RoutingProfileID,
					DevEUI:           d.DevEUI,
					FCntDown:         10,
				},
				RemainingPayloadSize: 242,
			}

			items := []storage.DeviceQueueItem{
				{
					DevEUI:     d.DevEUI,
					FRMPayload: []byte{1, 2, 3, 4},
					FCnt:       10,
					FPort:      1,
				},
				{
					DevEUI:     d.DevEUI,
					FRMPayload: []byte{4, 5, 6, 7},
					Confirmed:  true,
					FCnt:       11,
					FPort:      1,
				},
			}
			for i := range items {
				So(storage.CreateDeviceQueueItem(common.DB, &items[i]), ShouldBeNil)
			}

			tests := []struct {
				BeforeFunc                  func()
				Name                        string
				ExpecteddataContext         dataContext
				ExpectedNextDeviceQueueItem *storage.DeviceQueueItem
			}{
				{
					BeforeFunc: func() {
						ctx.DeviceSession.FCntDown = 12 // to skip all queue items
					},
					Name: "no queue items",
					ExpecteddataContext: dataContext{
						DeviceSession: storage.DeviceSession{
							RoutingProfileID: rp.RoutingProfile.RoutingProfileID,
							DevEUI:           d.DevEUI,
							FCntDown:         12,
						},
						RemainingPayloadSize: 242,
					},
				},
				{
					Name: "first queue item (unconfirmed)",
					ExpecteddataContext: dataContext{
						DeviceSession:        ctx.DeviceSession,
						RemainingPayloadSize: 242 - len(items[0].FRMPayload),
						Confirmed:            false,
						Data:                 items[0].FRMPayload,
						FPort:                items[0].FPort,
						MoreData:             true,
					},
					// the seconds item should be returned as the first item
					// has been popped from the queue
					ExpectedNextDeviceQueueItem: &storage.DeviceQueueItem{
						DevEUI:     d.DevEUI,
						FRMPayload: []byte{4, 5, 6, 7},
						FPort:      1,
						FCnt:       11,
						Confirmed:  true,
					},
				},
				{
					BeforeFunc: func() {
						ctx.DeviceSession.FCntDown = 11 // skip first queue item
					},
					Name: "second queue item (confirmed)",
					ExpecteddataContext: dataContext{
						DeviceSession: storage.DeviceSession{
							RoutingProfileID: rp.RoutingProfile.RoutingProfileID,
							DevEUI:           d.DevEUI,
							FCntDown:         11,
						},
						RemainingPayloadSize: 242 - len(items[1].FRMPayload),
						Confirmed:            true,
						Data:                 items[1].FRMPayload,
						FPort:                items[1].FPort,
						MoreData:             false,
					},
					ExpectedNextDeviceQueueItem: &storage.DeviceQueueItem{
						DevEUI:     d.DevEUI,
						FRMPayload: []byte{4, 5, 6, 7},
						Confirmed:  true,
						FPort:      1,
						FCnt:       11,
						IsPending:  true,
					},
				},
			}

			for i, test := range tests {
				Convey(fmt.Sprintf("Testing: %s [%d]", test.Name, i), func() {
					if test.BeforeFunc != nil {
						test.BeforeFunc()
					}

					So(getNextDeviceQueueItem(&ctx), ShouldBeNil)
					So(test.ExpecteddataContext, ShouldResemble, ctx)

					if test.ExpectedNextDeviceQueueItem != nil {
						qi, err := storage.GetNextDeviceQueueItemForDevEUI(common.DB, d.DevEUI)
						So(err, ShouldBeNil)

						So(qi.FRMPayload, ShouldResemble, test.ExpectedNextDeviceQueueItem.FRMPayload)
						So(qi.FPort, ShouldEqual, test.ExpectedNextDeviceQueueItem.FPort)
						So(qi.FCnt, ShouldEqual, test.ExpectedNextDeviceQueueItem.FCnt)
						So(qi.IsPending, ShouldEqual, test.ExpectedNextDeviceQueueItem.IsPending)
						So(qi.Confirmed, ShouldEqual, test.ExpectedNextDeviceQueueItem.Confirmed)
						if test.ExpectedNextDeviceQueueItem.IsPending {
							So(qi.TimeoutAfter, ShouldNotBeNil)
						}
					}
				})
			}
		})
	})
}
