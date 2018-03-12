package testsuite

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/downlink"
	"github.com/brocaar/loraserver/internal/gateway"
	"github.com/brocaar/loraserver/internal/gps"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/loraserver/internal/uplink"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
)

type uplinkClassBTestCase struct {
	BeforeFunc           func(*uplinkClassBTestCase) error
	Name                 string
	DeviceSession        storage.DeviceSession
	RXInfo               gw.RXInfo
	PHYPayload           lorawan.PHYPayload
	ExpectedBeaconLocked bool
}

func TestClassBUplink(t *testing.T) {
	conf := test.GetConfig()
	db, err := common.OpenDatabase(conf.PostgresDSN)
	if err != nil {
		t.Fatal(err)
	}

	config.C.PostgreSQL.DB = db
	config.C.Redis.Pool = common.NewRedisPool(conf.RedisURL)

	Convey("Given a clean database with test-data", t, func() {
		test.MustFlushRedis(config.C.Redis.Pool)
		test.MustResetDB(config.C.PostgreSQL.DB)

		asClient := test.NewApplicationClient()
		config.C.ApplicationServer.Pool = test.NewApplicationServerPool(asClient)
		config.C.NetworkController.Client = test.NewNetworkControllerClient()

		gw1 := gateway.Gateway{
			MAC:  [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			Name: "test-gateway",
			Location: gateway.GPSPoint{
				Latitude:  1.1234,
				Longitude: 1.1235,
			},
			Altitude: 10.5,
		}
		So(gateway.CreateGateway(db, &gw1), ShouldBeNil)

		// service-profile
		sp := storage.ServiceProfile{
			ServiceProfile: backend.ServiceProfile{
				AddGWMetadata: true,
			},
		}
		So(storage.CreateServiceProfile(config.C.PostgreSQL.DB, &sp), ShouldBeNil)

		// device-profile
		dp := storage.DeviceProfile{
			DeviceProfile: backend.DeviceProfile{},
		}
		So(storage.CreateDeviceProfile(config.C.PostgreSQL.DB, &dp), ShouldBeNil)

		// routing-profile
		rp := storage.RoutingProfile{
			RoutingProfile: backend.RoutingProfile{},
		}
		So(storage.CreateRoutingProfile(config.C.PostgreSQL.DB, &rp), ShouldBeNil)

		// device
		d := storage.Device{
			ServiceProfileID: sp.ServiceProfile.ServiceProfileID,
			DeviceProfileID:  dp.DeviceProfile.DeviceProfileID,
			RoutingProfileID: rp.RoutingProfile.RoutingProfileID,
			DevEUI:           lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
		}
		So(storage.CreateDevice(config.C.PostgreSQL.DB, &d), ShouldBeNil)

		queueItems := []storage.DeviceQueueItem{
			{
				DevEUI:     d.DevEUI,
				FRMPayload: []byte{1, 2, 3, 4},
				FPort:      1,
				FCnt:       1,
			},
			{
				DevEUI:     d.DevEUI,
				FRMPayload: []byte{1, 2, 3, 4},
				FPort:      1,
				FCnt:       2,
			},
			{
				DevEUI:     d.DevEUI,
				FRMPayload: []byte{1, 2, 3, 4},
				FPort:      1,
				FCnt:       3,
			},
		}
		for i := range queueItems {
			So(storage.CreateDeviceQueueItem(config.C.PostgreSQL.DB, &queueItems[i]), ShouldBeNil)
		}

		// device-session
		ds := storage.DeviceSession{
			DeviceProfileID:  d.DeviceProfileID,
			ServiceProfileID: d.ServiceProfileID,
			RoutingProfileID: d.RoutingProfileID,
			DevEUI:           d.DevEUI,
			JoinEUI:          lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},

			DevAddr:               lorawan.DevAddr{1, 2, 3, 4},
			NwkSKey:               [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			FCntUp:                8,
			FCntDown:              5,
			EnabledUplinkChannels: []int{0, 1, 2},
			PingSlotNb:            1,
		}

		now := time.Now().UTC().Truncate(time.Millisecond)
		timeSinceEpoch := gw.Duration(10 * time.Second)
		dr0, err := config.C.NetworkServer.Band.Band.GetDataRate(0)
		So(err, ShouldBeNil)
		c0, err := config.C.NetworkServer.Band.Band.GetUplinkChannel(0)
		So(err, ShouldBeNil)
		rxInfo := gw.RXInfo{
			MAC:               [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			Frequency:         c0.Frequency,
			DataRate:          dr0,
			LoRaSNR:           7,
			Time:              &now,
			TimeSinceGPSEpoch: &timeSinceEpoch,
		}

		Convey("Given a set of test-scenarios", func() {
			tests := []uplinkClassBTestCase{
				{
					Name:          "trigger beacon locked",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							Major: lorawan.LoRaWANR1,
							MType: lorawan.UnconfirmedDataUp,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    ds.FCntUp,
								FCtrl: lorawan.FCtrl{
									ClassB: true,
								},
							},
						},
					},
					ExpectedBeaconLocked: true,
				},
				{
					BeforeFunc: func(tc *uplinkClassBTestCase) error {
						tc.DeviceSession.BeaconLocked = true
						return nil
					},
					Name:          "trigger beacon unlocked",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							Major: lorawan.LoRaWANR1,
							MType: lorawan.UnconfirmedDataUp,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    ds.FCntUp,
								FCtrl: lorawan.FCtrl{
									ClassB: false,
								},
							},
						},
					},
					ExpectedBeaconLocked: false,
				},
			}

			for i, t := range tests {
				Convey(fmt.Sprintf("testing: %s [%d]", t.Name, i), func() {
					if t.BeforeFunc != nil {
						So(t.BeforeFunc(&t), ShouldBeNil)
					}

					// create device-session
					So(storage.SaveDeviceSession(config.C.Redis.Pool, t.DeviceSession), ShouldBeNil)

					// set MIC
					So(t.PHYPayload.SetMIC(t.DeviceSession.NwkSKey), ShouldBeNil)

					// create RXPacket and call HandleRXPacket
					rxPacket := gw.RXPacket{
						RXInfo:     t.RXInfo,
						PHYPayload: t.PHYPayload,
					}
					So(uplink.HandleRXPacket(rxPacket), ShouldBeNil)

					ds, err := storage.GetDeviceSession(config.C.Redis.Pool, t.DeviceSession.DevEUI)
					So(err, ShouldBeNil)
					So(ds.BeaconLocked, ShouldEqual, t.ExpectedBeaconLocked)

					if t.ExpectedBeaconLocked {
						queueItems, err := storage.GetDeviceQueueItemsForDevEUI(config.C.PostgreSQL.DB, t.DeviceSession.DevEUI)
						So(err, ShouldBeNil)

						for _, qi := range queueItems {
							So(qi.EmitAtTimeSinceGPSEpoch, ShouldNotBeNil)
							So(qi.TimeoutAfter, ShouldNotBeNil)
						}
					}
				})
			}
		})
	})
}

