package testsuite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gofrs/uuid"
	"github.com/golang/protobuf/ptypes"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/chirpstack-network-server/api/as"
	"github.com/brocaar/chirpstack-network-server/api/common"
	"github.com/brocaar/chirpstack-network-server/api/gw"
	"github.com/brocaar/chirpstack-network-server/api/nc"
	"github.com/brocaar/chirpstack-network-server/internal/band"
	"github.com/brocaar/chirpstack-network-server/internal/downlink"
	"github.com/brocaar/chirpstack-network-server/internal/helpers"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/chirpstack-network-server/internal/test"
	"github.com/brocaar/chirpstack-network-server/internal/uplink"
	"github.com/brocaar/lorawan"
)

func init() {
	if err := lorawan.RegisterProprietaryMACCommand(true, 0x80, 3); err != nil {
		panic(err)
	}

	if err := lorawan.RegisterProprietaryMACCommand(true, 0x81, 2); err != nil {
		panic(err)
	}
}

type ClassATestSuite struct {
	IntegrationTestSuite

	RXInfo gw.UplinkRXInfo
	TXInfo gw.UplinkTXInfo
}

func (ts *ClassATestSuite) SetupSuite() {
	ts.IntegrationTestSuite.SetupSuite()

	assert := require.New(ts.T())

	ts.CreateGateway(storage.Gateway{
		GatewayID: lorawan.EUI64{1, 2, 1, 2, 1, 2, 1, 2},
		Location: storage.GPSPoint{
			Latitude:  1,
			Longitude: 2,
		},
		Altitude: 3,
	})

	ts.CreateServiceProfile(storage.ServiceProfile{
		DRMax:         5,
		AddGWMetadata: true,
	})

	ts.CreateDevice(storage.Device{
		DevEUI: lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
	})

	ts.RXInfo = gw.UplinkRXInfo{
		GatewayId: ts.Gateway.GatewayID[:],
		LoraSnr:   7,
		Location: &common.Location{
			Latitude:  1,
			Longitude: 2,
			Altitude:  3,
		},
		Context: []byte{1, 2, 3, 4},
	}
	ts.RXInfo.Time = ptypes.TimestampNow()
	ts.RXInfo.TimeSinceGpsEpoch = ptypes.DurationProto(10 * time.Second)

	ts.TXInfo = gw.UplinkTXInfo{
		Frequency: 868100000,
	}
	assert.NoError(helpers.SetUplinkTXInfoDataRate(&ts.TXInfo, 0, band.Band()))
}

func (ts *ClassATestSuite) TestLW10Errors() {
	ts.CreateDeviceSession(storage.DeviceSession{
		MACVersion:            "1.0.2",
		JoinEUI:               lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
		DevAddr:               lorawan.DevAddr{1, 2, 3, 4},
		FNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		NwkSEncKey:            [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		FCntUp:                8,
		NFCntDown:             5,
		EnabledUplinkChannels: []int{0, 1, 2},
		RX2Frequency:          869525000,
	})

	var fPortOne uint8 = 1

	tests := []ClassATest{
		{
			Name:          "invalid frame-counter",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    7,
					},
					FPort: &fPortOne,
				},
				MIC: lorawan.MIC{48, 94, 26, 239},
			},
			ExpectedError: errors.New("get device-session error: device-session does not exist or invalid fcnt or mic"),
			Assert: []Assertion{
				AssertFCntUp(8),
				AssertNFCntDown(5),
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertClassATest(t, tst)
		})
	}
}

func (ts *ClassATestSuite) TestLW11Errors() {
	ts.CreateDeviceSession(storage.DeviceSession{
		MACVersion:            "1.1.0",
		JoinEUI:               lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
		DevAddr:               lorawan.DevAddr{1, 2, 3, 4},
		FNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		NwkSEncKey:            [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		FCntUp:                8,
		NFCntDown:             5,
		EnabledUplinkChannels: []int{0, 1, 2},
		RX2Frequency:          869525000,
	})

	var fPortOne uint8 = 1

	tests := []ClassATest{
		{
			Name: "the frequency is invalid (MIC)",
			BeforeFunc: func(tst *ClassATest) error {
				// the MIC is calculated for channel 0, we set it to channel 1
				tst.TXInfo.Frequency = 868300000
				return nil
			},
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
					FPort: &fPortOne,
				},
				MIC: lorawan.MIC{160, 195, 160, 195},
			},
			ExpectedError: errors.New("get device-session error: device-session does not exist or invalid fcnt or mic"),
		},
		{
			Name: "the data-rate is invalid (MIC)",
			BeforeFunc: func(tst *ClassATest) error {
				return helpers.SetUplinkTXInfoDataRate(&tst.TXInfo, 1, band.Band())
			},
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
					FPort: &fPortOne,
				},
				MIC: lorawan.MIC{160, 195, 160, 195},
			},
			ExpectedError: errors.New("get device-session error: device-session does not exist or invalid fcnt or mic"),
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertClassATest(t, tst)
		})
	}
}

func (ts *ClassATestSuite) TestLW10RelaxFrameCounter() {
	ts.CreateDeviceSession(storage.DeviceSession{
		MACVersion:            "1.0.2",
		JoinEUI:               lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
		DevAddr:               lorawan.DevAddr{1, 2, 3, 4},
		FNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		NwkSEncKey:            [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		FCntUp:                8,
		NFCntDown:             5,
		EnabledUplinkChannels: []int{0, 1, 2},
		RX2Frequency:          869525000,
		SkipFCntValidation:    true,
	})

	var fPortOne uint8 = 1

	tests := []ClassATest{
		{
			Name:          "the frame-counter is invalid but not 0",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    7,
					},
					FPort: &fPortOne,
				},
				MIC: lorawan.MIC{48, 94, 26, 239},
			},
			Assert: []Assertion{
				AssertFCntUp(8),
				AssertNFCntDown(5),
				AssertASHandleUplinkDataRequest(as.HandleUplinkDataRequest{
					DevEui:  ts.Device.DevEUI[:],
					JoinEui: ts.DeviceSession.JoinEUI[:],
					FCnt:    7,
					FPort:   1,
					Dr:      0,
					TxInfo:  &ts.TXInfo,
					RxInfo:  []*gw.UplinkRXInfo{&ts.RXInfo},
				}),
			},
		},
		{
			Name:          "the frame-counter is invalid and 0",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    0,
					},
					FPort: &fPortOne,
				},
				MIC: lorawan.MIC{131, 36, 83, 163},
			},
			Assert: []Assertion{
				AssertFCntUp(1),
				AssertNFCntDown(5),
				AssertASHandleUplinkDataRequest(as.HandleUplinkDataRequest{
					DevEui:  ts.Device.DevEUI[:],
					JoinEui: ts.DeviceSession.JoinEUI[:],
					FCnt:    0,
					FPort:   1,
					Dr:      0,
					TxInfo:  &ts.TXInfo,
					RxInfo:  []*gw.UplinkRXInfo{&ts.RXInfo},
				}),
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertClassATest(t, tst)
		})
	}
}

