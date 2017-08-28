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
	"github.com/brocaar/loraserver/internal/session"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
)

type classCTestCase struct {
	Name                string                        // name of the test
	PreFunc             func(ns *session.NodeSession) // function to call before running the test
	NodeSession         session.NodeSession           // node-session in the storage
	PushDataDownRequest ns.PushDataDownRequest        // class-c push data-down request
	MACCommandQueue     []maccommand.Block            // downlink mac-command queue

	ExpectedPushDataDownError error // expected error returned
	ExpectedFCntUp            uint32
	ExpectedFCntDown          uint32
	ExpectedTXInfo            *gw.TXInfo
	ExpectedPHYPayload        *lorawan.PHYPayload
	ExpectedMACCommandQueue   []maccommand.Block
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

		common.Gateway = test.NewGatewayBackend()
		common.Application = test.NewApplicationClient()

		api := api.NewNetworkServerAPI()

		sess := session.NodeSession{
			DevAddr:  lorawan.DevAddr{1, 2, 3, 4},
			DevEUI:   lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
			AppEUI:   lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
			NwkSKey:  lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			FCntUp:   8,
			FCntDown: 5,
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
					Name:        "unconfirmed data",
					NodeSession: sess,
					PushDataDownRequest: ns.PushDataDownRequest{
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
					Name:        "confirmed data",
					NodeSession: sess,
					PushDataDownRequest: ns.PushDataDownRequest{
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
					Name:        "mac-commands in the queue",
					NodeSession: sess,
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
					PushDataDownRequest: ns.PushDataDownRequest{
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
					Name:        "maximum payload exceeded",
					NodeSession: sess,
					PushDataDownRequest: ns.PushDataDownRequest{
						DevEUI:    []byte{1, 2, 3, 4, 5, 6, 7, 8},
						Data:      make([]byte, 300),
						Confirmed: false,
						FPort:     10,
						FCnt:      5,
					},

					ExpectedPushDataDownError: grpc.Errorf(codes.InvalidArgument, "maximum payload size exceeded"),
					ExpectedFCntUp:            8,
					ExpectedFCntDown:          5,
				},
				{
					Name:        "invalid FPort",
					NodeSession: sess,
					PushDataDownRequest: ns.PushDataDownRequest{
						DevEUI:    []byte{1, 2, 3, 4, 5, 6, 7, 8},
						Data:      []byte{1, 2, 3, 4, 5},
						Confirmed: false,
						FPort:     0,
						FCnt:      5,
					},
					ExpectedPushDataDownError: grpc.Errorf(codes.InvalidArgument, "FPort must not be 0"),
					ExpectedFCntUp:            8,
					ExpectedFCntDown:          5,
				},
				{
					Name:        "invalid DevEUI",
					NodeSession: sess,
					PushDataDownRequest: ns.PushDataDownRequest{
						DevEUI:    []byte{1, 1, 1, 1, 1, 1, 1, 1, 1},
						Data:      []byte{1, 2, 3, 4, 5},
						Confirmed: false,
						FPort:     10,
						FCnt:      5,
					},
					ExpectedPushDataDownError: grpc.Errorf(codes.NotFound, session.ErrDoesNotExist.Error()),
					ExpectedFCntUp:            8,
					ExpectedFCntDown:          5,
				},
				{
					Name:        "no last RXInfoSet available",
					NodeSession: sess,
					PreFunc: func(ns *session.NodeSession) {
						ns.LastRXInfoSet = []gw.RXInfo{}
					},
					PushDataDownRequest: ns.PushDataDownRequest{
						DevEUI:    []byte{1, 2, 3, 4, 5, 6, 7, 8},
						Data:      []byte{1, 2, 3, 4, 5},
						Confirmed: false,
						FPort:     10,
						FCnt:      5,
					},
					ExpectedPushDataDownError: grpc.Errorf(codes.FailedPrecondition, "no last RX-Info set available"),
					ExpectedFCntUp:            8,
					ExpectedFCntDown:          5,
				},
				{
					Name:        "given FCnt does not match the FCntDown",
					NodeSession: sess,
					PushDataDownRequest: ns.PushDataDownRequest{
						DevEUI:    []byte{1, 2, 3, 4, 5, 6, 7, 8},
						Data:      []byte{1, 2, 3, 4, 5},
						Confirmed: false,
						FPort:     10,
						FCnt:      6,
					},
					ExpectedPushDataDownError: grpc.Errorf(codes.InvalidArgument, "invalid FCnt (expected: 5)"),
					ExpectedFCntUp:            8,
					ExpectedFCntDown:          5,
				},
			}

			for i, t := range tests {
				Convey(fmt.Sprintf("When testing: %s [%d]", t.Name, i), func() {
					if t.PreFunc != nil {
						t.PreFunc(&t.NodeSession)
					}

					// create node-session
					So(session.SaveNodeSession(common.RedisPool, t.NodeSession), ShouldBeNil)

					// mac mac-command queue items
					for _, qi := range t.MACCommandQueue {
						So(maccommand.AddQueueItem(common.RedisPool, t.NodeSession.DevEUI, qi), ShouldBeNil)
					}

					// push the data
					_, err := api.PushDataDown(context.Background(), &t.PushDataDownRequest)
					So(err, ShouldResemble, t.ExpectedPushDataDownError)

					Convey("Then the frame-counters are as expected", func() {
						sess, err := session.GetNodeSession(common.RedisPool, t.NodeSession.DevEUI)
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
						items, err := maccommand.ReadQueueItems(common.RedisPool, t.NodeSession.DevEUI)
						So(err, ShouldBeNil)
						So(items, ShouldResemble, t.ExpectedMACCommandQueue)
					})
				})
			}
		})
	})
}
