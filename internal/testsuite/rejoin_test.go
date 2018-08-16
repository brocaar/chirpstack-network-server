package testsuite

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gomodule/redigo/redis"
	"github.com/jmoiron/sqlx"
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

type rejoinTestCase struct {
	BeforeFunc                 func(*rejoinTestCase) error
	Name                       string
	DeviceSession              storage.DeviceSession
	TXInfo                     gw.UplinkTXInfo
	RXInfo                     gw.UplinkRXInfo
	PHYPayload                 lorawan.PHYPayload
	JoinServerRejoinReqError   error
	JoinServerRejoinAnsPayload backend.RejoinAnsPayload

	ExpectedError            error
	ExpectedRejoinReqPayload backend.RejoinReqPayload
	ExpectedTXInfo           gw.DownlinkTXInfo
	ExpectedPHYPayload       lorawan.PHYPayload
	ExpectedDeviceSession    storage.DeviceSession
}

func TestRejoinScenarios(t *testing.T) {
	conf := test.GetConfig()
	db, err := common.OpenDatabase(conf.PostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	redisPool := common.NewRedisPool(conf.RedisURL)

	config.C.PostgreSQL.DB = db
	config.C.Redis.Pool = redisPool
	config.C.NetworkServer.NetID = lorawan.NetID{3, 2, 1}

	config.C.NetworkServer.Band.Band, _ = band.GetConfig(band.EU_863_870, false, lorawan.DwellTimeNoLimit)
	config.C.NetworkServer.Band.Band.AddChannel(867100000, 0, 5)
	config.C.NetworkServer.Band.Band.AddChannel(867300000, 0, 5)
	config.C.NetworkServer.Band.Band.AddChannel(867500000, 0, 5)
	config.C.NetworkServer.NetworkSettings.RX2DR = 3
	config.C.NetworkServer.NetworkSettings.RX1DROffset = 2
	config.C.NetworkServer.NetworkSettings.RX1Delay = 1

	Convey("Given a clean database with an activated device", t, func() {
		test.MustResetDB(db)
		test.MustFlushRedis(redisPool)

		asClient := test.NewApplicationClient()
		jsClient := test.NewJoinServerClient()

		config.C.ApplicationServer.Pool = test.NewApplicationServerPool(asClient)
		config.C.JoinServer.Pool = test.NewJoinServerPool(jsClient)
		config.C.NetworkServer.Gateway.Backend.Backend = test.NewGatewayBackend()

		sp := storage.ServiceProfile{}
		So(storage.CreateServiceProfile(db, &sp), ShouldBeNil)

		dp := storage.DeviceProfile{
			MACVersion:   "1.1.0",
			RXDelay1:     3,
			RXDROffset1:  1,
			RXDataRate2:  5,
			SupportsJoin: true,
		}
		So(storage.CreateDeviceProfile(db, &dp), ShouldBeNil)

		rp := storage.RoutingProfile{}
		So(storage.CreateRoutingProfile(db, &rp), ShouldBeNil)

		d := storage.Device{
			DevEUI:           lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
			DeviceProfileID:  dp.ID,
			RoutingProfileID: rp.ID,
			ServiceProfileID: sp.ID,
		}
		So(storage.CreateDevice(db, &d), ShouldBeNil)

		ds := storage.DeviceSession{
			DeviceProfileID:       dp.ID,
			ServiceProfileID:      sp.ID,
			RoutingProfileID:      rp.ID,
			DevEUI:                d.DevEUI,
			JoinEUI:               lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
			SNwkSIntKey:           lorawan.AES128Key{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
			FNwkSIntKey:           lorawan.AES128Key{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 2},
			NwkSEncKey:            lorawan.AES128Key{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 3},
			MACVersion:            "1.1.0",
			ExtraUplinkChannels:   make(map[int]band.Channel),
			RX2Frequency:          869525000,
			NbTrans:               1,
			EnabledUplinkChannels: []int{0, 1, 2},
			UplinkGatewayHistory:  make(map[lorawan.EUI64]storage.UplinkGatewayHistory),
		}

		c0, err := config.C.NetworkServer.Band.Band.GetDownlinkChannel(0)
		So(err, ShouldBeNil)

		rxInfo := gw.UplinkRXInfo{
			GatewayId: []byte{1, 1, 1, 1, 2, 2, 2, 2},
		}

		txInfo := gw.UplinkTXInfo{
			Frequency: uint32(c0.Frequency),
		}
		So(helpers.SetUplinkTXInfoDataRate(&txInfo, 0, config.C.NetworkServer.Band.Band), ShouldBeNil)

		jsEncKey := lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8}
		jsIntKey := lorawan.AES128Key{8, 7, 6, 5, 4, 3, 2, 1, 8, 7, 6, 5, 4, 3, 2, 1}

		Convey("Testing rejoin-request 0", func() {
			rjPHY := lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.RejoinRequest,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.RejoinRequestType02Payload{
					RejoinType: lorawan.RejoinRequestType0,
					NetID:      lorawan.NetID{3, 2, 1},
					DevEUI:     d.DevEUI,
					RJCount0:   123,
				},
			}
			So(rjPHY.SetUplinkJoinMIC(ds.SNwkSIntKey), ShouldBeNil)
			rjBytes, err := rjPHY.MarshalBinary()
			So(err, ShouldBeNil)

			jaPHY := lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.JoinAccept,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.JoinAcceptPayload{},
			}
			So(jaPHY.EncryptJoinAcceptPayload(jsEncKey), ShouldBeNil)
			jaBytes, err := jaPHY.MarshalBinary()
			So(err, ShouldBeNil)

			tests := []rejoinTestCase{
				{
					BeforeFunc: func(tc *rejoinTestCase) error {
						rejoinDS := ds
						rejoinDS.RXDelay = 1
						rejoinDS.RX1DROffset = 2
						rejoinDS.RX2DR = 3
						rejoinDS.RX2Frequency = 869525000
						rejoinDS.FCntUp = 0
						rejoinDS.NFCntDown = 0
						rejoinDS.AFCntDown = 0
						rejoinDS.EnabledUplinkChannels = []int{0, 1, 2, 3, 4, 5}
						rejoinDS.ExtraUplinkChannels = map[int]band.Channel{
							3: {Frequency: 867100000, MaxDR: 5},
							4: {Frequency: 867300000, MaxDR: 5},
							5: {Frequency: 867500000, MaxDR: 5},
						}
						rejoinDS.SNwkSIntKey = lorawan.AES128Key{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 1}
						rejoinDS.FNwkSIntKey = lorawan.AES128Key{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2}
						rejoinDS.NwkSEncKey = lorawan.AES128Key{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 3}
						rejoinDS.AppSKeyEvelope = &storage.KeyEnvelope{
							AESKey: []byte{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 4},
						}

						tc.ExpectedDeviceSession.RejoinCount0 = 124
						tc.ExpectedDeviceSession.PendingRejoinDeviceSession = &rejoinDS

						cFList := lorawan.CFList{
							CFListType: lorawan.CFListChannel,
							Payload: &lorawan.CFListChannelPayload{
								Channels: [5]uint32{
									867100000,
									867300000,
									867500000,
								},
							},
						}
						cFListB, err := cFList.MarshalBinary()
						if err != nil {
							return err
						}
						tc.ExpectedRejoinReqPayload.CFList = backend.HEXBytes(cFListB)

						return nil
					},
					Name:          "valid rejoin-request type 0",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload:    rjPHY,
					JoinServerRejoinAnsPayload: backend.RejoinAnsPayload{
						PHYPayload: backend.HEXBytes(jaBytes),
						Result: backend.Result{
							ResultCode: backend.Success,
						},
						SNwkSIntKey: &backend.KeyEnvelope{
							AESKey: []byte{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 1},
						},
						FNwkSIntKey: &backend.KeyEnvelope{
							AESKey: []byte{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2},
						},
						NwkSEncKey: &backend.KeyEnvelope{
							AESKey: []byte{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 3},
						},
						AppSKey: &backend.KeyEnvelope{
							AESKey: []byte{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 4},
						},
					},
					ExpectedRejoinReqPayload: backend.RejoinReqPayload{
						BasePayload: backend.BasePayload{
							ProtocolVersion: backend.ProtocolVersion1_0,
							SenderID:        "030201",
							ReceiverID:      "0807060504030201",
							MessageType:     backend.RejoinReq,
						},
						MACVersion: "1.1.0",
						PHYPayload: backend.HEXBytes(rjBytes),
						DevEUI:     d.DevEUI,
						DLSettings: lorawan.DLSettings{
							OptNeg:      true,
							RX2DataRate: 3,
							RX1DROffset: 2,
						},
						RxDelay: 1,
						// CFList is set in the BeforeFunc
					},
					ExpectedTXInfo: gw.DownlinkTXInfo{
						GatewayId:  []byte{1, 1, 1, 1, 2, 2, 2, 2},
						Timestamp:  5000000,
						Frequency:  uint32(c0.Frequency),
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
					ExpectedPHYPayload:    jaPHY,
					ExpectedDeviceSession: ds,
				},
				{
					BeforeFunc: func(tc *rejoinTestCase) error {
						config.C.JoinServer.KEK.Set = []struct {
							Label string
							KEK   string `mapstructure:"kek"`
						}{
							{
								Label: "010203",
								KEK:   "00000000000000000000000000000000",
							},
						}

						rejoinDS := ds
						rejoinDS.RXDelay = 1
						rejoinDS.RX1DROffset = 2
						rejoinDS.RX2DR = 3
						rejoinDS.RX2Frequency = 869525000
						rejoinDS.FCntUp = 0
						rejoinDS.NFCntDown = 0
						rejoinDS.AFCntDown = 0
						rejoinDS.EnabledUplinkChannels = []int{0, 1, 2, 3, 4, 5}
						rejoinDS.ExtraUplinkChannels = map[int]band.Channel{
							3: {Frequency: 867100000, MaxDR: 5},
							4: {Frequency: 867300000, MaxDR: 5},
							5: {Frequency: 867500000, MaxDR: 5},
						}
						rejoinDS.SNwkSIntKey = lorawan.AES128Key{88, 148, 152, 153, 48, 146, 207, 219, 95, 210, 224, 42, 199, 81, 11, 241}
						rejoinDS.FNwkSIntKey = lorawan.AES128Key{83, 127, 138, 174, 137, 108, 121, 224, 21, 209, 2, 208, 98, 134, 53, 78}
						rejoinDS.NwkSEncKey = lorawan.AES128Key{152, 152, 40, 60, 79, 102, 235, 108, 111, 213, 22, 88, 130, 4, 108, 64}
						rejoinDS.AppSKeyEvelope = &storage.KeyEnvelope{
							KEKLabel: "lora-app-server",
							AESKey:   []byte{248, 215, 201, 250, 55, 176, 209, 198, 53, 78, 109, 184, 225, 157, 157, 122, 180, 229, 199, 88, 30, 159, 30, 32},
						}

						tc.ExpectedDeviceSession.RejoinCount0 = 124
						tc.ExpectedDeviceSession.PendingRejoinDeviceSession = &rejoinDS

						cFList := lorawan.CFList{
							CFListType: lorawan.CFListChannel,
							Payload: &lorawan.CFListChannelPayload{
								Channels: [5]uint32{
									867100000,
									867300000,
									867500000,
								},
							},
						}
						cFListB, err := cFList.MarshalBinary()
						if err != nil {
							return err
						}
						tc.ExpectedRejoinReqPayload.CFList = backend.HEXBytes(cFListB)

						return nil
					},
					Name:          "valid rejoin-request type 0 (with KEK)",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload:    rjPHY,
					JoinServerRejoinAnsPayload: backend.RejoinAnsPayload{
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
					ExpectedRejoinReqPayload: backend.RejoinReqPayload{
						BasePayload: backend.BasePayload{
							ProtocolVersion: backend.ProtocolVersion1_0,
							SenderID:        "030201",
							ReceiverID:      "0807060504030201",
							MessageType:     backend.RejoinReq,
						},
						MACVersion: "1.1.0",
						PHYPayload: backend.HEXBytes(rjBytes),
						DevEUI:     d.DevEUI,
						DLSettings: lorawan.DLSettings{
							OptNeg:      true,
							RX2DataRate: 3,
							RX1DROffset: 2,
						},
						RxDelay: 1,
						// CFList is set in the BeforeFunc
					},
					ExpectedTXInfo: gw.DownlinkTXInfo{
						GatewayId:  []byte{1, 1, 1, 1, 2, 2, 2, 2},
						Timestamp:  5000000,
						Frequency:  uint32(c0.Frequency),
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
					ExpectedPHYPayload:    jaPHY,
					ExpectedDeviceSession: ds,
				},
				{
					BeforeFunc: func(tc *rejoinTestCase) error {
						tc.DeviceSession.RejoinCount0 = 124
						return nil
					},
					Name:          "invalid rejoin-counter",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload:    rjPHY,
					ExpectedError: errors.New("invalid RJcount0"),
				},
				{
					Name:                     "join-server returns error",
					DeviceSession:            ds,
					TXInfo:                   txInfo,
					RXInfo:                   rxInfo,
					PHYPayload:               rjPHY,
					JoinServerRejoinReqError: errors.New("boom"),
					ExpectedError:            errors.New("rejoin-request to join-server error: boom"),
				},
			}

			runRejoinTests(asClient, jsClient, redisPool, db, tests)
		})

		Convey("Testing rejoin-request 2", func() {
			rjPHY := lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.RejoinRequest,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.RejoinRequestType02Payload{
					RejoinType: lorawan.RejoinRequestType2,
					NetID:      lorawan.NetID{3, 2, 1},
					DevEUI:     d.DevEUI,
					RJCount0:   123,
				},
			}
			So(rjPHY.SetUplinkJoinMIC(ds.SNwkSIntKey), ShouldBeNil)
			rjBytes, err := rjPHY.MarshalBinary()
			So(err, ShouldBeNil)

			jaPHY := lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.JoinAccept,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.JoinAcceptPayload{
					JoinNonce: 12345,
					HomeNetID: config.C.NetworkServer.NetID,
					DLSettings: lorawan.DLSettings{
						RX2DataRate: 2,
						RX1DROffset: 1,
					},
					DevAddr: lorawan.DevAddr{1, 2, 3, 4},
					RXDelay: 3,
				},
			}
			So(jaPHY.SetDownlinkJoinMIC(lorawan.RejoinRequestType2, ds.JoinEUI, lorawan.DevNonce(123), jsIntKey), ShouldBeNil)
			So(jaPHY.EncryptJoinAcceptPayload(jsEncKey), ShouldBeNil)
			jaBytes, err := jaPHY.MarshalBinary()
			So(err, ShouldBeNil)

			tests := []rejoinTestCase{
				{
					BeforeFunc: func(tc *rejoinTestCase) error {
						rejoinDS := ds
						rejoinDS.FCntUp = 0
						rejoinDS.NFCntDown = 0
						rejoinDS.AFCntDown = 0
						rejoinDS.SNwkSIntKey = lorawan.AES128Key{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 1}
						rejoinDS.FNwkSIntKey = lorawan.AES128Key{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2}
						rejoinDS.NwkSEncKey = lorawan.AES128Key{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 3}
						rejoinDS.AppSKeyEvelope = &storage.KeyEnvelope{
							AESKey: []byte{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 4},
						}
						rejoinDS.RejoinCount0 = 0

						tc.ExpectedDeviceSession.RejoinCount0 = 124
						tc.ExpectedDeviceSession.PendingRejoinDeviceSession = &rejoinDS
						return nil
					},
					Name:          "valid rejoin-request type 2",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload:    rjPHY,
					JoinServerRejoinAnsPayload: backend.RejoinAnsPayload{
						PHYPayload: backend.HEXBytes(jaBytes),
						Result: backend.Result{
							ResultCode: backend.Success,
						},
						SNwkSIntKey: &backend.KeyEnvelope{
							AESKey: []byte{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 1},
						},
						FNwkSIntKey: &backend.KeyEnvelope{
							AESKey: []byte{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2},
						},
						NwkSEncKey: &backend.KeyEnvelope{
							AESKey: []byte{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 3},
						},
						AppSKey: &backend.KeyEnvelope{
							AESKey: []byte{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 4},
						},
					},
					ExpectedRejoinReqPayload: backend.RejoinReqPayload{
						BasePayload: backend.BasePayload{
							ProtocolVersion: backend.ProtocolVersion1_0,
							SenderID:        "030201",
							ReceiverID:      "0807060504030201",
							MessageType:     backend.RejoinReq,
						},
						MACVersion: "1.1.0",
						PHYPayload: backend.HEXBytes(rjBytes),
						DevEUI:     d.DevEUI,
						DLSettings: lorawan.DLSettings{
							OptNeg:      true,
							RX2DataRate: 3,
							RX1DROffset: 2,
						},
						RxDelay: 1,
					},
					ExpectedTXInfo: gw.DownlinkTXInfo{
						GatewayId:  []byte{1, 1, 1, 1, 2, 2, 2, 2},
						Timestamp:  5000000,
						Frequency:  uint32(c0.Frequency),
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
					ExpectedPHYPayload:    jaPHY,
					ExpectedDeviceSession: ds,
				},
				{
					BeforeFunc: func(tc *rejoinTestCase) error {
						rejoinDS := ds
						rejoinDS.FCntUp = 0
						rejoinDS.NFCntDown = 0
						rejoinDS.AFCntDown = 0
						rejoinDS.SNwkSIntKey = lorawan.AES128Key{88, 148, 152, 153, 48, 146, 207, 219, 95, 210, 224, 42, 199, 81, 11, 241}
						rejoinDS.FNwkSIntKey = lorawan.AES128Key{83, 127, 138, 174, 137, 108, 121, 224, 21, 209, 2, 208, 98, 134, 53, 78}
						rejoinDS.NwkSEncKey = lorawan.AES128Key{152, 152, 40, 60, 79, 102, 235, 108, 111, 213, 22, 88, 130, 4, 108, 64}
						rejoinDS.AppSKeyEvelope = &storage.KeyEnvelope{
							KEKLabel: "lora-app-server",
							AESKey:   []byte{248, 215, 201, 250, 55, 176, 209, 198, 53, 78, 109, 184, 225, 157, 157, 122, 180, 229, 199, 88, 30, 159, 30, 32},
						}
						rejoinDS.RejoinCount0 = 0

						tc.ExpectedDeviceSession.RejoinCount0 = 124
						tc.ExpectedDeviceSession.PendingRejoinDeviceSession = &rejoinDS
						return nil
					},
					Name:          "valid rejoin-request type 2 (with KEK)",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload:    rjPHY,
					JoinServerRejoinAnsPayload: backend.RejoinAnsPayload{
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
					ExpectedRejoinReqPayload: backend.RejoinReqPayload{
						BasePayload: backend.BasePayload{
							ProtocolVersion: backend.ProtocolVersion1_0,
							SenderID:        "030201",
							ReceiverID:      "0807060504030201",
							MessageType:     backend.RejoinReq,
						},
						MACVersion: "1.1.0",
						PHYPayload: backend.HEXBytes(rjBytes),
						DevEUI:     d.DevEUI,
						DLSettings: lorawan.DLSettings{
							OptNeg:      true,
							RX2DataRate: 3,
							RX1DROffset: 2,
						},
						RxDelay: 1,
					},
					ExpectedTXInfo: gw.DownlinkTXInfo{
						GatewayId:  []byte{1, 1, 1, 1, 2, 2, 2, 2},
						Timestamp:  5000000,
						Frequency:  uint32(c0.Frequency),
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
					ExpectedPHYPayload:    jaPHY,
					ExpectedDeviceSession: ds,
				},
			}

			runRejoinTests(asClient, jsClient, redisPool, db, tests)
		})
	})
}

func runRejoinTests(asClient *test.ApplicationClient, jsClient *test.JoinServerClient, redisPool *redis.Pool, db sqlx.Ext, tests []rejoinTestCase) {

	for i, t := range tests {
		Convey(fmt.Sprintf("Testing: %s [%d]", t.Name, i), func() {
			if t.BeforeFunc != nil {
				So(t.BeforeFunc(&t), ShouldBeNil)
			}

			// set mocks
			jsClient.RejoinAnsPayload = t.JoinServerRejoinAnsPayload
			jsClient.RejoinReqError = t.JoinServerRejoinReqError

			// create device-session
			So(storage.SaveDeviceSession(redisPool, t.DeviceSession), ShouldBeNil)

			phyB, err := t.PHYPayload.MarshalBinary()
			So(err, ShouldBeNil)

			err = uplink.HandleRXPacket(gw.UplinkFrame{
				PhyPayload: phyB,
				RxInfo:     &t.RXInfo,
				TxInfo:     &t.TXInfo,
			})
			if t.ExpectedError != nil {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldEqual, t.ExpectedError.Error())
				return
			}
			So(err, ShouldBeNil)

			Convey("Then the expected rejoin-request was made to the join-server", func() {
				So(jsClient.RejoinReqPayloadChan, ShouldHaveLength, 1)
				req := <-jsClient.RejoinReqPayloadChan

				So(req.BasePayload.TransactionID, ShouldNotEqual, "")
				req.BasePayload.TransactionID = 0

				So(req.DevAddr, ShouldNotEqual, lorawan.DevAddr{})
				req.DevAddr = lorawan.DevAddr{}

				So(req, ShouldResemble, t.ExpectedRejoinReqPayload)
			})

			Convey("Then te expected txinfo was used", func() {
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

				So(phy, ShouldResemble, t.ExpectedPHYPayload)
			})

			Convey("Then the device-session is as expected", func() {
				ds, err := storage.GetDeviceSession(redisPool, t.DeviceSession.DevEUI)
				So(err, ShouldBeNil)

				So(ds.PendingRejoinDeviceSession, ShouldNotBeNil)
				So(ds.PendingRejoinDeviceSession.DevAddr, ShouldNotEqual, lorawan.DevAddr{})
				ds.PendingRejoinDeviceSession.DevAddr = lorawan.DevAddr{}

				So(ds, ShouldResemble, t.ExpectedDeviceSession)
			})

			Convey("Then a device-activation record was created", func() {
				da, err := storage.GetLastDeviceActivationForDevEUI(db, t.DeviceSession.DevEUI)
				So(err, ShouldBeNil)
				So(da.DevAddr, ShouldNotEqual, lorawan.DevAddr{})
				So(da.SNwkSIntKey, ShouldEqual, t.ExpectedDeviceSession.PendingRejoinDeviceSession.SNwkSIntKey)
				So(da.FNwkSIntKey, ShouldEqual, t.ExpectedDeviceSession.PendingRejoinDeviceSession.FNwkSIntKey)
				So(da.NwkSEncKey, ShouldEqual, t.ExpectedDeviceSession.PendingRejoinDeviceSession.NwkSEncKey)
			})
		})
	}
}