func (ts *ClassATestSuite) TestLW10Uplink() {
	ts.CreateDeviceSession(storage.DeviceSession{
		MACVersion:            "1.0.2",
		JoinEUI:               lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
		DevAddr:               lorawan.DevAddr{1, 2, 3, 4},
		FNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		NwkSEncKey:            [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		FCntUp:                8,
		NFCntDown:             5,
		EnabledUplinkChannels: []int{0, 1, 2},
		RX2Frequency:          869525000,
	})

	var fPortOne uint8 = 1
	inTenMinutes := time.Now().Add(10 * time.Minute)

	tests := []ClassATest{
		{
			Name:          "unconfirmed uplink with payload",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
					FPort:      &fPortOne,
					FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
				},
				MIC: lorawan.MIC{104, 147, 35, 121},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(5),
				AssertASHandleUplinkDataRequest(as.HandleUplinkDataRequest{
					DevEui:  ts.Device.DevEUI[:],
					JoinEui: ts.DeviceSession.JoinEUI[:],
					FCnt:    10,
					FPort:   1,
					Dr:      0,
					TxInfo:  &ts.TXInfo,
					RxInfo:  []*gw.UplinkRXInfo{&ts.RXInfo},
					Data:    []byte{1, 2, 3, 4},
				}),
				AssertNCHandleUplinkMetaDataRequest(nc.HandleUplinkMetaDataRequest{
					DevEui:                      ts.DeviceSession.DevEUI[:],
					TxInfo:                      &ts.TXInfo,
					RxInfo:                      []*gw.UplinkRXInfo{&ts.RXInfo},
					MessageType:                 nc.MType_UNCONFIRMED_DATA_UP,
					PhyPayloadByteCount:         17,
					ApplicationPayloadByteCount: 4,
				}),
			},
		},
		{
			Name: "unconfirmed uplink with payload + AppSKey envelope",
			BeforeFunc: func(tst *ClassATest) error {
				tst.DeviceSession.AppSKeyEvelope = &storage.KeyEnvelope{
					KEKLabel: "lora-app-server",
					AESKey:   []byte{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
				}
				return nil
			},
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
					FPort:      &fPortOne,
					FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
				},
				MIC: lorawan.MIC{104, 147, 35, 121},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(5),
				AssertASHandleUplinkDataRequest(as.HandleUplinkDataRequest{
					DevEui:  ts.Device.DevEUI[:],
					JoinEui: ts.DeviceSession.JoinEUI[:],
					FCnt:    10,
					FPort:   1,
					Dr:      0,
					TxInfo:  &ts.TXInfo,
					RxInfo:  []*gw.UplinkRXInfo{&ts.RXInfo},
					Data:    []byte{1, 2, 3, 4},
					DeviceActivationContext: &as.DeviceActivationContext{
						DevAddr: ts.DeviceSession.DevAddr[:],
						AppSKey: &common.KeyEnvelope{
							KekLabel: "lora-app-server",
							AesKey:   []byte{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
						},
					},
				}),
			},
		},
		{
			Name: "unconfirmed uplink with payload + ACK",
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.Device.DevEUI, FRMPayload: []byte{1}, FPort: 1, FCnt: 4, IsPending: true, TimeoutAfter: &inTenMinutes},
			},
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
						FCtrl: lorawan.FCtrl{
							ACK: true,
						},
					},
					FPort:      &fPortOne,
					FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
				},
				MIC: lorawan.MIC{132, 250, 228, 10},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(5),
				AssertASHandleUplinkDataRequest(as.HandleUplinkDataRequest{
					DevEui:  ts.Device.DevEUI[:],
					JoinEui: ts.DeviceSession.JoinEUI[:],
					FCnt:    10,
					FPort:   1,
					Dr:      0,
					TxInfo:  &ts.TXInfo,
					RxInfo:  []*gw.UplinkRXInfo{&ts.RXInfo},
					Data:    []byte{1, 2, 3, 4},
				}),
				AssertASHandleDownlinkACKRequest(as.HandleDownlinkACKRequest{
					DevEui:       ts.Device.DevEUI[:],
					FCnt:         4,
					Acknowledged: true,
				}),
			},
		},
		{
			Name:          "unconfirmed uplink without payload (just FPort)",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
					FPort: &fPortOne,
				},
				MIC: lorawan.MIC{160, 195, 68, 8},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(5),
				AssertASHandleUplinkDataRequest(as.HandleUplinkDataRequest{
					DevEui:  ts.Device.DevEUI[:],
					JoinEui: ts.DeviceSession.JoinEUI[:],
					FCnt:    10,
					FPort:   1,
					Dr:      0,
					TxInfo:  &ts.TXInfo,
					RxInfo:  []*gw.UplinkRXInfo{&ts.RXInfo},
				}),
			},
		},
		{
			Name:          "confirmed uplink with payload",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.ConfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
					FPort:      &fPortOne,
					FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
				},
				MIC: lorawan.MIC{69, 90, 200, 95},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(6),
				AssertASHandleUplinkDataRequest(as.HandleUplinkDataRequest{
					DevEui:  ts.Device.DevEUI[:],
					JoinEui: ts.DeviceSession.JoinEUI[:],
					FCnt:    10,
					FPort:   1,
					Dr:      0,
					TxInfo:  &ts.TXInfo,
					RxInfo:  []*gw.UplinkRXInfo{&ts.RXInfo},
					Data:    []byte{1, 2, 3, 4},
				}),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId:  ts.Gateway.GatewayID[:],
					Frequency:  868100000,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.DeviceSession.DevAddr,
							FCnt:    5,
							FCtrl: lorawan.FCtrl{
								ACK: true,
								ADR: true,
							},
						},
					},
					MIC: lorawan.MIC{0xa1, 0xb3, 0xda, 0x68},
				}),
			},
		},
		{
			Name:          "confirmed uplink without payload",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.ConfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
					FPort: &fPortOne,
				},
				MIC: lorawan.MIC{210, 52, 52, 94},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(6),
				AssertASHandleUplinkDataRequest(as.HandleUplinkDataRequest{
					DevEui:  ts.Device.DevEUI[:],
					JoinEui: ts.DeviceSession.JoinEUI[:],
					FCnt:    10,
					FPort:   1,
					Dr:      0,
					TxInfo:  &ts.TXInfo,
					RxInfo:  []*gw.UplinkRXInfo{&ts.RXInfo},
				}),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId:  ts.Gateway.GatewayID[:],
					Frequency:  868100000,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.DeviceSession.DevAddr,
							FCnt:    5,
							FCtrl: lorawan.FCtrl{
								ACK: true,
								ADR: true,
							},
						},
					},
					MIC: lorawan.MIC{0xa1, 0xb3, 0xda, 0x68},
				}),
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertClassATest(t, tst)
		})
	}
}

func (ts *ClassATestSuite) TestLW11Uplink() {
	ts.CreateDeviceSession(storage.DeviceSession{
		MACVersion:            "1.1.0",
		JoinEUI:               lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
		DevAddr:               lorawan.DevAddr{1, 2, 3, 4},
		FNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		NwkSEncKey:            [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		FCntUp:                8,
		NFCntDown:             5,
		EnabledUplinkChannels: []int{0, 1, 2},
		RX2Frequency:          869525000,
	})

	var fPortOne uint8 = 1
	inTenMinutes := time.Now().Add(10 * time.Minute)

	tests := []ClassATest{
		{
			Name:          "unconfirmed uplink with payload",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
					FPort:      &fPortOne,
					FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
				},
				MIC: lorawan.MIC{104, 147, 104, 147},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(5),
				AssertASHandleUplinkDataRequest(as.HandleUplinkDataRequest{
					DevEui:  ts.Device.DevEUI[:],
					JoinEui: ts.DeviceSession.JoinEUI[:],
					FCnt:    10,
					FPort:   1,
					Dr:      0,
					TxInfo:  &ts.TXInfo,
					RxInfo:  []*gw.UplinkRXInfo{&ts.RXInfo},
					Data:    []byte{1, 2, 3, 4},
				}),
			},
		},
		{
			Name: "unconfirmed uplink with payload + ACK",
			BeforeFunc: func(tst *ClassATest) error {
				tst.DeviceSession.ConfFCnt = 4
				return nil
			},
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.Device.DevEUI, FRMPayload: []byte{1}, FPort: 1, FCnt: 4, IsPending: true, TimeoutAfter: &inTenMinutes},
			},
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
						FCtrl: lorawan.FCtrl{
							ACK: true,
						},
					},
					FPort:      &fPortOne,
					FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
				},
				MIC: lorawan.MIC{76, 46, 132, 250},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(5),
				AssertASHandleUplinkDataRequest(as.HandleUplinkDataRequest{
					DevEui:  ts.Device.DevEUI[:],
					JoinEui: ts.DeviceSession.JoinEUI[:],
					FCnt:    10,
					FPort:   1,
					Dr:      0,
					TxInfo:  &ts.TXInfo,
					RxInfo:  []*gw.UplinkRXInfo{&ts.RXInfo},
					Data:    []byte{1, 2, 3, 4},
				}),
				AssertASHandleDownlinkACKRequest(as.HandleDownlinkACKRequest{
					DevEui:       ts.Device.DevEUI[:],
					FCnt:         4,
					Acknowledged: true,
				}),
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertClassATest(t, tst)
		})
	}
}

