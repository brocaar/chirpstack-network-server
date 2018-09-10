package testsuite

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	. "github.com/smartystreets/goconvey/convey"

	"github.com/brocaar/loraserver/api/as"
	commonPB "github.com/brocaar/loraserver/api/common"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/api/nc"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/helpers"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/loraserver/internal/uplink"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
)

type uplinkTestCase struct {
	Name       string                         // name of the test
	BeforeFunc func(tc *uplinkTestCase) error // function to run before the test

	DeviceSession       storage.DeviceSession // device-session
	TXInfo              gw.UplinkTXInfo
	RXInfo              gw.UplinkRXInfo           // rx-info of the "received" packet
	PHYPayload          lorawan.PHYPayload        // (unencrypted) "received" PHYPayload
	MACCommandPending   []storage.MACCommandBlock // pending mac-commands
	DeviceQueueItems    []storage.DeviceQueueItem // items in the device-queue
	ASHandleDataUpError error                     // application-client publish data-up error

	ExpectedControllerHandleRXInfo            *nc.HandleUplinkMetaDataRequest    // expected network-controller publish rxinfo request
	ExpectedControllerHandleDataUpMACCommands []nc.HandleUplinkMACCommandRequest // expected network-controller publish dataup mac-command requests

	ExpectedASHandleDataUp      *as.HandleUplinkDataRequest  // expected application-server data up request
	ExpectedASHandleErrors      []as.HandleErrorRequest      // expected application-server error requests
	ExpectedASHandleDownlinkACK *as.HandleDownlinkACKRequest // expected application-server datadown ack request

	ExpectedUplinkMIC           lorawan.MIC
	ExpectedTXInfo              *gw.DownlinkTXInfo  // expected tx-info (downlink)
	ExpectedPHYPayload          *lorawan.PHYPayload // expected (plaintext) PHYPayload (downlink)
	ExpectedFCntUp              uint32              // expected uplink frame counter
	ExpectedNFCntDown           uint32              // expected downlink frame counter
	ExpectedAFCntDown           uint32
	ExpectedHandleRXPacketError error // expected handleRXPacket error
	ExpectedTXPowerIndex        int   // expected tx-power set by ADR
	ExpectedNbTrans             uint8 // expected nb trans set by ADR
	ExpectedEnabledChannels     []int // expected channels enabled on the node
}

func init() {
	if err := lorawan.RegisterProprietaryMACCommand(true, 0x80, 3); err != nil {
		panic(err)
	}

	if err := lorawan.RegisterProprietaryMACCommand(true, 0x81, 2); err != nil {
		panic(err)
	}
}

