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

	"github.com/brocaar/chirpstack-api/go/v3/as"
	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-api/go/v3/nc"
	"github.com/brocaar/chirpstack-network-server/v3/internal/band"
	"github.com/brocaar/chirpstack-network-server/v3/internal/config"
	"github.com/brocaar/chirpstack-network-server/v3/internal/helpers"
	"github.com/brocaar/chirpstack-network-server/v3/internal/storage"
	"github.com/brocaar/chirpstack-network-server/v3/internal/test"
	"github.com/brocaar/chirpstack-network-server/v3/internal/uplink"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
	loraband "github.com/brocaar/lorawan/band"
)

type OTAATestSuite struct {
	IntegrationTestSuite
}

func (ts *OTAATestSuite) SetupSuite() {
	ts.IntegrationTestSuite.SetupSuite()

	ts.JoinAcceptKey = lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	ts.CreateDeviceProfile(storage.DeviceProfile{
		MACVersion:   "1.0.2",
		RXDelay1:     3,
		RXDROffset1:  1,
		RXDataRate2:  5,
		SupportsJoin: true,
	})

	ts.CreateDevice(storage.Device{
		DevEUI:            lorawan.EUI64{2, 2, 3, 4, 5, 6, 7, 8},
		ReferenceAltitude: 5.6,
		Mode:              storage.DeviceModeB,
	})

	ts.CreateGateway(storage.Gateway{
		GatewayID: lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
	})
}