func (ts *ClassATestSuite) TestLW10RXDelay() {
	assert := require.New(ts.T())

	ts.CreateDeviceSession(storage.DeviceSession{
		MACVersion:            "1.0.2",
		JoinEUI:               lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
		DevAddr:               lorawan.DevAddr{1, 2, 3, 4},
		FNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		NwkSEncKey:            [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		FCntUp:                8,
		NFCntDown:             5,
		EnabledUplinkChannels: []int{0, 1, 2},
		RX2Frequency:          869525000,
		RXDelay:               3,
	})

	conf := test.GetConfig()
	conf.NetworkServer.NetworkSettings.RX1Delay = 3
	assert.NoError(uplink.Setup(conf))
	assert.NoError(downlink.Setup(conf))

	var fPortOne uint8 = 1

	tests := []ClassATest{
		{
			Name:          "confirmed uplink without payload (rxdelay = 3)",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.ConfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
					FPort: &fPortOne,
				},
				MIC: lorawan.MIC{210, 52, 52, 94},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(6),
				AssertASHandleUplinkDataRequest(as.HandleUplinkDataRequest{
					DevEui:  ts.Device.DevEUI[:],
					JoinEui: ts.DeviceSession.JoinEUI[:],
					FCnt:    10,
					FPort:   1,
					Dr:      0,
					TxInfo:  &ts.TXInfo,
					RxInfo:  []*gw.UplinkRXInfo{&ts.RXInfo},
				}),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId:  ts.Gateway.GatewayID[:],
					Frequency:  868100000,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second * 3),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.DeviceSession.DevAddr,
							FCnt:    5,
							FCtrl: lorawan.FCtrl{
								ACK: true,
								ADR: true,
							},
						},
					},
					MIC: lorawan.MIC{0xa1, 0xb3, 0xda, 0x68},
				}),
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertClassATest(t, tst)
		})
	}
}

func (ts *ClassATestSuite) TestLW10MACCommands() {
	ts.CreateDeviceSession(storage.DeviceSession{
		MACVersion:            "1.0.2",
		JoinEUI:               lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
		DevAddr:               lorawan.DevAddr{1, 2, 3, 4},
		FNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		NwkSEncKey:            [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		FCntUp:                8,
		NFCntDown:             5,
		EnabledUplinkChannels: []int{0, 1, 2},
		RX2Frequency:          869525000,
	})

	fPortZero := uint8(0)
	fPortThree := uint8(3)

	tests := []ClassATest{
		{
			Name:          "two uplink mac-commands (FOpts)",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
						FOpts: []lorawan.Payload{
							&lorawan.MACCommand{CID: 0x80, Payload: &lorawan.ProprietaryMACCommandPayload{Bytes: []byte{1, 2, 3}}},
							&lorawan.MACCommand{CID: 0x81, Payload: &lorawan.ProprietaryMACCommandPayload{Bytes: []byte{4, 5}}},
						},
					},
				},
				MIC: lorawan.MIC{218, 0, 109, 32},
			},
			Assert: []Assertion{
				AssertNCHandleUplinkMACCommandRequest(nc.HandleUplinkMACCommandRequest{
					DevEui:   ts.Device.DevEUI[:],
					Cid:      128,
					Commands: [][]byte{{128, 1, 2, 3}},
				}),
				AssertNCHandleUplinkMACCommandRequest(nc.HandleUplinkMACCommandRequest{
					DevEui:   ts.Device.DevEUI[:],
					Cid:      129,
					Commands: [][]byte{{129, 4, 5}},
				}),
				AssertNCHandleUplinkMetaDataRequest(nc.HandleUplinkMetaDataRequest{
					DevEui:                      ts.DeviceSession.DevEUI[:],
					TxInfo:                      &ts.TXInfo,
					RxInfo:                      []*gw.UplinkRXInfo{&ts.RXInfo},
					MessageType:                 nc.MType_UNCONFIRMED_DATA_UP,
					PhyPayloadByteCount:         19,
					MacCommandByteCount:         7,
					ApplicationPayloadByteCount: 0,
				}),
				AssertFCntUp(11),
				AssertNFCntDown(5),
			},
		},
		{
			Name: "two uplink mac-commands (FRMPayload)",
			BeforeFunc: func(ts *ClassATest) error {
				if err := ts.PHYPayload.EncryptFRMPayload(ts.DeviceSession.NwkSEncKey); err != nil {
					return err
				}
				if err := ts.PHYPayload.SetUplinkDataMIC(lorawan.LoRaWAN1_0, 0, 0, 0, ts.DeviceSession.FNwkSIntKey, ts.DeviceSession.SNwkSIntKey); err != nil {
					return err
				}
				return nil
			},
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
					FPort: &fPortZero,
					FRMPayload: []lorawan.Payload{
						&lorawan.MACCommand{CID: 0x80, Payload: &lorawan.ProprietaryMACCommandPayload{Bytes: []byte{1, 2, 3}}},
						&lorawan.MACCommand{CID: 0x81, Payload: &lorawan.ProprietaryMACCommandPayload{Bytes: []byte{4, 5}}},
					},
				},
			},
			Assert: []Assertion{
				AssertNCHandleUplinkMACCommandRequest(nc.HandleUplinkMACCommandRequest{
					DevEui:   ts.Device.DevEUI[:],
					Cid:      128,
					Commands: [][]byte{{128, 1, 2, 3}},
				}),
				AssertNCHandleUplinkMACCommandRequest(nc.HandleUplinkMACCommandRequest{
					DevEui:   ts.Device.DevEUI[:],
					Cid:      129,
					Commands: [][]byte{{129, 4, 5}},
				}),
				AssertNCHandleUplinkMetaDataRequest(nc.HandleUplinkMetaDataRequest{
					DevEui:                      ts.DeviceSession.DevEUI[:],
					TxInfo:                      &ts.TXInfo,
					RxInfo:                      []*gw.UplinkRXInfo{&ts.RXInfo},
					MessageType:                 nc.MType_UNCONFIRMED_DATA_UP,
					PhyPayloadByteCount:         20,
					MacCommandByteCount:         7,
					ApplicationPayloadByteCount: 0,
				}),
				AssertFCntUp(11),
				AssertNFCntDown(5),
			},
		},
		{
			Name: "unconfirmed uplink + dev-status request downlink (FOpts)",
			BeforeFunc: func(tst *ClassATest) error {
				if err := tst.PHYPayload.SetUplinkDataMIC(lorawan.LoRaWAN1_0, 0, 0, 0, tst.DeviceSession.FNwkSIntKey, tst.DeviceSession.SNwkSIntKey); err != nil {
					return err
				}

				ts.ServiceProfile.DevStatusReqFreq = 1
				return storage.UpdateServiceProfile(context.Background(), storage.DB(), ts.ServiceProfile)
			},
			AfterFunc: func(tst *ClassATest) error {
				ts.ServiceProfile.DevStatusReqFreq = 0
				return storage.UpdateServiceProfile(context.Background(), storage.DB(), ts.ServiceProfile)
			},
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
				},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(6),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId:  ts.RXInfo.GatewayId,
					Frequency:  ts.TXInfo.Frequency,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							FCtrl: lorawan.FCtrl{
								ADR: true,
							},
							DevAddr: ts.DeviceSession.DevAddr,
							FCnt:    5,
							FOpts: []lorawan.Payload{
								&lorawan.MACCommand{CID: lorawan.CID(6)},
							},
						},
					},
					MIC: lorawan.MIC{0xfa, 0xf0, 0x96, 0xdb},
				}),
			},
		},
		{
			Name: "unconfirmed uplink + dev-status request downlink (FOpts) + unconfirmed data down",
			BeforeFunc: func(tst *ClassATest) error {
				if err := tst.PHYPayload.SetUplinkDataMIC(lorawan.LoRaWAN1_0, 0, 0, 0, tst.DeviceSession.FNwkSIntKey, tst.DeviceSession.SNwkSIntKey); err != nil {
					return err
				}

				ts.ServiceProfile.DevStatusReqFreq = 1
				return storage.UpdateServiceProfile(context.Background(), storage.DB(), ts.ServiceProfile)
			},
			AfterFunc: func(tst *ClassATest) error {
				ts.ServiceProfile.DevStatusReqFreq = 0
				return storage.UpdateServiceProfile(context.Background(), storage.DB(), ts.ServiceProfile)
			},
			DeviceSession: *ts.DeviceSession,
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.Device.DevEUI, FPort: 3, FCnt: 5, FRMPayload: []byte{4, 5, 6}},
			},
			TXInfo: ts.TXInfo,
			RXInfo: ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
				},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(6),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId:  ts.RXInfo.GatewayId,
					Frequency:  ts.TXInfo.Frequency,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							FCtrl: lorawan.FCtrl{
								ADR: true,
							},
							DevAddr: ts.DeviceSession.DevAddr,
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
					MIC: lorawan.MIC{0xc4, 0x28, 0xa9, 0x32},
				}),
			},
		},
		{
			Name:          "RXTimingSetupAns is answered with an empty downlink",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
						FOpts: []lorawan.Payload{
							&lorawan.MACCommand{CID: lorawan.RXTimingSetupAns},
						},
					},
				},
				MIC: lorawan.MIC{0xb6, 0x20, 0xd2, 0x14},
			},
			Assert: []Assertion{
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId:  ts.RXInfo.GatewayId,
					Frequency:  ts.TXInfo.Frequency,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							FCtrl: lorawan.FCtrl{
								ADR: true,
							},
							DevAddr: ts.DeviceSession.DevAddr,
							FCnt:    5,
						},
					},
					MIC: lorawan.MIC{0xc1, 0x0a, 0x08, 0xd9},
				}),
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertClassATest(t, tst)
		})
	}
}

