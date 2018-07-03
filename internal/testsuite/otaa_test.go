package testsuite

import (
	"errors"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/loraserver/internal/uplink"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
	"github.com/brocaar/lorawan/band"
)

type otaaTestCase struct {
	BeforeFunc               func(*otaaTestCase) error
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
	config.C.PostgreSQL.DB = db
	config.C.Redis.Pool = common.NewRedisPool(conf.RedisURL)
	config.C.NetworkServer.NetID = [3]byte{3, 2, 1}

	Convey("Given a clean database with a device", t, func() {
		test.MustResetDB(config.C.PostgreSQL.DB)
		test.MustFlushRedis(config.C.Redis.Pool)

		asClient := test.NewApplicationClient()
		jsClient := test.NewJoinServerClient()

		config.C.ApplicationServer.Pool = test.NewApplicationServerPool(asClient)
		config.C.JoinServer.Pool = test.NewJoinServerPool(jsClient)
		config.C.NetworkServer.Gateway.Backend.Backend = test.NewGatewayBackend()

		sp := storage.ServiceProfile{}
		So(storage.CreateServiceProfile(config.C.PostgreSQL.DB, &sp), ShouldBeNil)

		dp := storage.DeviceProfile{
			MACVersion:   "1.0.2",
			RXDelay1:     3,
			RXDROffset1:  1,
			RXDataRate2:  5,
			SupportsJoin: true,
		}
		So(storage.CreateDeviceProfile(config.C.PostgreSQL.DB, &dp), ShouldBeNil)

		rp := storage.RoutingProfile{}
		So(storage.CreateRoutingProfile(config.C.PostgreSQL.DB, &rp), ShouldBeNil)

		d := storage.Device{
			DevEUI:           lorawan.EUI64{2, 2, 3, 4, 5, 6, 7, 8},
			DeviceProfileID:  dp.ID,
			RoutingProfileID: rp.ID,
			ServiceProfileID: sp.ID,
		}
		So(storage.CreateDevice(config.C.PostgreSQL.DB, &d), ShouldBeNil)

		appKey := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

		c0, err := config.C.NetworkServer.Band.Band.GetDownlinkChannel(0)
		So(err, ShouldBeNil)

		c0MinDR, err := config.C.NetworkServer.Band.Band.GetDataRate(c0.MinDR)
		So(err, ShouldBeNil)

		rxInfo := gw.RXInfo{
			Frequency: c0.Frequency,
			DataRate:  c0MinDR,
		}

		jrPayload := lorawan.PHYPayload{
			MHDR: lorawan.MHDR{
				MType: lorawan.JoinRequest,
				Major: lorawan.LoRaWANR1,
			},
			MACPayload: &lorawan.JoinRequestPayload{
				JoinEUI:  lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
				DevEUI:   d.DevEUI,
				DevNonce: 258,
			},
		}
		So(jrPayload.SetUplinkJoinMIC(appKey), ShouldBeNil)
		jrBytes, err := jrPayload.MarshalBinary()
		So(err, ShouldBeNil)

		jaPayload := lorawan.JoinAcceptPayload{
			JoinNonce: 197121,
			HomeNetID: config.C.NetworkServer.NetID,
			DLSettings: lorawan.DLSettings{
				RX2DataRate: 2,
				RX1DROffset: 1,
			},
			DevAddr: [4]byte{1, 2, 3, 4},
			RXDelay: 3,
			CFList: &lorawan.CFList{
				CFListType: lorawan.CFListChannel,
				Payload: &lorawan.CFListChannelPayload{
					Channels: [5]uint32{
						100,
						200,
						300,
						400,
						500,
					},
				},
			},
		}
		jaPHY := lorawan.PHYPayload{
			MHDR: lorawan.MHDR{
				MType: lorawan.JoinAccept,
				Major: lorawan.LoRaWANR1,
			},
			MACPayload: &jaPayload,
		}
		So(jaPHY.SetDownlinkJoinMIC(lorawan.JoinRequestType, lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}, lorawan.DevNonce(258), appKey), ShouldBeNil)
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
							DevEUI:      d.DevEUI,
							JoinEUI:     lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
							DevAddr:     lorawan.DevAddr{},
							SNwkSIntKey: lorawan.AES128Key{},
							FNwkSIntKey: lorawan.AES128Key{},
							NwkSEncKey:  lorawan.AES128Key{},
							JoinReqType: lorawan.JoinRequestType,
							DevNonce:    258,
						},
					},
					ExpectedError: errors.New("validate dev-nonce error: object already exists"),
				},
				{
					Name:       "join-request accepted using (NwkSKey)",
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
						MACVersion: dp.MACVersion,
						PHYPayload: backend.HEXBytes(jrBytes),
						DevEUI:     d.DevEUI,
						DLSettings: lorawan.DLSettings{
							RX2DataRate: uint8(config.C.NetworkServer.NetworkSettings.RX2DR),
							RX1DROffset: uint8(config.C.NetworkServer.NetworkSettings.RX1DROffset),
						},
						RxDelay: config.C.NetworkServer.NetworkSettings.RX1Delay,
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
						MACVersion:            "1.0.2",
						RoutingProfileID:      rp.ID,
						DeviceProfileID:       dp.ID,
						ServiceProfileID:      sp.ID,
						JoinEUI:               lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
						DevEUI:                lorawan.EUI64{2, 2, 3, 4, 5, 6, 7, 8},
						FNwkSIntKey:           lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
						SNwkSIntKey:           lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
						NwkSEncKey:            lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
						RXWindow:              storage.RX1,
						EnabledUplinkChannels: []int{0, 1, 2},
						ExtraUplinkChannels:   map[int]band.Channel{},
						UplinkGatewayHistory:  map[lorawan.EUI64]storage.UplinkGatewayHistory{},
						RX2Frequency:          config.C.NetworkServer.Band.Band.GetDefaults().RX2Frequency,
						NbTrans:               1,
					},
				},
				{
					BeforeFunc: func(tc *otaaTestCase) error {
						dp.MACVersion = "1.1.0"
						return storage.UpdateDeviceProfile(db, &dp)
					},
					Name:       "join-request accepted (SNwkSIntKey, FNwkSIntKey, NwkSEncKey)",
					RXInfo:     rxInfo,
					PHYPayload: jrPayload,
					AppKey:     appKey,
					JoinServerJoinAnsPayload: backend.JoinAnsPayload{
						PHYPayload: backend.HEXBytes(jaBytes),
						Result: backend.Result{
							ResultCode: backend.Success,
						},
						SNwkSIntKey: &backend.KeyEnvelope{
							AESKey: lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
						},
						FNwkSIntKey: &backend.KeyEnvelope{
							AESKey: lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 2},
						},
						NwkSEncKey: &backend.KeyEnvelope{
							AESKey: lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 3},
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
						MACVersion: "1.1.0",
						PHYPayload: backend.HEXBytes(jrBytes),
						DevEUI:     d.DevEUI,
						DLSettings: lorawan.DLSettings{
							OptNeg:      true,
							RX2DataRate: uint8(config.C.NetworkServer.NetworkSettings.RX2DR),
							RX1DROffset: uint8(config.C.NetworkServer.NetworkSettings.RX1DROffset),
						},
						RxDelay: config.C.NetworkServer.NetworkSettings.RX1Delay,
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
						MACVersion:            "1.1.0",
						RoutingProfileID:      rp.ID,
						DeviceProfileID:       dp.ID,
						ServiceProfileID:      sp.ID,
						JoinEUI:               lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
						DevEUI:                lorawan.EUI64{2, 2, 3, 4, 5, 6, 7, 8},
						SNwkSIntKey:           lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
						FNwkSIntKey:           lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 2},
						NwkSEncKey:            lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 3},
						RXWindow:              storage.RX1,
						EnabledUplinkChannels: []int{0, 1, 2},
						ExtraUplinkChannels:   map[int]band.Channel{},
						UplinkGatewayHistory:  map[lorawan.EUI64]storage.UplinkGatewayHistory{},
						RX2Frequency:          config.C.NetworkServer.Band.Band.GetDefaults().RX2Frequency,
						NbTrans:               1,
					},
				},
				{
					BeforeFunc: func(tc *otaaTestCase) error {
						d.SkipFCntCheck = true
						return storage.UpdateDevice(db, &d)
					},
					Name:       "join-request accepted + skip fcnt check",
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

					ExpectedJoinReqPayload: backend.JoinReqPayload{
						BasePayload: backend.BasePayload{
							ProtocolVersion: backend.ProtocolVersion1_0,
							SenderID:        "030201",
							ReceiverID:      "0102030405060708",
							MessageType:     backend.JoinReq,
						},
						MACVersion: dp.MACVersion,
						PHYPayload: jrBytes,
						DevEUI:     d.DevEUI,
						DLSettings: lorawan.DLSettings{
							RX2DataRate: uint8(config.C.NetworkServer.NetworkSettings.RX2DR),
							RX1DROffset: uint8(config.C.NetworkServer.NetworkSettings.RX1DROffset),
						},
						RxDelay: config.C.NetworkServer.NetworkSettings.RX1Delay,
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
						MACVersion:            "1.0.2",
						RoutingProfileID:      rp.ID,
						DeviceProfileID:       dp.ID,
						ServiceProfileID:      sp.ID,
						JoinEUI:               lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
						DevEUI:                lorawan.EUI64{2, 2, 3, 4, 5, 6, 7, 8},
						FNwkSIntKey:           lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
						SNwkSIntKey:           lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
						NwkSEncKey:            lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
						RXWindow:              storage.RX1,
						EnabledUplinkChannels: []int{0, 1, 2},
						ExtraUplinkChannels:   map[int]band.Channel{},
						UplinkGatewayHistory:  map[lorawan.EUI64]storage.UplinkGatewayHistory{},
						RX2Frequency:          config.C.NetworkServer.Band.Band.GetDefaults().RX2Frequency,
						SkipFCntValidation:    true,
						NbTrans:               1,
					},
				},
				{
					BeforeFunc: func(tc *otaaTestCase) error {
						cFList := lorawan.CFList{
							CFListType: lorawan.CFListChannel,
							Payload: &lorawan.CFListChannelPayload{
								Channels: [5]uint32{
									868600000,
									868700000,
									868800000,
								},
							},
						}
						cFListB, err := cFList.MarshalBinary()
						if err != nil {
							return err
						}
						tc.ExpectedJoinReqPayload.CFList = backend.HEXBytes(cFListB)

						return nil
					},
					Name:          "join-request using rx1 and CFList",
					RXInfo:        rxInfo,
					PHYPayload:    jrPayload,
					AppKey:        appKey,
					ExtraChannels: []int{868600000, 868700000, 868800000},
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
						MACVersion: dp.MACVersion,
						PHYPayload: backend.HEXBytes(jrBytes),
						DevEUI:     d.DevEUI,
						DLSettings: lorawan.DLSettings{
							RX2DataRate: uint8(config.C.NetworkServer.NetworkSettings.RX2DR),
							RX1DROffset: uint8(config.C.NetworkServer.NetworkSettings.RX1DROffset),
						},
						RxDelay: config.C.NetworkServer.NetworkSettings.RX1Delay,
						// CFList is set in the BeforeFunc
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
						MACVersion:            "1.0.2",
						RoutingProfileID:      rp.ID,
						DeviceProfileID:       dp.ID,
						ServiceProfileID:      sp.ID,
						JoinEUI:               lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
						DevEUI:                lorawan.EUI64{2, 2, 3, 4, 5, 6, 7, 8},
						FNwkSIntKey:           lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
						SNwkSIntKey:           lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
						NwkSEncKey:            lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
						RXWindow:              storage.RX1,
						EnabledUplinkChannels: []int{0, 1, 2, 3, 4, 5},
						ExtraUplinkChannels: map[int]band.Channel{
							3: band.Channel{Frequency: 868600000, MinDR: 0, MaxDR: 5},
							4: band.Channel{Frequency: 868700000, MinDR: 0, MaxDR: 5},
							5: band.Channel{Frequency: 868800000, MinDR: 0, MaxDR: 5},
						},
						UplinkGatewayHistory: map[lorawan.EUI64]storage.UplinkGatewayHistory{},
						RX2Frequency:         config.C.NetworkServer.Band.Band.GetDefaults().RX2Frequency,
						NbTrans:              1,
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
			if t.BeforeFunc != nil {
				So(t.BeforeFunc(&t), ShouldBeNil)
			}

			// reset band
			var err error
			config.C.NetworkServer.Band.Band, err = band.GetConfig(band.EU_863_870, false, lorawan.DwellTimeNoLimit)
			So(err, ShouldBeNil)
			for _, f := range t.ExtraChannels {
				So(config.C.NetworkServer.Band.Band.AddChannel(f, 0, 5), ShouldBeNil)
			}

			// set mocks
			jsClient.JoinAnsPayload = t.JoinServerJoinAnsPayload
			jsClient.JoinReqError = t.JoinServerJoinReqError

			// create device-activations
			for _, da := range t.DeviceActivations {
				So(storage.CreateDeviceActivation(config.C.PostgreSQL.DB, &da), ShouldBeNil)
			}

			// create device-queue items
			for _, qi := range t.DeviceQueueItems {
				So(storage.CreateDeviceQueueItem(config.C.PostgreSQL.DB, &qi), ShouldBeNil)
			}

			err = uplink.HandleRXPacket(gw.RXPacket{
				RXInfo:     t.RXInfo,
				PHYPayload: t.PHYPayload,
			})
			if err != nil {
				if t.ExpectedError == nil {
					So(err.Error(), ShouldEqual, "")
				} else {
					So(err.Error(), ShouldEqual, t.ExpectedError.Error())
				}
				return
			}
			So(t.ExpectedError, ShouldBeNil)

			Convey("Then the device-queue has been flushed", func() {
				items, err := storage.GetDeviceQueueItemsForDevEUI(config.C.PostgreSQL.DB, lorawan.EUI64{2, 2, 3, 4, 5, 6, 7, 8})
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
				So(config.C.NetworkServer.Gateway.Backend.Backend.(*test.GatewayBackend).TXPacketChan, ShouldHaveLength, 1)
				txPacket := <-config.C.NetworkServer.Gateway.Backend.Backend.(*test.GatewayBackend).TXPacketChan

				So(txPacket.Token, ShouldNotEqual, 0)
				So(txPacket.TXInfo, ShouldResemble, t.ExpectedTXInfo)
			})

			Convey("Then the expected PHYPayload was sent", func() {
				So(config.C.NetworkServer.Gateway.Backend.Backend.(*test.GatewayBackend).TXPacketChan, ShouldHaveLength, 1)
				txPacket := <-config.C.NetworkServer.Gateway.Backend.Backend.(*test.GatewayBackend).TXPacketChan

				So(txPacket.PHYPayload.DecryptJoinAcceptPayload(t.AppKey), ShouldBeNil)
				So(txPacket.PHYPayload, ShouldResemble, t.ExpectedPHYPayload)
			})

			Convey("Then the expected device-session was created", func() {
				ds, err := storage.GetDeviceSession(config.C.Redis.Pool, lorawan.EUI64{2, 2, 3, 4, 5, 6, 7, 8})
				So(err, ShouldBeNil)
				So(ds.DevAddr, ShouldNotEqual, lorawan.DevAddr{})
				ds.DevAddr = lorawan.DevAddr{}
				So(ds, ShouldResemble, t.ExpectedDeviceSession)
			})

			Convey("Then a device-activation record was created", func() {
				da, err := storage.GetLastDeviceActivationForDevEUI(config.C.PostgreSQL.DB, t.ExpectedDeviceSession.DevEUI)
				So(err, ShouldBeNil)
				So(da.DevAddr, ShouldNotEqual, lorawan.DevAddr{})
				So(da.SNwkSIntKey, ShouldEqual, t.ExpectedDeviceSession.SNwkSIntKey)
				So(da.FNwkSIntKey, ShouldEqual, t.ExpectedDeviceSession.FNwkSIntKey)
				So(da.NwkSEncKey, ShouldEqual, t.ExpectedDeviceSession.NwkSEncKey)
				So(da.JoinReqType, ShouldEqual, lorawan.JoinRequestType)
			})
		})
	}
}
