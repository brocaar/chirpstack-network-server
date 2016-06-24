package loraserver

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

type dataUpTestCase struct {
	Name                         string              // name of the test
	RXInfo                       models.RXInfo       // rx-info of the "received" packet
	EncryptFRMPayloadKey         lorawan.AES128Key   // key to use for encrypting the uplink FRMPayload
	DecryptExpectedFRMPayloadKey lorawan.AES128Key   // key to use for decrypting the downlink FRMPayload
	SetMICKey                    lorawan.AES128Key   // key to use for setting the mic
	PHYPayload                   lorawan.PHYPayload  // (unencrypted) "received" PHYPayload
	ApplicationBackendError      error               // error returned by the application backend
	TXPayloadQueue               []models.TXPayload  // tx-payload queue
	TXMACPayloadInProcess        *models.TXPayload   // tx-payload in-process queue
	TXMACPayloadQueue            []models.MACPayload // downlink mac-command queue

	ExpectedControllerRXInfoPayloads []models.RXInfoPayload
	ExpectedControllerRXMACPayloads  []models.MACPayload
	ExpectedControllerErrorPayloads  []models.ErrorPayload
	ExpectedApplicationRXPayloads    []models.RXPayload
	ExpectedApplicationNotifications []interface{} // to be refactored into separate types
	ExpectedGatewayTXPackets         []models.TXPacket
	ExpectedFCntUp                   uint32
	ExpectedFCntDown                 uint32
	ExpectedHandleRXPacketError      error
	ExpectedTXMACPayloadQueue        []models.MACPayload
	ExpectedGetTXPayloadFromQueue    *models.TXPayload
}