func TestUplinkScenarios(t *testing.T) {
	conf := test.GetConfig()
	db, err := common.OpenDatabase(conf.PostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	config.C.PostgreSQL.DB = db
	config.C.Redis.Pool = common.NewRedisPool(conf.RedisURL, 10, 0)
	config.C.NetworkServer.NetworkSettings.InstallationMargin = 5
	config.C.NetworkServer.Band.Band, _ = band.GetConfig(band.EU_863_870, false, lorawan.DwellTimeNoLimit)
	config.C.NetworkServer.NetworkSettings.RX2DR = 0
	config.C.NetworkServer.NetworkSettings.RX1DROffset = 0
	config.C.NetworkServer.NetworkSettings.RX1Delay = 0

	Convey("Given a clean database", t, func() {
		test.MustFlushRedis(config.C.Redis.Pool)
		test.MustResetDB(config.C.PostgreSQL.DB)

		asClient := test.NewApplicationClient()
		config.C.ApplicationServer.Pool = test.NewApplicationServerPool(asClient)
		config.C.NetworkServer.Gateway.Backend.Backend = test.NewGatewayBackend()
		config.C.NetworkController.Client = test.NewNetworkControllerClient()

		gw1 := storage.Gateway{
			GatewayID: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			Location: storage.GPSPoint{
				Latitude:  1.1234,
				Longitude: 1.1235,
			},
			Altitude: 10.5,
		}
		So(storage.CreateGateway(db, &gw1), ShouldBeNil)

		// service-profile
		sp := storage.ServiceProfile{
			AddGWMetadata: true,
		}
		So(storage.CreateServiceProfile(config.C.PostgreSQL.DB, &sp), ShouldBeNil)

		// device-profile
		dp := storage.DeviceProfile{}
		So(storage.CreateDeviceProfile(config.C.PostgreSQL.DB, &dp), ShouldBeNil)

		// routing-profile
		rp := storage.RoutingProfile{}
		So(storage.CreateRoutingProfile(config.C.PostgreSQL.DB, &rp), ShouldBeNil)

		// device
		d := storage.Device{
			ServiceProfileID: sp.ID,
			DeviceProfileID:  dp.ID,
			RoutingProfileID: rp.ID,
			DevEUI:           lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
		}
		So(storage.CreateDevice(config.C.PostgreSQL.DB, &d), ShouldBeNil)

		// device-session
		ds := storage.DeviceSession{
			MACVersion:       "1.0.2",
			DeviceProfileID:  d.DeviceProfileID,
			ServiceProfileID: d.ServiceProfileID,
			RoutingProfileID: d.RoutingProfileID,
			DevEUI:           d.DevEUI,
			JoinEUI:          lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},

			DevAddr:               lorawan.DevAddr{1, 2, 3, 4},
			FNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			SNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			NwkSEncKey:            [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			FCntUp:                8,
			NFCntDown:             5,
			EnabledUplinkChannels: []int{0, 1, 2},
			RX2Frequency:          869525000,
		}

		now := time.Now().UTC().Truncate(time.Millisecond)
		c0, err := config.C.NetworkServer.Band.Band.GetUplinkChannel(0)
		So(err, ShouldBeNil)

		rxInfo := gw.UplinkRXInfo{
			GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
			LoraSnr:   7,
		}
		rxInfo.Time, _ = ptypes.TimestampProto(now)
		rxInfo.TimeSinceGpsEpoch = ptypes.DurationProto(10 * time.Second)

		txInfo := gw.UplinkTXInfo{
			Frequency: uint32(c0.Frequency),
		}
		So(helpers.SetUplinkTXInfoDataRate(&txInfo, 0, config.C.NetworkServer.Band.Band), ShouldBeNil)

		var fPortZero uint8
		var fPortOne uint8 = 1

		expectedApplicationPushDataUpNoData := &as.HandleUplinkDataRequest{
			JoinEui: ds.JoinEUI[:],
			DevEui:  ds.DevEUI[:],
			FCnt:    10,
			FPort:   1,
			Data:    nil,
			TxInfo:  &txInfo,
			RxInfo:  []*gw.UplinkRXInfo{&rxInfo},
		}
		expectedApplicationPushDataUpNoData.RxInfo[0].Location = &commonPB.Location{
			Latitude:  gw1.Location.Latitude,
			Longitude: gw1.Location.Longitude,
			Altitude:  gw1.Altitude,
		}

		expectedControllerHandleRXInfo := &nc.HandleUplinkMetaDataRequest{
			DevEui: ds.DevEUI[:],
			TxInfo: expectedApplicationPushDataUpNoData.TxInfo,
			RxInfo: []*gw.UplinkRXInfo{&rxInfo},
		}

		Convey("Given a set of test-scenarios for error handling", func() {
			tests := []uplinkTestCase{
				{
					Name:          "the frame-counter is invalid",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    7,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedUplinkMIC:           lorawan.MIC{48, 94, 26, 239},
					ExpectedFCntUp:              8,
					ExpectedNFCntDown:           5,
					ExpectedHandleRXPacketError: errors.New("get device-session error: device-session does not exist or invalid fcnt or mic"),
					ExpectedEnabledChannels:     []int{0, 1, 2},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.DeviceSession.MACVersion = "1.1.0"

						// the MIC will be calculated for channel 0, we set it to channel 1
						tc.TXInfo.Frequency = 868300000
						return nil
					},
					Name:          "the frequency is invalid (MIC)",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedUplinkMIC:           lorawan.MIC{160, 195, 160, 195},
					ExpectedFCntUp:              8,
					ExpectedNFCntDown:           5,
					ExpectedHandleRXPacketError: errors.New("get device-session error: device-session does not exist or invalid fcnt or mic"),
					ExpectedEnabledChannels:     []int{0, 1, 2},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.DeviceSession.MACVersion = "1.1.0"

						// the MIC will be calculated for DR0, set it to DR1
						modInfo := tc.TXInfo.GetLoraModulationInfo()
						modInfo.SpreadingFactor = 11
						return nil
					},
					Name:          "the data-rate is invalid (MIC)",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedUplinkMIC:           lorawan.MIC{160, 195, 160, 195},
					ExpectedFCntUp:              8,
					ExpectedNFCntDown:           5,
					ExpectedHandleRXPacketError: errors.New("get device-session error: device-session does not exist or invalid fcnt or mic"),
					ExpectedEnabledChannels:     []int{0, 1, 2},
				},
			}

			runUplinkTests(asClient, tests)
		})

		Convey("Given a set of test-scenarios for relax frame-counter mode", func() {
			expectedApplicationPushDataUpNoData := &as.HandleUplinkDataRequest{
				JoinEui: ds.JoinEUI[:],
				DevEui:  ds.DevEUI[:],
				FCnt:    0,
				FPort:   1,
				Data:    nil,
				TxInfo:  expectedApplicationPushDataUpNoData.TxInfo,
				RxInfo:  expectedApplicationPushDataUpNoData.RxInfo,
			}

			expectedApplicationPushDataUpNoData7 := *expectedApplicationPushDataUpNoData
			expectedApplicationPushDataUpNoData7.FCnt = 7

			tests := []uplinkTestCase{
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.DeviceSession.SkipFCntValidation = true
						return nil
					},

					Name:          "the frame-counter is invalid but not 0",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    7,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{48, 94, 26, 239},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         &expectedApplicationPushDataUpNoData7,
					ExpectedFCntUp:                 8,
					ExpectedNFCntDown:              5,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.DeviceSession.SkipFCntValidation = true
						return nil
					},

					Name:          "the frame-counter is invalid and 0",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    0,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{131, 36, 83, 163},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedFCntUp:                 1,
					ExpectedNFCntDown:              5,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
			}

			runUplinkTests(asClient, tests)
		})

		Convey("Given a set of test-scenarios for basic flows (nothing in the queue)", func() {
			inTenMinutes := time.Now().Add(10 * time.Minute)

			tests := []uplinkTestCase{
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.ExpectedASHandleDataUp.Data = []byte{1, 2, 3, 4}
						return nil
					},

					Name:          "unconfirmed uplink data with payload",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
							},
							FPort:      &fPortOne,
							FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{104, 147, 35, 121},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedFCntUp:                 11,
					ExpectedNFCntDown:              5,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.DeviceSession.AppSKeyEvelope = &storage.KeyEnvelope{
							KEKLabel: "lora-app-server",
							AESKey:   []byte{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
						}

						tc.ExpectedASHandleDataUp.Data = []byte{1, 2, 3, 4}
						tc.ExpectedASHandleDataUp.DeviceActivationContext = &as.DeviceActivationContext{
							DevAddr: tc.DeviceSession.DevAddr[:],
							AppSKey: &commonPB.KeyEnvelope{
								KekLabel: "lora-app-server",
								AesKey:   []byte{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
							},
						}
						return nil
					},

					Name:          "unconfirmed uplink data with payload + AppSKey envelope",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
							},
							FPort:      &fPortOne,
							FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{104, 147, 35, 121},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedFCntUp:                 11,
					ExpectedNFCntDown:              5,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.ExpectedASHandleDataUp.Data = []byte{1, 2, 3, 4}
						tc.DeviceSession.MACVersion = "1.1.0"

						return nil
					},

					Name:          "unconfirmed uplink data with payload (LoRaWAN 1.1)",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
							},
							FPort:      &fPortOne,
							FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{104, 147, 104, 147},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedFCntUp:                 11,
					ExpectedNFCntDown:              5,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.ExpectedASHandleDataUp.Data = []byte{1, 2, 3, 4}
						return nil
					},

					Name: "unconfirmed uplink data with payload + ACK",
					DeviceQueueItems: []storage.DeviceQueueItem{
						{DevEUI: d.DevEUI, FRMPayload: []byte{1}, FPort: 1, FCnt: 4, IsPending: true, TimeoutAfter: &inTenMinutes},
					},
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
								FCtrl: lorawan.FCtrl{
									ACK: true,
								},
							},
							FPort:      &fPortOne,
							FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{132, 250, 228, 10},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedASHandleDownlinkACK:    &as.HandleDownlinkACKRequest{DevEui: d.DevEUI[:], FCnt: 4, Acknowledged: true},
					ExpectedFCntUp:                 11,
					ExpectedNFCntDown:              5,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						ds.ConfFCnt = 1
						tc.ExpectedASHandleDataUp.Data = []byte{1, 2, 3, 4}
						return nil
					},

					Name: "unconfirmed uplink data with payload + ACK (LoRaWAN 1.1)",
					DeviceQueueItems: []storage.DeviceQueueItem{
						{DevEUI: d.DevEUI, FRMPayload: []byte{1}, FPort: 1, FCnt: 4, IsPending: true, TimeoutAfter: &inTenMinutes},
					},
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
								FCtrl: lorawan.FCtrl{
									ACK: true,
								},
							},
							FPort:      &fPortOne,
							FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{132, 250, 228, 10},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedASHandleDownlinkACK:    &as.HandleDownlinkACKRequest{DevEui: d.DevEUI[:], FCnt: 4, Acknowledged: true},
					ExpectedFCntUp:                 11,
					ExpectedNFCntDown:              5,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
				{
					Name:          "unconfirmed uplink data without payload (just a FPort)",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{160, 195, 68, 8},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedFCntUp:                 11,
					ExpectedNFCntDown:              5,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.ExpectedASHandleDataUp.Data = []byte{1, 2, 3, 4}
						return nil
					},

					Name:          "confirmed uplink data with payload",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.ConfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
							},
							FPort:      &fPortOne,
							FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{69, 90, 200, 95},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedTXInfo: &gw.DownlinkTXInfo{
						GatewayId:  rxInfo.GatewayId,
						Frequency:  txInfo.Frequency,
						Power:      14,
						Timestamp:  rxInfo.Timestamp + 1000000,
						Modulation: commonPB.Modulation_LORA,
						ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
							LoraModulationInfo: &gw.LoRaModulationInfo{
								Bandwidth:             125,
								SpreadingFactor:       12,
								PolarizationInversion: true,
								CodeRate:              "4/5",
							},
						},
					},
					ExpectedPHYPayload: &lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataDown,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    5,
								FCtrl: lorawan.FCtrl{
									ACK: true,
									ADR: true,
								},
							},
						},
					},
					ExpectedFCntUp:          11,
					ExpectedNFCntDown:       6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					Name:          "confirmed uplink data without payload",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.ConfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{210, 52, 52, 94},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedTXInfo: &gw.DownlinkTXInfo{
						GatewayId:  rxInfo.GatewayId,
						Frequency:  txInfo.Frequency,
						Power:      14,
						Timestamp:  rxInfo.Timestamp + 1000000,
						Modulation: commonPB.Modulation_LORA,
						ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
							LoraModulationInfo: &gw.LoRaModulationInfo{
								Bandwidth:             125,
								SpreadingFactor:       12,
								PolarizationInversion: true,
								CodeRate:              "4/5",
							},
						},
					},
					ExpectedPHYPayload: &lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataDown,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    5,
								FCtrl: lorawan.FCtrl{
									ACK: true,
									ADR: true,
								},
							},
						},
					},
					ExpectedFCntUp:          11,
					ExpectedNFCntDown:       6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.DeviceSession.RXDelay = 3
						return nil
					},

					Name:          "confirmed uplink data without payload (with RXDelay=3)",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.ConfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{210, 52, 52, 94},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedTXInfo: &gw.DownlinkTXInfo{
						GatewayId:  rxInfo.GatewayId,
						Frequency:  txInfo.Frequency,
						Power:      14,
						Timestamp:  rxInfo.Timestamp + 3000000,
						Modulation: commonPB.Modulation_LORA,
						ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
							LoraModulationInfo: &gw.LoRaModulationInfo{
								Bandwidth:             125,
								SpreadingFactor:       12,
								PolarizationInversion: true,
								CodeRate:              "4/5",
							},
						},
					},
					ExpectedPHYPayload: &lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataDown,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    5,
								FCtrl: lorawan.FCtrl{
									ACK: true,
									ADR: true,
								},
							},
						},
					},
					ExpectedFCntUp:          11,
					ExpectedNFCntDown:       6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					Name:          "two uplink mac commands (FOpts)",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
								FOpts: []lorawan.Payload{
									&lorawan.MACCommand{CID: 0x80, Payload: &lorawan.ProprietaryMACCommandPayload{Bytes: []byte{1, 2, 3}}},
									&lorawan.MACCommand{CID: 0x81, Payload: &lorawan.ProprietaryMACCommandPayload{Bytes: []byte{4, 5}}},
								},
							},
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{218, 0, 109, 32},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedControllerHandleDataUpMACCommands: []nc.HandleUplinkMACCommandRequest{
						{DevEui: ds.DevEUI[:], Cid: 128, Commands: [][]byte{{128, 1, 2, 3}}},
						{DevEui: ds.DevEUI[:], Cid: 129, Commands: [][]byte{{129, 4, 5}}},
					},
					ExpectedFCntUp:          11,
					ExpectedNFCntDown:       5,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.DeviceSession.MACVersion = "1.1.0"
						return nil

					},
					Name:          "two uplink mac commands (FOpts) (LoRaWAN 1.1)",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
								FOpts: []lorawan.Payload{
									&lorawan.DataPayload{Bytes: []byte{}},
									&lorawan.MACCommand{CID: 0x80, Payload: &lorawan.ProprietaryMACCommandPayload{Bytes: []byte{1, 2, 3}}},
									&lorawan.MACCommand{CID: 0x81, Payload: &lorawan.ProprietaryMACCommandPayload{Bytes: []byte{4, 5}}},
								},
							},
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{205, 171, 205, 171},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedControllerHandleDataUpMACCommands: []nc.HandleUplinkMACCommandRequest{
						{DevEui: ds.DevEUI[:], Cid: 128, Commands: [][]byte{{128, 1, 2, 3}}},
						{DevEui: ds.DevEUI[:], Cid: 129, Commands: [][]byte{{129, 4, 5}}},
					},
					ExpectedFCntUp:          11,
					ExpectedNFCntDown:       5,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					Name:          "two uplink mac commands (FRMPayload)",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
							},
							FPort: &fPortZero,
							FRMPayload: []lorawan.Payload{
								&lorawan.MACCommand{CID: 0x80, Payload: &lorawan.ProprietaryMACCommandPayload{Bytes: []byte{1, 2, 3}}},
								&lorawan.MACCommand{CID: 0x81, Payload: &lorawan.ProprietaryMACCommandPayload{Bytes: []byte{4, 5}}},
							},
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{161, 39, 115, 252},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedControllerHandleDataUpMACCommands: []nc.HandleUplinkMACCommandRequest{
						{DevEui: ds.DevEUI[:], Cid: 128, Commands: [][]byte{{128, 1, 2, 3}}},
						{DevEui: ds.DevEUI[:], Cid: 129, Commands: [][]byte{{129, 4, 5}}},
					},
					ExpectedFCntUp:          11,
					ExpectedNFCntDown:       5,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.ExpectedASHandleDataUp.Data = []byte{1, 2, 3, 4}
						tc.ExpectedASHandleDataUp.FCnt = 65536

						tc.DeviceSession.FCntUp = 65535
						return nil
					},

					Name:          "unconfirmed uplink with FCnt rollover",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    65536,
							},
							FPort:      &fPortOne,
							FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{46, 116, 250, 252},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedFCntUp:                 65537,
					ExpectedNFCntDown:              5,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
				{

					BeforeFunc: func(tc *uplinkTestCase) error {
						// remove rx info set
						tc.ExpectedASHandleDataUp.RxInfo = nil
						tc.ExpectedASHandleDataUp.Data = []byte{1, 2, 3, 4}

						// set add gw meta-data to false
						sp.AddGWMetadata = false
						return storage.UpdateServiceProfile(config.C.PostgreSQL.DB, &sp)
					},

					Name:          "unconfirmed uplink data with payload (service-profile: no gateway info)",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
							},
							FPort:      &fPortOne,
							FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{104, 147, 35, 121},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedFCntUp:                 11,
					ExpectedNFCntDown:              5,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
			}

			runUplinkTests(asClient, tests)
		})

		Convey("Given a set of test-scenarios for mac-commands", func() {
			var fPortThree uint8 = 3

			tests := []uplinkTestCase{
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						sp.DevStatusReqFreq = 1
						So(storage.UpdateServiceProfile(config.C.PostgreSQL.DB, &sp), ShouldBeNil)

						tc.ExpectedASHandleDataUp.Data = []byte{1, 2, 3, 4}
						return nil
					},

					Name:          "unconfirmed uplink data + two downlink mac commands in queue (FOpts)",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
							},
							FPort:      &fPortOne,
							FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{104, 147, 35, 121},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedTXInfo: &gw.DownlinkTXInfo{
						GatewayId:  rxInfo.GatewayId,
						Frequency:  txInfo.Frequency,
						Power:      14,
						Timestamp:  rxInfo.Timestamp + 1000000,
						Modulation: commonPB.Modulation_LORA,
						ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
							LoraModulationInfo: &gw.LoRaModulationInfo{
								Bandwidth:             125,
								SpreadingFactor:       12,
								PolarizationInversion: true,
								CodeRate:              "4/5",
							},
						},
					},
					ExpectedPHYPayload: &lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataDown,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								FCtrl: lorawan.FCtrl{
									ADR: true,
								},
								DevAddr: ds.DevAddr,
								FCnt:    5,
								FOpts: []lorawan.Payload{
									&lorawan.MACCommand{CID: lorawan.CID(6)},
								},
							},
						},
					},
					ExpectedFCntUp:          11,
					ExpectedNFCntDown:       6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						sp.DevStatusReqFreq = 1
						So(storage.UpdateServiceProfile(config.C.PostgreSQL.DB, &sp), ShouldBeNil)

						tc.ExpectedASHandleDataUp.Data = []byte{1, 2, 3, 4}
						return nil
					},

					Name:          "unconfirmed uplink data + downlink mac command (FOpts) + unconfirmed data down",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					DeviceQueueItems: []storage.DeviceQueueItem{
						{DevEUI: d.DevEUI, FPort: 3, FCnt: 5, FRMPayload: []byte{4, 5, 6}},
					},
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
							},
							FPort:      &fPortOne,
							FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{104, 147, 35, 121},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedTXInfo: &gw.DownlinkTXInfo{
						GatewayId:  rxInfo.GatewayId,
						Frequency:  txInfo.Frequency,
						Power:      14,
						Timestamp:  rxInfo.Timestamp + 1000000,
						Modulation: commonPB.Modulation_LORA,
						ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
							LoraModulationInfo: &gw.LoRaModulationInfo{
								Bandwidth:             125,
								SpreadingFactor:       12,
								PolarizationInversion: true,
								CodeRate:              "4/5",
							},
						},
					},
					ExpectedPHYPayload: &lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataDown,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								FCtrl: lorawan.FCtrl{
									ADR: true,
								},
								DevAddr: ds.DevAddr,
								FCnt:    5,
								FOpts: []lorawan.Payload{
									&lorawan.MACCommand{CID: lorawan.CID(6)},
								},
							},
							FPort: &fPortThree,
							FRMPayload: []lorawan.Payload{
								&lorawan.DataPayload{Bytes: []byte{4, 5, 6}},
							},
						},
					},
					ExpectedFCntUp:          11,
					ExpectedNFCntDown:       6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
			}

			runUplinkTests(asClient, tests)
		})

		Convey("Given a set of test-scenarios for downlink queue (LoRaWAN 1.1)", func() {
			var fPortTen uint8 = 10
			ds.MACVersion = "1.1.0"

			tests := []uplinkTestCase{
				{
					Name:          "unconfirmed uplink data + one unconfirmed downlink payload in queue",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					DeviceQueueItems: []storage.DeviceQueueItem{
						{DevEUI: d.DevEUI, FPort: 10, FCnt: 3, FRMPayload: []byte{1, 2, 3, 4}},
					},
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{160, 195, 160, 195},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedTXInfo: &gw.DownlinkTXInfo{
						GatewayId:  rxInfo.GatewayId,
						Frequency:  txInfo.Frequency,
						Power:      14,
						Timestamp:  rxInfo.Timestamp + 1000000,
						Modulation: commonPB.Modulation_LORA,
						ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
							LoraModulationInfo: &gw.LoRaModulationInfo{
								Bandwidth:             125,
								SpreadingFactor:       12,
								PolarizationInversion: true,
								CodeRate:              "4/5",
							},
						},
					},
					ExpectedPHYPayload: &lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataDown,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    3,
								FCtrl: lorawan.FCtrl{
									ADR: true,
								},
							},
							FPort: &fPortTen,
							FRMPayload: []lorawan.Payload{
								&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}},
							},
						},
					},
					ExpectedFCntUp:          11,
					ExpectedNFCntDown:       5,
					ExpectedAFCntDown:       4,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					Name:          "unconfirmed uplink data + one confirmed downlink payload in queue",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					DeviceQueueItems: []storage.DeviceQueueItem{
						{DevEUI: d.DevEUI, FPort: 10, FCnt: 3, FRMPayload: []byte{1, 2, 3, 4}, Confirmed: true},
					},
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{160, 195, 160, 195},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedTXInfo: &gw.DownlinkTXInfo{
						GatewayId:  rxInfo.GatewayId,
						Frequency:  txInfo.Frequency,
						Power:      14,
						Timestamp:  rxInfo.Timestamp + 1000000,
						Modulation: commonPB.Modulation_LORA,
						ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
							LoraModulationInfo: &gw.LoRaModulationInfo{
								Bandwidth:             125,
								SpreadingFactor:       12,
								PolarizationInversion: true,
								CodeRate:              "4/5",
							},
						},
					},
					ExpectedPHYPayload: &lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.ConfirmedDataDown,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    3,
								FCtrl: lorawan.FCtrl{
									ADR: true,
								},
							},
							FPort: &fPortTen,
							FRMPayload: []lorawan.Payload{
								&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}},
							},
						},
					},
					ExpectedFCntUp:          11,
					ExpectedNFCntDown:       5,
					ExpectedAFCntDown:       4,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
			}

			runUplinkTests(asClient, tests)
		})

		Convey("Given a set of test-scenarios for downlink queue (LoRaWAN 1.0)", func() {
			var fPortTen uint8 = 10

			tests := []uplinkTestCase{
				{
					Name:          "unconfirmed uplink data + one unconfirmed downlink payload in queue",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					DeviceQueueItems: []storage.DeviceQueueItem{
						{DevEUI: d.DevEUI, FPort: 10, FCnt: 5, FRMPayload: []byte{1, 2, 3, 4}},
					},
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{160, 195, 68, 8},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedTXInfo: &gw.DownlinkTXInfo{
						GatewayId:  rxInfo.GatewayId,
						Frequency:  txInfo.Frequency,
						Power:      14,
						Timestamp:  rxInfo.Timestamp + 1000000,
						Modulation: commonPB.Modulation_LORA,
						ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
							LoraModulationInfo: &gw.LoRaModulationInfo{
								Bandwidth:             125,
								SpreadingFactor:       12,
								PolarizationInversion: true,
								CodeRate:              "4/5",
							},
						},
					},
					ExpectedPHYPayload: &lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataDown,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    5,
								FCtrl: lorawan.FCtrl{
									ADR: true,
								},
							},
							FPort: &fPortTen,
							FRMPayload: []lorawan.Payload{
								&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}},
							},
						},
					},
					ExpectedFCntUp:          11,
					ExpectedNFCntDown:       6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					Name:          "unconfirmed uplink data + two unconfirmed downlink payloads in queue",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					DeviceQueueItems: []storage.DeviceQueueItem{
						{DevEUI: d.DevEUI, FPort: 10, FCnt: 5, FRMPayload: []byte{1, 2, 3, 4}},
						{DevEUI: d.DevEUI, FPort: 10, FCnt: 6, FRMPayload: []byte{5, 6, 7, 8}},
					},
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{160, 195, 68, 8},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedTXInfo: &gw.DownlinkTXInfo{
						GatewayId:  rxInfo.GatewayId,
						Frequency:  txInfo.Frequency,
						Power:      14,
						Timestamp:  rxInfo.Timestamp + 1000000,
						Modulation: commonPB.Modulation_LORA,
						ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
							LoraModulationInfo: &gw.LoRaModulationInfo{
								Bandwidth:             125,
								SpreadingFactor:       12,
								PolarizationInversion: true,
								CodeRate:              "4/5",
							},
						},
					},
					ExpectedPHYPayload: &lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataDown,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    5,
								FCtrl: lorawan.FCtrl{
									FPending: true,
									ADR:      true,
								},
							},
							FPort: &fPortTen,
							FRMPayload: []lorawan.Payload{
								&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}},
							},
						},
					},
					ExpectedFCntUp:          11,
					ExpectedNFCntDown:       6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					Name:          "unconfirmed uplink data + one confirmed downlink payload in queue",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					DeviceQueueItems: []storage.DeviceQueueItem{
						{DevEUI: d.DevEUI, FPort: 10, FCnt: 5, FRMPayload: []byte{1, 2, 3, 4}, Confirmed: true},
					},
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{160, 195, 68, 8},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedTXInfo: &gw.DownlinkTXInfo{
						GatewayId:  rxInfo.GatewayId,
						Frequency:  txInfo.Frequency,
						Power:      14,
						Timestamp:  rxInfo.Timestamp + 1000000,
						Modulation: commonPB.Modulation_LORA,
						ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
							LoraModulationInfo: &gw.LoRaModulationInfo{
								Bandwidth:             125,
								SpreadingFactor:       12,
								PolarizationInversion: true,
								CodeRate:              "4/5",
							},
						},
					},
					ExpectedPHYPayload: &lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.ConfirmedDataDown,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    5,
								FCtrl: lorawan.FCtrl{
									ADR: true,
								},
							},
							FPort: &fPortTen,
							FRMPayload: []lorawan.Payload{
								&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}},
							},
						},
					},
					ExpectedFCntUp:          11,
					ExpectedNFCntDown:       6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					Name:          "unconfirmed uplink data + downlink payload which exceeds the max payload size (for dr 0)",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					DeviceQueueItems: []storage.DeviceQueueItem{
						{DevEUI: d.DevEUI, FPort: 10, FCnt: 5, FRMPayload: make([]byte, 52)},
					},
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{160, 195, 68, 8},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedFCntUp:                 11,
					ExpectedNFCntDown:              5, // payload has been discarded, nothing to transmit
					ExpectedEnabledChannels:        []int{0, 1, 2},
					ExpectedASHandleErrors: []as.HandleErrorRequest{
						{DevEui: d.DevEUI[:], Type: as.ErrorType_DEVICE_QUEUE_ITEM_SIZE, Error: "payload exceeds max payload size", FCnt: 5},
					},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						sp.DevStatusReqFreq = 1
						So(storage.UpdateServiceProfile(config.C.PostgreSQL.DB, &sp), ShouldBeNil)
						return nil
					},
					Name:          "unconfirmed uplink data + one unconfirmed downlink payload in queue (exactly max size for dr 0) + one mac command",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					DeviceQueueItems: []storage.DeviceQueueItem{
						{DevEUI: d.DevEUI, FPort: 10, FCnt: 5, FRMPayload: make([]byte, 51)},
					},
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{160, 195, 68, 8},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedTXInfo: &gw.DownlinkTXInfo{
						GatewayId:  rxInfo.GatewayId,
						Frequency:  txInfo.Frequency,
						Power:      14,
						Timestamp:  rxInfo.Timestamp + 1000000,
						Modulation: commonPB.Modulation_LORA,
						ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
							LoraModulationInfo: &gw.LoRaModulationInfo{
								Bandwidth:             125,
								SpreadingFactor:       12,
								PolarizationInversion: true,
								CodeRate:              "4/5",
							},
						},
					},
					ExpectedPHYPayload: &lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataDown,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    5,
								FCtrl: lorawan.FCtrl{
									FPending: true,
									ADR:      true,
								},
							},
							FPort: &fPortTen,
							FRMPayload: []lorawan.Payload{
								&lorawan.DataPayload{Bytes: make([]byte, 51)},
							},
						},
					},
					ExpectedFCntUp:          11,
					ExpectedNFCntDown:       6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
			}

			runUplinkTests(asClient, tests)
		})

		Convey("Given a set of test-scenarios for ADR", func() {
			tests := []uplinkTestCase{
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.DeviceSession.FCntUp = 10
						return nil
					},

					Name:          "adr triggered",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
								FCtrl: lorawan.FCtrl{
									ADR: true,
								},
							},
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{187, 243, 244, 117},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedFCntUp:                 11,
					ExpectedNFCntDown:              6,
					ExpectedTXInfo: &gw.DownlinkTXInfo{
						GatewayId:  rxInfo.GatewayId,
						Frequency:  txInfo.Frequency,
						Power:      14,
						Timestamp:  rxInfo.Timestamp + 1000000,
						Modulation: commonPB.Modulation_LORA,
						ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
							LoraModulationInfo: &gw.LoRaModulationInfo{
								Bandwidth:             125,
								SpreadingFactor:       12,
								PolarizationInversion: true,
								CodeRate:              "4/5",
							},
						},
					},
					ExpectedPHYPayload: &lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataDown,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    5,
								FCtrl: lorawan.FCtrl{
									ADR: true,
								},
								FOpts: []lorawan.Payload{
									&lorawan.MACCommand{
										CID: lorawan.LinkADRReq,
										Payload: &lorawan.LinkADRReqPayload{
											DataRate: 5,
											TXPower:  2,
											ChMask:   [16]bool{true, true, true},
											Redundancy: lorawan.Redundancy{
												ChMaskCntl: 0,
												NbRep:      1,
											},
										},
									},
								},
							},
						},
					},
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.DeviceSession.FCntUp = 10
						return nil
					},

					Name:          "adr interval matches, but node does not support adr",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
								FCtrl: lorawan.FCtrl{
									ADR: false,
								},
							},
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{122, 152, 152, 220},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedFCntUp:                 11,
					ExpectedNFCntDown:              5,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.DeviceSession.FCntUp = 10
						return nil
					},

					Name:          "acknowledgement of pending adr request",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					MACCommandPending: []storage.MACCommandBlock{
						{
							CID: lorawan.LinkADRReq,
							MACCommands: []lorawan.MACCommand{
								{
									CID: lorawan.LinkADRReq,
									Payload: &lorawan.LinkADRReqPayload{
										DataRate: 0,
										TXPower:  3,
										ChMask:   [16]bool{true, true, true},
										Redundancy: lorawan.Redundancy{
											ChMaskCntl: 0,
											NbRep:      1,
										},
									},
								},
							},
						},
					},
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
								FOpts: []lorawan.Payload{
									&lorawan.MACCommand{CID: lorawan.LinkADRAns, Payload: &lorawan.LinkADRAnsPayload{ChannelMaskACK: true, DataRateACK: true, PowerACK: true}},
								},
							},
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{235, 224, 96, 3},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedFCntUp:                 11,
					ExpectedNFCntDown:              5,
					ExpectedTXPowerIndex:           3,
					ExpectedNbTrans:                1,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.DeviceSession.FCntUp = 10
						return nil
					},

					Name:          "negative acknowledgement of pending adr request",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					MACCommandPending: []storage.MACCommandBlock{
						{
							CID: lorawan.LinkADRReq,
							MACCommands: []lorawan.MACCommand{
								{
									CID: lorawan.LinkADRReq,
									Payload: &lorawan.LinkADRReqPayload{
										DataRate: 5,
										TXPower:  3,
										ChMask:   [16]bool{true, true, true},
										Redundancy: lorawan.Redundancy{
											ChMaskCntl: 0,
											NbRep:      1,
										},
									},
								},
							},
						},
					},
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
								FOpts: []lorawan.Payload{
									&lorawan.MACCommand{CID: lorawan.LinkADRAns, Payload: &lorawan.LinkADRAnsPayload{ChannelMaskACK: false, DataRateACK: true, PowerACK: true}},
								},
							},
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{252, 17, 226, 74},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedFCntUp:                 11,
					ExpectedNFCntDown:              5,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
				{
					Name:          "adr ack requested",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
								FCtrl: lorawan.FCtrl{
									ADRACKReq: true,
								},
							},
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{73, 26, 32, 42},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedFCntUp:                 11,
					ExpectedNFCntDown:              6,
					ExpectedTXInfo: &gw.DownlinkTXInfo{
						GatewayId:  rxInfo.GatewayId,
						Frequency:  txInfo.Frequency,
						Power:      14,
						Timestamp:  rxInfo.Timestamp + 1000000,
						Modulation: commonPB.Modulation_LORA,
						ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
							LoraModulationInfo: &gw.LoRaModulationInfo{
								Bandwidth:             125,
								SpreadingFactor:       12,
								PolarizationInversion: true,
								CodeRate:              "4/5",
							},
						},
					},
					ExpectedPHYPayload: &lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataDown,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    5,
								FCtrl: lorawan.FCtrl{
									ADR: true,
								},
							},
						},
					},
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.DeviceSession.EnabledUplinkChannels = []int{0, 1, 2, 3, 4, 5, 6, 7}
						return nil
					},

					Name:          "channel re-configuration triggered",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
							},
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{122, 152, 152, 220},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedFCntUp:                 11,
					ExpectedNFCntDown:              6,
					ExpectedTXInfo: &gw.DownlinkTXInfo{
						GatewayId:  rxInfo.GatewayId,
						Frequency:  txInfo.Frequency,
						Power:      14,
						Timestamp:  rxInfo.Timestamp + 1000000,
						Modulation: commonPB.Modulation_LORA,
						ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
							LoraModulationInfo: &gw.LoRaModulationInfo{
								Bandwidth:             125,
								SpreadingFactor:       12,
								PolarizationInversion: true,
								CodeRate:              "4/5",
							},
						},
					},
					ExpectedPHYPayload: &lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataDown,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    5,
								FCtrl: lorawan.FCtrl{
									ADR: true,
								},
								FOpts: []lorawan.Payload{
									&lorawan.MACCommand{
										CID: lorawan.LinkADRReq,
										Payload: &lorawan.LinkADRReqPayload{
											TXPower: 0,
											ChMask:  lorawan.ChMask{true, true, true},
										},
									},
								},
							},
						},
					},
					ExpectedEnabledChannels: []int{0, 1, 2, 3, 4, 5, 6, 7},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.DeviceSession.EnabledUplinkChannels = []int{0, 1, 2, 3, 4, 5, 6, 7}
						return nil
					},

					Name:          "new channel re-configuration ack-ed",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					MACCommandPending: []storage.MACCommandBlock{
						{
							CID: lorawan.LinkADRReq,
							MACCommands: storage.MACCommands{
								{
									CID: lorawan.LinkADRReq,
									Payload: &lorawan.LinkADRReqPayload{
										TXPower: 1,
										ChMask:  lorawan.ChMask{true, true, true},
									},
								},
							},
						},
					},
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
								FOpts: []lorawan.Payload{
									&lorawan.MACCommand{
										CID: lorawan.LinkADRAns,
										Payload: &lorawan.LinkADRAnsPayload{
											ChannelMaskACK: true,
											DataRateACK:    true,
											PowerACK:       true,
										},
									},
								},
							},
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{235, 224, 96, 3},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedFCntUp:                 11,
					ExpectedNFCntDown:              5,
					ExpectedTXPowerIndex:           1,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.DeviceSession.EnabledUplinkChannels = []int{0, 1, 2, 3, 4, 5, 6, 7}
						return nil
					},

					Name:          "new channel re-configuration not ack-ed",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					MACCommandPending: []storage.MACCommandBlock{
						{
							CID: lorawan.LinkADRReq,
							MACCommands: storage.MACCommands{
								{
									CID: lorawan.LinkADRReq,
									Payload: &lorawan.LinkADRReqPayload{
										TXPower: 1,
										ChMask:  lorawan.ChMask{true, true, true},
									},
								},
							},
						},
					},
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
								FOpts: []lorawan.Payload{
									&lorawan.MACCommand{
										CID: lorawan.LinkADRAns,
										Payload: &lorawan.LinkADRAnsPayload{
											ChannelMaskACK: false,
											DataRateACK:    true,
											PowerACK:       true,
										},
									},
								},
							},
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{252, 17, 226, 74},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedFCntUp:                 11,
					ExpectedNFCntDown:              6,
					ExpectedTXInfo: &gw.DownlinkTXInfo{
						GatewayId:  rxInfo.GatewayId,
						Frequency:  txInfo.Frequency,
						Power:      14,
						Timestamp:  rxInfo.Timestamp + 1000000,
						Modulation: commonPB.Modulation_LORA,
						ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
							LoraModulationInfo: &gw.LoRaModulationInfo{
								Bandwidth:             125,
								SpreadingFactor:       12,
								PolarizationInversion: true,
								CodeRate:              "4/5",
							},
						},
					},
					ExpectedPHYPayload: &lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataDown,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    5,
								FCtrl: lorawan.FCtrl{
									ADR: true,
								},
								FOpts: []lorawan.Payload{
									&lorawan.MACCommand{
										CID: lorawan.LinkADRReq,
										Payload: &lorawan.LinkADRReqPayload{
											TXPower: 0,
											ChMask:  lorawan.ChMask{true, true, true},
										},
									},
								},
							},
						},
					},
					ExpectedEnabledChannels: []int{0, 1, 2, 3, 4, 5, 6, 7},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.DeviceSession.FCntUp = 10
						tc.DeviceSession.EnabledUplinkChannels = []int{0, 1, 2, 3, 4, 5, 6, 7}
						return nil
					},

					Name:          "channel re-configuration and adr triggered",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
								FCtrl: lorawan.FCtrl{
									ADR: true,
								},
							},
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{187, 243, 244, 117},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedFCntUp:                 11,
					ExpectedNFCntDown:              6,
					ExpectedTXInfo: &gw.DownlinkTXInfo{
						GatewayId:  rxInfo.GatewayId,
						Frequency:  txInfo.Frequency,
						Power:      14,
						Timestamp:  rxInfo.Timestamp + 1000000,
						Modulation: commonPB.Modulation_LORA,
						ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
							LoraModulationInfo: &gw.LoRaModulationInfo{
								Bandwidth:             125,
								SpreadingFactor:       12,
								PolarizationInversion: true,
								CodeRate:              "4/5",
							},
						},
					},
					ExpectedPHYPayload: &lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataDown,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    5,
								FCtrl: lorawan.FCtrl{
									ADR: true,
								},
								FOpts: []lorawan.Payload{
									&lorawan.MACCommand{
										CID: lorawan.LinkADRReq,
										Payload: &lorawan.LinkADRReqPayload{
											DataRate: 5,
											TXPower:  2,
											ChMask:   [16]bool{true, true, true},
											Redundancy: lorawan.Redundancy{
												ChMaskCntl: 0,
												NbRep:      1,
											},
										},
									},
								},
							},
						},
					},
					ExpectedEnabledChannels: []int{0, 1, 2, 3, 4, 5, 6, 7},
				},
			}

			runUplinkTests(asClient, tests)
		})

		Convey("Given a set of test-scenarios for device-status requests", func() {
			sp.DevStatusReqFreq = 24
			sp.ReportDevStatusBattery = true
			sp.ReportDevStatusMargin = true
			So(storage.UpdateServiceProfile(config.C.PostgreSQL.DB, &sp), ShouldBeNil)

			tests := []uplinkTestCase{
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.DeviceSession.LastDevStatusRequested = time.Now().Add(-61 * time.Minute)
						return nil
					},

					Name:          "must request device-status",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
							},
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{122, 152, 152, 220},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedFCntUp:                 11,
					ExpectedNFCntDown:              6,
					ExpectedTXInfo: &gw.DownlinkTXInfo{
						GatewayId:  rxInfo.GatewayId,
						Frequency:  txInfo.Frequency,
						Power:      14,
						Timestamp:  rxInfo.Timestamp + 1000000,
						Modulation: commonPB.Modulation_LORA,
						ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
							LoraModulationInfo: &gw.LoRaModulationInfo{
								Bandwidth:             125,
								SpreadingFactor:       12,
								PolarizationInversion: true,
								CodeRate:              "4/5",
							},
						},
					},
					ExpectedPHYPayload: &lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataDown,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    5,
								FCtrl: lorawan.FCtrl{
									ADR: true,
								},
								FOpts: []lorawan.Payload{
									&lorawan.MACCommand{
										CID: lorawan.DevStatusReq,
									},
								},
							},
						},
					},
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.DeviceSession.LastDevStatusRequested = time.Now().Add(-59 * time.Minute)
						return nil
					},

					Name:          "interval has not yet expired",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
							},
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{122, 152, 152, 220},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedFCntUp:                 11,
					ExpectedNFCntDown:              5,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.DeviceSession.LastDevStatusRequested = time.Now()
						tc.ExpectedASHandleDataUp.Data = []byte{1, 2, 3, 4}
						return nil
					},

					Name:          "device reports device-status",
					DeviceSession: ds,
					TXInfo:        txInfo,
					RXInfo:        rxInfo,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
								FOpts: []lorawan.Payload{
									&lorawan.MACCommand{
										CID: lorawan.DevStatusAns,
										Payload: &lorawan.DevStatusAnsPayload{
											Battery: 128,
											Margin:  10,
										},
									},
								},
							},
							FPort:      &fPortOne,
							FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
						},
					},
					ExpectedUplinkMIC:              lorawan.MIC{30, 172, 57, 148},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedFCntUp:                 11,
					ExpectedNFCntDown:              5,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
			}

			runUplinkTests(asClient, tests)
		})
	})
}

