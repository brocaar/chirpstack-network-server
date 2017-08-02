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
	"github.com/brocaar/loraserver/internal/gateway"
	"github.com/brocaar/loraserver/internal/maccommand"
	"github.com/brocaar/loraserver/internal/session"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/loraserver/internal/uplink"
	"github.com/brocaar/lorawan"
)

type uplinkTestCase struct {
	Name                 string              // name of the test
	NodeSession          session.NodeSession // node-session
	SetMICKey            lorawan.AES128Key   // key to use for setting the mic
	EncryptFRMPayloadKey *lorawan.AES128Key  // key to use for encrypting the uplink FRMPayload (e.g. for mac-commands in FRMPayload)
	DecryptFRMPayloadKey *lorawan.AES128Key  // key for decrypting the downlink FRMPayload (e.g. to validate FRMPayload mac-commands)
	RXInfo               gw.RXInfo           // rx-info of the "received" packet
	PHYPayload           lorawan.PHYPayload  // (unencrypted) "received" PHYPayload
	MACCommandQueue      []maccommand.Block  // downlink mac-command queue
	MACCommandPending    []maccommand.Block  // pending mac-commands

	ApplicationGetDataDown       as.GetDataDownResponse // application-server get data down response
	ApplicationHandleDataUpError error                  // application-client publish data-up error
	ApplicationGetDataDownError  error                  // application-server get data down error

	ExpectedControllerHandleRXInfo            *nc.HandleRXInfoRequest            // expected network-controller publish rxinfo request
	ExpectedControllerHandleDataUpMACCommands []nc.HandleDataUpMACCommandRequest // expected network-controller publish dataup mac-command requests

	ExpectedApplicationHandleDataUp      *as.HandleDataUpRequest      // expected application-server data up request
	ExpectedApplicationHandleErrors      []as.HandleErrorRequest      // expected application-server error requests
	ExpectedApplicationHandleDataDownACK *as.HandleDataDownACKRequest // expected application-server datadown ack request
	ExpectedApplicationGetDataDown       *as.GetDataDownRequest       // expected application-server get data down request

	ExpectedTXInfo              *gw.TXInfo          // expected tx-info (downlink)
	ExpectedPHYPayload          *lorawan.PHYPayload // expected (plaintext) PHYPayload (downlink)
	ExpectedFCntUp              uint32              // expected uplink frame counter
	ExpectedFCntDown            uint32              // expected downlink frame counter
	ExpectedHandleRXPacketError error               // expected handleRXPacket error
	ExpectedMACCommandQueue     []maccommand.Block  // expected downlink mac-command queue
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

	Convey("Given a clean database", t, func() {
		p := common.NewRedisPool(conf.RedisURL)
		test.MustFlushRedis(p)
		test.MustResetDB(db)

		ctx := common.Context{
			NetID:       [3]byte{3, 2, 1},
			RedisPool:   p,
			DB:          db,
			Gateway:     test.NewGatewayBackend(),
			Application: test.NewApplicationClient(),
			Controller:  test.NewNetworkControllerClient(),
		}

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

		rxInfo := gw.RXInfo{
			MAC:       [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			Frequency: common.Band.UplinkChannels[0].Frequency,
			DataRate:  common.Band.DataRates[common.Band.UplinkChannels[0].DataRates[0]],
			LoRaSNR:   7,
		}

		ns := session.NodeSession{
			DevAddr:         [4]byte{1, 2, 3, 4},
			DevEUI:          [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			NwkSKey:         [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			FCntUp:          8,
			FCntDown:        5,
			AppEUI:          [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
			EnabledChannels: []int{0, 1, 2},
		}

		nsFCntRollOver := session.NodeSession{
			DevAddr:         [4]byte{1, 2, 3, 4},
			DevEUI:          [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			NwkSKey:         [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			FCntUp:          65535,
			FCntDown:        5,
			AppEUI:          [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
			EnabledChannels: []int{0, 1, 2},
		}

		nsRelaxFCnt := session.NodeSession{
			DevAddr:         [4]byte{1, 2, 3, 4},
			DevEUI:          [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			NwkSKey:         [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			FCntUp:          8,
			FCntDown:        5,
			RelaxFCnt:       true,
			AppEUI:          [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
			EnabledChannels: []int{0, 1, 2},
		}

		nsDelay := session.NodeSession{
			DevAddr:         [4]byte{1, 2, 3, 4},
			DevEUI:          [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			NwkSKey:         [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			FCntUp:          8,
			FCntDown:        5,
			AppEUI:          [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
			RXDelay:         3,
			EnabledChannels: []int{0, 1, 2},
		}

		nsRX2 := session.NodeSession{
			DevAddr:         [4]byte{1, 2, 3, 4},
			DevEUI:          [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			NwkSKey:         [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			FCntUp:          8,
			FCntDown:        5,
			AppEUI:          [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
			RXWindow:        session.RX2,
			RX2DR:           3,
			EnabledChannels: []int{0, 1, 2},
		}

		nsRX2Delay := session.NodeSession{
			DevAddr:         [4]byte{1, 2, 3, 4},
			DevEUI:          [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			NwkSKey:         [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			FCntUp:          8,
			FCntDown:        5,
			AppEUI:          [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
			RXWindow:        session.RX2,
			RXDelay:         5,
			EnabledChannels: []int{0, 1, 2},
		}

		nsADREnabled := session.NodeSession{
			DevAddr:            [4]byte{1, 2, 3, 4},
			DevEUI:             [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			NwkSKey:            [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			FCntUp:             10,
			FCntDown:           5,
			AppEUI:             [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
			ADRInterval:        10,
			InstallationMargin: 5,
			EnabledChannels:    []int{0, 1, 2},
		}

		nsExtraChannels := session.NodeSession{
			DevAddr:         [4]byte{1, 2, 3, 4},
			DevEUI:          [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			NwkSKey:         [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			FCntUp:          8,
			FCntDown:        5,
			AppEUI:          [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
			EnabledChannels: []int{0, 1, 2, 3, 4, 5, 6, 7},
		}

		nsExtraChannelsADR := session.NodeSession{
			DevAddr:            [4]byte{1, 2, 3, 4},
			DevEUI:             [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			NwkSKey:            [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			FCntUp:             10,
			FCntDown:           5,
			AppEUI:             [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
			ADRInterval:        10,
			InstallationMargin: 5,
			EnabledChannels:    []int{0, 1, 2, 3, 4, 5, 6, 7},
		}

		var fPortZero uint8
		var fPortOne uint8 = 1

		expectedControllerHandleRXInfo := &nc.HandleRXInfoRequest{
			DevEUI: ns.DevEUI[:],
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

		expectedControllerHandleRXInfoADR := &nc.HandleRXInfoRequest{
			DevEUI: ns.DevEUI[:],
			TxInfo: &nc.TXInfo{
				Adr:       true,
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

		expectedApplicationPushDataUp := &as.HandleDataUpRequest{
			AppEUI: ns.AppEUI[:],
			DevEUI: ns.DevEUI[:],
			FCnt:   10,
			FPort:  1,
			Data:   []byte{1, 2, 3, 4},
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
		}

		expectedApplicationPushDataUpFCntRollOver := &as.HandleDataUpRequest{
			AppEUI: ns.AppEUI[:],
			DevEUI: ns.DevEUI[:],
			FCnt:   65536,
			FPort:  1,
			Data:   []byte{1, 2, 3, 4},
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
		}

		expectedApplicationPushDataUpNoData := &as.HandleDataUpRequest{
			AppEUI: ns.AppEUI[:],
			DevEUI: ns.DevEUI[:],
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
		}

		expectedGetDataDown := &as.GetDataDownRequest{
			AppEUI:         ns.AppEUI[:],
			DevEUI:         ns.DevEUI[:],
			MaxPayloadSize: 51,
			FCnt:           5,
		}

		Convey("Given a set of test-scenarios for error handling", func() {
			tests := []uplinkTestCase{
				{
					Name:                         "the application backend returns an error",
					ExpectedPHYPayload:           &lorawan.PHYPayload{},
					NodeSession:                  ns,
					RXInfo:                       rxInfo,
					SetMICKey:                    ns.NwkSKey,
					ApplicationHandleDataUpError: errors.New("BOOM"),
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
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
					Name:        "the frame-counter is invalid",
					NodeSession: ns,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
								FCnt:    7,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedFCntUp:              8,
					ExpectedFCntDown:            5,
					ExpectedHandleRXPacketError: errors.New("get node-session error: node-session does not exist or invalid fcnt or mic"),
					ExpectedEnabledChannels:     []int{0, 1, 2},
				},
				{
					Name:        "the mic is invalid",
					NodeSession: ns,
					RXInfo:      rxInfo,
					SetMICKey:   [16]byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
								FCnt:    10,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedFCntUp:              8,
					ExpectedFCntDown:            5,
					ExpectedHandleRXPacketError: errors.New("get node-session error: node-session does not exist or invalid fcnt or mic"),
					ExpectedEnabledChannels:     []int{0, 1, 2},
				},
			}

			runUplinkTests(ctx, tests)
		})

		Convey("Given a set of test-scenarios for relax frame-counter mode", func() {
			expectedApplicationPushDataUpNoData := &as.HandleDataUpRequest{
				AppEUI: ns.AppEUI[:],
				DevEUI: ns.DevEUI[:],
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
			}

			expectedGetDataDown := &as.GetDataDownRequest{
				AppEUI:         ns.AppEUI[:],
				DevEUI:         ns.DevEUI[:],
				MaxPayloadSize: 51,
				FCnt:           5,
			}

			expectedApplicationPushDataUpNoData7 := &as.HandleDataUpRequest{
				AppEUI: ns.AppEUI[:],
				DevEUI: ns.DevEUI[:],
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
			}

			expectedGetDataDown7 := &as.GetDataDownRequest{
				AppEUI:         ns.AppEUI[:],
				DevEUI:         ns.DevEUI[:],
				MaxPayloadSize: 51,
				FCnt:           5,
			}

			tests := []uplinkTestCase{
				{
					Name:        "the frame-counter is invalid but not 0",
					NodeSession: nsRelaxFCnt,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
								FCnt:    7,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedControllerHandleRXInfo:  expectedControllerHandleRXInfo,
					ExpectedApplicationHandleDataUp: expectedApplicationPushDataUpNoData7,
					ExpectedApplicationGetDataDown:  expectedGetDataDown7,
					ExpectedFCntUp:                  8,
					ExpectedFCntDown:                5,
					ExpectedEnabledChannels:         []int{0, 1, 2},
				},
				{
					Name:        "the frame-counter is invalid and 0",
					NodeSession: nsRelaxFCnt,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
								FCnt:    0,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedControllerHandleRXInfo:  expectedControllerHandleRXInfo,
					ExpectedApplicationHandleDataUp: expectedApplicationPushDataUpNoData,
					ExpectedApplicationGetDataDown:  expectedGetDataDown,
					ExpectedFCntUp:                  1,
					ExpectedFCntDown:                5,
					ExpectedEnabledChannels:         []int{0, 1, 2},
				},
			}

			runUplinkTests(ctx, tests)
		})

		// TODO: add ACK test
		Convey("Given a set of test-scenarios for basic flows (nothing in the queue)", func() {
			tests := []uplinkTestCase{
				{
					Name:        "unconfirmed uplink data with payload",
					NodeSession: ns,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
								FCnt:    10,
							},
							FPort:      &fPortOne,
							FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
						},
					},
					ExpectedControllerHandleRXInfo:  expectedControllerHandleRXInfo,
					ExpectedApplicationHandleDataUp: expectedApplicationPushDataUp,
					ExpectedApplicationGetDataDown:  expectedGetDataDown,
					ExpectedFCntUp:                  11,
					ExpectedFCntDown:                5,
					ExpectedEnabledChannels:         []int{0, 1, 2},
				},
				{
					Name:        "unconfirmed uplink data without payload (just a FPort)",
					NodeSession: ns,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
								FCnt:    10,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedControllerHandleRXInfo:  expectedControllerHandleRXInfo,
					ExpectedApplicationHandleDataUp: expectedApplicationPushDataUpNoData,
					ExpectedApplicationGetDataDown:  expectedGetDataDown,
					ExpectedFCntUp:                  11,
					ExpectedFCntDown:                5,
					ExpectedEnabledChannels:         []int{0, 1, 2},
				},
				{
					Name:        "confirmed uplink data with payload",
					NodeSession: ns,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.ConfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
								FCnt:    10,
							},
							FPort:      &fPortOne,
							FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
						},
					},
					ExpectedControllerHandleRXInfo:  expectedControllerHandleRXInfo,
					ExpectedApplicationHandleDataUp: expectedApplicationPushDataUp,
					ExpectedApplicationGetDataDown:  expectedGetDataDown,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: rxInfo.Timestamp + 1000000,
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
								DevAddr: ns.DevAddr,
								FCnt:    5,
								FCtrl: lorawan.FCtrl{
									ACK: true,
								},
							},
						},
					},
					ExpectedFCntUp:          11,
					ExpectedFCntDown:        6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					Name:        "confirmed uplink data without payload",
					NodeSession: ns,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.ConfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
								FCnt:    10,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedControllerHandleRXInfo:  expectedControllerHandleRXInfo,
					ExpectedApplicationHandleDataUp: expectedApplicationPushDataUpNoData,
					ExpectedApplicationGetDataDown:  expectedGetDataDown,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: rxInfo.Timestamp + 1000000,
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
								DevAddr: ns.DevAddr,
								FCnt:    5,
								FCtrl: lorawan.FCtrl{
									ACK: true,
								},
							},
						},
					},
					ExpectedFCntUp:          11,
					ExpectedFCntDown:        6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					Name:        "confirmed uplink data without payload (with RXDelay=3)",
					NodeSession: nsDelay,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.ConfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
								FCnt:    10,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedControllerHandleRXInfo:  expectedControllerHandleRXInfo,
					ExpectedApplicationHandleDataUp: expectedApplicationPushDataUpNoData,
					ExpectedApplicationGetDataDown:  expectedGetDataDown,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: rxInfo.Timestamp + 3000000,
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
								DevAddr: ns.DevAddr,
								FCnt:    5,
								FCtrl: lorawan.FCtrl{
									ACK: true,
								},
							},
						},
					},
					ExpectedFCntUp:          11,
					ExpectedFCntDown:        6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					Name:        "confirmed uplink data without payload (node-session has RXWindow=RX2)",
					NodeSession: nsRX2,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.ConfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
								FCnt:    10,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedControllerHandleRXInfo:  expectedControllerHandleRXInfo,
					ExpectedApplicationHandleDataUp: expectedApplicationPushDataUpNoData,
					ExpectedApplicationGetDataDown: &as.GetDataDownRequest{
						AppEUI:         ns.AppEUI[:],
						DevEUI:         ns.DevEUI[:],
						FCnt:           5,
						MaxPayloadSize: 115,
					},
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: rxInfo.Timestamp + 2000000,
						Frequency: common.Band.RX2Frequency,
						Power:     14,
						DataRate:  common.Band.DataRates[nsRX2.RX2DR],
					},
					ExpectedPHYPayload: &lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataDown,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
								FCnt:    5,
								FCtrl: lorawan.FCtrl{
									ACK: true,
								},
							},
						},
					},
					ExpectedFCntUp:          11,
					ExpectedFCntDown:        6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					Name:        "confirmed uplink data without payload (node-session has RXWindow=RX2 and RXDelay=5)",
					NodeSession: nsRX2Delay,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.ConfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
								FCnt:    10,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedControllerHandleRXInfo:  expectedControllerHandleRXInfo,
					ExpectedApplicationHandleDataUp: expectedApplicationPushDataUpNoData,
					ExpectedApplicationGetDataDown:  expectedGetDataDown,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: rxInfo.Timestamp + 6000000,
						Frequency: common.Band.RX2Frequency,
						Power:     14,
						DataRate:  common.Band.DataRates[nsRX2Delay.RX2DR],
					},
					ExpectedPHYPayload: &lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataDown,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
								FCnt:    5,
								FCtrl: lorawan.FCtrl{
									ACK: true,
								},
							},
						},
					},
					ExpectedFCntUp:          11,
					ExpectedFCntDown:        6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					Name:        "two uplink mac commands (FOpts)",
					NodeSession: ns,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
								FCnt:    10,
								FOpts: []lorawan.MACCommand{
									{CID: 0x80, Payload: &lorawan.ProprietaryMACCommandPayload{Bytes: []byte{1, 2, 3}}},
									{CID: 0x81, Payload: &lorawan.ProprietaryMACCommandPayload{Bytes: []byte{4, 5}}},
								},
							},
						},
					},
					ExpectedApplicationGetDataDown: expectedGetDataDown,
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedControllerHandleDataUpMACCommands: []nc.HandleDataUpMACCommandRequest{
						{DevEUI: ns.DevEUI[:], Cid: 128, Commands: [][]byte{{128, 1, 2, 3}}},
						{DevEUI: ns.DevEUI[:], Cid: 129, Commands: [][]byte{{129, 4, 5}}},
					},
					ExpectedFCntUp:          11,
					ExpectedFCntDown:        5,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					Name:                 "two uplink mac commands (FRMPayload)",
					NodeSession:          ns,
					RXInfo:               rxInfo,
					EncryptFRMPayloadKey: &ns.NwkSKey,
					SetMICKey:            ns.NwkSKey,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
								FCnt:    10,
							},
							FPort: &fPortZero,
							FRMPayload: []lorawan.Payload{
								&lorawan.MACCommand{CID: 0x80, Payload: &lorawan.ProprietaryMACCommandPayload{Bytes: []byte{1, 2, 3}}},
								&lorawan.MACCommand{CID: 0x81, Payload: &lorawan.ProprietaryMACCommandPayload{Bytes: []byte{4, 5}}},
							},
						},
					},
					ExpectedApplicationGetDataDown: expectedGetDataDown,
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedControllerHandleDataUpMACCommands: []nc.HandleDataUpMACCommandRequest{
						{DevEUI: ns.DevEUI[:], FrmPayload: true, Cid: 128, Commands: [][]byte{{128, 1, 2, 3}}},
						{DevEUI: ns.DevEUI[:], FrmPayload: true, Cid: 129, Commands: [][]byte{{129, 4, 5}}},
					},
					ExpectedFCntUp:          11,
					ExpectedFCntDown:        5,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					Name:        "unconfirmed uplink with FCnt rollover",
					NodeSession: nsFCntRollOver,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
								FCnt:    65536,
							},
							FPort:      &fPortOne,
							FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
						},
					},
					ExpectedControllerHandleRXInfo:  expectedControllerHandleRXInfo,
					ExpectedApplicationHandleDataUp: expectedApplicationPushDataUpFCntRollOver,
					ExpectedApplicationGetDataDown:  expectedGetDataDown,
					ExpectedFCntUp:                  65537,
					ExpectedFCntDown:                5,
					ExpectedEnabledChannels:         []int{0, 1, 2},
				},
			}

			runUplinkTests(ctx, tests)
		})

		Convey("Given a set of test-scenarios for mac-command queue", func() {
			var fPortThree uint8 = 3

			tests := []uplinkTestCase{
				{
					Name:        "unconfirmed uplink data + two downlink mac commands in queue (FOpts)",
					NodeSession: ns,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					MACCommandQueue: []maccommand.Block{
						{
							CID: lorawan.DevStatusReq,
							MACCommands: []lorawan.MACCommand{
								{
									CID: lorawan.DevStatusReq,
								},
							},
						},
						{
							CID: lorawan.RXTimingSetupReq,
							MACCommands: []lorawan.MACCommand{
								{
									CID:     lorawan.RXTimingSetupReq,
									Payload: &lorawan.RXTimingSetupReqPayload{Delay: 3},
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
								DevAddr: ns.DevAddr,
								FCnt:    10,
							},
							FPort:      &fPortOne,
							FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
						},
					},
					ExpectedControllerHandleRXInfo:  expectedControllerHandleRXInfo,
					ExpectedApplicationHandleDataUp: expectedApplicationPushDataUp,
					ExpectedApplicationGetDataDown:  expectedGetDataDown,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: rxInfo.Timestamp + 1000000,
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
								DevAddr: ns.DevAddr,
								FCnt:    5,
								FOpts: []lorawan.MACCommand{
									{CID: lorawan.CID(6)},
									{CID: lorawan.CID(8), Payload: &lorawan.RXTimingSetupReqPayload{Delay: 3}},
								},
							},
						},
					},
					ExpectedFCntUp:          11,
					ExpectedFCntDown:        6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					Name:        "unconfirmed uplink data + two downlink mac commands in queue (FOpts) + unconfirmed data down",
					NodeSession: ns,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					ApplicationGetDataDown: as.GetDataDownResponse{
						FPort: 3,
						Data:  []byte{4, 5, 6},
					},
					MACCommandQueue: []maccommand.Block{
						{
							CID: lorawan.DevStatusReq,
							MACCommands: []lorawan.MACCommand{
								{
									CID: lorawan.DevStatusReq,
								},
							},
						},
						{
							CID: lorawan.RXTimingSetupReq,
							MACCommands: []lorawan.MACCommand{
								{
									CID:     lorawan.RXTimingSetupReq,
									Payload: &lorawan.RXTimingSetupReqPayload{Delay: 3},
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
								DevAddr: ns.DevAddr,
								FCnt:    10,
							},
							FPort:      &fPortOne,
							FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
						},
					},
					ExpectedControllerHandleRXInfo:  expectedControllerHandleRXInfo,
					ExpectedApplicationHandleDataUp: expectedApplicationPushDataUp,
					ExpectedApplicationGetDataDown:  expectedGetDataDown,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: rxInfo.Timestamp + 1000000,
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
								DevAddr: ns.DevAddr,
								FCnt:    5,
								FOpts: []lorawan.MACCommand{
									{CID: lorawan.CID(6)},
									{CID: lorawan.CID(8), Payload: &lorawan.RXTimingSetupReqPayload{Delay: 3}},
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
				{
					Name:                 "unconfirmed uplink data + two downlink mac commands in queue (FRMPayload)",
					NodeSession:          ns,
					RXInfo:               rxInfo,
					SetMICKey:            ns.NwkSKey,
					DecryptFRMPayloadKey: &ns.NwkSKey,
					MACCommandQueue: []maccommand.Block{
						{
							FRMPayload: true,
							CID:        lorawan.DevStatusReq,
							MACCommands: []lorawan.MACCommand{
								{
									CID: lorawan.DevStatusReq,
								},
							},
						},
						{
							FRMPayload: true,
							CID:        lorawan.RXTimingSetupReq,
							MACCommands: []lorawan.MACCommand{
								{
									CID:     lorawan.RXTimingSetupReq,
									Payload: &lorawan.RXTimingSetupReqPayload{Delay: 3},
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
								DevAddr: ns.DevAddr,
								FCnt:    10,
							},
							FPort:      &fPortOne,
							FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
						},
					},
					ExpectedControllerHandleRXInfo:  expectedControllerHandleRXInfo,
					ExpectedApplicationHandleDataUp: expectedApplicationPushDataUp,
					ExpectedApplicationGetDataDown:  expectedGetDataDown,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: rxInfo.Timestamp + 1000000,
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
								DevAddr: ns.DevAddr,
								FCnt:    5,
							},
							FPort: &fPortZero,
							FRMPayload: []lorawan.Payload{
								&lorawan.MACCommand{CID: lorawan.CID(6)},
								&lorawan.MACCommand{CID: lorawan.CID(8), Payload: &lorawan.RXTimingSetupReqPayload{Delay: 3}},
							},
						},
					},
					ExpectedFCntUp:          11,
					ExpectedFCntDown:        6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					Name:        "unconfirmed uplink data + two downlink mac commands in queue (FRMPayload) + unconfirmed tx-payload in queue",
					NodeSession: ns,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					ApplicationGetDataDown: as.GetDataDownResponse{
						FPort: 3,
						Data:  []byte{4, 5, 6},
					},
					MACCommandQueue: []maccommand.Block{
						{
							FRMPayload: true,
							CID:        lorawan.DevStatusReq,
							MACCommands: []lorawan.MACCommand{
								{
									CID: lorawan.DevStatusReq,
								},
							},
						},
						{
							FRMPayload: true,
							CID:        lorawan.RXTimingSetupReq,
							MACCommands: []lorawan.MACCommand{
								{
									CID:     lorawan.RXTimingSetupReq,
									Payload: &lorawan.RXTimingSetupReqPayload{Delay: 3},
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
								DevAddr: ns.DevAddr,
								FCnt:    10,
							},
							FPort:      &fPortOne,
							FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
						},
					},
					ExpectedControllerHandleRXInfo:  expectedControllerHandleRXInfo,
					ExpectedApplicationHandleDataUp: expectedApplicationPushDataUp,
					ExpectedApplicationGetDataDown:  expectedGetDataDown,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: rxInfo.Timestamp + 1000000,
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
								DevAddr: ns.DevAddr,
								FCnt:    5,
								FCtrl: lorawan.FCtrl{
									FPending: true,
								},
							},
							FPort: &fPortThree,
							FRMPayload: []lorawan.Payload{
								&lorawan.DataPayload{Bytes: []byte{4, 5, 6}},
							},
						},
					},
					ExpectedMACCommandQueue: []maccommand.Block{
						{
							FRMPayload: true,
							CID:        lorawan.DevStatusReq,
							MACCommands: []lorawan.MACCommand{
								{
									CID: lorawan.DevStatusReq,
								},
							},
						},
						{
							FRMPayload: true,
							CID:        lorawan.RXTimingSetupReq,
							MACCommands: []lorawan.MACCommand{
								{
									CID:     lorawan.RXTimingSetupReq,
									Payload: &lorawan.RXTimingSetupReqPayload{Delay: 3},
								},
							},
						},
					},
					ExpectedFCntUp:          11,
					ExpectedFCntDown:        6,
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
			}

			runUplinkTests(ctx, tests)
		})

		Convey("Given a set of test-scenarios for tx-payload queue", func() {
			var fPortTen uint8 = 10

			tests := []uplinkTestCase{
				{
					Name:        "unconfirmed uplink data + one unconfirmed downlink payload in queue",
					NodeSession: ns,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					ApplicationGetDataDown: as.GetDataDownResponse{
						FPort: 10,
						Data:  []byte{1, 2, 3, 4},
					},
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
								FCnt:    10,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedControllerHandleRXInfo:  expectedControllerHandleRXInfo,
					ExpectedApplicationHandleDataUp: expectedApplicationPushDataUpNoData,
					ExpectedApplicationGetDataDown:  expectedGetDataDown,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: rxInfo.Timestamp + 1000000,
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
								DevAddr: ns.DevAddr,
								FCnt:    5,
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
					Name:        "unconfirmed uplink data + two unconfirmed downlink payloads in queue",
					NodeSession: ns,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					ApplicationGetDataDown: as.GetDataDownResponse{
						FPort:    10,
						Data:     []byte{1, 2, 3, 4},
						MoreData: true,
					},
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
								FCnt:    10,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedControllerHandleRXInfo:  expectedControllerHandleRXInfo,
					ExpectedApplicationHandleDataUp: expectedApplicationPushDataUpNoData,
					ExpectedApplicationGetDataDown:  expectedGetDataDown,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: rxInfo.Timestamp + 1000000,
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
								DevAddr: ns.DevAddr,
								FCnt:    5,
								FCtrl: lorawan.FCtrl{
									FPending: true,
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
					Name:        "unconfirmed uplink data + one confirmed downlink payload in queue",
					NodeSession: ns,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					ApplicationGetDataDown: as.GetDataDownResponse{
						FPort:     10,
						Data:      []byte{1, 2, 3, 4},
						Confirmed: true,
					},
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
								FCnt:    10,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedControllerHandleRXInfo:  expectedControllerHandleRXInfo,
					ExpectedApplicationHandleDataUp: expectedApplicationPushDataUpNoData,
					ExpectedApplicationGetDataDown:  expectedGetDataDown,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: rxInfo.Timestamp + 1000000,
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
								DevAddr: ns.DevAddr,
								FCnt:    5,
							},
							FPort: &fPortTen,
							FRMPayload: []lorawan.Payload{
								&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}},
							},
						},
					},
					ExpectedFCntUp:          11,
					ExpectedFCntDown:        5, // will be incremented after the node ACKs the frame
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
				{
					Name:        "unconfirmed uplink data + downlink payload which exceeds the max payload size (for dr 0)",
					NodeSession: ns,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					ApplicationGetDataDown: as.GetDataDownResponse{
						FPort: 10,
						Data:  make([]byte, 52),
					},
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
								FCnt:    10,
							},
							FPort: &fPortOne,
						},
					},
					ExpectedControllerHandleRXInfo:  expectedControllerHandleRXInfo,
					ExpectedApplicationHandleDataUp: expectedApplicationPushDataUpNoData,
					ExpectedApplicationGetDataDown:  expectedGetDataDown,
					ExpectedFCntUp:                  11,
					ExpectedFCntDown:                5, // payload has been discarded, nothing to transmit
					ExpectedEnabledChannels:         []int{0, 1, 2},
				},
				{
					Name:        "unconfirmed uplink data + one unconfirmed downlink payload in queue (exactly max size for dr 0) + one mac command",
					NodeSession: ns,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					ApplicationGetDataDown: as.GetDataDownResponse{
						FPort: 10,
						Data:  make([]byte, 51),
					},
					MACCommandQueue: []maccommand.Block{
						{
							CID: lorawan.DevStatusReq,
							MACCommands: []lorawan.MACCommand{
								{
									CID: lorawan.DevStatusReq,
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
								DevAddr: ns.DevAddr,
								FCnt:    10,
							},
							FPort: &fPortOne,
						},
					},

					ExpectedControllerHandleRXInfo:  expectedControllerHandleRXInfo,
					ExpectedApplicationHandleDataUp: expectedApplicationPushDataUpNoData,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: rxInfo.Timestamp + 1000000,
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
								DevAddr: ns.DevAddr,
								FCnt:    5,
								FCtrl: lorawan.FCtrl{
									FPending: true,
								},
							},
							FPort: &fPortTen,
							FRMPayload: []lorawan.Payload{
								&lorawan.DataPayload{Bytes: make([]byte, 51)},
							},
						},
					},
					ExpectedFCntUp:                 11,
					ExpectedFCntDown:               6,
					ExpectedApplicationGetDataDown: expectedGetDataDown,
					ExpectedMACCommandQueue: []maccommand.Block{
						{
							CID: lorawan.DevStatusReq,
							MACCommands: []lorawan.MACCommand{
								{
									CID: lorawan.DevStatusReq,
								},
							},
						},
					},
					ExpectedEnabledChannels: []int{0, 1, 2},
				},
			}

			runUplinkTests(ctx, tests)
		})

		Convey("Given a set of test-scenarios for ADR", func() {
			tests := []uplinkTestCase{
				{
					Name:        "adr triggered because of adr interval",
					NodeSession: nsADREnabled,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
								FCnt:    10,
								FCtrl: lorawan.FCtrl{
									ADR: true,
								},
							},
						},
					},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfoADR,
					ExpectedApplicationGetDataDown: expectedGetDataDown,
					ExpectedFCntUp:                 11,
					ExpectedFCntDown:               6,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: rxInfo.Timestamp + 1000000,
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
								DevAddr: ns.DevAddr,
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
					Name:        "adr interval matches, but node does not support adr",
					NodeSession: nsADREnabled,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
								FCnt:    10,
								FCtrl: lorawan.FCtrl{
									ADR: false,
								},
							},
						},
					},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedApplicationGetDataDown: expectedGetDataDown,
					ExpectedFCntUp:                 11,
					ExpectedFCntDown:               5,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
				{
					Name:        "acknowledgement of pending adr request",
					NodeSession: nsADREnabled,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					MACCommandPending: []maccommand.Block{
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
								DevAddr: ns.DevAddr,
								FCnt:    10,
								FOpts: []lorawan.MACCommand{
									{CID: lorawan.LinkADRAns, Payload: &lorawan.LinkADRAnsPayload{ChannelMaskACK: true, DataRateACK: true, PowerACK: true}},
								},
							},
						},
					},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedApplicationGetDataDown: expectedGetDataDown,
					ExpectedFCntUp:                 11,
					ExpectedFCntDown:               5,
					ExpectedTXPowerIndex:           3,
					ExpectedNbTrans:                1,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
				{
					Name:        "negative acknowledgement of pending adr request",
					NodeSession: nsADREnabled,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					MACCommandPending: []maccommand.Block{
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
								DevAddr: ns.DevAddr,
								FCnt:    10,
								FOpts: []lorawan.MACCommand{
									{CID: lorawan.LinkADRAns, Payload: &lorawan.LinkADRAnsPayload{ChannelMaskACK: false, DataRateACK: true, PowerACK: true}},
								},
							},
						},
					},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedApplicationGetDataDown: expectedGetDataDown,
					ExpectedFCntUp:                 11,
					ExpectedFCntDown:               5,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
				{
					Name:        "adr ack requested",
					NodeSession: nsADREnabled,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
								FCnt:    10,
								FCtrl: lorawan.FCtrl{
									ADRACKReq: true,
								},
							},
						},
					},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedApplicationGetDataDown: expectedGetDataDown,
					ExpectedFCntUp:                 11,
					ExpectedFCntDown:               6,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: rxInfo.Timestamp + 1000000,
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
								DevAddr: ns.DevAddr,
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
					Name:        "channel re-configuration triggered",
					NodeSession: nsExtraChannels,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
								FCnt:    10,
							},
						},
					},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfo,
					ExpectedApplicationGetDataDown: expectedGetDataDown,
					ExpectedFCntUp:                 11,
					ExpectedFCntDown:               6,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: rxInfo.Timestamp + 1000000,
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
								DevAddr: ns.DevAddr,
								FCnt:    5,
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
					Name:        "new channel re-configuration ack-ed",
					NodeSession: nsExtraChannels,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					MACCommandPending: []maccommand.Block{
						{
							CID: lorawan.LinkADRReq,
							MACCommands: maccommand.MACCommands{
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
								DevAddr: ns.DevAddr,
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
					ExpectedApplicationGetDataDown: expectedGetDataDown,
					ExpectedFCntUp:                 11,
					ExpectedFCntDown:               5,
					ExpectedTXPowerIndex:           1,
					ExpectedEnabledChannels:        []int{0, 1, 2},
				},
				{
					Name:        "new channel re-configuration not ack-ed",
					NodeSession: nsExtraChannels,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					MACCommandPending: []maccommand.Block{
						{
							CID: lorawan.LinkADRReq,
							MACCommands: maccommand.MACCommands{
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
								DevAddr: ns.DevAddr,
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
					ExpectedApplicationGetDataDown: expectedGetDataDown,
					ExpectedFCntUp:                 11,
					ExpectedFCntDown:               6,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: rxInfo.Timestamp + 1000000,
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
								DevAddr: ns.DevAddr,
								FCnt:    5,
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
					Name:        "channel re-configuration and adr triggered",
					NodeSession: nsExtraChannelsADR,
					RXInfo:      rxInfo,
					SetMICKey:   ns.NwkSKey,
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
								FCnt:    10,
								FCtrl: lorawan.FCtrl{
									ADR: true,
								},
							},
						},
					},
					ExpectedControllerHandleRXInfo: expectedControllerHandleRXInfoADR,
					ExpectedApplicationGetDataDown: expectedGetDataDown,
					ExpectedFCntUp:                 11,
					ExpectedFCntDown:               6,
					ExpectedTXInfo: &gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: rxInfo.Timestamp + 1000000,
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
								DevAddr: ns.DevAddr,
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

			runUplinkTests(ctx, tests)
		})
	})
}

