package testsuite

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/api/ns"
	"github.com/brocaar/loraserver/internal/api"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/maccommand"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
)

type classCTestCase struct {
	Name                    string                          // name of the test
	PreFunc                 func(ns *storage.DeviceSession) // function to call before running the test
	DeviceSession           storage.DeviceSession           // node-session in the storage
	SendDownlinkDataRequest ns.SendDownlinkDataRequest      // class-c push data-down request
	MACCommandQueue         []maccommand.Block              // downlink mac-command queue

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

		api := api.NewNetworkServerAPI()

		sp := storage.ServiceProfile{
			ServiceProfile: backend.ServiceProfile{},
		}
		So(storage.CreateServiceProfile(common.DB, &sp), ShouldBeNil)

		dp := storage.DeviceProfile{
			DeviceProfile: backend.DeviceProfile{},
		}
		So(storage.CreateDeviceProfile(common.DB, &dp), ShouldBeNil)

		rp := storage.RoutingProfile{
			RoutingProfile: backend.RoutingProfile{
				ASID: "as-test:1234",
			},
		}
		So(storage.CreateRoutingProfile(common.DB, &rp), ShouldBeNil)

		sess := storage.DeviceSession{
			ServiceProfileID: sp.ServiceProfile.ServiceProfileID,
			DeviceProfileID:  dp.DeviceProfile.DeviceProfileID,
			RoutingProfileID: rp.RoutingProfile.RoutingProfileID,
			DevAddr:          lorawan.DevAddr{1, 2, 3, 4},
			DevEUI:           lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
			JoinEUI:          lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
			NwkSKey:          lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			FCntUp:           8,
			FCntDown:         5,
			LastRXInfoSet: []gw.RXInfo{
				{MAC: lorawan.EUI64{1, 2, 1, 2, 1, 2, 1, 2}},
				{MAC: lorawan.EUI64{2, 1, 2, 1, 2, 1, 2, 1}},
			},
			RX2DR: 1,
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
					SendDownlinkDataRequest: ns.SendDownlinkDataRequest{
						DevEUI:    []byte{1, 2, 3, 4, 5, 6, 7, 8},
						Data:      []byte{5, 4, 3, 2, 1},
						Confirmed: false,
						FPort:     10,
						FCnt:      5,
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
								&lorawan.DataPayload{Bytes: []byte{5, 4, 3, 2, 1}},
							},
						},
					},
				},
				{
					Name:          "confirmed data",
					DeviceSession: sess,
					SendDownlinkDataRequest: ns.SendDownlinkDataRequest{
						DevEUI:    []byte{1, 2, 3, 4, 5, 6, 7, 8},
						Data:      []byte{5, 4, 3, 2, 1},
						Confirmed: true,
						FPort:     10,
						FCnt:      5,
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
					SendDownlinkDataRequest: ns.SendDownlinkDataRequest{
						DevEUI: []byte{1, 2, 3, 4, 5, 6, 7, 8},
						Data:   []byte{5, 4, 3, 2, 1},
						FPort:  10,
						FCnt:   5,
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
				// errors
				{
					Name:          "maximum payload exceeded",
					DeviceSession: sess,
					SendDownlinkDataRequest: ns.SendDownlinkDataRequest{
						DevEUI:    []byte{1, 2, 3, 4, 5, 6, 7, 8},
						Data:      make([]byte, 300),
						Confirmed: false,
						FPort:     10,
						FCnt:      5,
					},

					ExpectedSendDownlinkDataError: grpc.Errorf(codes.InvalidArgument, "maximum payload size exceeded"),
					ExpectedFCntUp:                8,
					ExpectedFCntDown:              5,
				},
				{
					Name:          "invalid FPort",
					DeviceSession: sess,
					SendDownlinkDataRequest: ns.SendDownlinkDataRequest{
						DevEUI:    []byte{1, 2, 3, 4, 5, 6, 7, 8},
						Data:      []byte{1, 2, 3, 4, 5},
						Confirmed: false,
						FPort:     0,
						FCnt:      5,
					},
					ExpectedSendDownlinkDataError: grpc.Errorf(codes.InvalidArgument, "FPort must not be 0"),
					ExpectedFCntUp:                8,
					ExpectedFCntDown:              5,
				},
				{
					Name:          "invalid DevEUI",
					DeviceSession: sess,
					SendDownlinkDataRequest: ns.SendDownlinkDataRequest{
						DevEUI:    []byte{1, 1, 1, 1, 1, 1, 1, 1, 1},
						Data:      []byte{1, 2, 3, 4, 5},
						Confirmed: false,
						FPort:     10,
						FCnt:      5,
					},
					ExpectedSendDownlinkDataError: grpc.Errorf(codes.NotFound, storage.ErrDoesNotExist.Error()),
					ExpectedFCntUp:                8,
					ExpectedFCntDown:              5,
				},
				{
					Name:          "no last RXInfoSet available",
					DeviceSession: sess,
					PreFunc: func(ns *storage.DeviceSession) {
						ns.LastRXInfoSet = []gw.RXInfo{}
					},
					SendDownlinkDataRequest: ns.SendDownlinkDataRequest{
						DevEUI:    []byte{1, 2, 3, 4, 5, 6, 7, 8},
						Data:      []byte{1, 2, 3, 4, 5},
						Confirmed: false,
						FPort:     10,
						FCnt:      5,
					},
					ExpectedSendDownlinkDataError: grpc.Errorf(codes.FailedPrecondition, "no last RX-Info set available"),
					ExpectedFCntUp:                8,
					ExpectedFCntDown:              5,
				},
				{
					Name:          "given FCnt does not match the FCntDown",
					DeviceSession: sess,
					SendDownlinkDataRequest: ns.SendDownlinkDataRequest{
						DevEUI:    []byte{1, 2, 3, 4, 5, 6, 7, 8},
						Data:      []byte{1, 2, 3, 4, 5},
						Confirmed: false,
						FPort:     10,
						FCnt:      6,
					},
					ExpectedSendDownlinkDataError: grpc.Errorf(codes.InvalidArgument, "invalid FCnt (expected: 5)"),
					ExpectedFCntUp:                8,
					ExpectedFCntDown:              5,
				},
			}

			for i, t := range tests {
				Convey(fmt.Sprintf("When testing: %s [%d]", t.Name, i), func() {
					if t.PreFunc != nil {
						t.PreFunc(&t.DeviceSession)
					}

					// create node-session
					So(storage.SaveDeviceSession(common.RedisPool, t.DeviceSession), ShouldBeNil)

					// mac mac-command queue items
					for _, qi := range t.MACCommandQueue {
						So(maccommand.AddQueueItem(common.RedisPool, t.DeviceSession.DevEUI, qi), ShouldBeNil)
					}

					// push the data
					_, err := api.SendDownlinkData(context.Background(), &t.SendDownlinkDataRequest)
					So(err, ShouldResemble, t.ExpectedSendDownlinkDataError)

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