func (ts *OTAATestSuite) TestGatewayFiltering() {
	assert := require.New(ts.T())

	conf := test.GetConfig()

	ts.DeviceProfile.MACVersion = "1.0.2"
	assert.NoError(storage.UpdateDeviceProfile(context.Background(), storage.DB(), ts.DeviceProfile))

	rxInfo := gw.UplinkRXInfo{
		GatewayId: ts.Gateway.GatewayID[:],
		Context:   []byte{1, 2, 3, 4},
		Location:  &common.Location{},
	}

	txInfo := gw.UplinkTXInfo{
		Frequency: 868100000,
	}
	assert.NoError(helpers.SetUplinkTXInfoDataRate(&txInfo, 0, band.Band()))

	jrPayload := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: lorawan.JoinRequest,
			Major: lorawan.LoRaWANR1,
		},
		MACPayload: &lorawan.JoinRequestPayload{
			JoinEUI:  lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
			DevEUI:   ts.Device.DevEUI,
			DevNonce: 258,
		},
	}
	assert.NoError(jrPayload.SetUplinkJoinMIC(ts.JoinAcceptKey))

	jaPayload := lorawan.JoinAcceptPayload{
		JoinNonce: 197121,
		HomeNetID: conf.NetworkServer.NetID,
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
	assert.NoError(jaPHY.SetDownlinkJoinMIC(lorawan.JoinRequestType, lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}, lorawan.DevNonce(258), ts.JoinAcceptKey))
	assert.NoError(jaPHY.EncryptJoinAcceptPayload(ts.JoinAcceptKey))
	jaBytes, err := jaPHY.MarshalBinary()
	assert.NoError(err)
	assert.NoError(jaPHY.DecryptJoinAcceptPayload(ts.JoinAcceptKey))

	tests := []OTAATest{
		{
			Name:       "public gateway",
			RXInfo:     rxInfo,
			TXInfo:     txInfo,
			PHYPayload: jrPayload,
			JoinServerJoinAnsPayload: backend.JoinAnsPayload{
				BasePayloadResult: backend.BasePayloadResult{
					Result: backend.Result{
						ResultCode: backend.Success,
					},
				},
				PHYPayload: backend.HEXBytes(jaBytes),
				NwkSKey: &backend.KeyEnvelope{
					AESKey: []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
				},
				AppSKey: &backend.KeyEnvelope{
					AESKey: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
				},
			},
			Assert: []Assertion{
				AssertDownlinkFrame(ts.Gateway.GatewayID, gw.DownlinkTXInfo{
					Frequency:  txInfo.Frequency,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							CodeRate:              "4/5",
							PolarizationInversion: true,
						},
					},
					Context: []byte{1, 2, 3, 4},
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(5 * time.Second),
						},
					},
				}, jaPHY),
			},
		},
		{
			Name: "private gateway - same service-profile",
			BeforeFunc: func(tst *OTAATest) error {
				ts.ServiceProfile.GwsPrivate = true
				return storage.UpdateServiceProfile(context.Background(), storage.DB(), ts.ServiceProfile)
			},
			AfterFunc: func(tst *OTAATest) error {
				ts.ServiceProfile.GwsPrivate = false
				return storage.UpdateServiceProfile(context.Background(), storage.DB(), ts.ServiceProfile)
			},
			RXInfo:     rxInfo,
			TXInfo:     txInfo,
			PHYPayload: jrPayload,
			JoinServerJoinAnsPayload: backend.JoinAnsPayload{
				BasePayloadResult: backend.BasePayloadResult{
					Result: backend.Result{
						ResultCode: backend.Success,
					},
				},
				PHYPayload: backend.HEXBytes(jaBytes),
				NwkSKey: &backend.KeyEnvelope{
					AESKey: []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
				},
				AppSKey: &backend.KeyEnvelope{
					AESKey: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
				},
			},
			Assert: []Assertion{
				AssertDownlinkFrame(ts.Gateway.GatewayID, gw.DownlinkTXInfo{
					Frequency:  txInfo.Frequency,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							CodeRate:              "4/5",
							PolarizationInversion: true,
						},
					},
					Context: []byte{1, 2, 3, 4},
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(5 * time.Second),
						},
					},
				}, jaPHY),
			},
		},
		{
			Name: "private gateway - different service-profile",
			BeforeFunc: func(tst *OTAATest) error {
				sp := storage.ServiceProfile{
					GwsPrivate: true,
				}
				if err := storage.CreateServiceProfile(context.Background(), storage.DB(), &sp); err != nil {
					return err
				}

				ts.Gateway.ServiceProfileID = &sp.ID
				return storage.UpdateGateway(context.Background(), storage.DB(), ts.Gateway)
			},
			AfterFunc: func(tst *OTAATest) error {
				ts.Gateway.ServiceProfileID = &ts.ServiceProfile.ID
				return storage.UpdateGateway(context.Background(), storage.DB(), ts.Gateway)
			},
			RXInfo:     rxInfo,
			TXInfo:     txInfo,
			PHYPayload: jrPayload,
			JoinServerJoinAnsPayload: backend.JoinAnsPayload{
				BasePayloadResult: backend.BasePayloadResult{
					Result: backend.Result{
						ResultCode: backend.Success,
					},
				},
				PHYPayload: backend.HEXBytes(jaBytes),
				NwkSKey: &backend.KeyEnvelope{
					AESKey: []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
				},
				AppSKey: &backend.KeyEnvelope{
					AESKey: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
				},
			},
			Assert: []Assertion{
				AssertNoDownlinkFrame,
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertOTAATest(t, tst)
		})
	}
}

