package testsuite

import (
	"errors"
	"fmt"
	"testing"

	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/uplink"

	"github.com/brocaar/lorawan/band"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
)

type otaaTestCase struct {
	Name                     string                     // name of the test
	RXInfo                   gw.RXInfo                  // rx-info of the "received" packet
	PHYPayload               lorawan.PHYPayload         // received PHYPayload
	JoinServerJoinReqError   error                      // error returned by the join-req method
	JoinServerJoinAnsPayload backend.JoinAnsPayload     // join-server join-ans payload
	AppKey                   lorawan.AES128Key          // app-key (used to decrypt the expected PHYPayload)
	ExtraChannels            []int                      // extra channels for CFList
	DeviceActivations        []storage.DeviceActivation // existing device-activations
	DeviceQueueItems         []storage.DeviceQueueItem  // existing device-queue items from a previous activation

	ExpectedError          error                  // expected error
	ExpectedJoinReqPayload backend.JoinReqPayload // expected join-request request
	ExpectedTXInfo         gw.TXInfo              // expected tx-info
	ExpectedPHYPayload     lorawan.PHYPayload     // expected (plaintext) PHYPayload
	ExpectedDeviceSession  storage.DeviceSession  // expected node-session
}

func TestOTAAScenarios(t *testing.T) {
	conf := test.GetConfig()
	db, err := common.OpenDatabase(conf.PostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	common.DB = db
	common.RedisPool = common.NewRedisPool(conf.RedisURL)
	common.NetID = [3]byte{3, 2, 1}

	Convey("Given a clean database with a device", t, func() {
		test.MustResetDB(common.DB)
		test.MustFlushRedis(common.RedisPool)

		asClient := test.NewApplicationClient()
		jsClient := test.NewJoinServerClient()

		common.ApplicationServerPool = test.NewApplicationServerPool(asClient)
		common.JoinServerPool = test.NewJoinServerPool(jsClient)
		common.Gateway = test.NewGatewayBackend()

		sp := storage.ServiceProfile{
			ServiceProfile: backend.ServiceProfile{},
		}
		So(storage.CreateServiceProfile(common.DB, &sp), ShouldBeNil)

		dp := storage.DeviceProfile{
			DeviceProfile: backend.DeviceProfile{
				RXDelay1:    3,
				RXDROffset1: 1,
				RXDataRate2: 5,
			},
		}
		So(storage.CreateDeviceProfile(common.DB, &dp), ShouldBeNil)

		rp := storage.RoutingProfile{
			RoutingProfile: backend.RoutingProfile{},
		}
		So(storage.CreateRoutingProfile(common.DB, &rp), ShouldBeNil)

		d := storage.Device{
			DevEUI:           lorawan.EUI64{2, 2, 3, 4, 5, 6, 7, 8},
			DeviceProfileID:  dp.DeviceProfile.DeviceProfileID,
			RoutingProfileID: rp.RoutingProfile.RoutingProfileID,
			ServiceProfileID: sp.ServiceProfile.ServiceProfileID,
		}
		So(storage.CreateDevice(common.DB, &d), ShouldBeNil)

		appKey := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

		rxInfo := gw.RXInfo{
			Frequency: common.Band.UplinkChannels[0].Frequency,
			DataRate:  common.Band.DataRates[common.Band.UplinkChannels[0].DataRates[0]],
		}

		jrPayload := lorawan.PHYPayload{
			MHDR: lorawan.MHDR{
				MType: lorawan.JoinRequest,
				Major: lorawan.LoRaWANR1,
			},
			MACPayload: &lorawan.JoinRequestPayload{
				AppEUI:   lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
				DevEUI:   d.DevEUI,
				DevNonce: [2]byte{1, 2},
			},
		}
		So(jrPayload.SetMIC(appKey), ShouldBeNil)
		jrBytes, err := jrPayload.MarshalBinary()
		So(err, ShouldBeNil)

		jaPayload := lorawan.JoinAcceptPayload{
			AppNonce: [3]byte{3, 2, 1},
			NetID:    common.NetID,
			DLSettings: lorawan.DLSettings{
				RX2DataRate: 2,
				RX1DROffset: 1,
			},
			DevAddr: [4]byte{1, 2, 3, 4},
			RXDelay: 3,
			CFList:  &lorawan.CFList{100, 200, 300, 400, 500},
		}
		jaPHY := lorawan.PHYPayload{
			MHDR: lorawan.MHDR{
				MType: lorawan.JoinAccept,
				Major: lorawan.LoRaWANR1,
			},
			MACPayload: &jaPayload,
		}
		So(jaPHY.SetMIC(appKey), ShouldBeNil)
		So(jaPHY.EncryptJoinAcceptPayload(appKey), ShouldBeNil)
		jaBytes, err := jaPHY.MarshalBinary()
		So(err, ShouldBeNil)
		So(jaPHY.DecryptJoinAcceptPayload(appKey), ShouldBeNil)

		Convey("Given a set of test-scenarios", func() {
			timestamp := rxInfo.Timestamp + 5000000

			tests := []otaaTestCase{
				{
					Name:                   "join-server returns an error",
					RXInfo:                 rxInfo,
					PHYPayload:             jrPayload,
					JoinServerJoinReqError: errors.New("invalid deveui"),
					ExpectedError:          errors.New("join-request to join-server error: invalid deveui"),
				},
				{
					Name:       "device alreay activated with dev-nonce",
					RXInfo:     rxInfo,
					PHYPayload: jrPayload,
					DeviceActivations: []storage.DeviceActivation{
						{
							DevEUI:   d.DevEUI,
							JoinEUI:  lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
							DevAddr:  lorawan.DevAddr{},
							NwkSKey:  lorawan.AES128Key{},
							DevNonce: lorawan.DevNonce{1, 2},
						},
					},
					ExpectedError: errors.New("validate dev-nonce error: object already exists"),
				},
				{
					Name:       "join-request accepted using rx1",
					RXInfo:     rxInfo,
					PHYPayload: jrPayload,
					AppKey:     appKey,
					JoinServerJoinAnsPayload: backend.JoinAnsPayload{
						PHYPayload: backend.HEXBytes(jaBytes),
						Result: backend.Result{
							ResultCode: backend.Success,
						},
						NwkSKey: &backend.KeyEnvelope{
							AESKey: lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
						},
					},
					DeviceQueueItems: []storage.DeviceQueueItem{
						{
							DevEUI:     lorawan.EUI64{2, 2, 3, 4, 5, 6, 7, 8},
							FRMPayload: []byte{1, 2, 3, 4},
							FCnt:       10,
							FPort:      1,
						},
					},

					ExpectedJoinReqPayload: backend.JoinReqPayload{
						BasePayload: backend.BasePayload{
							ProtocolVersion: backend.ProtocolVersion1_0,
							SenderID:        "030201",
							ReceiverID:      "0102030405060708",
							MessageType:     backend.JoinReq,
						},
						MACVersion: dp.DeviceProfile.MACVersion,
						PHYPayload: backend.HEXBytes(jrBytes),
						DevEUI:     d.DevEUI,
						DLSettings: lorawan.DLSettings{
							RX2DataRate: uint8(common.RX2DR),
							RX1DROffset: uint8(common.RX1DROffset),
						},
						RxDelay: common.RX1Delay,
					},
					ExpectedTXInfo: gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: &timestamp,
						Frequency: rxInfo.Frequency,
						Power:     14,
						DataRate:  rxInfo.DataRate,
						CodeRate:  rxInfo.CodeRate,
					},
					ExpectedPHYPayload: jaPHY,
					ExpectedDeviceSession: storage.DeviceSession{
						RoutingProfileID:    rp.RoutingProfile.RoutingProfileID,
						DeviceProfileID:     dp.DeviceProfile.DeviceProfileID,
						ServiceProfileID:    sp.ServiceProfile.ServiceProfileID,
						JoinEUI:             lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
						DevEUI:              lorawan.EUI64{2, 2, 3, 4, 5, 6, 7, 8},
						NwkSKey:             lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
						RXWindow:            storage.RX1,
						EnabledChannels:     []int{0, 1, 2},
						LastRXInfoSet:       []models.RXInfo{{}},
						LastDevStatusMargin: 127,
					},
				},
				{
					Name:          "join-request using rx1 and CFList",
					RXInfo:        rxInfo,
					PHYPayload:    jrPayload,
					AppKey:        appKey,
					ExtraChannels: []int{868400000, 868500000, 868600000},
					JoinServerJoinAnsPayload: backend.JoinAnsPayload{
						PHYPayload: backend.HEXBytes(jaBytes),
						Result: backend.Result{
							ResultCode: backend.Success,
						},
						NwkSKey: &backend.KeyEnvelope{
							AESKey: lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
						},
					},
					DeviceQueueItems: []storage.DeviceQueueItem{
						{
							DevEUI:     lorawan.EUI64{2, 2, 3, 4, 5, 6, 7, 8},
							FRMPayload: []byte{1, 2, 3, 4},
							FCnt:       10,
							FPort:      1,
						},
					},

					ExpectedJoinReqPayload: backend.JoinReqPayload{
						BasePayload: backend.BasePayload{
							ProtocolVersion: backend.ProtocolVersion1_0,
							SenderID:        "030201",
							ReceiverID:      "0102030405060708",
							MessageType:     backend.JoinReq,
						},
						MACVersion: dp.DeviceProfile.MACVersion,
						PHYPayload: backend.HEXBytes(jrBytes),
						DevEUI:     d.DevEUI,
						DLSettings: lorawan.DLSettings{
							RX2DataRate: uint8(common.RX2DR),
							RX1DROffset: uint8(common.RX1DROffset),
						},
						RxDelay: common.RX1Delay,
						CFList:  &lorawan.CFList{868400000, 868500000, 868600000},
					},
					ExpectedTXInfo: gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: &timestamp,
						Frequency: rxInfo.Frequency,
						Power:     14,
						DataRate:  rxInfo.DataRate,
						CodeRate:  rxInfo.CodeRate,
					},
					ExpectedPHYPayload: jaPHY,
					ExpectedDeviceSession: storage.DeviceSession{
						RoutingProfileID:    rp.RoutingProfile.RoutingProfileID,
						DeviceProfileID:     dp.DeviceProfile.DeviceProfileID,
						ServiceProfileID:    sp.ServiceProfile.ServiceProfileID,
						JoinEUI:             lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
						DevEUI:              lorawan.EUI64{2, 2, 3, 4, 5, 6, 7, 8},
						NwkSKey:             lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
						RXWindow:            storage.RX1,
						EnabledChannels:     []int{0, 1, 2, 3, 4, 5},
						LastRXInfoSet:       []models.RXInfo{{}},
						LastDevStatusMargin: 127,
					},
				},
			}

			runOTAATests(asClient, jsClient, tests)
		})
	})
}