func (ts *ClassATestSuite) TestLW10MACCommandsDisabled() {
	assert := require.New(ts.T())

	ts.CreateDeviceSession(storage.DeviceSession{
		MACVersion:            "1.0.2",
		JoinEUI:               lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
		DevAddr:               lorawan.DevAddr{1, 2, 3, 4},
		FNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		NwkSEncKey:            [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		FCntUp:                8,
		NFCntDown:             5,
		EnabledUplinkChannels: []int{0, 1, 2},
		RX2Frequency:          869525000,
	})

	fPortZero := uint8(0)

	conf := test.GetConfig()
	conf.NetworkServer.NetworkSettings.DisableMACCommands = true
	assert.NoError(uplink.Setup(conf))
	assert.NoError(downlink.Setup(conf))

	tests := []ClassATest{
		{
			Name:          "uplink with link-check request (FOpts)",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
						FOpts: []lorawan.Payload{
							&lorawan.MACCommand{CID: lorawan.LinkCheckReq},
						},
					},
				},
				MIC: lorawan.MIC{0x6a, 0x0e, 0x7c, 0xd4},
			},
			Assert: []Assertion{
				AssertNCHandleUplinkMACCommandRequest(nc.HandleUplinkMACCommandRequest{
					DevEui:   ts.Device.DevEUI[:],
					Cid:      uint32(lorawan.LinkCheckReq),
					Commands: [][]byte{{byte(lorawan.LinkCheckReq)}},
				}),
				AssertFCntUp(11),
				AssertNFCntDown(5),
			},
		},
		{
			Name: "uplink with link-check request (FRMPayload)",
			BeforeFunc: func(ts *ClassATest) error {
				if err := ts.PHYPayload.EncryptFRMPayload(ts.DeviceSession.NwkSEncKey); err != nil {
					return err
				}
				if err := ts.PHYPayload.SetUplinkDataMIC(lorawan.LoRaWAN1_0, 0, 0, 0, ts.DeviceSession.FNwkSIntKey, ts.DeviceSession.SNwkSIntKey); err != nil {
					return err
				}
				return nil
			},
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
					FPort: &fPortZero,
					FRMPayload: []lorawan.Payload{
						&lorawan.MACCommand{CID: lorawan.LinkCheckReq},
					},
				},
			},
			Assert: []Assertion{
				AssertNCHandleUplinkMACCommandRequest(nc.HandleUplinkMACCommandRequest{
					DevEui:   ts.Device.DevEUI[:],
					Cid:      uint32(lorawan.LinkCheckReq),
					Commands: [][]byte{{byte(lorawan.LinkCheckReq)}},
				}),
				AssertFCntUp(11),
				AssertNFCntDown(5),
			},
		},
		{
			Name:          "uplink with downlink mac-command response (external)",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			MACCommandQueueItems: []storage.MACCommandBlock{
				{
					CID:      lorawan.LinkCheckAns,
					External: true,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.LinkCheckAns,
							Payload: &lorawan.LinkCheckAnsPayload{
								Margin: 10,
								GwCnt:  2,
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
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
				},
				MIC: lorawan.MIC{0x7a, 0x98, 0x98, 0xdc},
			},
			Assert: []Assertion{
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId:  ts.RXInfo.GatewayId,
					Frequency:  ts.TXInfo.Frequency,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.DeviceSession.DevAddr,
							FCnt:    5,
							FCtrl: lorawan.FCtrl{
								ADR: true,
							},
							FOpts: []lorawan.Payload{
								&lorawan.MACCommand{
									CID: lorawan.LinkCheckAns,
									Payload: &lorawan.LinkCheckAnsPayload{
										Margin: 10,
										GwCnt:  2,
									},
								},
							},
						},
					},
					MIC: lorawan.MIC{0xde, 0xb, 0x5d, 0x4e},
				}),
				AssertFCntUp(11),
				AssertNFCntDown(6),
			},
		},
		{
			Name:          "uplink with discarded downlink mac-command response (internal)",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			MACCommandQueueItems: []storage.MACCommandBlock{
				{
					CID:      lorawan.LinkCheckAns,
					External: false,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.LinkCheckAns,
							Payload: &lorawan.LinkCheckAnsPayload{
								Margin: 10,
								GwCnt:  2,
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
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
				},
				MIC: lorawan.MIC{0x7a, 0x98, 0x98, 0xdc},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(5),
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertClassATest(t, tst)
		})
	}
}

func (ts *ClassATestSuite) TestLW11MACCommands() {
	ts.CreateDeviceSession(storage.DeviceSession{
		MACVersion:            "1.1.0",
		JoinEUI:               lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
		DevAddr:               lorawan.DevAddr{1, 2, 3, 4},
		FNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		NwkSEncKey:            [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		FCntUp:                8,
		NFCntDown:             5,
		EnabledUplinkChannels: []int{0, 1, 2},
		RX2Frequency:          869525000,
	})

	tests := []ClassATest{
		{
			Name: "two uplink mac-commands (FOpts)",
			BeforeFunc: func(tst *ClassATest) error {
				// encrypt the FOpts and set MIC afterwards
				if err := tst.PHYPayload.EncryptFOpts(tst.DeviceSession.NwkSEncKey); err != nil {
					return err
				}
				if err := tst.PHYPayload.SetUplinkDataMIC(lorawan.LoRaWAN1_1, 0, 0, 0, ts.DeviceSession.FNwkSIntKey, ts.DeviceSession.SNwkSIntKey); err != nil {
					return err
				}
				return nil
			},
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
						FOpts: []lorawan.Payload{
							&lorawan.MACCommand{CID: 0x80, Payload: &lorawan.ProprietaryMACCommandPayload{Bytes: []byte{1, 2, 3}}},
							&lorawan.MACCommand{CID: 0x81, Payload: &lorawan.ProprietaryMACCommandPayload{Bytes: []byte{4, 5}}},
						},
					},
				},
			},
			Assert: []Assertion{
				AssertNCHandleUplinkMACCommandRequest(nc.HandleUplinkMACCommandRequest{
					DevEui:   ts.Device.DevEUI[:],
					Cid:      128,
					Commands: [][]byte{{128, 1, 2, 3}},
				}),
				AssertNCHandleUplinkMACCommandRequest(nc.HandleUplinkMACCommandRequest{
					DevEui:   ts.Device.DevEUI[:],
					Cid:      129,
					Commands: [][]byte{{129, 4, 5}},
				}),
				AssertFCntUp(11),
				AssertNFCntDown(5),
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertClassATest(t, tst)
		})
	}
}

