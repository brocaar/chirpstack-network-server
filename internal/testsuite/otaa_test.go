package testsuite

import (
	"errors"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	commonPB "github.com/brocaar/loraserver/api/common"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/helpers"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/loraserver/internal/uplink"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
	"github.com/brocaar/lorawan/band"
)

type otaaTestCase struct {
	BeforeFunc               func(*otaaTestCase) error
	Name                     string // name of the test
	TXInfo                   gw.UplinkTXInfo
	RXInfo                   gw.UplinkRXInfo            // rx-info of the "received" packet
	PHYPayload               lorawan.PHYPayload         // received PHYPayload
	JoinServerJoinReqError   error                      // error returned by the join-req method
	JoinServerJoinAnsPayload backend.JoinAnsPayload     // join-server join-ans payload
	AppKey                   lorawan.AES128Key          // app-key (used to decrypt the expected PHYPayload)
	ExtraChannels            []int                      // extra channels for CFList
	DeviceActivations        []storage.DeviceActivation // existing device-activations
	DeviceQueueItems         []storage.DeviceQueueItem  // existing device-queue items from a previous activation

	ExpectedError          error                  // expected error
	ExpectedJoinReqPayload backend.JoinReqPayload // expected join-request request
	ExpectedTXInfo         gw.DownlinkTXInfo      // expected tx-info
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

		rxInfo := gw.UplinkRXInfo{
			GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
		}

		txInfo := gw.UplinkTXInfo{
			Frequency: uint32(c0.Frequency),
		}

		So(helpers.SetUplinkTXInfoDataRate(&txInfo, 0, config.C.NetworkServer.Band.Band), ShouldBeNil)

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
			tests := []otaaTestCase{
				{
					Name:                   "join-server returns an error",
					RXInfo:                 rxInfo,
					TXInfo:                 txInfo,
					PHYPayload:             jrPayload,
					JoinServerJoinReqError: errors.New("invalid deveui"),
					ExpectedError:          errors.New("join-request to join-server error: invalid deveui"),
				},
				{
					Name:       "device alreay activated with dev-nonce",
					RXInfo:     rxInfo,
					TXInfo:     txInfo,
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
					TXInfo:     txInfo,
					PHYPayload: jrPayload,
					AppKey:     appKey,
					JoinServerJoinAnsPayload: backend.JoinAnsPayload{
						PHYPayload: backend.HEXBytes(jaBytes),
						Result: backend.Result{
							ResultCode: backend.Success,
						},
						NwkSKey: &backend.KeyEnvelope{
							AESKey: []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
						},
						AppSKey: &backend.KeyEnvelope{
							AESKey: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
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
					ExpectedTXInfo: gw.DownlinkTXInfo{
						GatewayId:  rxInfo.GatewayId,
						Timestamp:  rxInfo.Timestamp + 5000000,
						Frequency:  txInfo.Frequency,
						Power:      14,
						Modulation: commonPB.Modulation_LORA,
						ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
							LoraModulationInfo: &gw.LoRaModulationInfo{
								Bandwidth:             125,
								SpreadingFactor:       12,
								CodeRate:              "4/5",
								PolarizationInversion: true,
							},
						},
					},
					ExpectedPHYPayload: jaPHY,
					ExpectedDeviceSession: storage.DeviceSession{
						MACVersion:       "1.0.2",
						RoutingProfileID: rp.ID,
						DeviceProfileID:  dp.ID,
						ServiceProfileID: sp.ID,
						JoinEUI:          lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
						DevEUI:           lorawan.EUI64{2, 2, 3, 4, 5, 6, 7, 8},
						FNwkSIntKey:      lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
						SNwkSIntKey:      lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
						NwkSEncKey:       lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
						AppSKeyEvelope: &storage.KeyEnvelope{
							AESKey: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
						},
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
					TXInfo:     txInfo,
					PHYPayload: jrPayload,
					AppKey:     appKey,
					JoinServerJoinAnsPayload: backend.JoinAnsPayload{
						PHYPayload: backend.HEXBytes(jaBytes),
						Result: backend.Result{
							ResultCode: backend.Success,
						},
						SNwkSIntKey: &backend.KeyEnvelope{
							AESKey: []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
						},
						FNwkSIntKey: &backend.KeyEnvelope{
							AESKey: []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 2},
						},
						NwkSEncKey: &backend.KeyEnvelope{
							AESKey: []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 3},
						},
						AppSKey: &backend.KeyEnvelope{
							AESKey: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
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
					ExpectedTXInfo: gw.DownlinkTXInfo{
						GatewayId:  rxInfo.GatewayId,
						Timestamp:  rxInfo.Timestamp + 5000000,
						Frequency:  txInfo.Frequency,
						Power:      14,
						Modulation: commonPB.Modulation_LORA,
						ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
							LoraModulationInfo: &gw.LoRaModulationInfo{
								Bandwidth:             125,
								SpreadingFactor:       12,
								CodeRate:              "4/5",
								PolarizationInversion: true,
							},
						},
					},
					ExpectedPHYPayload: jaPHY,
					ExpectedDeviceSession: storage.DeviceSession{
						MACVersion:       "1.1.0",
						RoutingProfileID: rp.ID,
						DeviceProfileID:  dp.ID,
						ServiceProfileID: sp.ID,
						JoinEUI:          lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
						DevEUI:           lorawan.EUI64{2, 2, 3, 4, 5, 6, 7, 8},
						SNwkSIntKey:      lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
						FNwkSIntKey:      lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 2},
						NwkSEncKey:       lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 3},
						AppSKeyEvelope: &storage.KeyEnvelope{
							AESKey: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
						},
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
						config.C.JoinServer.KEK.Set = []struct {
							Label string
							KEK   string `mapstructure:"kek"`
						}{
							{
								Label: "010203",
								KEK:   "00000000000000000000000000000000",
							},
						}

						dp.MACVersion = "1.1.0"
						return storage.UpdateDeviceProfile(db, &dp)
					},
					Name:       "join-request accepted (SNwkSIntKey, FNwkSIntKey, NwkSEncKey with KEK)",
					TXInfo:     txInfo,
					RXInfo:     rxInfo,
					PHYPayload: jrPayload,
					AppKey:     appKey,
					JoinServerJoinAnsPayload: backend.JoinAnsPayload{
						PHYPayload: backend.HEXBytes(jaBytes),
						Result: backend.Result{
							ResultCode: backend.Success,
						},
						SNwkSIntKey: &backend.KeyEnvelope{
							KEKLabel: "010203",
							AESKey:   []byte{246, 176, 184, 31, 61, 48, 41, 18, 85, 145, 192, 176, 184, 141, 118, 201, 59, 72, 172, 164, 4, 22, 133, 211},
						},
						FNwkSIntKey: &backend.KeyEnvelope{
							KEKLabel: "010203",
							AESKey:   []byte{87, 85, 230, 195, 36, 30, 231, 230, 100, 111, 15, 254, 135, 120, 122, 0, 44, 249, 228, 176, 131, 73, 143, 0},
						},
						NwkSEncKey: &backend.KeyEnvelope{
							KEKLabel: "010203",
							AESKey:   []byte{78, 225, 236, 219, 189, 151, 82, 239, 109, 226, 140, 65, 233, 189, 174, 37, 39, 206, 241, 242, 2, 127, 157, 247},
						},
						AppSKey: &backend.KeyEnvelope{
							KEKLabel: "lora-app-server",
							AESKey:   []byte{248, 215, 201, 250, 55, 176, 209, 198, 53, 78, 109, 184, 225, 157, 157, 122, 180, 229, 199, 88, 30, 159, 30, 32},
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
					ExpectedTXInfo: gw.DownlinkTXInfo{
						GatewayId:  rxInfo.GatewayId,
						Timestamp:  rxInfo.Timestamp + 5000000,
						Frequency:  txInfo.Frequency,
						Power:      14,
						Modulation: commonPB.Modulation_LORA,
						ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
							LoraModulationInfo: &gw.LoRaModulationInfo{
								Bandwidth:             125,
								SpreadingFactor:       12,
								CodeRate:              "4/5",
								PolarizationInversion: true,
							},
						},
					},
					ExpectedPHYPayload: jaPHY,
					ExpectedDeviceSession: storage.DeviceSession{
						MACVersion:       "1.1.0",
						RoutingProfileID: rp.ID,
						DeviceProfileID:  dp.ID,
						ServiceProfileID: sp.ID,
						JoinEUI:          lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
						DevEUI:           lorawan.EUI64{2, 2, 3, 4, 5, 6, 7, 8},
						SNwkSIntKey:      lorawan.AES128Key{88, 148, 152, 153, 48, 146, 207, 219, 95, 210, 224, 42, 199, 81, 11, 241},
						FNwkSIntKey:      lorawan.AES128Key{83, 127, 138, 174, 137, 108, 121, 224, 21, 209, 2, 208, 98, 134, 53, 78},
						NwkSEncKey:       lorawan.AES128Key{152, 152, 40, 60, 79, 102, 235, 108, 111, 213, 22, 88, 130, 4, 108, 64},
						AppSKeyEvelope: &storage.KeyEnvelope{
							KEKLabel: "lora-app-server",
							AESKey:   []byte{248, 215, 201, 250, 55, 176, 209, 198, 53, 78, 109, 184, 225, 157, 157, 122, 180, 229, 199, 88, 30, 159, 30, 32},
						},
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
					TXInfo:     txInfo,
					PHYPayload: jrPayload,
					AppKey:     appKey,
					JoinServerJoinAnsPayload: backend.JoinAnsPayload{
						PHYPayload: backend.HEXBytes(jaBytes),
						Result: backend.Result{
							ResultCode: backend.Success,
						},
						NwkSKey: &backend.KeyEnvelope{
							AESKey: []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
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
					ExpectedTXInfo: gw.DownlinkTXInfo{
						GatewayId:  rxInfo.GatewayId,
						Timestamp:  rxInfo.Timestamp + 5000000,
						Frequency:  txInfo.Frequency,
						Power:      14,
						Modulation: commonPB.Modulation_LORA,
						ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
							LoraModulationInfo: &gw.LoRaModulationInfo{
								Bandwidth:             125,
								SpreadingFactor:       12,
								CodeRate:              "4/5",
								PolarizationInversion: true,
							},
						},
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
					TXInfo:        txInfo,
					PHYPayload:    jrPayload,
					AppKey:        appKey,
					ExtraChannels: []int{868600000, 868700000, 868800000},
					JoinServerJoinAnsPayload: backend.JoinAnsPayload{
						PHYPayload: backend.HEXBytes(jaBytes),
						Result: backend.Result{
							ResultCode: backend.Success,
						},
						NwkSKey: &backend.KeyEnvelope{
							AESKey: []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
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
					ExpectedTXInfo: gw.DownlinkTXInfo{
						GatewayId:  rxInfo.GatewayId,
						Timestamp:  rxInfo.Timestamp + 5000000,
						Frequency:  txInfo.Frequency,
						Power:      14,
						Modulation: commonPB.Modulation_LORA,
						ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
							LoraModulationInfo: &gw.LoRaModulationInfo{
								Bandwidth:             125,
								SpreadingFactor:       12,
								CodeRate:              "4/5",
								PolarizationInversion: true,
							},
						},
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

			phyB, err := t.PHYPayload.MarshalBinary()
			So(err, ShouldBeNil)

			err = uplink.HandleRXPacket(gw.UplinkFrame{
				RxInfo:     &t.RXInfo,
				TxInfo:     &t.TXInfo,
				PhyPayload: phyB,
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

				txPacket.TxInfo.XXX_sizecache = 0
				modInfo := txPacket.TxInfo.GetLoraModulationInfo()
				modInfo.XXX_sizecache = 0

				So(txPacket.Token, ShouldNotEqual, 0)
				So(txPacket.TxInfo, ShouldResemble, &t.ExpectedTXInfo)
			})

			Convey("Then the expected PHYPayload was sent", func() {
				So(config.C.NetworkServer.Gateway.Backend.Backend.(*test.GatewayBackend).TXPacketChan, ShouldHaveLength, 1)
				txPacket := <-config.C.NetworkServer.Gateway.Backend.Backend.(*test.GatewayBackend).TXPacketChan

				var phy lorawan.PHYPayload
				So(phy.UnmarshalBinary(txPacket.PhyPayload), ShouldBeNil)
				So(phy.DecryptJoinAcceptPayload(t.AppKey), ShouldBeNil)
				So(phy, ShouldResemble, t.ExpectedPHYPayload)
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