func (ts *OTAATestSuite) TestLW10() {
	assert := require.New(ts.T())

	conf := test.GetConfig()

	ts.DeviceProfile.MACVersion = "1.0.2"
	assert.NoError(storage.UpdateDeviceProfile(context.Background(), storage.DB(), ts.DeviceProfile))

	rxInfo := gw.UplinkRXInfo{
		GatewayId: ts.Gateway.GatewayID[:],
		Context:   []byte{1, 2, 3, 4},
		Location:  &common.Location{},
	}

	txInfo := gw.UplinkTXInfo{
		Frequency: 868100000,
	}
	assert.NoError(helpers.SetUplinkTXInfoDataRate(&txInfo, 0, band.Band()))

	jrPayload := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: lorawan.JoinRequest,
			Major: lorawan.LoRaWANR1,
		},
		MACPayload: &lorawan.JoinRequestPayload{
			JoinEUI:  lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
			DevEUI:   ts.Device.DevEUI,
			DevNonce: 258,
		},
	}
	assert.NoError(jrPayload.SetUplinkJoinMIC(ts.JoinAcceptKey))
	jrBytes, err := jrPayload.MarshalBinary()
	assert.NoError(err)

	jaPayload := lorawan.JoinAcceptPayload{
		JoinNonce: 197121,
		HomeNetID: conf.NetworkServer.NetID,
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
	assert.NoError(jaPHY.SetDownlinkJoinMIC(lorawan.JoinRequestType, lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}, lorawan.DevNonce(258), ts.JoinAcceptKey))
	assert.NoError(jaPHY.EncryptJoinAcceptPayload(ts.JoinAcceptKey))
	jaBytes, err := jaPHY.MarshalBinary()
	assert.NoError(err)
	assert.NoError(jaPHY.DecryptJoinAcceptPayload(ts.JoinAcceptKey))

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
	assert.NoError(err)

	tests := []OTAATest{
		{
			Name:       "device already activated with dev-nonce",
			RXInfo:     rxInfo,
			TXInfo:     txInfo,
			PHYPayload: jrPayload,
			DeviceActivations: []storage.DeviceActivation{
				{
					DevEUI:      ts.Device.DevEUI,
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
			Assert: []Assertion{
				AssertASHandleErrorRequest(as.HandleErrorRequest{
					DevEui: ts.Device.DevEUI[:],
					Type:   as.ErrorType_OTAA,
					Error:  "validate dev-nonce error",
				}),
			},
		},
		{
			Name:       "join-request accepted (rx1 + rx2)",
			RXInfo:     rxInfo,
			TXInfo:     txInfo,
			PHYPayload: jrPayload,
			JoinServerJoinAnsPayload: backend.JoinAnsPayload{
				BasePayloadResult: backend.BasePayloadResult{
					Result: backend.Result{
						ResultCode: backend.Success,
					},
				},
				PHYPayload: backend.HEXBytes(jaBytes),
				NwkSKey: &backend.KeyEnvelope{
					AESKey: []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
				},
				AppSKey: &backend.KeyEnvelope{
					AESKey: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
				},
			},
			DeviceQueueItems: []storage.DeviceQueueItem{
				{
					DevEUI:     ts.Device.DevEUI,
					FRMPayload: []byte{1, 2, 3, 4},
					FCnt:       10,
					FPort:      1,
				},
			},

			Assert: []Assertion{
				AssertJSJoinReqPayload(backend.JoinReqPayload{
					BasePayload: backend.BasePayload{
						ProtocolVersion: backend.ProtocolVersion1_0,
						SenderID:        "030201",
						ReceiverID:      "0102030405060708",
						MessageType:     backend.JoinReq,
					},
					MACVersion: ts.DeviceProfile.MACVersion,
					PHYPayload: backend.HEXBytes(jrBytes),
					DevEUI:     ts.Device.DevEUI,
					DLSettings: lorawan.DLSettings{
						RX2DataRate: uint8(conf.NetworkServer.NetworkSettings.RX2DR),
						RX1DROffset: uint8(conf.NetworkServer.NetworkSettings.RX1DROffset),
					},
					RxDelay: conf.NetworkServer.NetworkSettings.RX1Delay,
				}),
				AssertDownlinkFrame(ts.Gateway.GatewayID, gw.DownlinkTXInfo{
					Frequency:  txInfo.Frequency,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							CodeRate:              "4/5",
							PolarizationInversion: true,
						},
					},
					Context: []byte{1, 2, 3, 4},
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(5 * time.Second),
						},
					},
				}, jaPHY),
				AssertDownlinkFrameSaved(ts.Gateway.GatewayID, ts.Device.DevEUI, uuid.Nil, gw.DownlinkTXInfo{
					Frequency:  txInfo.Frequency,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							CodeRate:              "4/5",
							PolarizationInversion: true,
						},
					},
					Context: []byte{1, 2, 3, 4},
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(5 * time.Second),
						},
					},
				}, jaPHY),
				AssertDownlinkFrameSaved(ts.Gateway.GatewayID, ts.Device.DevEUI, uuid.Nil, gw.DownlinkTXInfo{
					Frequency:  869525000,
					Power:      27,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							CodeRate:              "4/5",
							PolarizationInversion: true,
						},
					},
					Context: []byte{1, 2, 3, 4},
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(6 * time.Second),
						},
					},
				}, jaPHY),
				AssertDeviceSession(storage.DeviceSession{
					MACVersion:       "1.0.2",
					RoutingProfileID: ts.RoutingProfile.ID,
					DeviceProfileID:  ts.DeviceProfile.ID,
					ServiceProfileID: ts.ServiceProfile.ID,
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
					ExtraUplinkChannels:   map[int]loraband.Channel{},
					RX2Frequency:          band.Band().GetDefaults().RX2Frequency,
					NbTrans:               1,
					ReferenceAltitude:     5.6,
					MACCommandErrorCount:  make(map[lorawan.CID]int),
				}),
				AssertDeviceQueueItems([]storage.DeviceQueueItem{}),
				AssertDeviceMode(storage.DeviceModeA),
				AssertNCHandleUplinkMetaDataRequest(nc.HandleUplinkMetaDataRequest{
					DevEui:              ts.Device.DevEUI[:],
					TxInfo:              &txInfo,
					RxInfo:              []*gw.UplinkRXInfo{&rxInfo},
					MessageType:         nc.MType_JOIN_REQUEST,
					PhyPayloadByteCount: 23,
				}),
			},
		},
		{
			Name: "join-request accepted + skip fcnt check set (rx1 + rx2)",
			BeforeFunc: func(*OTAATest) error {
				ts.Device.SkipFCntCheck = true
				return storage.UpdateDevice(context.Background(), storage.DB(), ts.Device)
			},
			RXInfo:     rxInfo,
			TXInfo:     txInfo,
			PHYPayload: jrPayload,
			JoinServerJoinAnsPayload: backend.JoinAnsPayload{
				BasePayloadResult: backend.BasePayloadResult{
					Result: backend.Result{
						ResultCode: backend.Success,
					},
				},
				PHYPayload: backend.HEXBytes(jaBytes),
				NwkSKey: &backend.KeyEnvelope{
					AESKey: []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
				},
			},

			Assert: []Assertion{
				AssertJSJoinReqPayload(backend.JoinReqPayload{
					BasePayload: backend.BasePayload{
						ProtocolVersion: backend.ProtocolVersion1_0,
						SenderID:        "030201",
						ReceiverID:      "0102030405060708",
						MessageType:     backend.JoinReq,
					},
					MACVersion: ts.DeviceProfile.MACVersion,
					PHYPayload: jrBytes,
					DevEUI:     ts.Device.DevEUI,
					DLSettings: lorawan.DLSettings{
						RX2DataRate: uint8(conf.NetworkServer.NetworkSettings.RX2DR),
						RX1DROffset: uint8(conf.NetworkServer.NetworkSettings.RX1DROffset),
					},
					RxDelay: conf.NetworkServer.NetworkSettings.RX1Delay,
				}),
				AssertDownlinkFrame(ts.Gateway.GatewayID, gw.DownlinkTXInfo{
					Frequency:  txInfo.Frequency,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							CodeRate:              "4/5",
							PolarizationInversion: true,
						},
					},
					Context: []byte{1, 2, 3, 4},
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(5 * time.Second),
						},
					},
				}, jaPHY),
				AssertDownlinkFrameSaved(ts.Gateway.GatewayID, ts.Device.DevEUI, uuid.Nil, gw.DownlinkTXInfo{
					Frequency:  txInfo.Frequency,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							CodeRate:              "4/5",
							PolarizationInversion: true,
						},
					},
					Context: []byte{1, 2, 3, 4},
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(5 * time.Second),
						},
					},
				}, jaPHY),
				AssertDownlinkFrameSaved(ts.Gateway.GatewayID, ts.Device.DevEUI, uuid.Nil, gw.DownlinkTXInfo{
					Frequency:  869525000,
					Power:      27,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							CodeRate:              "4/5",
							PolarizationInversion: true,
						},
					},
					Context: []byte{1, 2, 3, 4},
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(6 * time.Second),
						},
					},
				}, jaPHY),
				AssertDeviceSession(storage.DeviceSession{
					MACVersion:            "1.0.2",
					RoutingProfileID:      ts.RoutingProfile.ID,
					DeviceProfileID:       ts.DeviceProfile.ID,
					ServiceProfileID:      ts.ServiceProfile.ID,
					JoinEUI:               lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
					DevEUI:                lorawan.EUI64{2, 2, 3, 4, 5, 6, 7, 8},
					FNwkSIntKey:           lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
					SNwkSIntKey:           lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
					NwkSEncKey:            lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
					RXWindow:              storage.RX1,
					EnabledUplinkChannels: []int{0, 1, 2},
					ExtraUplinkChannels:   map[int]loraband.Channel{},
					RX2Frequency:          band.Band().GetDefaults().RX2Frequency,
					SkipFCntValidation:    true,
					NbTrans:               1,
					ReferenceAltitude:     5.6,
					MACCommandErrorCount:  make(map[lorawan.CID]int),
				}),
				AssertDeviceMode(storage.DeviceModeA),
			},
		},
		{
			Name: "join-request accepted + cflist channels",
			BeforeFunc: func(*OTAATest) error {
				ts.Device.SkipFCntCheck = false
				return storage.UpdateDevice(context.Background(), storage.DB(), ts.Device)
			},
			RXInfo:        rxInfo,
			TXInfo:        txInfo,
			PHYPayload:    jrPayload,
			ExtraChannels: []int{868600000, 868700000, 868800000},
			JoinServerJoinAnsPayload: backend.JoinAnsPayload{
				BasePayloadResult: backend.BasePayloadResult{
					Result: backend.Result{
						ResultCode: backend.Success,
					},
				},
				PHYPayload: backend.HEXBytes(jaBytes),
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

			Assert: []Assertion{
				AssertJSJoinReqPayload(backend.JoinReqPayload{
					BasePayload: backend.BasePayload{
						ProtocolVersion: backend.ProtocolVersion1_0,
						SenderID:        "030201",
						ReceiverID:      "0102030405060708",
						MessageType:     backend.JoinReq,
					},
					MACVersion: ts.DeviceProfile.MACVersion,
					PHYPayload: backend.HEXBytes(jrBytes),
					DevEUI:     ts.Device.DevEUI,
					DLSettings: lorawan.DLSettings{
						RX2DataRate: uint8(conf.NetworkServer.NetworkSettings.RX2DR),
						RX1DROffset: uint8(conf.NetworkServer.NetworkSettings.RX1DROffset),
					},
					RxDelay: conf.NetworkServer.NetworkSettings.RX1Delay,
					CFList:  backend.HEXBytes(cFListB),
				}),
				AssertDeviceMode(storage.DeviceModeA),
			},
		},
		{
			Name: "join-request accepted, Class-B supported",
			BeforeFunc: func(*OTAATest) error {
				ts.DeviceProfile.SupportsClassB = true
				ts.DeviceProfile.SupportsClassC = false
				return storage.UpdateDeviceProfile(context.Background(), storage.DB(), ts.DeviceProfile)
			},
			RXInfo:     rxInfo,
			TXInfo:     txInfo,
			PHYPayload: jrPayload,
			JoinServerJoinAnsPayload: backend.JoinAnsPayload{
				BasePayloadResult: backend.BasePayloadResult{
					Result: backend.Result{
						ResultCode: backend.Success,
					},
				},
				PHYPayload: backend.HEXBytes(jaBytes),
				NwkSKey: &backend.KeyEnvelope{
					AESKey: []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
				},
			},
			Assert: []Assertion{
				AssertDeviceMode(storage.DeviceModeA),
			},
		},
		{
			Name: "join-request accepted, Class-C supported",
			BeforeFunc: func(*OTAATest) error {
				ts.DeviceProfile.SupportsClassB = false
				ts.DeviceProfile.SupportsClassC = true
				return storage.UpdateDeviceProfile(context.Background(), storage.DB(), ts.DeviceProfile)
			},
			RXInfo:     rxInfo,
			TXInfo:     txInfo,
			PHYPayload: jrPayload,
			JoinServerJoinAnsPayload: backend.JoinAnsPayload{
				BasePayloadResult: backend.BasePayloadResult{
					Result: backend.Result{
						ResultCode: backend.Success,
					},
				},
				PHYPayload: backend.HEXBytes(jaBytes),
				NwkSKey: &backend.KeyEnvelope{
					AESKey: []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
				},
			},
			Assert: []Assertion{
				AssertDeviceMode(storage.DeviceModeC),
			},
		},
		{
			Name: "device disabled",
			BeforeFunc: func(*OTAATest) error {
				ts.Device.IsDisabled = true
				return storage.UpdateDevice(context.Background(), storage.DB(), ts.Device)
			},
			RXInfo:     rxInfo,
			TXInfo:     txInfo,
			PHYPayload: jrPayload,
			Assert: []Assertion{
				AssertNoDownlinkFrame,
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertOTAATest(t, tst)
		})
	}
}