func (ts *ClassATestSuite) TestLW10AddGWMetadata() {
	assert := require.New(ts.T())

	ts.CreateDeviceSession(storage.DeviceSession{
		MACVersion:            "1.0.2",
		JoinEUI:               lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
		DevAddr:               lorawan.DevAddr{1, 2, 3, 4},
		FNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		NwkSEncKey:            [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		FCntUp:                8,
		NFCntDown:             5,
		EnabledUplinkChannels: []int{0, 1, 2},
		RX2Frequency:          869525000,
	})

	ts.ServiceProfile.AddGWMetadata = false
	assert.NoError(storage.UpdateServiceProfile(context.Background(), storage.DB(), ts.ServiceProfile))

	fPortOne := uint8(1)

	tests := []ClassATest{
		{
			Name:          "unconfirmed uplink with payload (service-profile: no gw meta-data)",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
					FPort:      &fPortOne,
					FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
				},
				MIC: lorawan.MIC{104, 147, 35, 121},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(5),
				AssertASHandleUplinkDataRequest(as.HandleUplinkDataRequest{
					DevEui:  ts.Device.DevEUI[:],
					JoinEui: ts.DeviceSession.JoinEUI[:],
					FCnt:    10,
					FPort:   1,
					Dr:      0,
					TxInfo:  &ts.TXInfo,
					RxInfo:  []*gw.UplinkRXInfo{},
					Data:    []byte{1, 2, 3, 4},
				}),
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertClassATest(t, tst)
		})
	}

	ts.ServiceProfile.AddGWMetadata = true
	assert.NoError(storage.UpdateServiceProfile(context.Background(), storage.DB(), ts.ServiceProfile))
}

func (ts *ClassATestSuite) TestLW11DeviceQueue() {
	ts.CreateDeviceSession(storage.DeviceSession{
		MACVersion:            "1.1.0",
		JoinEUI:               lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
		DevAddr:               lorawan.DevAddr{1, 2, 3, 4},
		FNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		NwkSEncKey:            [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		FCntUp:                8,
		NFCntDown:             5,
		AFCntDown:             3,
		EnabledUplinkChannels: []int{0, 1, 2},
		RX2Frequency:          869525000,
	})

	fPortOne := uint8(1)
	fPortTen := uint8(10)

	tests := []ClassATest{
		{
			Name:          "unconfirmed uplink + one unconfirmed downlink payload in queue",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.Device.DevEUI, FPort: 10, FCnt: 3, FRMPayload: []byte{1, 2, 3, 4}},
			},
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
					FPort: &fPortOne,
				},
				MIC: lorawan.MIC{160, 195, 160, 195},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertAFCntDown(4),
				AssertASHandleUplinkDataRequest(as.HandleUplinkDataRequest{
					DevEui:  ts.Device.DevEUI[:],
					JoinEui: ts.DeviceSession.JoinEUI[:],
					FCnt:    10,
					FPort:   1,
					Dr:      0,
					TxInfo:  &ts.TXInfo,
					RxInfo:  []*gw.UplinkRXInfo{&ts.RXInfo},
				}),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId:  ts.RXInfo.GatewayId,
					Frequency:  ts.TXInfo.Frequency,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.DeviceSession.DevAddr,
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
					MIC: lorawan.MIC{0x35, 0xc9, 0x57, 0x5},
				}),
			},
		},
		{
			Name:          "unconfirmed uplink + one confirmed downlink payload in queue",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.Device.DevEUI, FPort: 10, FCnt: 3, FRMPayload: []byte{1, 2, 3, 4}, Confirmed: true},
			},
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
					FPort: &fPortOne,
				},
				MIC: lorawan.MIC{160, 195, 160, 195},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertAFCntDown(4),
				AssertASHandleUplinkDataRequest(as.HandleUplinkDataRequest{
					DevEui:  ts.Device.DevEUI[:],
					JoinEui: ts.DeviceSession.JoinEUI[:],
					FCnt:    10,
					FPort:   1,
					Dr:      0,
					TxInfo:  &ts.TXInfo,
					RxInfo:  []*gw.UplinkRXInfo{&ts.RXInfo},
				}),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId:  ts.RXInfo.GatewayId,
					Frequency:  ts.TXInfo.Frequency,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.ConfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.DeviceSession.DevAddr,
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
					MIC: lorawan.MIC{0xa7, 0xbe, 0xb3, 0x3d},
				}),
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertClassATest(t, tst)
		})
	}
}

func (ts *ClassATestSuite) TestLW10DeviceQueue() {
	ts.CreateDeviceSession(storage.DeviceSession{
		MACVersion:            "1.0.2",
		JoinEUI:               lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
		DevAddr:               lorawan.DevAddr{1, 2, 3, 4},
		FNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		NwkSEncKey:            [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		FCntUp:                8,
		NFCntDown:             5,
		EnabledUplinkChannels: []int{0, 1, 2},
		RX2Frequency:          869525000,
	})

	fPortOne := uint8(1)
	fPortTen := uint8(10)

	tests := []ClassATest{
		{
			Name:          "unconfirmed uplink + one unconfirmed downlink payload in queue",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.Device.DevEUI, FPort: 10, FCnt: 5, FRMPayload: []byte{1, 2, 3, 4}},
			},
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
					FPort: &fPortOne,
				},
				MIC: lorawan.MIC{160, 195, 68, 8},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(6),
				AssertASHandleUplinkDataRequest(as.HandleUplinkDataRequest{
					DevEui:  ts.Device.DevEUI[:],
					JoinEui: ts.DeviceSession.JoinEUI[:],
					FCnt:    10,
					FPort:   1,
					Dr:      0,
					TxInfo:  &ts.TXInfo,
					RxInfo:  []*gw.UplinkRXInfo{&ts.RXInfo},
				}),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId:  ts.RXInfo.GatewayId,
					Frequency:  ts.TXInfo.Frequency,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.DeviceSession.DevAddr,
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
					MIC: lorawan.MIC{0x16, 0x3c, 0xe3, 0xe6},
				}),
			},
		},
		{
			Name:          "unconfirmed uplink + two unconfirmed downlink payloads in queue",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.Device.DevEUI, FPort: 10, FCnt: 5, FRMPayload: []byte{1, 2, 3, 4}},
				{DevEUI: ts.Device.DevEUI, FPort: 10, FCnt: 6, FRMPayload: []byte{1, 2, 3, 4}},
			},
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
					FPort: &fPortOne,
				},
				MIC: lorawan.MIC{160, 195, 68, 8},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(6),
				AssertASHandleUplinkDataRequest(as.HandleUplinkDataRequest{
					DevEui:  ts.Device.DevEUI[:],
					JoinEui: ts.DeviceSession.JoinEUI[:],
					FCnt:    10,
					FPort:   1,
					Dr:      0,
					TxInfo:  &ts.TXInfo,
					RxInfo:  []*gw.UplinkRXInfo{&ts.RXInfo},
				}),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId:  ts.RXInfo.GatewayId,
					Frequency:  ts.TXInfo.Frequency,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.DeviceSession.DevAddr,
							FCnt:    5,
							FCtrl: lorawan.FCtrl{
								ADR:      true,
								FPending: true,
							},
						},
						FPort: &fPortTen,
						FRMPayload: []lorawan.Payload{
							&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}},
						},
					},
					MIC: lorawan.MIC{0x82, 0xf3, 0x14, 0xa6},
				}),
			},
		},
		{
			Name:          "unconfirmed uplink + one confirmed downlink payload in queue",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.Device.DevEUI, FPort: 10, FCnt: 5, FRMPayload: []byte{1, 2, 3, 4}, Confirmed: true},
			},
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
					FPort: &fPortOne,
				},
				MIC: lorawan.MIC{160, 195, 68, 8},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(6),
				AssertASHandleUplinkDataRequest(as.HandleUplinkDataRequest{
					DevEui:  ts.Device.DevEUI[:],
					JoinEui: ts.DeviceSession.JoinEUI[:],
					FCnt:    10,
					FPort:   1,
					Dr:      0,
					TxInfo:  &ts.TXInfo,
					RxInfo:  []*gw.UplinkRXInfo{&ts.RXInfo},
				}),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId:  ts.RXInfo.GatewayId,
					Frequency:  ts.TXInfo.Frequency,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.ConfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.DeviceSession.DevAddr,
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
					MIC: lorawan.MIC{0xff, 0x75, 0xc8, 0x4},
				}),
			},
		},
		{
			Name:          "unconfirmed uplink data + downlink payload which exceeds the max payload size (for dr 0)",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.Device.DevEUI, FPort: 10, FCnt: 5, FRMPayload: make([]byte, 52)},
			},
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
					FPort: &fPortOne,
				},
				MIC: lorawan.MIC{160, 195, 68, 8},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(5),
				AssertASHandleUplinkDataRequest(as.HandleUplinkDataRequest{
					DevEui:  ts.Device.DevEUI[:],
					JoinEui: ts.DeviceSession.JoinEUI[:],
					FCnt:    10,
					FPort:   1,
					Dr:      0,
					TxInfo:  &ts.TXInfo,
					RxInfo:  []*gw.UplinkRXInfo{&ts.RXInfo},
				}),
				AssertASHandleErrorRequest(as.HandleErrorRequest{
					DevEui: ts.Device.DevEUI[:],
					Type:   as.ErrorType_DEVICE_QUEUE_ITEM_SIZE,
					Error:  "payload exceeds max payload size", FCnt: 5,
				}),
				AssertNoDownlinkFrame,
			},
		},
		{
			Name: "unconfirmed uplink data + one unconfirmed downlink payload in queue (exactly max size for dr 0) + one mac command",
			BeforeFunc: func(tst *ClassATest) error {
				ts.ServiceProfile.DevStatusReqFreq = 1
				return storage.UpdateServiceProfile(context.Background(), storage.DB(), ts.ServiceProfile)
			},
			AfterFunc: func(tst *ClassATest) error {
				ts.ServiceProfile.DevStatusReqFreq = 0
				return storage.UpdateServiceProfile(context.Background(), storage.DB(), ts.ServiceProfile)
			},
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.Device.DevEUI, FPort: 10, FCnt: 5, FRMPayload: make([]byte, 51)},
			},
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
					FPort: &fPortOne,
				},
				MIC: lorawan.MIC{160, 195, 68, 8},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(6),
				AssertASHandleUplinkDataRequest(as.HandleUplinkDataRequest{
					DevEui:  ts.Device.DevEUI[:],
					JoinEui: ts.DeviceSession.JoinEUI[:],
					FCnt:    10,
					FPort:   1,
					Dr:      0,
					TxInfo:  &ts.TXInfo,
					RxInfo:  []*gw.UplinkRXInfo{&ts.RXInfo},
				}),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId:  ts.RXInfo.GatewayId,
					Frequency:  ts.TXInfo.Frequency,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.DeviceSession.DevAddr,
							FCnt:    5,
							FCtrl: lorawan.FCtrl{
								ADR:      true,
								FPending: true,
							},
						},
						FPort: &fPortTen,
						FRMPayload: []lorawan.Payload{
							&lorawan.DataPayload{Bytes: make([]byte, 51)},
						},
					},
					MIC: lorawan.MIC{0xd1, 0x2f, 0x7c, 0xa8},
				}),
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertClassATest(t, tst)
		})
	}
}

