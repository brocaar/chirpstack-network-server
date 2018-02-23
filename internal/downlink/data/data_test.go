package data

import (
	"fmt"
	"testing"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
	. "github.com/smartystreets/goconvey/convey"
)

func TestGetNextDeviceQueueItem(t *testing.T) {
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
				So(storage.CreateDeviceQueueItem(config.C.PostgreSQL.DB, &items[i]), ShouldBeNil)
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
						qi, err := storage.GetNextDeviceQueueItemForDevEUI(config.C.PostgreSQL.DB, d.DevEUI)
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

func TestSetMACCommandsSet(t *testing.T) {
	conf := test.GetConfig()
	config.C.Redis.Pool = common.NewRedisPool(conf.RedisURL)
	test.MustFlushRedis(config.C.Redis.Pool)

	Convey("Given a set of tests", t, func() {
		tests := []struct {
			BeforeFunc          func() error
			Name                string
			Context             dataContext
			ExpectedMACCommands []storage.MACCommandBlock
		}{
			{
				Name: "trigger channel-reconfiguration",
				Context: dataContext{
					RemainingPayloadSize: 200,
					DeviceSession: storage.DeviceSession{
						EnabledUplinkChannels: []int{0, 1},
						TXPowerIndex:          2,
						DR:                    5,
						NbTrans:               2,
					},
				},
				ExpectedMACCommands: []storage.MACCommandBlock{
					{
						CID: lorawan.LinkADRReq,
						MACCommands: storage.MACCommands{
							{
								CID: lorawan.LinkADRReq,
								Payload: &lorawan.LinkADRReqPayload{
									DataRate: 5,
									TXPower:  2,
									ChMask:   [16]bool{true, true, true},
									Redundancy: lorawan.Redundancy{
										NbRep: 2,
									},
								},
							},
						},
					},
				},
			},
			{
				Name: "trigger adr request change",
				Context: dataContext{
					RemainingPayloadSize: 200,
					DeviceSession: storage.DeviceSession{
						ADR: true,
						DR:  0,
						UplinkHistory: []storage.UplinkHistory{
							{FCnt: 0, MaxSNR: 5, TXPowerIndex: 0, GatewayCount: 1},
						},
					},
				},
				ExpectedMACCommands: []storage.MACCommandBlock{
					{
						CID: lorawan.LinkADRReq,
						MACCommands: storage.MACCommands{
							{
								CID: lorawan.LinkADRReq,
								Payload: &lorawan.LinkADRReqPayload{
									DataRate: 5,
									TXPower:  3,
									ChMask:   [16]bool{true, true, true},
									Redundancy: lorawan.Redundancy{
										NbRep: 1,
									},
								},
							},
						},
					},
				},
			},
			{
				Name: "request device-status",
				Context: dataContext{
					RemainingPayloadSize: 200,
					ServiceProfile: storage.ServiceProfile{
						ServiceProfile: backend.ServiceProfile{
							DevStatusReqFreq: 1,
						},
					},
					DeviceSession: storage.DeviceSession{
						EnabledUplinkChannels: []int{0, 1, 2},
					},
				},
				ExpectedMACCommands: []storage.MACCommandBlock{
					{
						CID: lorawan.DevStatusReq,
						MACCommands: storage.MACCommands{
							{
								CID: lorawan.DevStatusReq,
							},
						},
					},
				},
			},
			{
				BeforeFunc: func() error {
					config.C.NetworkServer.NetworkSettings.ClassB.PingSlotDR = 3
					config.C.NetworkServer.NetworkSettings.ClassB.PingSlotFrequency = 868100000
					return nil
				},
				Name: "trigger ping-slot parameters",
				Context: dataContext{
					RemainingPayloadSize: 200,
					DeviceProfile: storage.DeviceProfile{
						DeviceProfile: backend.DeviceProfile{
							SupportsClassB: true,
						},
					},
					DeviceSession: storage.DeviceSession{
						PingSlotDR:            2,
						PingSlotFrequency:     868300000,
						EnabledUplinkChannels: []int{0, 1, 2},
					},
				},
				ExpectedMACCommands: []storage.MACCommandBlock{
					{
						CID: lorawan.PingSlotChannelReq,
						MACCommands: storage.MACCommands{
							{
								CID: lorawan.PingSlotChannelReq,
								Payload: &lorawan.PingSlotChannelReqPayload{
									Frequency: 868100000,
									DR:        3,
								},
							},
						},
					},
				},
			},
		}

		for i, test := range tests {
			Convey(fmt.Sprintf("Testing: %s [%d]", test.Name, i), func() {
				if test.BeforeFunc != nil {
					So(test.BeforeFunc(), ShouldBeNil)
				}

				err := setMACCommandsSet(&test.Context)
				So(err, ShouldBeNil)

				So(test.Context.MACCommands, ShouldResemble, test.ExpectedMACCommands)
			})
		}
	})
}
