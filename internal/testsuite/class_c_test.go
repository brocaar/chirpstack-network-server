package testsuite

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/downlink"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
)

type classCTestCase struct {
	Name             string                          // name of the test
	PreFunc          func(ns *storage.DeviceSession) // function to call before running the test
	DeviceSession    storage.DeviceSession           // node-session in the storage
	DeviceQueueItems []storage.DeviceQueueItem       // items in the device-queue

	ExpectedSendDownlinkDataError error // expected error returned
	ExpectedFCntUp                uint32
	ExpectedFCntDown              uint32
	ExpectedTXInfo                *gw.TXInfo
	ExpectedPHYPayload            *lorawan.PHYPayload
}

func TestClassCScenarios(t *testing.T) {
	conf := test.GetConfig()
	db, err := common.OpenDatabase(conf.PostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	config.C.PostgreSQL.DB = db
	config.C.Redis.Pool = common.NewRedisPool(conf.RedisURL)

	Convey("Given a clean state", t, func() {
		test.MustResetDB(config.C.PostgreSQL.DB)
		test.MustFlushRedis(config.C.Redis.Pool)

		asClient := test.NewApplicationClient()
		config.C.ApplicationServer.Pool = test.NewApplicationServerPool(asClient)
		config.C.NetworkServer.Gateway.Backend.Backend = test.NewGatewayBackend()

		sp := storage.ServiceProfile{
			ServiceProfile: backend.ServiceProfile{},
		}
		So(storage.CreateServiceProfile(config.C.PostgreSQL.DB, &sp), ShouldBeNil)

		dp := storage.DeviceProfile{
			DeviceProfile: backend.DeviceProfile{
				SupportsClassC: true,
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
			ServiceProfileID: d.ServiceProfileID,
			RoutingProfileID: d.RoutingProfileID,
			DeviceProfileID:  d.DeviceProfileID,
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
			EnabledChannels: []int{0, 1, 2},
			RX2DR:           5,
		}

		txInfo := gw.TXInfo{
			MAC:         lorawan.EUI64{1, 2, 1, 2, 1, 2, 1, 2},
			Immediately: true,
			Frequency:   config.C.NetworkServer.Band.Band.RX2Frequency,
			Power:       config.C.NetworkServer.Band.Band.DefaultTXPower,
			DataRate:    config.C.NetworkServer.Band.Band.DataRates[sess.RX2DR],
			CodeRate:    "4/5",
		}

		fPortTen := uint8(10)

		Convey("Given a set of test-scenarios for Class-C", func() {
			tests := []classCTestCase{
				{
					Name:          "unconfirmed data",
					DeviceSession: sess,
					DeviceQueueItems: []storage.DeviceQueueItem{
						{DevEUI: sess.DevEUI, FPort: 10, FCnt: 5, FRMPayload: make([]byte, 242)},
					},

					ExpectedFCntUp:   8,
					ExpectedFCntDown: 6,
					ExpectedTXInfo:   &txInfo,
					ExpectedPHYPayload: &lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataDown,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: sess.DevAddr,
								FCnt:    5,
								FCtrl:   lorawan.FCtrl{},
							},
							FPort: &fPortTen,
							FRMPayload: []lorawan.Payload{
								&lorawan.DataPayload{Bytes: make([]byte, 242)},
							},
						},
					},
				},
				{
					Name:          "unconfirmed data (only the first item is emitted because of class-c downlink lock",
					DeviceSession: sess,
					DeviceQueueItems: []storage.DeviceQueueItem{
						{DevEUI: sess.DevEUI, FPort: 10, FCnt: 5, FRMPayload: make([]byte, 242)},
						{DevEUI: sess.DevEUI, FPort: 10, FCnt: 6, FRMPayload: make([]byte, 242)},
					},

					ExpectedFCntUp:   8,
					ExpectedFCntDown: 6,
					ExpectedTXInfo:   &txInfo,
					ExpectedPHYPayload: &lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataDown,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: sess.DevAddr,
								FCnt:    5,
								FCtrl:   lorawan.FCtrl{},
							},
							FPort: &fPortTen,
							FRMPayload: []lorawan.Payload{
								&lorawan.DataPayload{Bytes: make([]byte, 242)},
							},
						},
					},
				},
				{
					Name:          "confirmed data",
					DeviceSession: sess,
					DeviceQueueItems: []storage.DeviceQueueItem{
						{DevEUI: sess.DevEUI, FPort: 10, FCnt: 5, Confirmed: true, FRMPayload: []byte{5, 4, 3, 2, 1}},
					},

					ExpectedFCntUp:   8,
					ExpectedFCntDown: 6,
					ExpectedTXInfo:   &txInfo,
					ExpectedPHYPayload: &lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.ConfirmedDataDown,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: sess.DevAddr,
								FCnt:    5,
								FCtrl:   lorawan.FCtrl{},
							},
							FPort: &fPortTen,
							FRMPayload: []lorawan.Payload{
								&lorawan.DataPayload{Bytes: []byte{5, 4, 3, 2, 1}},
							},
						},
					},
				},
				{
					PreFunc: func(ds *storage.DeviceSession) {
						sp.ServiceProfile.DevStatusReqFreq = 1
						So(storage.UpdateServiceProfile(config.C.PostgreSQL.DB, &sp), ShouldBeNil)
					},
					Name:          "with mac-command",
					DeviceSession: sess,
					DeviceQueueItems: []storage.DeviceQueueItem{
						{DevEUI: sess.DevEUI, FPort: 10, FCnt: 5, FRMPayload: []byte{5, 4, 3, 2, 1}},
					},

					ExpectedFCntUp:   8,
					ExpectedFCntDown: 6,
					ExpectedTXInfo:   &txInfo,
					ExpectedPHYPayload: &lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataDown,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: sess.DevAddr,
								FCnt:    5,
								FCtrl:   lorawan.FCtrl{},
								FOpts: []lorawan.MACCommand{
									{CID: lorawan.CID(6)},
								},
							},
							FPort: &fPortTen,
							FRMPayload: []lorawan.Payload{
								&lorawan.DataPayload{Bytes: []byte{5, 4, 3, 2, 1}},
							},
						},
					},
				},
				// nothing scheduled, e.g. the queue item was discarded
				{
					Name:          "queue item discarded (e.g. max payload size exceeded, wrong frame-counter, ...)",
					DeviceSession: sess,
					DeviceQueueItems: []storage.DeviceQueueItem{
						{DevEUI: d.DevEUI, FPort: 10, FCnt: 10, FRMPayload: make([]byte, 300)},
					},
					ExpectedFCntUp:   8,
					ExpectedFCntDown: 5,
				},
			}

			for i, t := range tests {
				Convey(fmt.Sprintf("Testing: %s [%d]", t.Name, i), func() {
					if t.PreFunc != nil {
						t.PreFunc(&t.DeviceSession)
					}

					// create device-session
					So(storage.SaveDeviceSession(config.C.Redis.Pool, t.DeviceSession), ShouldBeNil)

					// add device-queue items
					for _, qi := range t.DeviceQueueItems {
						So(storage.CreateDeviceQueueItem(config.C.PostgreSQL.DB, &qi), ShouldBeNil)
					}

					// run queue scheduler
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
							So(&txPacket.TXInfo, ShouldResemble, t.ExpectedTXInfo)
						})
					} else {
						So(config.C.NetworkServer.Gateway.Backend.Backend.(*test.GatewayBackend).TXPacketChan, ShouldHaveLength, 0)
					}
				})
			}
		})
	})
}