func (ts *ClassATestSuite) TestLW10ADR() {
	ts.CreateDeviceSession(storage.DeviceSession{
		MACVersion:            "1.0.2",
		JoinEUI:               lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
		DevAddr:               lorawan.DevAddr{1, 2, 3, 4},
		FNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		NwkSEncKey:            [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		FCntUp:                10,
		NFCntDown:             5,
		EnabledUplinkChannels: []int{0, 1, 2},
		RX2Frequency:          869525000,
	})

	tests := []ClassATest{
		{
			Name:          "adr triggered",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
						FCtrl: lorawan.FCtrl{
							ADR: true,
						},
					},
				},
				MIC: lorawan.MIC{187, 243, 244, 117},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(6),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId:  ts.RXInfo.GatewayId,
					Frequency:  ts.TXInfo.Frequency,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.DeviceSession.DevAddr,
							FCnt:    5,
							FCtrl: lorawan.FCtrl{
								ADR: true,
							},
							FOpts: []lorawan.Payload{
								&lorawan.MACCommand{
									CID: lorawan.LinkADRReq,
									Payload: &lorawan.LinkADRReqPayload{
										DataRate: 5,
										TXPower:  4,
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
					MIC: lorawan.MIC{0xec, 0xef, 0xdc, 0x41},
				}),
			},
		},
		{
			Name:          "adr interval matches, but node does not support adr",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
						FCtrl: lorawan.FCtrl{
							ADR: false,
						},
					},
				},
				MIC: lorawan.MIC{122, 152, 152, 220},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(5),
				AssertNoDownlinkFrame,
			},
		},
		{
			Name:          "acknowledgement of pending adr request",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PendingMACCommands: []storage.MACCommandBlock{
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
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
						FOpts: []lorawan.Payload{
							&lorawan.MACCommand{CID: lorawan.LinkADRAns, Payload: &lorawan.LinkADRAnsPayload{ChannelMaskACK: true, DataRateACK: true, PowerACK: true}},
						},
					},
				},
				MIC: lorawan.MIC{235, 224, 96, 3},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(5),
				AssertTXPowerIndex(3),
				AssertNbTrans(1),
				AssertEnabledUplinkChannels([]int{0, 1, 2}),
			},
		},
		{
			Name:          "negative acknowledgement of pending adr request",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PendingMACCommands: []storage.MACCommandBlock{
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
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
						FOpts: []lorawan.Payload{
							&lorawan.MACCommand{CID: lorawan.LinkADRAns, Payload: &lorawan.LinkADRAnsPayload{ChannelMaskACK: false, DataRateACK: true, PowerACK: true}},
						},
					},
				},
				MIC: lorawan.MIC{252, 17, 226, 74},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(5),
				AssertTXPowerIndex(0),
				AssertNbTrans(0),
				AssertEnabledUplinkChannels([]int{0, 1, 2}),
			},
		},
		{
			Name:          "adr ack requested",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
						FCtrl: lorawan.FCtrl{
							ADRACKReq: true,
						},
					},
				},
				MIC: lorawan.MIC{73, 26, 32, 42},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(6),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId:  ts.RXInfo.GatewayId,
					Frequency:  ts.TXInfo.Frequency,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.DeviceSession.DevAddr,
							FCnt:    5,
							FCtrl: lorawan.FCtrl{
								ADR: true,
							},
						},
					},
					MIC: lorawan.MIC{0xc1, 0xa, 0x8, 0xd9},
				}),
			},
		},
		{
			Name: "channel re-configuration triggered",
			BeforeFunc: func(tst *ClassATest) error {
				tst.DeviceSession.EnabledUplinkChannels = []int{0, 1, 2, 3, 4, 5, 6, 7}
				return nil
			},
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
				},
				MIC: lorawan.MIC{122, 152, 152, 220},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(6),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId:  ts.RXInfo.GatewayId,
					Frequency:  ts.TXInfo.Frequency,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.DeviceSession.DevAddr,
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
					MIC: lorawan.MIC{0x8, 0xee, 0xdd, 0x34},
				}),
				AssertEnabledUplinkChannels([]int{0, 1, 2, 3, 4, 5, 6, 7}),
			},
		},
		{
			Name: "new channel re-configuration ack-ed",
			BeforeFunc: func(tst *ClassATest) error {
				tst.DeviceSession.EnabledUplinkChannels = []int{0, 1, 2, 3, 4, 5, 6, 7}
				return nil
			},
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PendingMACCommands: []storage.MACCommandBlock{
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
						DevAddr: ts.DeviceSession.DevAddr,
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
				MIC: lorawan.MIC{235, 224, 96, 3},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(5),
				AssertNoDownlinkFrame,
				AssertEnabledUplinkChannels([]int{0, 1, 2}),
			},
		},
		{
			Name: "new channel re-configuration not ack-ed",
			BeforeFunc: func(tst *ClassATest) error {
				tst.DeviceSession.EnabledUplinkChannels = []int{0, 1, 2, 3, 4, 5, 6, 7}
				return nil
			},
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PendingMACCommands: []storage.MACCommandBlock{
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
						DevAddr: ts.DeviceSession.DevAddr,
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
				MIC: lorawan.MIC{252, 17, 226, 74},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(6),
				AssertEnabledUplinkChannels([]int{0, 1, 2, 3, 4, 5, 6, 7}),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId:  ts.RXInfo.GatewayId,
					Frequency:  ts.TXInfo.Frequency,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.DeviceSession.DevAddr,
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
					MIC: lorawan.MIC{0x8, 0xee, 0xdd, 0x34},
				}),
			},
		},
		{
			Name: "channel re-configuration and adr triggered",
			BeforeFunc: func(tst *ClassATest) error {
				tst.DeviceSession.EnabledUplinkChannels = []int{0, 1, 2, 3, 4, 5, 6, 7}
				return nil
			},
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
						FCtrl: lorawan.FCtrl{
							ADR: true,
						},
					},
				},
				MIC: lorawan.MIC{187, 243, 244, 117},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(6),
				AssertEnabledUplinkChannels([]int{0, 1, 2, 3, 4, 5, 6, 7}),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId:  ts.RXInfo.GatewayId,
					Frequency:  ts.TXInfo.Frequency,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.DeviceSession.DevAddr,
							FCnt:    5,
							FCtrl: lorawan.FCtrl{
								ADR: true,
							},
							FOpts: []lorawan.Payload{
								&lorawan.MACCommand{
									CID: lorawan.LinkADRReq,
									Payload: &lorawan.LinkADRReqPayload{
										DataRate: 5,
										TXPower:  4,
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
					MIC: lorawan.MIC{0xec, 0xef, 0xdc, 0x41},
				}),
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertClassATest(t, tst)
		})
	}
}

