package testsuite

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/gateway"
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

			DevAddr:         lorawan.DevAddr{1, 2, 3, 4},
			NwkSKey:         [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			FCntUp:          8,
			FCntDown:        5,
			EnabledChannels: []int{0, 1, 2},
			PingSlotNb:      1,
		}

		now := time.Now().UTC().Truncate(time.Millisecond)
		timeSinceEpoch := gw.Duration(10 * time.Second)
		rxInfo := gw.RXInfo{
			MAC:               [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			Frequency:         config.C.NetworkServer.Band.Band.UplinkChannels[0].Frequency,
			DataRate:          config.C.NetworkServer.Band.Band.DataRates[0],
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