func runUplinkTests(ctx common.Context, tests []uplinkTestCase) {
	for i, t := range tests {
		Convey(fmt.Sprintf("When testing: %s [%d]", t.Name, i), func() {
			// set application-server mocks
			ctx.Application.(*test.ApplicationClient).HandleDataUpErr = t.ApplicationHandleDataUpError
			ctx.Application.(*test.ApplicationClient).GetDataDownResponse = t.ApplicationGetDataDown
			ctx.Application.(*test.ApplicationClient).GetDataDownErr = t.ApplicationGetDataDownError

			// populate session and queues
			So(session.SaveNodeSession(ctx.RedisPool, t.NodeSession), ShouldBeNil)
			for _, block := range t.MACCommandQueue {
				So(maccommand.AddQueueItem(ctx.RedisPool, t.NodeSession.DevEUI, block), ShouldBeNil)
			}
			for _, pending := range t.MACCommandPending {
				So(maccommand.SetPending(ctx.RedisPool, t.NodeSession.DevEUI, pending), ShouldBeNil)
			}

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
			So(uplink.HandleRXPacket(ctx, rxPacket), ShouldResemble, t.ExpectedHandleRXPacketError)

			// network-controller validations
			if t.ExpectedControllerHandleRXInfo != nil {
				Convey("Then the expected rx-info is published to the network-controller", func() {
					So(ctx.Controller.(*test.NetworkControllerClient).HandleRXInfoChan, ShouldHaveLength, 1)
					pl := <-ctx.Controller.(*test.NetworkControllerClient).HandleRXInfoChan
					So(&pl, ShouldResemble, t.ExpectedControllerHandleRXInfo)
				})
			} else {
				So(ctx.Controller.(*test.NetworkControllerClient).HandleRXInfoChan, ShouldHaveLength, 0)
			}

			Convey("Then the expected mac-commands are received by the network-controller", func() {
				So(ctx.Controller.(*test.NetworkControllerClient).HandleDataUpMACCommandChan, ShouldHaveLength, len(t.ExpectedControllerHandleDataUpMACCommands))
				for _, expPl := range t.ExpectedControllerHandleDataUpMACCommands {
					pl := <-ctx.Controller.(*test.NetworkControllerClient).HandleDataUpMACCommandChan
					So(pl, ShouldResemble, expPl)
				}
			})

			// application-server validations
			if t.ExpectedApplicationHandleDataUp != nil {
				Convey("Then the expected rx-payloads are received by the application-server", func() {
					So(ctx.Application.(*test.ApplicationClient).HandleDataUpChan, ShouldHaveLength, 1)
					req := <-ctx.Application.(*test.ApplicationClient).HandleDataUpChan
					So(&req, ShouldResemble, t.ExpectedApplicationHandleDataUp)
				})
			} else {
				So(ctx.Application.(*test.ApplicationClient).HandleDataUpChan, ShouldHaveLength, 0)
			}

			Convey("Then the expected error payloads are sent to the application-server", func() {
				So(ctx.Application.(*test.ApplicationClient).HandleErrorChan, ShouldHaveLength, len(t.ExpectedApplicationHandleErrors))
				for _, expPL := range t.ExpectedApplicationHandleErrors {
					pl := <-ctx.Application.(*test.ApplicationClient).HandleErrorChan
					So(pl, ShouldResemble, expPL)
				}
			})

			if t.ExpectedApplicationHandleDataDownACK != nil {
				Convey("Then the expected downlink ACK was sent to the application-server", func() {
					So(ctx.Application.(*test.ApplicationClient).HandleDataDownACKChan, ShouldHaveLength, 1)
					req := <-ctx.Application.(*test.ApplicationClient).HandleDataDownACKChan
					So(&req, ShouldResemble, t.ExpectedApplicationHandleDataDownACK)
				})
			} else {
				So(ctx.Application.(*test.ApplicationClient).HandleDataDownACKChan, ShouldHaveLength, 0)
			}

			if t.ExpectedApplicationGetDataDown != nil {
				Convey("Then the expected get data down request was made to the application-server", func() {
					So(ctx.Application.(*test.ApplicationClient).GetDataDownChan, ShouldHaveLength, 1)
					req := <-ctx.Application.(*test.ApplicationClient).GetDataDownChan
					So(&req, ShouldResemble, t.ExpectedApplicationGetDataDown)
				})
			} else {
				So(ctx.Application.(*test.ApplicationClient).GetDataDownChan, ShouldHaveLength, 0)
			}

			// gateway validations
			if t.ExpectedTXInfo != nil {
				Convey("Then the expected downlink txinfo is used", func() {
					So(ctx.Gateway.(*test.GatewayBackend).TXPacketChan, ShouldHaveLength, 1)
					txPacket := <-ctx.Gateway.(*test.GatewayBackend).TXPacketChan
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
				So(ctx.Gateway.(*test.GatewayBackend).TXPacketChan, ShouldHaveLength, 0)
			}

			// node session validations
			Convey("Then the frame-counters are as expected", func() {
				ns, err := session.GetNodeSession(ctx.RedisPool, t.NodeSession.DevEUI)
				So(err, ShouldBeNil)
				So(ns.FCntDown, ShouldEqual, t.ExpectedFCntDown)
				So(ns.FCntUp, ShouldEqual, t.ExpectedFCntUp)
			})

			// ADR variables validations
			Convey("Then the Channels, TXPower and NbTrans are as expected", func() {
				ns, err := session.GetNodeSession(ctx.RedisPool, t.NodeSession.DevEUI)
				So(err, ShouldBeNil)
				So(ns.TXPowerIndex, ShouldEqual, t.ExpectedTXPowerIndex)
				So(ns.NbTrans, ShouldEqual, t.ExpectedNbTrans)
				So(ns.EnabledChannels, ShouldResemble, t.ExpectedEnabledChannels)
			})

			// queue validations
			Convey("Then the mac-command queue is as expected", func() {
				macQueue, err := maccommand.ReadQueueItems(ctx.RedisPool, t.NodeSession.DevEUI)
				So(err, ShouldBeNil)
				So(macQueue, ShouldResemble, t.ExpectedMACCommandQueue)
			})

			if t.ExpectedHandleRXPacketError == nil {
				Convey("Then the expected RSInfoSet has been added to the node-session", func() {
					ns, err := session.GetNodeSession(ctx.RedisPool, t.NodeSession.DevEUI)
					So(err, ShouldBeNil)
					So(ns.LastRXInfoSet, ShouldResemble, []gw.RXInfo{t.RXInfo})
				})
			}
		})
	}
}
