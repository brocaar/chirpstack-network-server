package testsuite

import (
	"errors"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/api/nc"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/queue"
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
	TXMACPayloadQueue    []queue.MACPayload  // downlink mac-command queue

	ApplicationGetDataDown        as.GetDataDownResponse // application-server get data down response
	ApplicationPublishDataUpError error                  // application-client publish data-up error
	ApplicationGetDataDownError   error                  // application-server get data down error

	ExpectedControllerPublishRXInfo            *nc.PublishRXInfoRequest            // expected network-controller publish rxinfo request
	ExpectedControllerPublishDataUpMACCommands []nc.PublishDataUpMACCommandRequest // expected network-controller publish dataup mac-command requests
	ExpectedControllerPublishErrors            []nc.PublishErrorRequest            // expected network-controller error requests

	ExpectedApplicationPublishDataUp      *as.PublishDataUpRequest      // expected application-server data up request
	ExpectedApplicationPublishErrors      []as.PublishErrorRequest      // expected application-server error requests
	ExpectedApplicationPublishDataDownACK *as.PublishDataDownACKRequest // expected application-server datadown ack request
	ExpectedApplicationGetDataDown        *as.GetDataDownRequest        // expected application-server get data down request

	ExpectedTXInfo              *gw.TXInfo          // expected tx-info (downlink)
	ExpectedPHYPayload          *lorawan.PHYPayload // expected (plaintext) PHYPayload (downlink)
	ExpectedFCntUp              uint32              // expected uplink frame counter
	ExpectedFCntDown            uint32              // expected downlink frame counter
	ExpectedHandleRXPacketError error               // expected handleRXPacket error
	ExpectedTXMACPayloadQueue   []queue.MACPayload  // expected downlink mac-command queue
}

