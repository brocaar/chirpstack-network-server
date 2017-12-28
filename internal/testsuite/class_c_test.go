package testsuite

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/downlink"
	"github.com/brocaar/loraserver/internal/maccommand"
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
	MACCommandQueue  []maccommand.Block              // downlink mac-command queue

	ExpectedSendDownlinkDataError error // expected error returned
	ExpectedFCntUp                uint32
	ExpectedFCntDown              uint32
	ExpectedTXInfo                *gw.TXInfo
	ExpectedPHYPayload            *lorawan.PHYPayload
	ExpectedMACCommandQueue       []maccommand.Block
}

func TestClassCScenarios(t *testing.T) {
	conf := test.GetConfig()
	db, err := common.OpenDatabase(conf.PostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	common.DB = db
	common.RedisPool = common.NewRedisPool(conf.RedisURL)

	Convey("Given a clean state", t, func() {
		test.MustResetDB(common.DB)
		test.MustFlushRedis(common.RedisPool)

		asClient := test.NewApplicationClient()
		common.ApplicationServerPool = test.NewApplicationServerPool(asClient)
		common.Gateway = test.NewGatewayBackend()

		sp := storage.ServiceProfile{
			ServiceProfile: backend.ServiceProfile{},
		}
		So(storage.CreateServiceProfile(common.DB, &sp), ShouldBeNil)

		dp := storage.DeviceProfile{
			DeviceProfile: backend.DeviceProfile{
				SupportsClassC: true,
			},
		}
		So(storage.CreateDeviceProfile(common.DB, &dp), ShouldBeNil)

		rp := storage.RoutingProfile{
			RoutingProfile: backend.RoutingProfile{
				ASID: "as-test:1234",
			},
		}
		So(storage.CreateRoutingProfile(common.DB, &rp), ShouldBeNil)

		d := storage.Device{
			DevEUI:           lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
			RoutingProfileID: rp.RoutingProfile.RoutingProfileID,
			ServiceProfileID: sp.ServiceProfile.ServiceProfileID,
			DeviceProfileID:  dp.DeviceProfile.DeviceProfileID,
		}
		So(storage.CreateDevice(common.DB, &d), ShouldBeNil)

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
			LastRXInfoSet: []gw.RXInfo{
				{MAC: lorawan.EUI64{1, 2, 1, 2, 1, 2, 1, 2}},
				{MAC: lorawan.EUI64{2, 1, 2, 1, 2, 1, 2, 1}},
			},
			RX2DR: 5,
		}

		txInfo := gw.TXInfo{
			MAC:         lorawan.EUI64{1, 2, 1, 2, 1, 2, 1, 2},
			Immediately: true,
			Frequency:   common.Band.RX2Frequency,
			Power:       common.Band.DefaultTXPower,
			DataRate:    common.Band.DataRates[sess.RX2DR],
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
					Name:          "mac-commands in the queue",
					DeviceSession: sess,
					MACCommandQueue: []maccommand.Block{
						{
							CID: lorawan.DevStatusReq,
						},
						{
							CID: lorawan.RXTimingSetupReq,
							MACCommands: []lorawan.MACCommand{
								{
									CID:     lorawan.RXTimingSetupAns,
									Payload: &lorawan.RXTimingSetupReqPayload{Delay: 3},
								},
							},
						},
					},
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
									{CID: lorawan.CID(8), Payload: &lorawan.RXTimingSetupReqPayload{Delay: 3}},
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
					So(storage.SaveDeviceSession(common.RedisPool, t.DeviceSession), ShouldBeNil)

					// mac mac-command queue items
					for _, qi := range t.MACCommandQueue {
						So(maccommand.AddQueueItem(common.RedisPool, t.DeviceSession.DevEUI, qi), ShouldBeNil)
					}

					// add device-queue items
					for _, qi := range t.DeviceQueueItems {
						So(storage.CreateDeviceQueueItem(common.DB, &qi), ShouldBeNil)
					}

					// run queue scheduler
					So(downlink.ClassCScheduleBatch(1), ShouldBeNil)

					Convey("Then the frame-counters are as expected", func() {
						sess, err := storage.GetDeviceSession(common.RedisPool, t.DeviceSession.DevEUI)
						So(err, ShouldBeNil)
						So(sess.FCntUp, ShouldEqual, t.ExpectedFCntUp)
						So(sess.FCntDown, ShouldEqual, t.ExpectedFCntDown)
					})

					if t.ExpectedTXInfo != nil && t.ExpectedPHYPayload != nil {
						Convey("Then the expected frame was sent", func() {
							So(common.Gateway.(*test.GatewayBackend).TXPacketChan, ShouldHaveLength, 1)
							txPacket := <-common.Gateway.(*test.GatewayBackend).TXPacketChan
							So(txPacket.Token, ShouldNotEqual, 0)
							So(&txPacket.TXInfo, ShouldResemble, t.ExpectedTXInfo)
						})
					} else {
						So(common.Gateway.(*test.GatewayBackend).TXPacketChan, ShouldHaveLength, 0)
					}

					Convey("Then the mac-command queue contains the expected items", func() {
						items, err := maccommand.ReadQueueItems(common.RedisPool, t.DeviceSession.DevEUI)
						So(err, ShouldBeNil)
						So(items, ShouldResemble, t.ExpectedMACCommandQueue)
					})
				})
			}
		})
	})
}