func runUplinkTests(asClient *test.ApplicationClient, tests []uplinkTestCase) {
	for i, t := range tests {
		Convey(fmt.Sprintf("When testing: %s [%d]", t.Name, i), func() {
			if t.BeforeFunc != nil {
				So(t.BeforeFunc(&t), ShouldBeNil)
			}

			// create device-queue items
			for i := range t.DeviceQueueItems {
				So(storage.CreateDeviceQueueItem(config.C.PostgreSQL.DB, &t.DeviceQueueItems[i]), ShouldBeNil)
			}

			// set application-server mocks
			asClient.HandleDataUpErr = t.ASHandleDataUpError

			// populate session and queues
			So(storage.SaveDeviceSession(config.C.Redis.Pool, t.DeviceSession), ShouldBeNil)
			for _, pending := range t.MACCommandPending {
				So(storage.SetPendingMACCommand(config.C.Redis.Pool, t.DeviceSession.DevEUI, pending), ShouldBeNil)
			}

			// update global config to avoid triggering mac-commands
			config.C.NetworkServer.NetworkSettings.RX1Delay = int(t.DeviceSession.RXDelay)

			macPL, ok := t.PHYPayload.MACPayload.(*lorawan.MACPayload)
			if ok {
				if macPL.FPort != nil && *macPL.FPort == 0 {
					So(t.PHYPayload.EncryptFRMPayload(t.DeviceSession.NwkSEncKey), ShouldBeNil)
				}
			}

			if t.DeviceSession.GetMACVersion() != lorawan.LoRaWAN1_0 {
				So(t.PHYPayload.EncryptFOpts(t.DeviceSession.NwkSEncKey), ShouldBeNil)
			}

			So(t.PHYPayload.SetUplinkDataMIC(t.DeviceSession.GetMACVersion(), t.DeviceSession.ConfFCnt, 0, 0, t.DeviceSession.FNwkSIntKey, t.DeviceSession.SNwkSIntKey), ShouldBeNil)

			So(t.PHYPayload.MIC[:], ShouldResemble, t.ExpectedUplinkMIC[:])

			phyB, err := t.PHYPayload.MarshalBinary()

			// create RXPacket and call HandleRXPacket
			rxPacket := gw.UplinkFrame{
				PhyPayload: phyB,
				RxInfo:     &t.RXInfo,
				TxInfo:     &t.TXInfo,
			}
			err = uplink.HandleRXPacket(rxPacket)
			if err != nil {
				if t.ExpectedHandleRXPacketError == nil {
					So(err.Error(), ShouldEqual, "")
				}
				So(err.Error(), ShouldEqual, t.ExpectedHandleRXPacketError.Error())
			} else {
				So(t.ExpectedHandleRXPacketError, ShouldBeNil)
			}

			// network-controller validations
			if t.ExpectedControllerHandleRXInfo != nil {
				Convey("Then the expected rx-info is published to the network-controller", func() {
					So(config.C.NetworkController.Client.(*test.NetworkControllerClient).HandleRXInfoChan, ShouldHaveLength, 1)
					pl := <-config.C.NetworkController.Client.(*test.NetworkControllerClient).HandleRXInfoChan
					if !proto.Equal(&pl, t.ExpectedControllerHandleRXInfo) {
						// when it is not equal, this will print the diff :-)
						So(&pl, ShouldResemble, t.ExpectedControllerHandleRXInfo)
					}
				})
			} else {
				So(config.C.NetworkController.Client.(*test.NetworkControllerClient).HandleRXInfoChan, ShouldHaveLength, 0)
			}

			Convey("Then the expected mac-commands are received by the network-controller", func() {
				So(config.C.NetworkController.Client.(*test.NetworkControllerClient).HandleDataUpMACCommandChan, ShouldHaveLength, len(t.ExpectedControllerHandleDataUpMACCommands))
				for _, expPl := range t.ExpectedControllerHandleDataUpMACCommands {
					pl := <-config.C.NetworkController.Client.(*test.NetworkControllerClient).HandleDataUpMACCommandChan
					So(pl, ShouldResemble, expPl)
				}
			})

			// application-server validations
			if t.ExpectedASHandleDataUp != nil {
				Convey("Then the expected rx-payloads are received by the application-server", func() {
					So(asClient.HandleDataUpChan, ShouldHaveLength, 1)
					req := <-asClient.HandleDataUpChan
					So(proto.Equal(&req, t.ExpectedASHandleDataUp), ShouldBeTrue)
				})
			} else {
				So(asClient.HandleDataUpChan, ShouldHaveLength, 0)
			}

			Convey("Then the expected error payloads are sent to the application-server", func() {
				So(asClient.HandleErrorChan, ShouldHaveLength, len(t.ExpectedASHandleErrors))
				for _, expPL := range t.ExpectedASHandleErrors {
					pl := <-asClient.HandleErrorChan
					So(pl, ShouldResemble, expPL)
				}
			})

			if t.ExpectedASHandleDownlinkACK != nil {
				Convey("Then the expected downlink ACK was sent to the application-server", func() {
					So(asClient.HandleDownlinkACKChan, ShouldHaveLength, 1)
					req := <-asClient.HandleDownlinkACKChan
					So(&req, ShouldResemble, t.ExpectedASHandleDownlinkACK)
				})
			} else {
				So(asClient.HandleDownlinkACKChan, ShouldHaveLength, 0)
			}

			// gateway validations
			if t.ExpectedTXInfo != nil {
				Convey("Then the expected downlink txinfo is used", func() {
					So(config.C.NetworkServer.Gateway.Backend.Backend.(*test.GatewayBackend).TXPacketChan, ShouldHaveLength, 1)
					txPacket := <-config.C.NetworkServer.Gateway.Backend.Backend.(*test.GatewayBackend).TXPacketChan
					if !proto.Equal(txPacket.TxInfo, t.ExpectedTXInfo) {
						// just to see the difference, instead of "false"
						So(txPacket.TxInfo, ShouldResemble, t.ExpectedTXInfo)
					}

					if t.ExpectedPHYPayload != nil {
						var phy lorawan.PHYPayload
						So(phy.UnmarshalBinary(txPacket.PhyPayload), ShouldBeNil)

						macPL, ok := phy.MACPayload.(*lorawan.MACPayload)
						if ok {
							if macPL.FPort != nil && *macPL.FPort == 0 {
								So(phy.DecryptFRMPayload(t.DeviceSession.NwkSEncKey), ShouldBeNil)
							}
						}
						t.ExpectedPHYPayload.MIC = phy.MIC

						phyB, err := t.ExpectedPHYPayload.MarshalBinary()
						So(err, ShouldBeNil)

						So(txPacket.PhyPayload, ShouldResemble, phyB)
					}
				})
			} else {
				So(config.C.NetworkServer.Gateway.Backend.Backend.(*test.GatewayBackend).TXPacketChan, ShouldHaveLength, 0)
			}

			// node session validations
			Convey("Then the frame-counters are as expected", func() {
				ns, err := storage.GetDeviceSession(config.C.Redis.Pool, t.DeviceSession.DevEUI)
				So(err, ShouldBeNil)
				So(ns.NFCntDown, ShouldEqual, t.ExpectedNFCntDown)
				So(ns.AFCntDown, ShouldEqual, t.ExpectedAFCntDown)
				So(ns.FCntUp, ShouldEqual, t.ExpectedFCntUp)
			})

			// ADR variables validations
			Convey("Then the Channels, TXPower and NbTrans are as expected", func() {
				ns, err := storage.GetDeviceSession(config.C.Redis.Pool, t.DeviceSession.DevEUI)
				So(err, ShouldBeNil)
				So(ns.TXPowerIndex, ShouldEqual, t.ExpectedTXPowerIndex)
				So(ns.NbTrans, ShouldEqual, t.ExpectedNbTrans)
				So(ns.EnabledUplinkChannels, ShouldResemble, t.ExpectedEnabledChannels)
			})

			if t.ExpectedHandleRXPacketError == nil {
				Convey("Then the expected RXInfoSet has been added to the node-session", func() {
					var gatewayID lorawan.EUI64
					copy(gatewayID[:], t.RXInfo.GatewayId)
					ns, err := storage.GetDeviceSession(config.C.Redis.Pool, t.DeviceSession.DevEUI)
					So(err, ShouldBeNil)
					So(ns.UplinkGatewayHistory, ShouldResemble, map[lorawan.EUI64]storage.UplinkGatewayHistory{
						gatewayID: storage.UplinkGatewayHistory{},
					})
				})
			}
		})
	}
}