func (ts *OTAATestSuite) TestLW11() {
	assert := require.New(ts.T())

	conf := test.GetConfig()

	ts.DeviceProfile.SupportsClassB = false
	ts.DeviceProfile.SupportsClassC = false
	ts.DeviceProfile.MACVersion = "1.1.0"
	assert.NoError(storage.UpdateDeviceProfile(context.Background(), storage.DB(), ts.DeviceProfile))

	ts.Device.IsDisabled = false
	assert.NoError(storage.UpdateDevice(context.Background(), storage.DB(), ts.Device))

	rxInfo := gw.UplinkRXInfo{
		GatewayId: ts.Gateway.GatewayID[:],
		Context:   []byte{1, 2, 3, 4},
	}

	txInfo := gw.UplinkTXInfo{
		Frequency: 868100000,
	}
	assert.NoError(helpers.SetUplinkTXInfoDataRate(&txInfo, 0, band.Band()))

	jrPayload := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: lorawan.JoinRequest,
			Major: lorawan.LoRaWANR1,
		},
		MACPayload: &lorawan.JoinRequestPayload{
			JoinEUI:  lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
			DevEUI:   ts.Device.DevEUI,
			DevNonce: 258,
		},
	}
	assert.NoError(jrPayload.SetUplinkJoinMIC(ts.JoinAcceptKey))
	jrBytes, err := jrPayload.MarshalBinary()
	assert.NoError(err)

	jaPayload := lorawan.JoinAcceptPayload{
		JoinNonce: 197121,
		HomeNetID: conf.NetworkServer.NetID,
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
	assert.NoError(jaPHY.SetDownlinkJoinMIC(lorawan.JoinRequestType, lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}, lorawan.DevNonce(258), ts.JoinAcceptKey))
	assert.NoError(jaPHY.EncryptJoinAcceptPayload(ts.JoinAcceptKey))
	jaBytes, err := jaPHY.MarshalBinary()
	assert.NoError(err)
	assert.NoError(jaPHY.DecryptJoinAcceptPayload(ts.JoinAcceptKey))

	tests := []OTAATest{
		{
			Name:       "join-request accepted (rx1 + rx2)",
			RXInfo:     rxInfo,
			TXInfo:     txInfo,
			PHYPayload: jrPayload,
			JoinServerJoinAnsPayload: backend.JoinAnsPayload{
				BasePayloadResult: backend.BasePayloadResult{
					Result: backend.Result{
						ResultCode: backend.Success,
					},
				},
				PHYPayload: backend.HEXBytes(jaBytes),
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

			Assert: []Assertion{
				AssertJSJoinReqPayload(backend.JoinReqPayload{
					BasePayload: backend.BasePayload{
						ProtocolVersion: backend.ProtocolVersion1_0,
						SenderID:        "030201",
						ReceiverID:      "0102030405060708",
						MessageType:     backend.JoinReq,
					},
					MACVersion: "1.1.0",
					PHYPayload: backend.HEXBytes(jrBytes),
					DevEUI:     ts.Device.DevEUI,
					DLSettings: lorawan.DLSettings{
						OptNeg:      true,
						RX2DataRate: uint8(conf.NetworkServer.NetworkSettings.RX2DR),
						RX1DROffset: uint8(conf.NetworkServer.NetworkSettings.RX1DROffset),
					},
					RxDelay: conf.NetworkServer.NetworkSettings.RX1Delay,
				}),
				AssertDownlinkFrame(ts.Gateway.GatewayID, gw.DownlinkTXInfo{
					Frequency:  txInfo.Frequency,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							CodeRate:              "4/5",
							PolarizationInversion: true,
						},
					},
					Context: []byte{1, 2, 3, 4},
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(5 * time.Second),
						},
					},
				}, jaPHY),
				AssertDownlinkFrameSaved(ts.Gateway.GatewayID, ts.Device.DevEUI, uuid.Nil, gw.DownlinkTXInfo{
					Frequency:  txInfo.Frequency,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							CodeRate:              "4/5",
							PolarizationInversion: true,
						},
					},
					Context: []byte{1, 2, 3, 4},
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(5 * time.Second),
						},
					},
				}, jaPHY),
				AssertDownlinkFrameSaved(ts.Gateway.GatewayID, ts.Device.DevEUI, uuid.Nil, gw.DownlinkTXInfo{
					Frequency:  869525000,
					Power:      27,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							CodeRate:              "4/5",
							PolarizationInversion: true,
						},
					},
					Context: []byte{1, 2, 3, 4},
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(6 * time.Second),
						},
					},
				}, jaPHY),
				AssertDeviceSession(storage.DeviceSession{
					MACVersion:       "1.1.0",
					RoutingProfileID: ts.RoutingProfile.ID,
					DeviceProfileID:  ts.DeviceProfile.ID,
					ServiceProfileID: ts.ServiceProfile.ID,
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
					ExtraUplinkChannels:   map[int]loraband.Channel{},
					RX2Frequency:          band.Band().GetDefaults().RX2Frequency,
					NbTrans:               1,
					ReferenceAltitude:     5.6,
					MACCommandErrorCount:  make(map[lorawan.CID]int),
				}),
				AssertDeviceQueueItems([]storage.DeviceQueueItem{}),
				AssertDeviceMode(storage.DeviceModeA),
			},
		},
		{
			Name: "join-request accepted (session-keys encrypted with KEK) (rx1 + rx2)",
			BeforeFunc: func(*OTAATest) error {
				conf := test.GetConfig()
				conf.JoinServer.KEK.Set = []config.KEK{
					{
						Label: "010203",
						KEK:   "00000000000000000000000000000000",
					},
				}
				return uplink.Setup(conf)
			},
			TXInfo:     txInfo,
			RXInfo:     rxInfo,
			PHYPayload: jrPayload,
			JoinServerJoinAnsPayload: backend.JoinAnsPayload{
				BasePayloadResult: backend.BasePayloadResult{
					Result: backend.Result{
						ResultCode: backend.Success,
					},
				},
				PHYPayload: backend.HEXBytes(jaBytes),
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

			Assert: []Assertion{
				AssertJSJoinReqPayload(backend.JoinReqPayload{
					BasePayload: backend.BasePayload{
						ProtocolVersion: backend.ProtocolVersion1_0,
						SenderID:        "030201",
						ReceiverID:      "0102030405060708",
						MessageType:     backend.JoinReq,
					},
					MACVersion: "1.1.0",
					PHYPayload: backend.HEXBytes(jrBytes),
					DevEUI:     ts.Device.DevEUI,
					DLSettings: lorawan.DLSettings{
						OptNeg:      true,
						RX2DataRate: uint8(conf.NetworkServer.NetworkSettings.RX2DR),
						RX1DROffset: uint8(conf.NetworkServer.NetworkSettings.RX1DROffset),
					},
					RxDelay: conf.NetworkServer.NetworkSettings.RX1Delay,
				}),
				AssertDownlinkFrame(ts.Gateway.GatewayID, gw.DownlinkTXInfo{
					Frequency:  txInfo.Frequency,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							CodeRate:              "4/5",
							PolarizationInversion: true,
						},
					},
					Context: []byte{1, 2, 3, 4},
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(5 * time.Second),
						},
					},
				}, jaPHY),
				AssertDownlinkFrameSaved(ts.Gateway.GatewayID, ts.Device.DevEUI, uuid.Nil, gw.DownlinkTXInfo{
					Frequency:  txInfo.Frequency,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							CodeRate:              "4/5",
							PolarizationInversion: true,
						},
					},
					Context: []byte{1, 2, 3, 4},
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(5 * time.Second),
						},
					},
				}, jaPHY),
				AssertDownlinkFrameSaved(ts.Gateway.GatewayID, ts.Device.DevEUI, uuid.Nil, gw.DownlinkTXInfo{
					Frequency:  869525000,
					Power:      27,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							CodeRate:              "4/5",
							PolarizationInversion: true,
						},
					},
					Context: []byte{1, 2, 3, 4},
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(6 * time.Second),
						},
					},
				}, jaPHY),
				AssertDeviceSession(storage.DeviceSession{
					MACVersion:       "1.1.0",
					RoutingProfileID: ts.RoutingProfile.ID,
					DeviceProfileID:  ts.DeviceProfile.ID,
					ServiceProfileID: ts.ServiceProfile.ID,
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
					ExtraUplinkChannels:   map[int]loraband.Channel{},
					RX2Frequency:          band.Band().GetDefaults().RX2Frequency,
					NbTrans:               1,
					ReferenceAltitude:     5.6,
					MACCommandErrorCount:  make(map[lorawan.CID]int),
				}),
				AssertDeviceQueueItems([]storage.DeviceQueueItem{}),
				AssertDeviceMode(storage.DeviceModeA),
			},
		},
		{
			Name: "join-request accepted, Class-B supported",
			BeforeFunc: func(*OTAATest) error {
				ts.DeviceProfile.SupportsClassB = true
				ts.DeviceProfile.SupportsClassC = false
				return storage.UpdateDeviceProfile(context.Background(), storage.DB(), ts.DeviceProfile)
			},
			RXInfo:     rxInfo,
			TXInfo:     txInfo,
			PHYPayload: jrPayload,
			JoinServerJoinAnsPayload: backend.JoinAnsPayload{
				BasePayloadResult: backend.BasePayloadResult{
					Result: backend.Result{
						ResultCode: backend.Success,
					},
				},
				PHYPayload: backend.HEXBytes(jaBytes),
				NwkSKey: &backend.KeyEnvelope{
					AESKey: []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
				},
			},
			Assert: []Assertion{
				AssertDeviceMode(storage.DeviceModeA),
			},
		},
		{
			Name: "join-request accepted, Class-C supported",
			BeforeFunc: func(*OTAATest) error {
				ts.DeviceProfile.SupportsClassB = false
				ts.DeviceProfile.SupportsClassC = true
				return storage.UpdateDeviceProfile(context.Background(), storage.DB(), ts.DeviceProfile)
			},
			RXInfo:     rxInfo,
			TXInfo:     txInfo,
			PHYPayload: jrPayload,
			JoinServerJoinAnsPayload: backend.JoinAnsPayload{
				BasePayloadResult: backend.BasePayloadResult{
					Result: backend.Result{
						ResultCode: backend.Success,
					},
				},
				PHYPayload: backend.HEXBytes(jaBytes),
				NwkSKey: &backend.KeyEnvelope{
					AESKey: []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
				},
			},
			Assert: []Assertion{
				AssertDeviceMode(storage.DeviceModeC),
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertOTAATest(t, tst)
		})
	}
}

func TestOTAA(t *testing.T) {
	suite.Run(t, new(OTAATestSuite))
}