func runOTAATests(asClient *test.ApplicationClient, jsClient *test.JoinServerClient, tests []otaaTestCase) {
	for i, t := range tests {
		Convey(fmt.Sprintf("When testing: %s [%d]", t.Name, i), func() {
			// reset band
			var err error
			common.Band, err = band.GetConfig(band.EU_863_870, false, lorawan.DwellTimeNoLimit)
			So(err, ShouldBeNil)
			for _, f := range t.ExtraChannels {
				So(common.Band.AddChannel(f), ShouldBeNil)
			}

			// set mocks
			jsClient.JoinAnsPayload = t.JoinServerJoinAnsPayload
			jsClient.JoinReqError = t.JoinServerJoinReqError

			// create device-activations
			for _, da := range t.DeviceActivations {
				So(storage.CreateDeviceActivation(common.DB, &da), ShouldBeNil)
			}

			// create device-queue items
			for _, qi := range t.DeviceQueueItems {
				So(storage.CreateDeviceQueueItem(common.DB, &qi), ShouldBeNil)
			}

			err = uplink.HandleRXPacket(gw.RXPacket{
				RXInfo:     t.RXInfo,
				PHYPayload: t.PHYPayload,
			})
			if err != nil {
				So(err.Error(), ShouldEqual, t.ExpectedError.Error())
				return
			}
			So(t.ExpectedError, ShouldBeNil)

			Convey("Then the device-queue has been flushed", func() {
				items, err := storage.GetDeviceQueueItemsForDevEUI(common.DB, lorawan.EUI64{2, 2, 3, 4, 5, 6, 7, 8})
				So(err, ShouldBeNil)
				So(items, ShouldHaveLength, 0)
			})

			Convey("Then the expected join-request was made to the join-server", func() {
				So(jsClient.JoinReqPayloadChan, ShouldHaveLength, 1)
				req := <-jsClient.JoinReqPayloadChan

				So(req.BasePayload.TransactionID, ShouldNotEqual, "")
				req.BasePayload.TransactionID = 0

				So(req.DevAddr, ShouldNotEqual, lorawan.DevAddr{})
				req.DevAddr = lorawan.DevAddr{}

				So(req, ShouldResemble, t.ExpectedJoinReqPayload)
			})

			Convey("Then the expected txinfo was used", func() {
				So(common.Gateway.(*test.GatewayBackend).TXPacketChan, ShouldHaveLength, 1)
				txPacket := <-common.Gateway.(*test.GatewayBackend).TXPacketChan

				So(txPacket.Token, ShouldNotEqual, 0)
				So(txPacket.TXInfo, ShouldResemble, t.ExpectedTXInfo)
			})

			Convey("Then the expected PHYPayload was sent", func() {
				So(common.Gateway.(*test.GatewayBackend).TXPacketChan, ShouldHaveLength, 1)
				txPacket := <-common.Gateway.(*test.GatewayBackend).TXPacketChan

				So(txPacket.PHYPayload.DecryptJoinAcceptPayload(t.AppKey), ShouldBeNil)
				So(txPacket.PHYPayload, ShouldResemble, t.ExpectedPHYPayload)
			})

			Convey("Then the expected device-session was created", func() {
				ds, err := storage.GetDeviceSession(common.RedisPool, lorawan.EUI64{2, 2, 3, 4, 5, 6, 7, 8})
				So(err, ShouldBeNil)
				So(ds.DevAddr, ShouldNotEqual, lorawan.DevAddr{})
				ds.DevAddr = lorawan.DevAddr{}
				So(ds, ShouldResemble, t.ExpectedDeviceSession)
			})

			Convey("Then a device-activation record was created", func() {
				da, err := storage.GetLastDeviceActivationForDevEUI(common.DB, t.ExpectedDeviceSession.DevEUI)
				So(err, ShouldBeNil)
				So(da.DevAddr, ShouldNotEqual, lorawan.DevAddr{})
				So(da.NwkSKey, ShouldEqual, t.ExpectedDeviceSession.NwkSKey)
			})
		})
	}
}