func (ts *ClassATestSuite) TestLW10DeviceStatusRequest() {
	assert := require.New(ts.T())

	ts.CreateDeviceSession(storage.DeviceSession{
		MACVersion:            "1.0.2",
		JoinEUI:               lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
		DevAddr:               lorawan.DevAddr{1, 2, 3, 4},
		FNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		NwkSEncKey:            [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		FCntUp:                10,
		NFCntDown:             5,
		EnabledUplinkChannels: []int{0, 1, 2},
		RX2Frequency:          869525000,
	})

	ts.ServiceProfile.DevStatusReqFreq = 24
	ts.ServiceProfile.ReportDevStatusBattery = true
	ts.ServiceProfile.ReportDevStatusMargin = true
	assert.NoError(storage.UpdateServiceProfile(context.Background(), storage.DB(), ts.ServiceProfile))

	fPortOne := uint8(1)

	tests := []ClassATest{
		{
			Name: "must request device-status",
			BeforeFunc: func(tst *ClassATest) error {
				tst.DeviceSession.LastDevStatusRequested = time.Now().Add(-61 * time.Minute)
				return nil
			},
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
				},
				MIC: lorawan.MIC{122, 152, 152, 220},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(6),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId:  ts.RXInfo.GatewayId,
					Frequency:  ts.TXInfo.Frequency,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.DeviceSession.DevAddr,
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
					MIC: lorawan.MIC{0xfa, 0xf0, 0x96, 0xdb},
				}),
				AssertASHandleUplinkDataRequest(as.HandleUplinkDataRequest{
					DevEui:  ts.Device.DevEUI[:],
					JoinEui: ts.DeviceSession.JoinEUI[:],
					FCnt:    10,
					FPort:   0,
					Dr:      0,
					TxInfo:  &ts.TXInfo,
					RxInfo:  []*gw.UplinkRXInfo{&ts.RXInfo},
				}),
			},
		},
		{
			Name: "interval has not yet expired",
			BeforeFunc: func(tst *ClassATest) error {
				tst.DeviceSession.LastDevStatusRequested = time.Now().Add(-59 * time.Minute)
				return nil
			},
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
				},
				MIC: lorawan.MIC{122, 152, 152, 220},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(5),
				AssertASHandleUplinkDataRequest(as.HandleUplinkDataRequest{
					DevEui:  ts.Device.DevEUI[:],
					JoinEui: ts.DeviceSession.JoinEUI[:],
					FCnt:    10,
					FPort:   0,
					Dr:      0,
					TxInfo:  &ts.TXInfo,
					RxInfo:  []*gw.UplinkRXInfo{&ts.RXInfo},
				}),
				AssertNoDownlinkFrame,
			},
		},
		{
			Name: "report device-status to application-server",
			BeforeFunc: func(tst *ClassATest) error {
				tst.DeviceSession.LastDevStatusRequested = time.Now()
				return nil
			},
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
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
				MIC: lorawan.MIC{30, 172, 57, 148},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(5),
				AssertNoDownlinkFrame,
				AssertASHandleUplinkDataRequest(as.HandleUplinkDataRequest{
					DevEui:  ts.Device.DevEUI[:],
					JoinEui: ts.DeviceSession.JoinEUI[:],
					FCnt:    10,
					FPort:   1,
					Dr:      0,
					TxInfo:  &ts.TXInfo,
					RxInfo:  []*gw.UplinkRXInfo{&ts.RXInfo},
					Data:    []byte{1, 2, 3, 4},
				}),
				AssertASSetDeviceStatusRequest(as.SetDeviceStatusRequest{
					DevEui:       ts.Device.DevEUI[:],
					Battery:      128,
					Margin:       10,
					BatteryLevel: float32(128) / float32(254) * float32(100),
				}),
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertClassATest(t, tst)
		})
	}

	ts.ServiceProfile.DevStatusReqFreq = 0
	ts.ServiceProfile.ReportDevStatusBattery = false
	ts.ServiceProfile.ReportDevStatusMargin = false
	assert.NoError(storage.UpdateServiceProfile(context.Background(), storage.DB(), ts.ServiceProfile))
}