func TestUplinkScenarios(t *testing.T) {
	conf := test.GetTestConfig()

	Convey("Given a clean state", t, func() {
		p := common.NewRedisPool(conf.RedisURL)
		test.MustFlushRedis(p)

		ctx := common.Context{
			NetID:       [3]byte{3, 2, 1},
			RedisPool:   p,
			Gateway:     test.NewTestGatewayBackend(),
			Application: test.NewTestApplicationClient(),
			Controller:  test.NewTestNetworkControllerClient(),
		}

		rxInfo := gw.RXInfo{
			Frequency: common.Band.UplinkChannels[0].Frequency,
			DataRate:  common.Band.DataRates[common.Band.UplinkChannels[0].DataRates[0]],
		}

		ns := session.NodeSession{
			DevAddr:  [4]byte{1, 2, 3, 4},
			DevEUI:   [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			NwkSKey:  [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			FCntUp:   8,
			FCntDown: 5,
			AppEUI:   [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
		}

		nsDelay := session.NodeSession{
			DevAddr:  [4]byte{1, 2, 3, 4},
			DevEUI:   [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			NwkSKey:  [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			FCntUp:   8,
			FCntDown: 5,
			AppEUI:   [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
			RXDelay:  3,
		}

		nsRX2 := session.NodeSession{
			DevAddr:  [4]byte{1, 2, 3, 4},
			DevEUI:   [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			NwkSKey:  [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			FCntUp:   8,
			FCntDown: 5,
			AppEUI:   [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
			RXWindow: session.RX2,
			RX2DR:    3,
		}

		nsRX2Delay := session.NodeSession{
			DevAddr:  [4]byte{1, 2, 3, 4},
			DevEUI:   [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			NwkSKey:  [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			FCntUp:   8,
			FCntDown: 5,
			AppEUI:   [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
			RXWindow: session.RX2,
			RXDelay:  5,
		}

		var fPortZero uint8
		var fPortOne uint8 = 1

		expectedControllerPublishRXInfo := &nc.PublishRXInfoRequest{
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
					Time:    rxInfo.Time.String(),
					Rssi:    int32(rxInfo.RSSI),
					LoRaSNR: rxInfo.LoRaSNR,
				},
			},
		}

		expectedApplicationPushDataUp := &as.PublishDataUpRequest{
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
					Mac:     rxInfo.MAC[:],
					Time:    rxInfo.Time.String(),
					Rssi:    int32(rxInfo.RSSI),
					LoRaSNR: rxInfo.LoRaSNR,
				},
			},
		}

		expectedApplicationPushDataUpNoData := &as.PublishDataUpRequest{
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
					Mac:     rxInfo.MAC[:],
					Time:    rxInfo.Time.String(),
					Rssi:    int32(rxInfo.RSSI),
					LoRaSNR: rxInfo.LoRaSNR,
				},
			},
		}

		expectedGetDataDown := &as.GetDataDownRequest{
			DevEUI:         ns.DevEUI[:],
			MaxPayloadSize: 51,
			FCnt:           5,
		}

		Convey("Given a set of test-scenarios for error handling", func() {
			tests := []uplinkTestCase{
				{
					Name:                          "the application backend returns an error",
					NodeSession:                   ns,
					RXInfo:                        rxInfo,
					SetMICKey:                     ns.NwkSKey,
					ApplicationPublishDataUpError: errors.New("BOOM!"),
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
					ExpectedControllerPublishRXInfo: expectedControllerPublishRXInfo,
					ExpectedFCntUp:                  8,
					ExpectedFCntDown:                5,
					ExpectedHandleRXPacketError:     errors.New("publish data up to application-server error: BOOM!"),
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
					ExpectedHandleRXPacketError: errors.New("invalid FCnt or too many dropped frames"),
					ExpectedApplicationPublishErrors: []as.PublishErrorRequest{
						{
							DevEUI: ns.DevEUI[:],
							Type:   as.ErrorType_DATA_UP_FCNT,
							Error:  "invalid FCnt or too many dropped frames (server_fcnt: 8, packet_fcnt: 7)",
						},
					},
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
					ExpectedHandleRXPacketError: errors.New("invalid MIC"),
					ExpectedApplicationPublishErrors: []as.PublishErrorRequest{
						{
							DevEUI: ns.DevEUI[:],
							Type:   as.ErrorType_DATA_UP_MIC,
							Error:  "invalid MIC",
						},
					},
				},
			}

			runUplinkTests(ctx, tests)
		})

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
					ExpectedControllerPublishRXInfo:  expectedControllerPublishRXInfo,
					ExpectedApplicationPublishDataUp: expectedApplicationPushDataUp,
					ExpectedApplicationGetDataDown:   expectedGetDataDown,
					ExpectedFCntUp:                   10,
					ExpectedFCntDown:                 5,
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
					ExpectedControllerPublishRXInfo:  expectedControllerPublishRXInfo,
					ExpectedApplicationPublishDataUp: expectedApplicationPushDataUpNoData,
					ExpectedApplicationGetDataDown:   expectedGetDataDown,
					ExpectedFCntUp:                   10,
					ExpectedFCntDown:                 5,
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
					ExpectedControllerPublishRXInfo:  expectedControllerPublishRXInfo,
					ExpectedApplicationPublishDataUp: expectedApplicationPushDataUp,
					ExpectedApplicationGetDataDown:   expectedGetDataDown,
					ExpectedTXInfo: &gw.TXInfo{
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
					ExpectedFCntUp:   10,
					ExpectedFCntDown: 6,
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
					ExpectedControllerPublishRXInfo:  expectedControllerPublishRXInfo,
					ExpectedApplicationPublishDataUp: expectedApplicationPushDataUpNoData,
					ExpectedApplicationGetDataDown:   expectedGetDataDown,
					ExpectedTXInfo: &gw.TXInfo{
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
					ExpectedFCntUp:   10,
					ExpectedFCntDown: 6,
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
					ExpectedControllerPublishRXInfo:  expectedControllerPublishRXInfo,
					ExpectedApplicationPublishDataUp: expectedApplicationPushDataUpNoData,
					ExpectedApplicationGetDataDown:   expectedGetDataDown,
					ExpectedTXInfo: &gw.TXInfo{
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
					ExpectedFCntUp:   10,
					ExpectedFCntDown: 6,
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
					ExpectedControllerPublishRXInfo:  expectedControllerPublishRXInfo,
					ExpectedApplicationPublishDataUp: expectedApplicationPushDataUpNoData,
					ExpectedApplicationGetDataDown: &as.GetDataDownRequest{
						DevEUI:         ns.DevEUI[:],
						FCnt:           5,
						MaxPayloadSize: 115,
					},
					ExpectedTXInfo: &gw.TXInfo{
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
					ExpectedFCntUp:   10,
					ExpectedFCntDown: 6,
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
					ExpectedControllerPublishRXInfo:  expectedControllerPublishRXInfo,
					ExpectedApplicationPublishDataUp: expectedApplicationPushDataUpNoData,
					ExpectedApplicationGetDataDown:   expectedGetDataDown,
					ExpectedTXInfo: &gw.TXInfo{
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
					ExpectedFCntUp:   10,
					ExpectedFCntDown: 6,
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
									{CID: lorawan.LinkCheckReq},
									{CID: lorawan.LinkADRAns, Payload: &lorawan.LinkADRAnsPayload{ChannelMaskACK: true}},
								},
							},
						},
					},
					ExpectedApplicationGetDataDown:  expectedGetDataDown,
					ExpectedControllerPublishRXInfo: expectedControllerPublishRXInfo,
					ExpectedControllerPublishDataUpMACCommands: []nc.PublishDataUpMACCommandRequest{
						{DevEUI: ns.DevEUI[:], Data: []byte{2}},
						{DevEUI: ns.DevEUI[:], Data: []byte{3, 1}},
					},
					ExpectedFCntUp:   10,
					ExpectedFCntDown: 5,
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
								&lorawan.MACCommand{CID: lorawan.LinkCheckReq},
								&lorawan.MACCommand{CID: lorawan.LinkADRAns, Payload: &lorawan.LinkADRAnsPayload{ChannelMaskACK: true}},
							},
						},
					},
					ExpectedApplicationGetDataDown:  expectedGetDataDown,
					ExpectedControllerPublishRXInfo: expectedControllerPublishRXInfo,
					ExpectedControllerPublishDataUpMACCommands: []nc.PublishDataUpMACCommandRequest{
						{DevEUI: ns.DevEUI[:], FrmPayload: true, Data: []byte{2}},
						{DevEUI: ns.DevEUI[:], FrmPayload: true, Data: []byte{3, 1}},
					},
					ExpectedFCntUp:   10,
					ExpectedFCntDown: 5,
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
					TXMACPayloadQueue: []queue.MACPayload{
						{DevEUI: ns.DevEUI, Data: []byte{6}},
						{DevEUI: ns.DevEUI, Data: []byte{8, 3}},
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
					ExpectedControllerPublishRXInfo:  expectedControllerPublishRXInfo,
					ExpectedApplicationPublishDataUp: expectedApplicationPushDataUp,
					ExpectedApplicationGetDataDown:   expectedGetDataDown,
					ExpectedTXInfo: &gw.TXInfo{
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
					ExpectedFCntUp:   10,
					ExpectedFCntDown: 6,
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
					TXMACPayloadQueue: []queue.MACPayload{
						{DevEUI: ns.DevEUI, Data: []byte{6}},
						{DevEUI: ns.DevEUI, Data: []byte{8, 3}},
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
					ExpectedControllerPublishRXInfo:  expectedControllerPublishRXInfo,
					ExpectedApplicationPublishDataUp: expectedApplicationPushDataUp,
					ExpectedApplicationGetDataDown:   expectedGetDataDown,
					ExpectedTXInfo: &gw.TXInfo{
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
					ExpectedFCntUp:   10,
					ExpectedFCntDown: 6,
				},
				{
					Name:                 "unconfirmed uplink data + two downlink mac commands in queue (FRMPayload)",
					NodeSession:          ns,
					RXInfo:               rxInfo,
					SetMICKey:            ns.NwkSKey,
					DecryptFRMPayloadKey: &ns.NwkSKey,
					TXMACPayloadQueue: []queue.MACPayload{
						{DevEUI: ns.DevEUI, FRMPayload: true, Data: []byte{6}},
						{DevEUI: ns.DevEUI, FRMPayload: true, Data: []byte{8, 3}},
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
					ExpectedControllerPublishRXInfo:  expectedControllerPublishRXInfo,
					ExpectedApplicationPublishDataUp: expectedApplicationPushDataUp,
					ExpectedApplicationGetDataDown:   expectedGetDataDown,
					ExpectedTXInfo: &gw.TXInfo{
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
					ExpectedFCntUp:   10,
					ExpectedFCntDown: 6,
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
					TXMACPayloadQueue: []queue.MACPayload{
						{DevEUI: ns.DevEUI, FRMPayload: true, Data: []byte{6}},
						{DevEUI: ns.DevEUI, FRMPayload: true, Data: []byte{8, 3}},
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
					ExpectedControllerPublishRXInfo:  expectedControllerPublishRXInfo,
					ExpectedApplicationPublishDataUp: expectedApplicationPushDataUp,
					ExpectedApplicationGetDataDown:   expectedGetDataDown,
					ExpectedTXInfo: &gw.TXInfo{
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
					ExpectedTXMACPayloadQueue: []queue.MACPayload{
						{DevEUI: ns.DevEUI, FRMPayload: true, Data: []byte{6}},
						{DevEUI: ns.DevEUI, FRMPayload: true, Data: []byte{8, 3}},
					},
					ExpectedFCntUp:   10,
					ExpectedFCntDown: 6,
				},
				{
					Name:                 "unconfirmed uplink data + 18 bytes of MAC commands (FOpts) of which one is invalid",
					NodeSession:          ns,
					RXInfo:               rxInfo,
					SetMICKey:            ns.NwkSKey,
					DecryptFRMPayloadKey: &ns.NwkSKey,
					TXMACPayloadQueue: []queue.MACPayload{
						{DevEUI: ns.DevEUI, Data: []byte{2, 10, 3}},
						{DevEUI: ns.DevEUI, Data: []byte{6}},
						{DevEUI: ns.DevEUI, Data: []byte{4, 15}},
						{DevEUI: ns.DevEUI, Data: []byte{4, 15, 16}}, // invalid payload, should be discarded + error notification
						{DevEUI: ns.DevEUI, Data: []byte{2, 10, 4}},
						{DevEUI: ns.DevEUI, Data: []byte{2, 10, 5}},
						{DevEUI: ns.DevEUI, Data: []byte{2, 10, 6}}, // this payload should stay in the queue
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
					ExpectedControllerPublishRXInfo:  expectedControllerPublishRXInfo,
					ExpectedApplicationPublishDataUp: expectedApplicationPushDataUp,
					ExpectedApplicationGetDataDown:   expectedGetDataDown,
					ExpectedControllerPublishErrors: []nc.PublishErrorRequest{
						{DevEUI: ns.DevEUI[:], Error: "unmarshal mac command error: lorawan: 1 byte of data is expected (command: 040F10)"},
					},
					ExpectedTXInfo: &gw.TXInfo{
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
								FOpts: []lorawan.MACCommand{
									{CID: lorawan.LinkCheckAns, Payload: &lorawan.LinkCheckAnsPayload{Margin: 10, GwCnt: 3}},
									{CID: lorawan.DevStatusReq},
									{CID: lorawan.DutyCycleReq, Payload: &lorawan.DutyCycleReqPayload{MaxDCCycle: 15}},
									{CID: lorawan.LinkCheckAns, Payload: &lorawan.LinkCheckAnsPayload{Margin: 10, GwCnt: 4}},
									{CID: lorawan.LinkCheckAns, Payload: &lorawan.LinkCheckAnsPayload{Margin: 10, GwCnt: 5}},
								},
							},
						},
					},

					ExpectedTXMACPayloadQueue: []queue.MACPayload{
						{DevEUI: ns.DevEUI, Data: []byte{2, 10, 6}},
					},
					ExpectedFCntUp:   10,
					ExpectedFCntDown: 6,
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
					ExpectedControllerPublishRXInfo:  expectedControllerPublishRXInfo,
					ExpectedApplicationPublishDataUp: expectedApplicationPushDataUpNoData,
					ExpectedApplicationGetDataDown:   expectedGetDataDown,
					ExpectedTXInfo: &gw.TXInfo{
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
					ExpectedFCntUp:   10,
					ExpectedFCntDown: 6,
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
					ExpectedControllerPublishRXInfo:  expectedControllerPublishRXInfo,
					ExpectedApplicationPublishDataUp: expectedApplicationPushDataUpNoData,
					ExpectedApplicationGetDataDown:   expectedGetDataDown,
					ExpectedTXInfo: &gw.TXInfo{
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
					ExpectedFCntUp:   10,
					ExpectedFCntDown: 6,
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
					ExpectedControllerPublishRXInfo:  expectedControllerPublishRXInfo,
					ExpectedApplicationPublishDataUp: expectedApplicationPushDataUpNoData,
					ExpectedApplicationGetDataDown:   expectedGetDataDown,
					ExpectedTXInfo: &gw.TXInfo{
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
					ExpectedFCntUp:   10,
					ExpectedFCntDown: 5, // will be incremented after the node ACKs the frame
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
					ExpectedControllerPublishRXInfo:  expectedControllerPublishRXInfo,
					ExpectedApplicationPublishDataUp: expectedApplicationPushDataUpNoData,
					ExpectedApplicationGetDataDown:   expectedGetDataDown,
					ExpectedFCntUp:                   10,
					ExpectedFCntDown:                 5, // payload has been discarded, nothing to transmit
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
					TXMACPayloadQueue: []queue.MACPayload{
						{DevEUI: ns.DevEUI, Data: []byte{6}},
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

					ExpectedControllerPublishRXInfo:  expectedControllerPublishRXInfo,
					ExpectedApplicationPublishDataUp: expectedApplicationPushDataUpNoData,
					ExpectedTXInfo: &gw.TXInfo{
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
					ExpectedFCntUp:                 10,
					ExpectedFCntDown:               6,
					ExpectedApplicationGetDataDown: expectedGetDataDown,
					ExpectedTXMACPayloadQueue: []queue.MACPayload{
						{DevEUI: ns.DevEUI, Data: []byte{6}},
					},
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
			ctx.Application.(*test.TestApplicationClient).PublishDataUpErr = t.ApplicationPublishDataUpError
			ctx.Application.(*test.TestApplicationClient).GetDataDownResponse = t.ApplicationGetDataDown
			ctx.Application.(*test.TestApplicationClient).GetDataDownErr = t.ApplicationGetDataDownError

			// populate session and queues
			So(session.CreateNodeSession(ctx.RedisPool, t.NodeSession), ShouldBeNil)
			for _, pl := range t.TXMACPayloadQueue {
				So(queue.AddMACPayloadToTXQueue(ctx.RedisPool, pl), ShouldBeNil)
			}

			// encrypt FRMPayload and set MIC
			if t.EncryptFRMPayloadKey != nil {
				So(t.PHYPayload.EncryptFRMPayload(*t.EncryptFRMPayloadKey), ShouldBeNil)
			}
			So(t.PHYPayload.SetMIC(t.SetMICKey), ShouldBeNil)

			// create RXPacket and call HandleRXPacket
			rxPacket := gw.RXPacket{
				RXInfo:     t.RXInfo,
				PHYPayload: t.PHYPayload,
			}
			So(uplink.HandleRXPacket(ctx, rxPacket), ShouldResemble, t.ExpectedHandleRXPacketError)

			// network-controller validations
			if t.ExpectedControllerPublishRXInfo != nil {
				Convey("Then the expected rx-info is published to the network-controller", func() {
					So(ctx.Controller.(*test.TestNetworkControllerClient).PublishRXInfoChan, ShouldHaveLength, 1)
					pl := <-ctx.Controller.(*test.TestNetworkControllerClient).PublishRXInfoChan
					So(&pl, ShouldResemble, t.ExpectedControllerPublishRXInfo)
				})
			} else {
				So(ctx.Controller.(*test.TestNetworkControllerClient).PublishRXInfoChan, ShouldHaveLength, 0)
			}

			Convey("Then the expected error payloads are sent to the network-controller", func() {
				So(ctx.Controller.(*test.TestNetworkControllerClient).PublishErrorChan, ShouldHaveLength, len(t.ExpectedControllerPublishErrors))
				for _, expPL := range t.ExpectedControllerPublishErrors {
					pl := <-ctx.Controller.(*test.TestNetworkControllerClient).PublishErrorChan
					So(pl, ShouldResemble, expPL)
				}
			})

			Convey("Then the expected mac-commands are received by the network-controller", func() {
				So(ctx.Controller.(*test.TestNetworkControllerClient).PublishDataUpMACCommandChan, ShouldHaveLength, len(t.ExpectedControllerPublishDataUpMACCommands))
				for _, expPl := range t.ExpectedControllerPublishDataUpMACCommands {
					pl := <-ctx.Controller.(*test.TestNetworkControllerClient).PublishDataUpMACCommandChan
					So(pl, ShouldResemble, expPl)
				}
			})

			// application-server validations
			if t.ExpectedApplicationPublishDataUp != nil {
				Convey("Then the expected rx-payloads are received by the application-server", func() {
					So(ctx.Application.(*test.TestApplicationClient).PublishDataUpChan, ShouldHaveLength, 1)
					req := <-ctx.Application.(*test.TestApplicationClient).PublishDataUpChan
					So(&req, ShouldResemble, t.ExpectedApplicationPublishDataUp)
				})
			} else {
				So(ctx.Application.(*test.TestApplicationClient).PublishDataUpChan, ShouldHaveLength, 0)
			}

			Convey("Then the expected error payloads are sent to the application-server", func() {
				So(ctx.Application.(*test.TestApplicationClient).PublishErrorChan, ShouldHaveLength, len(t.ExpectedApplicationPublishErrors))
				for _, expPL := range t.ExpectedApplicationPublishErrors {
					pl := <-ctx.Application.(*test.TestApplicationClient).PublishErrorChan
					So(pl, ShouldResemble, expPL)
				}
			})

			if t.ExpectedApplicationPublishDataDownACK != nil {
				Convey("Then the expected downlink ACK was sent to the application-server", func() {
					So(ctx.Application.(*test.TestApplicationClient).PublishDataDownACKChan, ShouldHaveLength, 1)
					req := <-ctx.Application.(*test.TestApplicationClient).PublishDataDownACKChan
					So(&req, ShouldResemble, t.ExpectedApplicationPublishDataDownACK)
				})
			} else {
				So(ctx.Application.(*test.TestApplicationClient).PublishDataDownACKChan, ShouldHaveLength, 0)
			}

			if t.ExpectedApplicationGetDataDown != nil {
				Convey("Then the expected get data down request was made to the application-server", func() {
					So(ctx.Application.(*test.TestApplicationClient).GetDataDownChan, ShouldHaveLength, 1)
					req := <-ctx.Application.(*test.TestApplicationClient).GetDataDownChan
					So(&req, ShouldResemble, t.ExpectedApplicationGetDataDown)
				})
			} else {
				So(ctx.Application.(*test.TestApplicationClient).GetDataDownChan, ShouldHaveLength, 0)
			}

			// gateway validations
			if t.ExpectedTXInfo != nil {
				Convey("Then the expected downlink txinfo is used", func() {
					So(ctx.Gateway.(*test.TestGatewayBackend).TXPacketChan, ShouldHaveLength, 1)
					txPacket := <-ctx.Gateway.(*test.TestGatewayBackend).TXPacketChan
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
				So(ctx.Gateway.(*test.TestGatewayBackend).TXPacketChan, ShouldHaveLength, 0)
			}

			// node session validations
			Convey("Then the frame-counters are as expected", func() {
				ns, err := session.GetNodeSessionByDevEUI(ctx.RedisPool, t.NodeSession.DevEUI)
				So(err, ShouldBeNil)
				So(ns.FCntDown, ShouldEqual, t.ExpectedFCntDown)
				So(ns.FCntUp, ShouldEqual, t.ExpectedFCntUp)
			})

			// queue validations
			Convey("Then the mac-command queue is as expected", func() {
				macQueue, err := queue.ReadMACPayloadTXQueue(ctx.RedisPool, t.NodeSession.DevAddr)
				So(err, ShouldBeNil)
				So(macQueue, ShouldResemble, t.ExpectedTXMACPayloadQueue)
			})
		})
	}
}