type downlinkClassBTestCase struct {
	BeforeFunc       func(*downlinkClassBTestCase) error
	Name             string
	DeviceSession    storage.DeviceSession
	DeviceQueueItems []storage.DeviceQueueItem

	ExpectedFCntUp     uint32
	ExpectedFCntDown   uint32
	ExpectedTXInfo     *gw.TXInfo
	ExpectedPHYPayload *lorawan.PHYPayload
}

func TestClassBDownlink(t *testing.T) {
	conf := test.GetConfig()
	db, err := common.OpenDatabase(conf.PostgresDSN)
	if err != nil {
		t.Fatal(err)
	}

	config.C.PostgreSQL.DB = db
	config.C.Redis.Pool = common.NewRedisPool(conf.RedisURL)

	Convey("Given a clean database with test-data", t, func() {
		test.MustResetDB(config.C.PostgreSQL.DB)
		test.MustFlushRedis(config.C.Redis.Pool)

		config.C.NetworkServer.Gateway.Backend.Backend = test.NewGatewayBackend()
		config.C.NetworkServer.NetworkSettings.ClassB.PingSlotDR = 2
		config.C.NetworkServer.NetworkSettings.ClassB.PingSlotFrequency = 868300000

		sp := storage.ServiceProfile{
			ServiceProfile: backend.ServiceProfile{},
		}
		So(storage.CreateServiceProfile(config.C.PostgreSQL.DB, &sp), ShouldBeNil)

		dp := storage.DeviceProfile{
			DeviceProfile: backend.DeviceProfile{
				SupportsClassB: true,
			},
		}
		So(storage.CreateDeviceProfile(config.C.PostgreSQL.DB, &dp), ShouldBeNil)

		rp := storage.RoutingProfile{
			RoutingProfile: backend.RoutingProfile{
				ASID: "as-test:1234",
			},
		}
		So(storage.CreateRoutingProfile(config.C.PostgreSQL.DB, &rp), ShouldBeNil)

		d := storage.Device{
			DevEUI:           lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
			RoutingProfileID: rp.RoutingProfile.RoutingProfileID,
			ServiceProfileID: sp.ServiceProfile.ServiceProfileID,
			DeviceProfileID:  dp.DeviceProfile.DeviceProfileID,
		}
		So(storage.CreateDevice(config.C.PostgreSQL.DB, &d), ShouldBeNil)

		sess := storage.DeviceSession{
			RoutingProfileID: rp.RoutingProfile.RoutingProfileID,
			ServiceProfileID: sp.ServiceProfile.ServiceProfileID,
			DeviceProfileID:  dp.DeviceProfile.DeviceProfileID,
			DevEUI:           d.DevEUI,
			DevAddr:          lorawan.DevAddr{1, 2, 3, 4},
			JoinEUI:          lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
			NwkSKey:          lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			FCntUp:           8,
			FCntDown:         5,
			LastRXInfoSet: []models.RXInfo{
				{MAC: lorawan.EUI64{1, 2, 1, 2, 1, 2, 1, 2}},
				{MAC: lorawan.EUI64{2, 1, 2, 1, 2, 1, 2, 1}},
			},
			EnabledUplinkChannels: []int{0, 1, 2},
			BeaconLocked:          true,
			PingSlotFrequency:     868300000,
			PingSlotDR:            2,
		}

		fPortTen := uint8(10)
		emitTime := gps.Time(time.Now()).TimeSinceGPSEpoch()
		emitTimeGW := gw.Duration(emitTime)
		dr2, err := config.C.NetworkServer.Band.Band.GetDataRate(2)
		So(err, ShouldBeNil)

		txInfo := gw.TXInfo{
			MAC:               lorawan.EUI64{1, 2, 1, 2, 1, 2, 1, 2},
			Frequency:         sess.PingSlotFrequency,
			Power:             config.C.NetworkServer.Band.Band.GetDownlinkTXPower(sess.PingSlotFrequency),
			DataRate:          dr2,
			CodeRate:          "4/5",
			TimeSinceGPSEpoch: &emitTimeGW,
		}

		tests := []downlinkClassBTestCase{
			{
				Name:          "class-b downlink",
				DeviceSession: sess,
				DeviceQueueItems: []storage.DeviceQueueItem{
					{DevEUI: sess.DevEUI, FPort: 10, FCnt: 5, FRMPayload: []byte{1, 2, 3}, EmitAtTimeSinceGPSEpoch: &emitTime},
				},
				ExpectedFCntUp:   8,
				ExpectedFCntDown: 6,
				ExpectedTXInfo:   &txInfo,
				ExpectedPHYPayload: &lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MIC: lorawan.MIC{164, 6, 172, 129},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: sess.DevAddr,
							FCnt:    5,
							FCtrl: lorawan.FCtrl{
								ADR: true,
							},
						},
						FPort: &fPortTen,
						FRMPayload: []lorawan.Payload{
							&lorawan.DataPayload{Bytes: []byte{1, 2, 3}},
						},
					},
				},
			},
			{
				Name:          "class-b downlink with more data",
				DeviceSession: sess,
				DeviceQueueItems: []storage.DeviceQueueItem{
					{DevEUI: sess.DevEUI, FPort: 10, FCnt: 5, FRMPayload: []byte{1, 2, 3}, EmitAtTimeSinceGPSEpoch: &emitTime},
					{DevEUI: sess.DevEUI, FPort: 10, FCnt: 6, FRMPayload: []byte{1, 2, 3}, EmitAtTimeSinceGPSEpoch: &emitTime},
				},
				ExpectedFCntUp:   8,
				ExpectedFCntDown: 6,
				ExpectedTXInfo:   &txInfo,
				ExpectedPHYPayload: &lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MIC: lorawan.MIC{39, 244, 225, 101},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: sess.DevAddr,
							FCnt:    5,
							FCtrl: lorawan.FCtrl{
								ADR:      true,
								FPending: true,
							},
						},
						FPort: &fPortTen,
						FRMPayload: []lorawan.Payload{
							&lorawan.DataPayload{Bytes: []byte{1, 2, 3}},
						},
					},
				},
			},
			{
				BeforeFunc: func(tc *downlinkClassBTestCase) error {
					tc.DeviceSession.BeaconLocked = false
					return nil
				},
				Name:          "class-b downlink, but no beacon lock",
				DeviceSession: sess,
				DeviceQueueItems: []storage.DeviceQueueItem{
					{DevEUI: sess.DevEUI, FPort: 10, FCnt: 5, FRMPayload: []byte{1, 2, 3}, EmitAtTimeSinceGPSEpoch: &emitTime},
				},
				ExpectedFCntUp:   8,
				ExpectedFCntDown: 5,
			},
			{
				BeforeFunc: func(tc *downlinkClassBTestCase) error {
					config.C.NetworkServer.NetworkSettings.ClassB.PingSlotFrequency = 0

					tc.DeviceSession.PingSlotFrequency = 0
					tc.ExpectedTXInfo.Frequency = 869525000
					return nil
				},
				Name:          "class-b downlink, with default band frequency plan",
				DeviceSession: sess,
				DeviceQueueItems: []storage.DeviceQueueItem{
					{DevEUI: sess.DevEUI, FPort: 10, FCnt: 5, FRMPayload: []byte{1, 2, 3}, EmitAtTimeSinceGPSEpoch: &emitTime},
				},
				ExpectedFCntUp:   8,
				ExpectedFCntDown: 6,
				ExpectedTXInfo:   &txInfo,
				ExpectedPHYPayload: &lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MIC: lorawan.MIC{164, 6, 172, 129},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: sess.DevAddr,
							FCnt:    5,
							FCtrl: lorawan.FCtrl{
								ADR: true,
							},
						},
						FPort: &fPortTen,
						FRMPayload: []lorawan.Payload{
							&lorawan.DataPayload{Bytes: []byte{1, 2, 3}},
						},
					},
				},
			},
		}

		for i, t := range tests {
			Convey(fmt.Sprintf("Testing: %s [%d]", t.Name, i), func() {
				if t.BeforeFunc != nil {
					So(t.BeforeFunc(&t), ShouldBeNil)
				}

				So(storage.SaveDeviceSession(config.C.Redis.Pool, t.DeviceSession), ShouldBeNil)

				for _, qi := range t.DeviceQueueItems {
					So(storage.CreateDeviceQueueItem(config.C.PostgreSQL.DB, &qi), ShouldBeNil)
				}

				So(downlink.ScheduleBatch(1), ShouldBeNil)

				Convey("Then the frame-counters are as expected", func() {
					sess, err := storage.GetDeviceSession(config.C.Redis.Pool, t.DeviceSession.DevEUI)
					So(err, ShouldBeNil)
					So(sess.FCntUp, ShouldEqual, t.ExpectedFCntUp)
					So(sess.FCntDown, ShouldEqual, t.ExpectedFCntDown)
				})

				if t.ExpectedTXInfo != nil && t.ExpectedPHYPayload != nil {
					Convey("Then the expected frame was sent", func() {
						So(config.C.NetworkServer.Gateway.Backend.Backend.(*test.GatewayBackend).TXPacketChan, ShouldHaveLength, 1)
						txPacket := <-config.C.NetworkServer.Gateway.Backend.Backend.(*test.GatewayBackend).TXPacketChan
						So(txPacket.Token, ShouldNotEqual, 0)
						So(txPacket.TXInfo, ShouldResemble, *t.ExpectedTXInfo)
						So(txPacket.PHYPayload, ShouldResemble, *t.ExpectedPHYPayload)
					})
				} else {
					So(config.C.NetworkServer.Gateway.Backend.Backend.(*test.GatewayBackend).TXPacketChan, ShouldHaveLength, 0)
				}
			})
		}
	})
}