func (ts *ClassATestSuite) TestLW11ReceiveWindowSelection() {
	ts.CreateDeviceSession(storage.DeviceSession{
		MACVersion:            "1.1.0",
		JoinEUI:               lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
		DevAddr:               lorawan.DevAddr{1, 2, 3, 4},
		FNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		NwkSEncKey:            [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		FCntUp:                8,
		NFCntDown:             5,
		AFCntDown:             4,
		EnabledUplinkChannels: []int{0, 1, 2},
		RX2Frequency:          869525000,
	})

	var fPortOne uint8 = 1

	tests := []ClassATest{
		{
			Name: "unconfirmed uplink with payload (rx1)",
			BeforeFunc: func(tst *ClassATest) error {
				conf := test.GetConfig()
				conf.NetworkServer.NetworkSettings.RXWindow = 1

				return downlink.Setup(conf)
			},
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.Device.DevEUI, FRMPayload: []byte{1}, FPort: 1, FCnt: 4},
			},
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
					FPort:      &fPortOne,
					FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
				},
				MIC: lorawan.MIC{104, 147, 104, 147},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(5),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId:  ts.Gateway.GatewayID[:],
					Frequency:  868100000,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.DeviceSession.DevAddr,
							FCnt:    4,
							FCtrl: lorawan.FCtrl{
								ADR: true,
							},
						},
						FPort:      &fPortOne,
						FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1}}},
					},
					MIC: lorawan.MIC{0xc3, 0xe2, 0xfc, 0x50},
				}),
			},
		},
		{
			Name: "unconfirmed uplink with payload (rxdelay = 0, rx2)",
			BeforeFunc: func(tst *ClassATest) error {
				conf := test.GetConfig()
				conf.NetworkServer.NetworkSettings.RXWindow = 2
				conf.NetworkServer.NetworkSettings.RX1Delay = 0

				return downlink.Setup(conf)
			},
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.Device.DevEUI, FRMPayload: []byte{1}, FPort: 1, FCnt: 4},
			},
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
					FPort:      &fPortOne,
					FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
				},
				MIC: lorawan.MIC{104, 147, 104, 147},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(5),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId:  ts.Gateway.GatewayID[:],
					Frequency:  869525000,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second * 2),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.DeviceSession.DevAddr,
							FCnt:    4,
							FCtrl: lorawan.FCtrl{
								ADR: true,
							},
						},
						FPort:      &fPortOne,
						FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1}}},
					},
					MIC: lorawan.MIC{0xc3, 0xe2, 0xfc, 0x50},
				}),
			},
		},
		{
			Name: "unconfirmed uplink with payload (rxdelay = 1, rx2)",
			BeforeFunc: func(tst *ClassATest) error {
				conf := test.GetConfig()
				conf.NetworkServer.NetworkSettings.RXWindow = 2
				conf.NetworkServer.NetworkSettings.RX1Delay = 1

				tst.DeviceSession.RXDelay = 1

				return downlink.Setup(conf)
			},
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.Device.DevEUI, FRMPayload: []byte{1}, FPort: 1, FCnt: 4},
			},
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
					FPort:      &fPortOne,
					FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
				},
				MIC: lorawan.MIC{104, 147, 104, 147},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(5),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId:  ts.Gateway.GatewayID[:],
					Frequency:  869525000,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second * 2),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.DeviceSession.DevAddr,
							FCnt:    4,
							FCtrl: lorawan.FCtrl{
								ADR: true,
							},
						},
						FPort:      &fPortOne,
						FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1}}},
					},
					MIC: lorawan.MIC{0xc3, 0xe2, 0xfc, 0x50},
				}),
			},
		},
		{
			Name: "unconfirmed uplink with payload (rx1 + rx2)",
			BeforeFunc: func(tst *ClassATest) error {
				conf := test.GetConfig()
				conf.NetworkServer.NetworkSettings.RXWindow = 0

				return downlink.Setup(conf)
			},
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.Device.DevEUI, FRMPayload: []byte{1}, FPort: 1, FCnt: 4},
			},
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
					FPort:      &fPortOne,
					FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
				},
				MIC: lorawan.MIC{104, 147, 104, 147},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(5),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId:  ts.Gateway.GatewayID[:],
					Frequency:  868100000,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.DeviceSession.DevAddr,
							FCnt:    4,
							FCtrl: lorawan.FCtrl{
								ADR: true,
							},
						},
						FPort:      &fPortOne,
						FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1}}},
					},
					MIC: lorawan.MIC{0xc3, 0xe2, 0xfc, 0x50},
				}),
				AssertDownlinkFrameSaved(ts.Device.DevEUI, uuid.Nil, gw.DownlinkTXInfo{
					GatewayId:  ts.Gateway.GatewayID[:],
					Frequency:  868100000,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.DeviceSession.DevAddr,
							FCnt:    4,
							FCtrl: lorawan.FCtrl{
								ADR: true,
							},
						},
						FPort:      &fPortOne,
						FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1}}},
					},
					MIC: lorawan.MIC{0xc3, 0xe2, 0xfc, 0x50},
				}),
				AssertDownlinkFrameSaved(ts.Device.DevEUI, uuid.Nil, gw.DownlinkTXInfo{
					GatewayId:  ts.Gateway.GatewayID[:],
					Frequency:  869525000,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second * 2),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.DeviceSession.DevAddr,
							FCnt:    4,
							FCtrl: lorawan.FCtrl{
								ADR: true,
							},
						},
						FPort:      &fPortOne,
						FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1}}},
					},
					MIC: lorawan.MIC{0xc3, 0xe2, 0xfc, 0x50},
				}),
			},
		},
		{
			Name: "unconfirmed uplink with payload (rx1, payload exceeds rx2 limit)",
			BeforeFunc: func(tst *ClassATest) error {
				conf := test.GetConfig()
				conf.NetworkServer.NetworkSettings.RXWindow = 0
				downlink.Setup(conf)
				return helpers.SetUplinkTXInfoDataRate(&tst.TXInfo, 5, band.Band())
			},
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.Device.DevEUI, FRMPayload: make([]byte, 100), FPort: 1, FCnt: 4},
			},
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
					FPort:      &fPortOne,
					FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
				},
				MIC: lorawan.MIC{0xd4, 0x59, 0x68, 0x93},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(5),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId:  ts.Gateway.GatewayID[:],
					Frequency:  868100000,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       7,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.DeviceSession.DevAddr,
							FCnt:    4,
							FCtrl: lorawan.FCtrl{
								ADR: true,
							},
						},
						FPort:      &fPortOne,
						FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: make([]byte, 100)}},
					},
					MIC: lorawan.MIC{0x6e, 0xc6, 0xc2, 0x7c},
				}),
				AssertDownlinkFrameSaved(ts.Device.DevEUI, uuid.Nil, gw.DownlinkTXInfo{
					GatewayId:  ts.Gateway.GatewayID[:],
					Frequency:  868100000,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       7,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.DeviceSession.DevAddr,
							FCnt:    4,
							FCtrl: lorawan.FCtrl{
								ADR: true,
							},
						},
						FPort:      &fPortOne,
						FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: make([]byte, 100)}},
					},
					MIC: lorawan.MIC{0x6e, 0xc6, 0xc2, 0x7c},
				}),
			},
		},
		{
			Name: "unconfirmed uplink with payload (rx1, mac-command stripped as it exceeds rx2 limit)",
			BeforeFunc: func(tst *ClassATest) error {
				conf := test.GetConfig()
				conf.NetworkServer.NetworkSettings.RXWindow = 0
				downlink.Setup(conf)

				ts.ServiceProfile.DevStatusReqFreq = 1
				if err := storage.UpdateServiceProfile(context.Background(), storage.DB(), ts.ServiceProfile); err != nil {
					return err
				}

				return helpers.SetUplinkTXInfoDataRate(&tst.TXInfo, 5, band.Band())
			},
			AfterFunc: func(tst *ClassATest) error {
				ts.ServiceProfile.DevStatusReqFreq = 0
				return storage.UpdateServiceProfile(context.Background(), storage.DB(), ts.ServiceProfile)
			},
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.Device.DevEUI, FRMPayload: make([]byte, 51), FPort: 1, FCnt: 4},
			},
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    10,
					},
					FPort:      &fPortOne,
					FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
				},
				MIC: lorawan.MIC{0xd4, 0x59, 0x68, 0x93},
			},
			Assert: []Assertion{
				AssertFCntUp(11),
				AssertNFCntDown(5),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId:  ts.Gateway.GatewayID[:],
					Frequency:  868100000,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       7,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.DeviceSession.DevAddr,
							FCnt:    4,
							FCtrl: lorawan.FCtrl{
								ADR: true,
							},
							FOpts: []lorawan.Payload{
								&lorawan.MACCommand{
									CID: 181, // encrypted
								},
							},
						},
						FPort:      &fPortOne,
						FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: make([]byte, 51)}},
					},
					MIC: lorawan.MIC{0x17, 0x4e, 0xc6, 0x6e},
				}),
				AssertDownlinkFrameSaved(ts.Device.DevEUI, uuid.Nil, gw.DownlinkTXInfo{
					GatewayId:  ts.Gateway.GatewayID[:],
					Frequency:  868100000,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       7,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.DeviceSession.DevAddr,
							FCnt:    4,
							FCtrl: lorawan.FCtrl{
								ADR: true,
							},
							FOpts: []lorawan.Payload{
								&lorawan.MACCommand{
									CID: 181, // encrypted
								},
							},
						},
						FPort:      &fPortOne,
						FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: make([]byte, 51)}},
					},
					MIC: lorawan.MIC{0x17, 0x4e, 0xc6, 0x6e},
				}),
				AssertDownlinkFrameSaved(ts.Device.DevEUI, uuid.Nil, gw.DownlinkTXInfo{
					GatewayId:  ts.Gateway.GatewayID[:],
					Frequency:  869525000,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							PolarizationInversion: true,
							CodeRate:              "4/5",
						},
					},
					Context: ts.RXInfo.Context,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second * 2),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.DeviceSession.DevAddr,
							FCnt:    4,
							FCtrl: lorawan.FCtrl{
								ADR: true,
							},
						},
						FPort:      &fPortOne,
						FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: make([]byte, 51)}},
					},
					MIC: lorawan.MIC{0x3c, 0xab, 0x49, 0xe9},
				}),
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertClassATest(t, tst)
		})
	}
}

func TestClassA(t *testing.T) {
	suite.Run(t, new(ClassATestSuite))
}