func TestHandleDataUpScenarios(t *testing.T) {
	conf := getConfig()

	Convey("Given a clean state, test backends and a node-session", t, func() {
		app := &testApplicationBackend{
			rxPayloadChan:           make(chan models.RXPayload, 1),
			txPayloadChan:           make(chan models.TXPayload, 1),
			notificationPayloadChan: make(chan interface{}, 10),
		}
		gw := &testGatewayBackend{
			rxPacketChan: make(chan models.RXPacket, 1),
			txPacketChan: make(chan models.TXPacket, 1),
		}
		ctrl := &testControllerBackend{
			rxMACPayloadChan:  make(chan models.MACPayload, 2),
			txMACPayloadChan:  make(chan models.MACPayload, 1),
			rxInfoPayloadChan: make(chan models.RXInfoPayload, 1),
			errorPayloadChan:  make(chan models.ErrorPayload, 1),
		}
		p := NewRedisPool(conf.RedisURL)
		mustFlushRedis(p)

		ctx := Context{
			RedisPool:   p,
			Gateway:     gw,
			Application: app,
			Controller:  ctrl,
		}

		ns := models.NodeSession{
			DevAddr:  [4]byte{1, 2, 3, 4},
			DevEUI:   [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			AppSKey:  [16]byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
			NwkSKey:  [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			FCntUp:   8,
			FCntDown: 5,
			AppEUI:   [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
		}
		So(createNodeSession(ctx.RedisPool, ns), ShouldBeNil)

		rxInfo := models.RXInfo{
			Frequency: Band.UplinkChannels[0].Frequency,
			DataRate:  Band.DataRates[Band.UplinkChannels[0].DataRates[0]],
		}

		var fPortZero uint8
		var fPortOne uint8 = 1

		Convey("Given a set of test-scenarios for basic flows (nothing in queue)", func() {
			tests := []dataUpTestCase{
				// errors
				{
					Name:                    "the application backend returns an error",
					RXInfo:                  rxInfo,
					EncryptFRMPayloadKey:    ns.AppSKey,
					SetMICKey:               ns.NwkSKey,
					ApplicationBackendError: errors.New("BOOM!"),

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

					ExpectedControllerRXInfoPayloads: []models.RXInfoPayload{
						{DevEUI: ns.DevEUI, FCnt: 10, RXInfo: []models.RXInfo{rxInfo}},
					},
					ExpectedFCntUp:              8,
					ExpectedFCntDown:            5,
					ExpectedHandleRXPacketError: errors.New("send rx payload to application error: BOOM!"),
				},
				{
					Name:                 "the frame-counter is invalid",
					RXInfo:               rxInfo,
					EncryptFRMPayloadKey: ns.AppSKey,
					SetMICKey:            ns.NwkSKey,

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
				},
				{
					Name:                 "the mic is invalid",
					RXInfo:               rxInfo,
					EncryptFRMPayloadKey: ns.AppSKey,
					SetMICKey:            ns.AppSKey,

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
				},
				// basic flows
				{
					Name:                 "unconfirmed uplink data with payload",
					RXInfo:               rxInfo,
					EncryptFRMPayloadKey: ns.AppSKey,
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
							FPort:      &fPortOne,
							FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
						},
					},

					ExpectedControllerRXInfoPayloads: []models.RXInfoPayload{
						{DevEUI: ns.DevEUI, FCnt: 10, RXInfo: []models.RXInfo{rxInfo}},
					},
					ExpectedApplicationRXPayloads: []models.RXPayload{
						{DevEUI: ns.DevEUI, FPort: 1, GatewayCount: 1, Data: []byte{1, 2, 3, 4}},
					},
					ExpectedFCntUp:   10,
					ExpectedFCntDown: 5,
				},
				{
					Name:                 "unconfirmed uplink data without payload (just a FPort)",
					RXInfo:               rxInfo,
					EncryptFRMPayloadKey: ns.AppSKey,
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
							FPort: &fPortOne,
						},
					},

					ExpectedControllerRXInfoPayloads: []models.RXInfoPayload{
						{DevEUI: ns.DevEUI, FCnt: 10, RXInfo: []models.RXInfo{rxInfo}},
					},
					ExpectedApplicationRXPayloads: []models.RXPayload{
						{DevEUI: ns.DevEUI, FPort: 1, GatewayCount: 1},
					},
					ExpectedFCntUp:   10,
					ExpectedFCntDown: 5,
				},
				{
					Name:                 "confirmed uplink data with payload",
					RXInfo:               rxInfo,
					EncryptFRMPayloadKey: ns.AppSKey,
					SetMICKey:            ns.NwkSKey,

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

					ExpectedControllerRXInfoPayloads: []models.RXInfoPayload{
						{DevEUI: ns.DevEUI, FCnt: 10, RXInfo: []models.RXInfo{rxInfo}},
					},
					ExpectedApplicationRXPayloads: []models.RXPayload{
						{DevEUI: ns.DevEUI, FPort: 1, GatewayCount: 1, Data: []byte{1, 2, 3, 4}},
					},
					ExpectedGatewayTXPackets: []models.TXPacket{
						{
							TXInfo: models.TXInfo{
								Timestamp: rxInfo.Timestamp + 1000000,
								Frequency: rxInfo.Frequency,
								Power:     14,
								DataRate:  rxInfo.DataRate,
							},
							PHYPayload: lorawan.PHYPayload{
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
						},
					},
					ExpectedFCntUp:   10,
					ExpectedFCntDown: 6,
				},
				{
					Name:                 "confirmed uplink data without payload",
					RXInfo:               rxInfo,
					EncryptFRMPayloadKey: ns.AppSKey,
					SetMICKey:            ns.NwkSKey,

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

					ExpectedControllerRXInfoPayloads: []models.RXInfoPayload{
						{DevEUI: ns.DevEUI, FCnt: 10, RXInfo: []models.RXInfo{rxInfo}},
					},
					ExpectedApplicationRXPayloads: []models.RXPayload{
						{DevEUI: ns.DevEUI, FPort: 1, GatewayCount: 1},
					},
					ExpectedGatewayTXPackets: []models.TXPacket{
						{
							TXInfo: models.TXInfo{
								Timestamp: rxInfo.Timestamp + 1000000,
								Frequency: rxInfo.Frequency,
								Power:     14,
								DataRate:  rxInfo.DataRate,
							},
							PHYPayload: lorawan.PHYPayload{
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
						},
					},
					ExpectedFCntUp:   10,
					ExpectedFCntDown: 6,
				},
				{
					Name:                 "two uplink mac commands (FOpts)",
					RXInfo:               rxInfo,
					EncryptFRMPayloadKey: ns.NwkSKey,
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
								FOpts: []lorawan.MACCommand{
									{CID: lorawan.LinkCheckReq},
									{CID: lorawan.LinkADRAns, Payload: &lorawan.LinkADRAnsPayload{ChannelMaskACK: true}},
								},
							},
						},
					},

					ExpectedControllerRXInfoPayloads: []models.RXInfoPayload{
						{DevEUI: ns.DevEUI, FCnt: 10, RXInfo: []models.RXInfo{rxInfo}},
					},
					ExpectedControllerRXMACPayloads: []models.MACPayload{
						{DevEUI: ns.DevEUI, MACCommand: []byte{2}},
						{DevEUI: ns.DevEUI, MACCommand: []byte{3, 1}},
					},
					ExpectedFCntUp:   10,
					ExpectedFCntDown: 5,
				},
				{
					Name:                 "two uplink mac commands (FRMPayload)",
					RXInfo:               rxInfo,
					EncryptFRMPayloadKey: ns.NwkSKey,
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

					ExpectedControllerRXInfoPayloads: []models.RXInfoPayload{
						{DevEUI: ns.DevEUI, FCnt: 10, RXInfo: []models.RXInfo{rxInfo}},
					},
					ExpectedControllerRXMACPayloads: []models.MACPayload{
						{DevEUI: ns.DevEUI, FRMPayload: true, MACCommand: []byte{2}},
						{DevEUI: ns.DevEUI, FRMPayload: true, MACCommand: []byte{3, 1}},
					},
					ExpectedFCntUp:   10,
					ExpectedFCntDown: 5,
				},
			}

			runDataUpTests(ctx, ns.DevEUI, ns.DevAddr, tests)
		})

		Convey("Given a set of test-scenarios for mac-command queue", func() {
			var fPortThree uint8 = 3

			tests := []dataUpTestCase{
				{
					Name:                 "unconfirmed uplink data + two downlink mac commands in queue (FOpts)",
					RXInfo:               rxInfo,
					EncryptFRMPayloadKey: ns.AppSKey,
					SetMICKey:            ns.NwkSKey,
					TXMACPayloadQueue: []models.MACPayload{
						{DevEUI: ns.DevEUI, MACCommand: []byte{6}},
						{DevEUI: ns.DevEUI, MACCommand: []byte{8, 3}},
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

					ExpectedControllerRXInfoPayloads: []models.RXInfoPayload{
						{DevEUI: ns.DevEUI, FCnt: 10, RXInfo: []models.RXInfo{rxInfo}},
					},
					ExpectedApplicationRXPayloads: []models.RXPayload{
						{DevEUI: ns.DevEUI, FPort: 1, GatewayCount: 1, Data: []byte{1, 2, 3, 4}},
					},
					ExpectedGatewayTXPackets: []models.TXPacket{
						{
							TXInfo: models.TXInfo{
								Timestamp: rxInfo.Timestamp + 1000000,
								Frequency: rxInfo.Frequency,
								Power:     14,
								DataRate:  rxInfo.DataRate,
							},
							PHYPayload: lorawan.PHYPayload{
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
						},
					},
					ExpectedFCntUp:   10,
					ExpectedFCntDown: 6,
				},
				{
					Name:                         "unconfirmed uplink data + two downlink mac commands in queue (FOpts) + unconfirmed tx-payload in queue",
					RXInfo:                       rxInfo,
					EncryptFRMPayloadKey:         ns.AppSKey,
					DecryptExpectedFRMPayloadKey: ns.AppSKey,
					SetMICKey:                    ns.NwkSKey,
					TXPayloadQueue: []models.TXPayload{
						{DevEUI: ns.DevEUI, FPort: 3, Data: []byte{4, 5, 6}},
					},
					TXMACPayloadQueue: []models.MACPayload{
						{DevEUI: ns.DevEUI, MACCommand: []byte{6}},
						{DevEUI: ns.DevEUI, MACCommand: []byte{8, 3}},
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

					ExpectedControllerRXInfoPayloads: []models.RXInfoPayload{
						{DevEUI: ns.DevEUI, FCnt: 10, RXInfo: []models.RXInfo{rxInfo}},
					},
					ExpectedApplicationRXPayloads: []models.RXPayload{
						{DevEUI: ns.DevEUI, FPort: 1, GatewayCount: 1, Data: []byte{1, 2, 3, 4}},
					},
					ExpectedGatewayTXPackets: []models.TXPacket{
						{
							TXInfo: models.TXInfo{
								Timestamp: rxInfo.Timestamp + 1000000,
								Frequency: rxInfo.Frequency,
								Power:     14,
								DataRate:  rxInfo.DataRate,
							},
							PHYPayload: lorawan.PHYPayload{
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
						},
					},
					ExpectedFCntUp:   10,
					ExpectedFCntDown: 6,
				},
				{
					Name:                         "unconfirmed uplink data + two downlink mac commands in queue (FRMPayload)",
					RXInfo:                       rxInfo,
					EncryptFRMPayloadKey:         ns.AppSKey,
					DecryptExpectedFRMPayloadKey: ns.NwkSKey,
					SetMICKey:                    ns.NwkSKey,
					TXMACPayloadQueue: []models.MACPayload{
						{DevEUI: ns.DevEUI, FRMPayload: true, MACCommand: []byte{6}},
						{DevEUI: ns.DevEUI, FRMPayload: true, MACCommand: []byte{8, 3}},
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

					ExpectedControllerRXInfoPayloads: []models.RXInfoPayload{
						{DevEUI: ns.DevEUI, FCnt: 10, RXInfo: []models.RXInfo{rxInfo}},
					},
					ExpectedApplicationRXPayloads: []models.RXPayload{
						{DevEUI: ns.DevEUI, FPort: 1, GatewayCount: 1, Data: []byte{1, 2, 3, 4}},
					},
					ExpectedGatewayTXPackets: []models.TXPacket{
						{
							TXInfo: models.TXInfo{
								Timestamp: rxInfo.Timestamp + 1000000,
								Frequency: rxInfo.Frequency,
								Power:     14,
								DataRate:  rxInfo.DataRate,
							},
							PHYPayload: lorawan.PHYPayload{
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
						},
					},
					ExpectedFCntUp:   10,
					ExpectedFCntDown: 6,
				},
				{
					Name:                         "unconfirmed uplink data + two downlink mac commands in queue (FRMPayload) + unconfirmed tx-payload in queue",
					RXInfo:                       rxInfo,
					EncryptFRMPayloadKey:         ns.AppSKey,
					DecryptExpectedFRMPayloadKey: ns.NwkSKey,
					SetMICKey:                    ns.NwkSKey,
					TXPayloadQueue: []models.TXPayload{
						{DevEUI: ns.DevEUI, FPort: 3, Data: []byte{4, 5, 6}},
					},
					TXMACPayloadQueue: []models.MACPayload{
						{DevEUI: ns.DevEUI, FRMPayload: true, MACCommand: []byte{6}},
						{DevEUI: ns.DevEUI, FRMPayload: true, MACCommand: []byte{8, 3}},
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

					ExpectedControllerRXInfoPayloads: []models.RXInfoPayload{
						{DevEUI: ns.DevEUI, FCnt: 10, RXInfo: []models.RXInfo{rxInfo}},
					},
					ExpectedApplicationRXPayloads: []models.RXPayload{
						{DevEUI: ns.DevEUI, FPort: 1, GatewayCount: 1, Data: []byte{1, 2, 3, 4}},
					},
					ExpectedGatewayTXPackets: []models.TXPacket{
						{
							TXInfo: models.TXInfo{
								Timestamp: rxInfo.Timestamp + 1000000,
								Frequency: rxInfo.Frequency,
								Power:     14,
								DataRate:  rxInfo.DataRate,
							},
							PHYPayload: lorawan.PHYPayload{
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
									FPort: &fPortZero,
									FRMPayload: []lorawan.Payload{
										&lorawan.MACCommand{CID: lorawan.CID(6)},
										&lorawan.MACCommand{CID: lorawan.CID(8), Payload: &lorawan.RXTimingSetupReqPayload{Delay: 3}},
									},
								},
							},
						},
					},
					ExpectedGetTXPayloadFromQueue: &models.TXPayload{DevEUI: ns.DevEUI, FPort: 3, Data: []byte{4, 5, 6}},
					ExpectedFCntUp:                10,
					ExpectedFCntDown:              6,
				},
				{
					Name:                 "unconfirmed uplink data + 18 bytes of MAC commands (FOpts) of which one is invalid",
					RXInfo:               rxInfo,
					EncryptFRMPayloadKey: ns.AppSKey,
					SetMICKey:            ns.NwkSKey,
					TXMACPayloadQueue: []models.MACPayload{
						{Reference: "a", DevEUI: ns.DevEUI, MACCommand: []byte{2, 10, 5}},
						{Reference: "b", DevEUI: ns.DevEUI, MACCommand: []byte{6}},
						{Reference: "c", DevEUI: ns.DevEUI, MACCommand: []byte{4, 15}},
						{Reference: "d", DevEUI: ns.DevEUI, MACCommand: []byte{4, 15, 16}}, // invalid payload, should be discarded + error notification
						{Reference: "e", DevEUI: ns.DevEUI, MACCommand: []byte{2, 10, 5}},
						{Reference: "f", DevEUI: ns.DevEUI, MACCommand: []byte{2, 10, 5}},
						{Reference: "g", DevEUI: ns.DevEUI, MACCommand: []byte{2, 10, 6}}, // this payload should stay in the queue
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

					ExpectedControllerRXInfoPayloads: []models.RXInfoPayload{
						{DevEUI: ns.DevEUI, FCnt: 10, RXInfo: []models.RXInfo{rxInfo}},
					},
					ExpectedControllerErrorPayloads: []models.ErrorPayload{
						{Reference: "d", DevEUI: ns.DevEUI, Message: "unmarshal mac command error: lorawan: 1 byte of data is expected"},
					},
					ExpectedApplicationRXPayloads: []models.RXPayload{
						{DevEUI: ns.DevEUI, FPort: 1, GatewayCount: 1, Data: []byte{1, 2, 3, 4}},
					},
					ExpectedGatewayTXPackets: []models.TXPacket{
						{
							TXInfo: models.TXInfo{
								Timestamp: rxInfo.Timestamp + 1000000,
								Frequency: rxInfo.Frequency,
								Power:     14,
								DataRate:  rxInfo.DataRate,
							},
							PHYPayload: lorawan.PHYPayload{
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
											{CID: lorawan.LinkCheckAns, Payload: &lorawan.LinkCheckAnsPayload{Margin: 10, GwCnt: 5}},
											{CID: lorawan.DevStatusReq},
											{CID: lorawan.DutyCycleReq, Payload: &lorawan.DutyCycleReqPayload{MaxDCCycle: 15}},
											{CID: lorawan.LinkCheckAns, Payload: &lorawan.LinkCheckAnsPayload{Margin: 10, GwCnt: 5}},
											{CID: lorawan.LinkCheckAns, Payload: &lorawan.LinkCheckAnsPayload{Margin: 10, GwCnt: 5}},
										},
									},
								},
							},
						},
					},
					ExpectedTXMACPayloadQueue: []models.MACPayload{
						{Reference: "g", DevEUI: ns.DevEUI, MACCommand: []byte{2, 10, 6}},
					},
					ExpectedFCntUp:   10,
					ExpectedFCntDown: 6,
				},
			}

			runDataUpTests(ctx, ns.DevEUI, ns.DevAddr, tests)
		})

		Convey("Given a set of test-scenarios for tx-payload queue", func() {
			var fPortTen uint8 = 10

			tests := []dataUpTestCase{
				{
					Name:                         "unconfirmed uplink data + one unconfirmed downlink payload in queue",
					RXInfo:                       rxInfo,
					EncryptFRMPayloadKey:         ns.AppSKey,
					DecryptExpectedFRMPayloadKey: ns.AppSKey,
					SetMICKey:                    ns.NwkSKey,
					TXPayloadQueue: []models.TXPayload{
						{Reference: "a", DevEUI: ns.DevEUI, FPort: 10, Data: []byte{1, 2, 3, 4}},
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

					ExpectedControllerRXInfoPayloads: []models.RXInfoPayload{
						{DevEUI: ns.DevEUI, FCnt: 10, RXInfo: []models.RXInfo{rxInfo}},
					},
					ExpectedApplicationRXPayloads: []models.RXPayload{
						{DevEUI: ns.DevEUI, FPort: 1, GatewayCount: 1},
					},
					ExpectedGatewayTXPackets: []models.TXPacket{
						{
							TXInfo: models.TXInfo{
								Timestamp: rxInfo.Timestamp + 1000000,
								Frequency: rxInfo.Frequency,
								Power:     14,
								DataRate:  rxInfo.DataRate,
							},
							PHYPayload: lorawan.PHYPayload{
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
						},
					},

					ExpectedFCntUp:   10,
					ExpectedFCntDown: 6,
				},
				{
					Name:                         "unconfirmed uplink data + two unconfirmed downlink payloads in queue",
					RXInfo:                       rxInfo,
					EncryptFRMPayloadKey:         ns.AppSKey,
					DecryptExpectedFRMPayloadKey: ns.AppSKey,
					SetMICKey:                    ns.NwkSKey,
					TXPayloadQueue: []models.TXPayload{
						{Reference: "a", DevEUI: ns.DevEUI, FPort: 10, Data: []byte{1, 2, 3, 4}},
						{Reference: "b", DevEUI: ns.DevEUI, FPort: 10, Data: []byte{4, 3, 2, 1}},
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

					ExpectedControllerRXInfoPayloads: []models.RXInfoPayload{
						{DevEUI: ns.DevEUI, FCnt: 10, RXInfo: []models.RXInfo{rxInfo}},
					},
					ExpectedApplicationRXPayloads: []models.RXPayload{
						{DevEUI: ns.DevEUI, FPort: 1, GatewayCount: 1},
					},
					ExpectedGatewayTXPackets: []models.TXPacket{
						{
							TXInfo: models.TXInfo{
								Timestamp: rxInfo.Timestamp + 1000000,
								Frequency: rxInfo.Frequency,
								Power:     14,
								DataRate:  rxInfo.DataRate,
							},
							PHYPayload: lorawan.PHYPayload{
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
						},
					},

					ExpectedFCntUp:                10,
					ExpectedFCntDown:              6,
					ExpectedGetTXPayloadFromQueue: &models.TXPayload{Reference: "b", DevEUI: ns.DevEUI, FPort: 10, Data: []byte{4, 3, 2, 1}},
				},
				{
					Name:                         "unconfirmed uplink data + one confirmed downlink payload in queue",
					RXInfo:                       rxInfo,
					EncryptFRMPayloadKey:         ns.AppSKey,
					DecryptExpectedFRMPayloadKey: ns.AppSKey,
					SetMICKey:                    ns.NwkSKey,
					TXPayloadQueue: []models.TXPayload{
						{Reference: "a", Confirmed: true, DevEUI: ns.DevEUI, FPort: 10, Data: []byte{1, 2, 3, 4}},
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

					ExpectedControllerRXInfoPayloads: []models.RXInfoPayload{
						{DevEUI: ns.DevEUI, FCnt: 10, RXInfo: []models.RXInfo{rxInfo}},
					},
					ExpectedApplicationRXPayloads: []models.RXPayload{
						{DevEUI: ns.DevEUI, FPort: 1, GatewayCount: 1},
					},
					ExpectedGatewayTXPackets: []models.TXPacket{
						{
							TXInfo: models.TXInfo{
								Timestamp: rxInfo.Timestamp + 1000000,
								Frequency: rxInfo.Frequency,
								Power:     14,
								DataRate:  rxInfo.DataRate,
							},
							PHYPayload: lorawan.PHYPayload{
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
						},
					},

					ExpectedFCntUp:                10,
					ExpectedFCntDown:              5, // will be incremented after the node ACKs the frame
					ExpectedGetTXPayloadFromQueue: &models.TXPayload{Reference: "a", Confirmed: true, DevEUI: ns.DevEUI, FPort: 10, Data: []byte{1, 2, 3, 4}},
				},
				{
					Name:                         "unconfirmed uplink data with ACK + one confirmed downlink payload in in-process queue",
					RXInfo:                       rxInfo,
					EncryptFRMPayloadKey:         ns.AppSKey,
					DecryptExpectedFRMPayloadKey: ns.AppSKey,
					SetMICKey:                    ns.NwkSKey,
					TXMACPayloadInProcess:        &models.TXPayload{Reference: "a", Confirmed: true, DevEUI: ns.DevEUI, FPort: 10, Data: []byte{1, 2, 3, 4}},

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
									ACK: true,
								},
							},
							FPort: &fPortOne,
						},
					},

					ExpectedControllerRXInfoPayloads: []models.RXInfoPayload{
						{DevEUI: ns.DevEUI, FCnt: 10, RXInfo: []models.RXInfo{rxInfo}},
					},
					ExpectedApplicationRXPayloads: []models.RXPayload{
						{DevEUI: ns.DevEUI, FPort: 1, GatewayCount: 1},
					},
					ExpectedApplicationNotifications: []interface{}{
						models.ACKNotification{Reference: "a", DevEUI: ns.DevEUI},
					},

					ExpectedFCntUp:   10,
					ExpectedFCntDown: 6, // has been updated because of the ACK
				},
				{
					Name:                         "unconfirmed uplink data + two unconfirmed downlink payload in queue of which the first exceeds the max payload size (for dr 0)",
					RXInfo:                       rxInfo,
					EncryptFRMPayloadKey:         ns.AppSKey,
					DecryptExpectedFRMPayloadKey: ns.AppSKey,
					SetMICKey:                    ns.NwkSKey,
					TXPayloadQueue: []models.TXPayload{
						{Reference: "a", DevEUI: ns.DevEUI, FPort: 10, Data: make([]byte, 52)},
						{Reference: "b", DevEUI: ns.DevEUI, FPort: 10, Data: []byte{1, 2, 3, 4}},
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

					ExpectedControllerRXInfoPayloads: []models.RXInfoPayload{
						{DevEUI: ns.DevEUI, FCnt: 10, RXInfo: []models.RXInfo{rxInfo}},
					},
					ExpectedApplicationRXPayloads: []models.RXPayload{
						{DevEUI: ns.DevEUI, FPort: 1, GatewayCount: 1},
					},
					ExpectedApplicationNotifications: []interface{}{
						models.ErrorPayload{Reference: "a", DevEUI: ns.DevEUI, Message: "downlink payload max size exceeded (dr: 0, allowed: 51, got: 52)"},
					},
					ExpectedGatewayTXPackets: []models.TXPacket{
						{
							TXInfo: models.TXInfo{
								Timestamp: rxInfo.Timestamp + 1000000,
								Frequency: rxInfo.Frequency,
								Power:     14,
								DataRate:  rxInfo.DataRate,
							},
							PHYPayload: lorawan.PHYPayload{
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
						},
					},

					ExpectedFCntUp:   10,
					ExpectedFCntDown: 6,
				},
				{
					Name:                         "unconfirmed uplink data + one unconfirmed downlink payload in queue (exactly max size for dr 0) + one mac command",
					RXInfo:                       rxInfo,
					EncryptFRMPayloadKey:         ns.AppSKey,
					DecryptExpectedFRMPayloadKey: ns.AppSKey,
					SetMICKey:                    ns.NwkSKey,
					TXPayloadQueue: []models.TXPayload{
						{Reference: "a", DevEUI: ns.DevEUI, FPort: 10, Data: make([]byte, 51)},
					},
					TXMACPayloadQueue: []models.MACPayload{
						{DevEUI: ns.DevEUI, MACCommand: []byte{6}},
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

					ExpectedControllerRXInfoPayloads: []models.RXInfoPayload{
						{DevEUI: ns.DevEUI, FCnt: 10, RXInfo: []models.RXInfo{rxInfo}},
					},
					ExpectedApplicationRXPayloads: []models.RXPayload{
						{DevEUI: ns.DevEUI, FPort: 1, GatewayCount: 1},
					},
					ExpectedGatewayTXPackets: []models.TXPacket{
						{
							TXInfo: models.TXInfo{
								Timestamp: rxInfo.Timestamp + 1000000,
								Frequency: rxInfo.Frequency,
								Power:     14,
								DataRate:  rxInfo.DataRate,
							},
							PHYPayload: lorawan.PHYPayload{
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
											{CID: lorawan.CID(6)},
										},
									},
								},
							},
						},
					},

					ExpectedFCntUp:                10,
					ExpectedFCntDown:              6,
					ExpectedGetTXPayloadFromQueue: &models.TXPayload{Reference: "a", DevEUI: ns.DevEUI, FPort: 10, Data: make([]byte, 51)},
				},
			}

			runDataUpTests(ctx, ns.DevEUI, ns.DevAddr, tests)
		})

	})

}

func TestHandleJoinRequestPackets(t *testing.T) {
	conf := getConfig()

	Convey("Given a dummy gateway and application backend and a clean Postgres and Redis database", t, func() {
		a := &testApplicationBackend{
			rxPayloadChan:           make(chan models.RXPayload, 1),
			notificationPayloadChan: make(chan interface{}, 10),
		}
		g := &testGatewayBackend{
			rxPacketChan: make(chan models.RXPacket, 1),
			txPacketChan: make(chan models.TXPacket, 1),
		}
		p := NewRedisPool(conf.RedisURL)
		mustFlushRedis(p)
		db, err := OpenDatabase(conf.PostgresDSN)
		So(err, ShouldBeNil)
		mustResetDB(db)

		ctx := Context{
			RedisPool:   p,
			Gateway:     g,
			Application: a,
			DB:          db,
		}

		Convey("Given a node and application in the database", func() {
			app := models.Application{
				AppEUI: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				Name:   "test app",
			}
			So(createApplication(ctx.DB, app), ShouldBeNil)

			node := models.Node{
				DevEUI: [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
				AppEUI: app.AppEUI,
				AppKey: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},

				RXDelay:     3,
				RX1DROffset: 2,
			}
			So(createNode(ctx.DB, node), ShouldBeNil)

			Convey("Given a JoinRequest packet", func() {
				phy := lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.JoinRequest,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.JoinRequestPayload{
						AppEUI:   app.AppEUI,
						DevEUI:   node.DevEUI,
						DevNonce: [2]byte{1, 2},
					},
				}
				So(phy.SetMIC(node.AppKey), ShouldBeNil)

				rxPacket := models.RXPacket{
					PHYPayload: phy,
					RXInfo: models.RXInfo{
						Frequency: Band.UplinkChannels[0].Frequency,
						DataRate:  Band.DataRates[Band.UplinkChannels[0].DataRates[0]],
					},
				}

				Convey("When calling handleRXPacket", func() {
					So(handleRXPacket(ctx, rxPacket), ShouldBeNil)

					Convey("Then a JoinAccept was sent to the node", func() {
						txPacket := <-g.txPacketChan
						phy := txPacket.PHYPayload
						So(phy.DecryptJoinAcceptPayload(node.AppKey), ShouldBeNil)
						So(phy.MHDR.MType, ShouldEqual, lorawan.JoinAccept)

						Convey("Then it was sent after 5s", func() {
							So(txPacket.TXInfo.Timestamp, ShouldEqual, rxPacket.RXInfo.Timestamp+uint32(5*time.Second/time.Microsecond))
						})

						Convey("Then the RXDelay is set to 3s", func() {
							jaPL := phy.MACPayload.(*lorawan.JoinAcceptPayload)
							So(jaPL.RXDelay, ShouldEqual, 3)
						})

						Convey("Then the DLSettings are set correctly", func() {
							jaPL := phy.MACPayload.(*lorawan.JoinAcceptPayload)
							So(jaPL.DLSettings.RX2DataRate, ShouldEqual, uint8(Band.RX2DataRate))
							So(jaPL.DLSettings.RX1DROffset, ShouldEqual, node.RX1DROffset)
						})

						Convey("Then a node-session was created", func() {
							jaPL := phy.MACPayload.(*lorawan.JoinAcceptPayload)

							_, err := getNodeSession(ctx.RedisPool, jaPL.DevAddr)
							So(err, ShouldBeNil)
						})

						Convey("Then the dev-nonce was added to the used dev-nonces", func() {
							node, err := getNode(ctx.DB, node.DevEUI)
							So(err, ShouldBeNil)
							So([2]byte{1, 2}, ShouldBeIn, node.UsedDevNonces)
						})

						Convey("Then a join notification was sent to the application", func() {
							notification := <-a.notificationPayloadChan
							join, ok := notification.(models.JoinNotification)
							So(ok, ShouldBeTrue)
							So(join.DevEUI, ShouldResemble, node.DevEUI)
						})
					})
				})
			})
		})
	})
}

func runDataUpTests(ctx Context, devEUI lorawan.EUI64, devAddr lorawan.DevAddr, tests []dataUpTestCase) {
	for i, test := range tests {
		Convey(fmt.Sprintf("When testing: %s [%d]", test.Name, i), func() {
			ctx.Application.(*testApplicationBackend).err = test.ApplicationBackendError

			for _, pl := range test.TXPayloadQueue {
				So(addTXPayloadToQueue(ctx.RedisPool, pl), ShouldBeNil)
			}

			for _, mac := range test.TXMACPayloadQueue {
				So(addMACPayloadToTXQueue(ctx.RedisPool, mac), ShouldBeNil)
			}

			if test.TXMACPayloadInProcess != nil {
				So(addTXPayloadToQueue(ctx.RedisPool, *test.TXMACPayloadInProcess), ShouldBeNil)
				_, err := getTXPayloadFromQueue(ctx.RedisPool, devEUI) // getting an item from the queue will put it into in-process
				So(err, ShouldBeNil)
			}

			So(test.PHYPayload.EncryptFRMPayload(test.EncryptFRMPayloadKey), ShouldBeNil)
			So(test.PHYPayload.SetMIC(test.SetMICKey), ShouldBeNil)

			rxPacket := models.RXPacket{
				PHYPayload: test.PHYPayload,
				RXInfo:     test.RXInfo,
			}

			So(handleRXPacket(ctx, rxPacket), ShouldResemble, test.ExpectedHandleRXPacketError)

			Convey("Then the expected rx-info payloads are sent to the network-controller", func() {
				So(ctx.Controller.(*testControllerBackend).rxInfoPayloadChan, ShouldHaveLength, len(test.ExpectedControllerRXInfoPayloads))
				for _, expPL := range test.ExpectedControllerRXInfoPayloads {
					pl := <-ctx.Controller.(*testControllerBackend).rxInfoPayloadChan
					So(pl, ShouldResemble, expPL)
				}
			})

			Convey("Then the expected error payloads are sent to the network-controller", func() {
				So(ctx.Controller.(*testControllerBackend).errorPayloadChan, ShouldHaveLength, len(test.ExpectedControllerErrorPayloads))
				for _, expPL := range test.ExpectedControllerErrorPayloads {
					pl := <-ctx.Controller.(*testControllerBackend).errorPayloadChan
					So(pl, ShouldResemble, expPL)
				}
			})

			Convey("Then the expected mac-commands are received by the network-controller", func() {
				So(ctx.Controller.(*testControllerBackend).rxMACPayloadChan, ShouldHaveLength, len(test.ExpectedControllerRXMACPayloads))
				for _, expPl := range test.ExpectedControllerRXMACPayloads {
					pl := <-ctx.Controller.(*testControllerBackend).rxMACPayloadChan
					So(pl, ShouldResemble, expPl)
				}
			})

			Convey("Then the expected rx-payloads are received by the application-backend", func() {
				So(ctx.Application.(*testApplicationBackend).rxPayloadChan, ShouldHaveLength, len(test.ExpectedApplicationRXPayloads))
				for _, expPL := range test.ExpectedApplicationRXPayloads {
					pl := <-ctx.Application.(*testApplicationBackend).rxPayloadChan
					So(pl, ShouldResemble, expPL)
				}
			})

			Convey("Then the expected notifications are received by the application-backend", func() {
				So(ctx.Application.(*testApplicationBackend).notificationPayloadChan, ShouldHaveLength, len(test.ExpectedApplicationNotifications))
				for _, expPL := range test.ExpectedApplicationNotifications {
					pl := <-ctx.Application.(*testApplicationBackend).notificationPayloadChan
					So(pl, ShouldResemble, expPL)
				}
			})

			Convey("Then the expected tx-packets are received by the gateway (FRMPayload decrypted and MIC ignored)", func() {
				So(ctx.Gateway.(*testGatewayBackend).txPacketChan, ShouldHaveLength, len(test.ExpectedGatewayTXPackets))
				for _, expPL := range test.ExpectedGatewayTXPackets {
					pl := <-ctx.Gateway.(*testGatewayBackend).txPacketChan
					So(pl.PHYPayload.DecryptFRMPayload(test.DecryptExpectedFRMPayloadKey), ShouldBeNil)
					expPL.PHYPayload.MIC = pl.PHYPayload.MIC
					So(pl, ShouldResemble, expPL)
				}
			})

			Convey("Then the frame-counters are as expected", func() {
				ns, err := getNodeSessionByDevEUI(ctx.RedisPool, devEUI)
				So(err, ShouldBeNil)
				So(ns.FCntDown, ShouldEqual, test.ExpectedFCntDown)
				So(ns.FCntUp, ShouldEqual, test.ExpectedFCntUp)
			})

			Convey("Then the MACPayload tx-queue is as expected", func() {
				macQueue, err := readMACPayloadTXQueue(ctx.RedisPool, devAddr)
				So(err, ShouldBeNil)
				So(macQueue, ShouldResemble, test.ExpectedTXMACPayloadQueue)
			})

			Convey("Then the next TXPayload is as expected", func() {
				txPL, err := getTXPayloadFromQueue(ctx.RedisPool, devEUI)
				if test.ExpectedGetTXPayloadFromQueue == nil {
					So(err, ShouldResemble, errEmptyQueue)
				} else {
					So(err, ShouldBeNil)
					So(txPL, ShouldResemble, *test.ExpectedGetTXPayloadFromQueue)
				}
			})
		})
	}
}
