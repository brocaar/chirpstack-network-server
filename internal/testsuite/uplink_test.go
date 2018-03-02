package testsuite

import (
	"errors"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/api/nc"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/gateway"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/loraserver/internal/uplink"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
)

type uplinkTestCase struct {
	Name       string                         // name of the test
	BeforeFunc func(tc *uplinkTestCase) error // function to run before the test

	DeviceSession        storage.DeviceSession     // device-session
	SetMICKey            lorawan.AES128Key         // key to use for setting the mic
	EncryptFRMPayloadKey *lorawan.AES128Key        // key to use for encrypting the uplink FRMPayload (e.g. for mac-commands in FRMPayload)
	DecryptFRMPayloadKey *lorawan.AES128Key        // key for decrypting the downlink FRMPayload (e.g. to validate FRMPayload mac-commands)
	RXInfo               gw.RXInfo                 // rx-info of the "received" packet
	PHYPayload           lorawan.PHYPayload        // (unencrypted) "received" PHYPayload
	MACCommandPending    []storage.MACCommandBlock // pending mac-commands
	DeviceQueueItems     []storage.DeviceQueueItem // items in the device-queue
	ASHandleDataUpError  error                     // application-client publish data-up error

	ExpectedControllerHandleRXInfo            *nc.HandleRXInfoRequest            // expected network-controller publish rxinfo request
	ExpectedControllerHandleDataUpMACCommands []nc.HandleDataUpMACCommandRequest // expected network-controller publish dataup mac-command requests

	ExpectedASHandleDataUp      *as.HandleUplinkDataRequest  // expected application-server data up request
	ExpectedASHandleErrors      []as.HandleErrorRequest      // expected application-server error requests
	ExpectedASHandleDownlinkACK *as.HandleDownlinkACKRequest // expected application-server datadown ack request

	ExpectedTXInfo              *gw.TXInfo          // expected tx-info (downlink)
	ExpectedPHYPayload          *lorawan.PHYPayload // expected (plaintext) PHYPayload (downlink)
	ExpectedFCntUp              uint32              // expected uplink frame counter
	ExpectedFCntDown            uint32              // expected downlink frame counter
	ExpectedHandleRXPacketError error               // expected handleRXPacket error
	ExpectedTXPowerIndex        int                 // expected tx-power set by ADR
	ExpectedNbTrans             uint8               // expected nb trans set by ADR
	ExpectedEnabledChannels     []int               // expected channels enabled on the node
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
	config.C.Redis.Pool = common.NewRedisPool(conf.RedisURL)
	config.C.NetworkServer.NetworkSettings.InstallationMargin = 5

	Convey("Given a clean database", t, func() {
		test.MustFlushRedis(config.C.Redis.Pool)
		test.MustResetDB(config.C.PostgreSQL.DB)

		asClient := test.NewApplicationClient()
		config.C.ApplicationServer.Pool = test.NewApplicationServerPool(asClient)
		config.C.NetworkServer.Gateway.Backend.Backend = test.NewGatewayBackend()
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

		// device-session
		ds := storage.DeviceSession{
			DeviceProfileID:  d.DeviceProfileID,
			ServiceProfileID: d.ServiceProfileID,
			RoutingProfileID: d.RoutingProfileID,
			DevEUI:           d.DevEUI,
			JoinEUI:          lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},

			DevAddr:               lorawan.DevAddr{1, 2, 3, 4},
			NwkSKey:               [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			FCntUp:                8,
			FCntDown:              5,
			EnabledUplinkChannels: []int{0, 1, 2},
			RX2Frequency:          869525000,
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

		var fPortZero uint8
		var fPortOne uint8 = 1

		expectedControllerHandleRXInfo := &nc.HandleRXInfoRequest{
			DevEUI: ds.DevEUI[:],
			TxInfo: &nc.TXInfo{
				Frequency: int64(rxInfo.Frequency),
				DataRate: &nc.DataRate{
					Modulation:   string(rxInfo.DataRate.Modulation),
					BandWidth:    uint32(rxInfo.DataRate.Bandwidth),
					SpreadFactor: uint32(rxInfo.DataRate.SpreadFactor),
					Bitrate:      uint32(rxInfo.DataRate.BitRate),
				},
			},
			RxInfo: []*nc.RXInfo{
				{
					Mac:     rxInfo.MAC[:],
					Time:    rxInfo.Time.Format(time.RFC3339Nano),
					Rssi:    int32(rxInfo.RSSI),
					LoRaSNR: rxInfo.LoRaSNR,
				},
			},
		}

		expectedApplicationPushDataUpNoData := &as.HandleUplinkDataRequest{
			AppEUI: ds.JoinEUI[:],
			DevEUI: ds.DevEUI[:],
			FCnt:   10,
			FPort:  1,
			Data:   nil,
			TxInfo: &as.TXInfo{
				Frequency: int64(rxInfo.Frequency),
				DataRate: &as.DataRate{
					Modulation:   string(rxInfo.DataRate.Modulation),
					BandWidth:    uint32(rxInfo.DataRate.Bandwidth),
					SpreadFactor: uint32(rxInfo.DataRate.SpreadFactor),
					Bitrate:      uint32(rxInfo.DataRate.BitRate),
				},
			},
			RxInfo: []*as.RXInfo{
				{
					Mac:       rxInfo.MAC[:],
					Name:      gw1.Name,
					Time:      rxInfo.Time.Format(time.RFC3339Nano),
					Rssi:      int32(rxInfo.RSSI),
					LoRaSNR:   rxInfo.LoRaSNR,
					Altitude:  gw1.Altitude,
					Latitude:  gw1.Location.Latitude,
					Longitude: gw1.Location.Longitude,
				},
			},
			DeviceStatusBattery: 256,
			DeviceStatusMargin:  256,
		}

		Convey("Given a set of test-scenarios for error handling", func() {
			tests := []uplinkTestCase{
				{
					Name:                "the application backend returns an error",
					ExpectedPHYPayload:  &lorawan.PHYPayload{},
					DeviceSession:       ds,
					RXInfo:              rxInfo,
					SetMICKey:           ds.NwkSKey,
					ASHandleDataUpError: errors.New("BOOM"),
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedFCntUp:                 8,
					ExpectedFCntDown:               5,
					ExpectedHandleRXPacketError:    errors.New("publish data up to application-server error: BOOM"),
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
				{
					Name:          "the frame-counter is invalid",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
					ExpectedFCntUp:              8,
					ExpectedFCntDown:            5,
					ExpectedHandleRXPacketError: errors.New("get device-session error: device-session does not exist or invalid fcnt or mic"),
					ExpectedEnabledChannels:     []int{0, 1, 2},
				},
				{
					Name:          "the mic is invalid",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					SetMICKey:     [16]byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
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
					ExpectedFCntUp:              8,
					ExpectedFCntDown:            5,
					ExpectedHandleRXPacketError: errors.New("get device-session error: device-session does not exist or invalid fcnt or mic"),
					ExpectedEnabledChannels:     []int{0, 1, 2},
				},
			}

			runUplinkTests(asClient, tests)
		})

		Convey("Given a set of test-scenarios for relax frame-counter mode", func() {
			expectedApplicationPushDataUpNoData := &as.HandleUplinkDataRequest{
				AppEUI: ds.JoinEUI[:],
				DevEUI: ds.DevEUI[:],
				FCnt:   0,
				FPort:  1,
				Data:   nil,
				TxInfo: &as.TXInfo{
					Frequency: int64(rxInfo.Frequency),
					DataRate: &as.DataRate{
						Modulation:   string(rxInfo.DataRate.Modulation),
						BandWidth:    uint32(rxInfo.DataRate.Bandwidth),
						SpreadFactor: uint32(rxInfo.DataRate.SpreadFactor),
						Bitrate:      uint32(rxInfo.DataRate.BitRate),
					},
				},
				RxInfo: []*as.RXInfo{
					{
						Mac:       rxInfo.MAC[:],
						Name:      gw1.Name,
						Time:      rxInfo.Time.Format(time.RFC3339Nano),
						Rssi:      int32(rxInfo.RSSI),
						LoRaSNR:   rxInfo.LoRaSNR,
						Altitude:  gw1.Altitude,
						Latitude:  gw1.Location.Latitude,
						Longitude: gw1.Location.Longitude,
					},
				},
				DeviceStatusBattery: 256,
				DeviceStatusMargin:  256,
			}

			expectedApplicationPushDataUpNoData7 := &as.HandleUplinkDataRequest{
				AppEUI: ds.JoinEUI[:],
				DevEUI: ds.DevEUI[:],
				FCnt:   7,
				FPort:  1,
				Data:   nil,
				TxInfo: &as.TXInfo{
					Frequency: int64(rxInfo.Frequency),
					DataRate: &as.DataRate{
						Modulation:   string(rxInfo.DataRate.Modulation),
						BandWidth:    uint32(rxInfo.DataRate.Bandwidth),
						SpreadFactor: uint32(rxInfo.DataRate.SpreadFactor),
						Bitrate:      uint32(rxInfo.DataRate.BitRate),
					},
				},
				RxInfo: []*as.RXInfo{
					{
						Mac:       rxInfo.MAC[:],
						Name:      gw1.Name,
						Time:      rxInfo.Time.Format(time.RFC3339Nano),
						Rssi:      int32(rxInfo.RSSI),
						LoRaSNR:   rxInfo.LoRaSNR,
						Altitude:  gw1.Altitude,
						Latitude:  gw1.Location.Latitude,
						Longitude: gw1.Location.Longitude,
					},
				},
				DeviceStatusBattery: 256,
				DeviceStatusMargin:  256,
			}

			tests := []uplinkTestCase{
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.DeviceSession.SkipFCntValidation = true
						return nil
					},

					Name:          "the frame-counter is invalid but not 0",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData7,
					ExpectedFCntUp:                 8,
					ExpectedFCntDown:               5,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.DeviceSession.SkipFCntValidation = true
						return nil
					},

					Name:          "the frame-counter is invalid and 0",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedFCntUp:                 1,
					ExpectedFCntDown:               5,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
			}

			runUplinkTests(asClient, tests)
		})

		Convey("Given a set of test-scenarios for basic flows (nothing in the queue)", func() {
			inTenMinutes := time.Now().Add(10 * time.Minute)
			timestamp := rxInfo.Timestamp + 1000000
			timestamp3S := rxInfo.Timestamp + 3000000
			timestamp2S := rxInfo.Timestamp + 2000000
			timestamp6S := rxInfo.Timestamp + 6000000

			tests := []uplinkTestCase{
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.ExpectedASHandleDataUp.Data = []byte{1, 2, 3, 4}
						return nil
					},

					Name:          "unconfirmed uplink data with payload",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedFCntUp:                 11,
					ExpectedFCntDown:               5,
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
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedASHandleDownlinkACK:    &as.HandleDownlinkACKRequest{DevEUI: d.DevEUI[:], FCnt: 4, Acknowledged: true},
					ExpectedFCntUp:                 11,
					ExpectedFCntDown:               5,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
				{
					Name:          "unconfirmed uplink data without payload (just a FPort)",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedFCntUp:                 11,
					ExpectedFCntDown:               5,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.ExpectedASHandleDataUp.Data = []byte{1, 2, 3, 4}
						return nil
					},

					Name:          "confirmed uplink data with payload",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: &timestamp,
						Frequency: rxInfo.Frequency,
						Power:     14,
						DataRate:  rxInfo.DataRate,
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
					ExpectedFCntDown:        6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					Name:          "confirmed uplink data without payload",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: &timestamp,
						Frequency: rxInfo.Frequency,
						Power:     14,
						DataRate:  rxInfo.DataRate,
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
					ExpectedFCntDown:        6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.DeviceSession.RXDelay = 3
						return nil
					},

					Name:          "confirmed uplink data without payload (with RXDelay=3)",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: &timestamp3S,
						Frequency: rxInfo.Frequency,
						Power:     14,
						DataRate:  rxInfo.DataRate,
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
					ExpectedFCntDown:        6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.DeviceSession.RXWindow = storage.RX2
						tc.DeviceSession.RX2DR = 0
						return nil
					},

					Name:          "confirmed uplink data without payload (node-session has RXWindow=RX2)",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: &timestamp2S,
						Frequency: config.C.NetworkServer.Band.Band.RX2Frequency,
						Power:     14,
						DataRate:  config.C.NetworkServer.Band.Band.DataRates[0],
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
					ExpectedFCntDown:        6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.DeviceSession.RXWindow = storage.RX2
						tc.DeviceSession.RXDelay = 5
						tc.DeviceSession.RX2DR = 0
						return nil
					},

					Name:          "confirmed uplink data without payload (node-session has RXWindow=RX2 and RXDelay=5)",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: &timestamp6S,
						Frequency: config.C.NetworkServer.Band.Band.RX2Frequency,
						Power:     14,
						DataRate:  config.C.NetworkServer.Band.Band.DataRates[0],
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
					ExpectedFCntDown:        6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					Name:          "two uplink mac commands (FOpts)",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
								FOpts: []lorawan.MACCommand{
									{CID: 0x80, Payload: &lorawan.ProprietaryMACCommandPayload{Bytes: []byte{1, 2, 3}}},
									{CID: 0x81, Payload: &lorawan.ProprietaryMACCommandPayload{Bytes: []byte{4, 5}}},
								},
							},
						},
					},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedControllerHandleDataUpMACCommands: []nc.HandleDataUpMACCommandRequest{
						{DevEUI: ds.DevEUI[:], Cid: 128, Commands: [][]byte{{128, 1, 2, 3}}},
						{DevEUI: ds.DevEUI[:], Cid: 129, Commands: [][]byte{{129, 4, 5}}},
					},
					ExpectedFCntUp:          11,
					ExpectedFCntDown:        5,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					Name:                 "two uplink mac commands (FRMPayload)",
					DeviceSession:        ds,
					RXInfo:               rxInfo,
					EncryptFRMPayloadKey: &ds.NwkSKey,
					SetMICKey:            ds.NwkSKey,
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedControllerHandleDataUpMACCommands: []nc.HandleDataUpMACCommandRequest{
						{DevEUI: ds.DevEUI[:], Cid: 128, Commands: [][]byte{{128, 1, 2, 3}}},
						{DevEUI: ds.DevEUI[:], Cid: 129, Commands: [][]byte{{129, 4, 5}}},
					},
					ExpectedFCntUp:          11,
					ExpectedFCntDown:        5,
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
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedFCntUp:                 65537,
					ExpectedFCntDown:               5,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
				{

					BeforeFunc: func(tc *uplinkTestCase) error {
						// remove rx info set
						tc.ExpectedASHandleDataUp.RxInfo = nil
						tc.ExpectedASHandleDataUp.Data = []byte{1, 2, 3, 4}

						// set add gw meta-data to false
						sp.ServiceProfile.AddGWMetadata = false
						return storage.UpdateServiceProfile(config.C.PostgreSQL.DB, &sp)
					},

					Name:          "unconfirmed uplink data with payload (service-profile: no gateway info)",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedFCntUp:                 11,
					ExpectedFCntDown:               5,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
			}

			runUplinkTests(asClient, tests)
		})

		Convey("Given a set of test-scenarios for mac-commands", func() {
			var fPortThree uint8 = 3
			timestamp1S := rxInfo.Timestamp + 1000000

			tests := []uplinkTestCase{
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						sp.ServiceProfile.DevStatusReqFreq = 1
						So(storage.UpdateServiceProfile(config.C.PostgreSQL.DB, &sp), ShouldBeNil)

						tc.ExpectedASHandleDataUp.Data = []byte{1, 2, 3, 4}
						return nil
					},

					Name:          "unconfirmed uplink data + two downlink mac commands in queue (FOpts)",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: &timestamp1S,
						Frequency: rxInfo.Frequency,
						Power:     14,
						DataRate:  rxInfo.DataRate,
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
								FOpts: []lorawan.MACCommand{
									{CID: lorawan.CID(6)},
								},
							},
						},
					},
					ExpectedFCntUp:          11,
					ExpectedFCntDown:        6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						sp.ServiceProfile.DevStatusReqFreq = 1
						So(storage.UpdateServiceProfile(config.C.PostgreSQL.DB, &sp), ShouldBeNil)

						tc.ExpectedASHandleDataUp.Data = []byte{1, 2, 3, 4}
						return nil
					},

					Name:          "unconfirmed uplink data + downlink mac command (FOpts) + unconfirmed data down",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: &timestamp1S,
						Frequency: rxInfo.Frequency,
						Power:     14,
						DataRate:  rxInfo.DataRate,
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
								FOpts: []lorawan.MACCommand{
									{CID: lorawan.CID(6)},
								},
							},
							FPort: &fPortThree,
							FRMPayload: []lorawan.Payload{
								&lorawan.DataPayload{Bytes: []byte{4, 5, 6}},
							},
						},
					},
					ExpectedFCntUp:          11,
					ExpectedFCntDown:        6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
			}

			runUplinkTests(asClient, tests)
		})

		Convey("Given a set of test-scenarios for tx-payload queue", func() {
			var fPortTen uint8 = 10
			timestamp1S := rxInfo.Timestamp + 1000000

			tests := []uplinkTestCase{
				{
					Name:          "unconfirmed uplink data + one unconfirmed downlink payload in queue",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: &timestamp1S,
						Frequency: rxInfo.Frequency,
						Power:     14,
						DataRate:  rxInfo.DataRate,
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
					ExpectedFCntDown:        6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					Name:          "unconfirmed uplink data + two unconfirmed downlink payloads in queue",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: &timestamp1S,
						Frequency: rxInfo.Frequency,
						Power:     14,
						DataRate:  rxInfo.DataRate,
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
					ExpectedFCntDown:        6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					Name:          "unconfirmed uplink data + one confirmed downlink payload in queue",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: &timestamp1S,
						Frequency: rxInfo.Frequency,
						Power:     14,
						DataRate:  rxInfo.DataRate,
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
					ExpectedFCntDown:        6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					Name:          "unconfirmed uplink data + downlink payload which exceeds the max payload size (for dr 0)",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedFCntUp:                 11,
					ExpectedFCntDown:               5, // payload has been discarded, nothing to transmit
					ExpectedEnabledChannels:        []int{0, 1, 2},
					ExpectedASHandleErrors: []as.HandleErrorRequest{
						{DevEUI: d.DevEUI[:], Type: as.ErrorType_DEVICE_QUEUE_ITEM_SIZE, Error: "payload exceeds max payload size", FCnt: 5},
					},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						sp.ServiceProfile.DevStatusReqFreq = 1
						So(storage.UpdateServiceProfile(config.C.PostgreSQL.DB, &sp), ShouldBeNil)
						return nil
					},
					Name:          "unconfirmed uplink data + one unconfirmed downlink payload in queue (exactly max size for dr 0) + one mac command",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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

					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: &timestamp1S,
						Frequency: rxInfo.Frequency,
						Power:     14,
						DataRate:  rxInfo.DataRate,
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
					ExpectedFCntDown:        6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
			}

			runUplinkTests(asClient, tests)
		})

		Convey("Given a set of test-scenarios for ADR", func() {
			timestamp1S := rxInfo.Timestamp + 1000000

			tests := []uplinkTestCase{
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.ExpectedControllerHandleRXInfo.TxInfo.Adr = true

						tc.DeviceSession.FCntUp = 10
						return nil
					},

					Name:          "adr triggered",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedFCntUp:                 11,
					ExpectedFCntDown:               6,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: &timestamp1S,
						Frequency: rxInfo.Frequency,
						Power:     14,
						DataRate:  rxInfo.DataRate,
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
								FOpts: []lorawan.MACCommand{
									{
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
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedFCntUp:                 11,
					ExpectedFCntDown:               5,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.DeviceSession.FCntUp = 10
						return nil
					},

					Name:          "acknowledgement of pending adr request",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
								FOpts: []lorawan.MACCommand{
									{CID: lorawan.LinkADRAns, Payload: &lorawan.LinkADRAnsPayload{ChannelMaskACK: true, DataRateACK: true, PowerACK: true}},
								},
							},
						},
					},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedFCntUp:                 11,
					ExpectedFCntDown:               5,
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
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
								FOpts: []lorawan.MACCommand{
									{CID: lorawan.LinkADRAns, Payload: &lorawan.LinkADRAnsPayload{ChannelMaskACK: false, DataRateACK: true, PowerACK: true}},
								},
							},
						},
					},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedFCntUp:                 11,
					ExpectedFCntDown:               5,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
				{
					Name:          "adr ack requested",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedFCntUp:                 11,
					ExpectedFCntDown:               6,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: &timestamp1S,
						Frequency: rxInfo.Frequency,
						Power:     14,
						DataRate:  rxInfo.DataRate,
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
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedFCntUp:                 11,
					ExpectedFCntDown:               6,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: &timestamp1S,
						Frequency: rxInfo.Frequency,
						Power:     14,
						DataRate:  rxInfo.DataRate,
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
								FOpts: []lorawan.MACCommand{
									{
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
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
								FOpts: []lorawan.MACCommand{
									{
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedFCntUp:                 11,
					ExpectedFCntDown:               5,
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
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
								FOpts: []lorawan.MACCommand{
									{
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedFCntUp:                 11,
					ExpectedFCntDown:               6,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: &timestamp1S,
						Frequency: rxInfo.Frequency,
						Power:     14,
						DataRate:  rxInfo.DataRate,
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
								FOpts: []lorawan.MACCommand{
									{
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
						tc.ExpectedControllerHandleRXInfo.TxInfo.Adr = true

						tc.DeviceSession.FCntUp = 10
						tc.DeviceSession.EnabledUplinkChannels = []int{0, 1, 2, 3, 4, 5, 6, 7}
						return nil
					},

					Name:          "channel re-configuration and adr triggered",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedFCntUp:                 11,
					ExpectedFCntDown:               6,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: &timestamp1S,
						Frequency: rxInfo.Frequency,
						Power:     14,
						DataRate:  rxInfo.DataRate,
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
								FOpts: []lorawan.MACCommand{
									{
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
			timestamp1S := rxInfo.Timestamp + 1000000

			tests := []uplinkTestCase{
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.DeviceSession.LastDevStatusRequested = time.Now().Add(-61 * time.Minute)
						return nil
					},

					Name:          "must request device-status",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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

					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedFCntUp:                 11,
					ExpectedFCntDown:               6,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: &timestamp1S,
						Frequency: rxInfo.Frequency,
						Power:     14,
						DataRate:  rxInfo.DataRate,
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
								FOpts: []lorawan.MACCommand{
									{
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
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
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

					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedFCntUp:                 11,
					ExpectedFCntDown:               5,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
				{
					BeforeFunc: func(tc *uplinkTestCase) error {
						tc.DeviceSession.LastDevStatusRequested = time.Now()
						tc.ExpectedASHandleDataUp.Data = []byte{1, 2, 3, 4}
						tc.ExpectedASHandleDataUp.DeviceStatusBattery = 128
						tc.ExpectedASHandleDataUp.DeviceStatusMargin = 10
						return nil
					},

					Name:          "device reports device-status",
					DeviceSession: ds,
					RXInfo:        rxInfo,
					SetMICKey:     ds.NwkSKey,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ds.DevAddr,
								FCnt:    10,
								FOpts: []lorawan.MACCommand{
									{
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
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedASHandleDataUp:         expectedApplicationPushDataUpNoData,
					ExpectedFCntUp:                 11,
					ExpectedFCntDown:               5,
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

			// encrypt FRMPayload and set MIC
			if t.EncryptFRMPayloadKey != nil {
				So(t.PHYPayload.EncryptFRMPayload(*t.EncryptFRMPayloadKey), ShouldBeNil)
			}
			So(t.PHYPayload.SetMIC(t.SetMICKey), ShouldBeNil)

			// marshal and unmarshal the PHYPayload to make sure the FCnt gets
			// truncated to to 16 bit
			var phy lorawan.PHYPayload
			b, err := t.PHYPayload.MarshalBinary()
			So(err, ShouldBeNil)
			So(phy.UnmarshalBinary(b), ShouldBeNil)

			// create RXPacket and call HandleRXPacket
			rxPacket := gw.RXPacket{
				RXInfo:     t.RXInfo,
				PHYPayload: phy,
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
					So(&pl, ShouldResemble, t.ExpectedControllerHandleRXInfo)
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
					So(&req, ShouldResemble, t.ExpectedASHandleDataUp)
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
					So(&txPacket.TXInfo, ShouldResemble, t.ExpectedTXInfo)

					if t.ExpectedPHYPayload != nil {
						if t.DecryptFRMPayloadKey != nil {
							So(txPacket.PHYPayload.DecryptFRMPayload(*t.DecryptFRMPayloadKey), ShouldBeNil)
						}
						t.ExpectedPHYPayload.MIC = txPacket.PHYPayload.MIC
						So(&txPacket.PHYPayload, ShouldResemble, t.ExpectedPHYPayload)
					}
				})
			} else {
				So(config.C.NetworkServer.Gateway.Backend.Backend.(*test.GatewayBackend).TXPacketChan, ShouldHaveLength, 0)
			}

			// node session validations
			Convey("Then the frame-counters are as expected", func() {
				ns, err := storage.GetDeviceSession(config.C.Redis.Pool, t.DeviceSession.DevEUI)
				So(err, ShouldBeNil)
				So(ns.FCntDown, ShouldEqual, t.ExpectedFCntDown)
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
					ns, err := storage.GetDeviceSession(config.C.Redis.Pool, t.DeviceSession.DevEUI)
					So(err, ShouldBeNil)
					So(ns.LastRXInfoSet, ShouldResemble, models.RXInfoSet{
						{
							MAC:               t.RXInfo.MAC,
							Time:              t.RXInfo.Time,
							TimeSinceGPSEpoch: t.RXInfo.TimeSinceGPSEpoch,
							Timestamp:         t.RXInfo.Timestamp,
							RSSI:              t.RXInfo.RSSI,
							LoRaSNR:           t.RXInfo.LoRaSNR,
							Board:             t.RXInfo.Board,
							Antenna:           t.RXInfo.Antenna,
						},
					})
				})
			}
		})
	}
}
