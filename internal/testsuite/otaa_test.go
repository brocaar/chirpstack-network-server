package testsuite

import (
	"errors"
	"testing"

	commonPB "github.com/brocaar/loraserver/api/common"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/helpers"
	"github.com/brocaar/lorawan/backend"
	"github.com/brocaar/lorawan/band"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
)

type OTAATestSuite struct {
	IntegrationTestSuite
}

func (ts *OTAATestSuite) SetupSuite() {
	ts.DatabaseTestSuiteBase.SetupSuite()

	ts.AppKey = lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

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
	})

	ts.CreateGateway(storage.Gateway{
		GatewayID: lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
	})
}

func (ts *OTAATestSuite) TestLW10() {
	assert := require.New(ts.T())

	ts.DeviceProfile.MACVersion = "1.0.2"
	assert.NoError(storage.UpdateDeviceProfile(ts.DB(), ts.DeviceProfile))

	rxInfo := gw.UplinkRXInfo{
		GatewayId: ts.Gateway.GatewayID[:],
	}

	txInfo := gw.UplinkTXInfo{
		Frequency: 868100000,
	}
	assert.NoError(helpers.SetUplinkTXInfoDataRate(&txInfo, 0, config.C.NetworkServer.Band.Band))

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
	assert.NoError(jrPayload.SetUplinkJoinMIC(ts.AppKey))
	jrBytes, err := jrPayload.MarshalBinary()
	assert.NoError(err)

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
	assert.NoError(jaPHY.SetDownlinkJoinMIC(lorawan.JoinRequestType, lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}, lorawan.DevNonce(258), ts.AppKey))
	assert.NoError(jaPHY.EncryptJoinAcceptPayload(ts.AppKey))
	jaBytes, err := jaPHY.MarshalBinary()
	assert.NoError(err)
	assert.NoError(jaPHY.DecryptJoinAcceptPayload(ts.AppKey))

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
			Name:                          "join-server returns an error",
			RXInfo:                        rxInfo,
			TXInfo:                        txInfo,
			PHYPayload:                    jrPayload,
			JoinServerJoinAnsPayloadError: errors.New("invalid deveui"),
			ExpectedError:                 errors.New("join-request to join-server error: invalid deveui"),
		},
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
		},
		{
			Name:       "join-request accepted",
			RXInfo:     rxInfo,
			TXInfo:     txInfo,
			PHYPayload: jrPayload,
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
						RX2DataRate: uint8(config.C.NetworkServer.NetworkSettings.RX2DR),
						RX1DROffset: uint8(config.C.NetworkServer.NetworkSettings.RX1DROffset),
					},
					RxDelay: config.C.NetworkServer.NetworkSettings.RX1Delay,
				}),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
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
					ExtraUplinkChannels:   map[int]band.Channel{},
					UplinkGatewayHistory:  map[lorawan.EUI64]storage.UplinkGatewayHistory{},
					RX2Frequency:          config.C.NetworkServer.Band.Band.GetDefaults().RX2Frequency,
					NbTrans:               1,
					ReferenceAltitude:     5.6,
				}),
				AssertDeviceQueueItems([]storage.DeviceQueueItem{}),
			},
		},
		{
			Name: "join-request accepted + skip fcnt check set",
			BeforeFunc: func(*OTAATest) error {
				ts.Device.SkipFCntCheck = true
				return storage.UpdateDevice(ts.DB(), ts.Device)
			},
			RXInfo:     rxInfo,
			TXInfo:     txInfo,
			PHYPayload: jrPayload,
			JoinServerJoinAnsPayload: backend.JoinAnsPayload{
				PHYPayload: backend.HEXBytes(jaBytes),
				Result: backend.Result{
					ResultCode: backend.Success,
				},
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
						RX2DataRate: uint8(config.C.NetworkServer.NetworkSettings.RX2DR),
						RX1DROffset: uint8(config.C.NetworkServer.NetworkSettings.RX1DROffset),
					},
					RxDelay: config.C.NetworkServer.NetworkSettings.RX1Delay,
				}),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
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
					ExtraUplinkChannels:   map[int]band.Channel{},
					UplinkGatewayHistory:  map[lorawan.EUI64]storage.UplinkGatewayHistory{},
					RX2Frequency:          config.C.NetworkServer.Band.Band.GetDefaults().RX2Frequency,
					SkipFCntValidation:    true,
					NbTrans:               1,
					ReferenceAltitude:     5.6,
				}),
			},
		},
		{
			Name: "join-request accepted + cflist channels",
			BeforeFunc: func(*OTAATest) error {
				ts.Device.SkipFCntCheck = false
				return storage.UpdateDevice(ts.DB(), ts.Device)
			},
			RXInfo:        rxInfo,
			TXInfo:        txInfo,
			PHYPayload:    jrPayload,
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
						RX2DataRate: uint8(config.C.NetworkServer.NetworkSettings.RX2DR),
						RX1DROffset: uint8(config.C.NetworkServer.NetworkSettings.RX1DROffset),
					},
					RxDelay: config.C.NetworkServer.NetworkSettings.RX1Delay,
					CFList:  backend.HEXBytes(cFListB),
				}),
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

	ts.DeviceProfile.MACVersion = "1.1.0"
	assert.NoError(storage.UpdateDeviceProfile(ts.DB(), ts.DeviceProfile))

	rxInfo := gw.UplinkRXInfo{
		GatewayId: ts.Gateway.GatewayID[:],
	}

	txInfo := gw.UplinkTXInfo{
		Frequency: 868100000,
	}
	assert.NoError(helpers.SetUplinkTXInfoDataRate(&txInfo, 0, config.C.NetworkServer.Band.Band))

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
	assert.NoError(jrPayload.SetUplinkJoinMIC(ts.AppKey))
	jrBytes, err := jrPayload.MarshalBinary()
	assert.NoError(err)

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
	assert.NoError(jaPHY.SetDownlinkJoinMIC(lorawan.JoinRequestType, lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}, lorawan.DevNonce(258), ts.AppKey))
	assert.NoError(jaPHY.EncryptJoinAcceptPayload(ts.AppKey))
	jaBytes, err := jaPHY.MarshalBinary()
	assert.NoError(err)
	assert.NoError(jaPHY.DecryptJoinAcceptPayload(ts.AppKey))

	tests := []OTAATest{
		{
			Name:       "join-request accepted",
			RXInfo:     rxInfo,
			TXInfo:     txInfo,
			PHYPayload: jrPayload,
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
						RX2DataRate: uint8(config.C.NetworkServer.NetworkSettings.RX2DR),
						RX1DROffset: uint8(config.C.NetworkServer.NetworkSettings.RX1DROffset),
					},
					RxDelay: config.C.NetworkServer.NetworkSettings.RX1Delay,
				}),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
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
					ExtraUplinkChannels:   map[int]band.Channel{},
					UplinkGatewayHistory:  map[lorawan.EUI64]storage.UplinkGatewayHistory{},
					RX2Frequency:          config.C.NetworkServer.Band.Band.GetDefaults().RX2Frequency,
					NbTrans:               1,
					ReferenceAltitude:     5.6,
				}),
				AssertDeviceQueueItems([]storage.DeviceQueueItem{}),
			},
		},
		{
			Name: "join-request accepted (session-keys encrypted with KEK",
			BeforeFunc: func(*OTAATest) error {
				config.C.JoinServer.KEK.Set = []struct {
					Label string
					KEK   string `mapstructure:"kek"`
				}{
					{
						Label: "010203",
						KEK:   "00000000000000000000000000000000",
					},
				}
				return nil
			},
			TXInfo:     txInfo,
			RXInfo:     rxInfo,
			PHYPayload: jrPayload,
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
						RX2DataRate: uint8(config.C.NetworkServer.NetworkSettings.RX2DR),
						RX1DROffset: uint8(config.C.NetworkServer.NetworkSettings.RX1DROffset),
					},
					RxDelay: config.C.NetworkServer.NetworkSettings.RX1Delay,
				}),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
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
					ExtraUplinkChannels:   map[int]band.Channel{},
					UplinkGatewayHistory:  map[lorawan.EUI64]storage.UplinkGatewayHistory{},
					RX2Frequency:          config.C.NetworkServer.Band.Band.GetDefaults().RX2Frequency,
					NbTrans:               1,
					ReferenceAltitude:     5.6,
				}),
				AssertDeviceQueueItems([]storage.DeviceQueueItem{}),
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertOTAATest(t, tst)

			config.C.JoinServer.KEK.Set = nil
		})
	}
}

func TestOTAA(t *testing.T) {
	suite.Run(t, new(OTAATestSuite))
}
