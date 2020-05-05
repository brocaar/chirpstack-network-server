package testsuite

import (
	"errors"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-api/go/v3/nc"
	"github.com/brocaar/chirpstack-network-server/internal/band"
	"github.com/brocaar/chirpstack-network-server/internal/downlink"
	"github.com/brocaar/chirpstack-network-server/internal/helpers"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/chirpstack-network-server/internal/test"
	"github.com/brocaar/chirpstack-network-server/internal/uplink"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
	loraband "github.com/brocaar/lorawan/band"
)

type RejoinTestSuite struct {
	IntegrationTestSuite

	RXInfo gw.UplinkRXInfo
	TXInfo gw.UplinkTXInfo
}

func (ts *RejoinTestSuite) SetupSuite() {
	ts.IntegrationTestSuite.SetupSuite()
	assert := require.New(ts.T())

	ts.JoinAcceptKey = lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8}

	ts.CreateDeviceProfile(storage.DeviceProfile{
		MACVersion:   "1.1.0",
		RXDelay1:     3,
		RXDROffset1:  1,
		RXDataRate2:  5,
		SupportsJoin: true,
	})

	ts.CreateDevice(storage.Device{
		DevEUI: lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
	})

	ts.CreateDeviceSession(storage.DeviceSession{
		DevAddr:               lorawan.DevAddr{1, 2, 3, 4},
		JoinEUI:               lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
		SNwkSIntKey:           lorawan.AES128Key{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
		FNwkSIntKey:           lorawan.AES128Key{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 2},
		NwkSEncKey:            lorawan.AES128Key{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 3},
		MACVersion:            "1.1.0",
		ExtraUplinkChannels:   make(map[int]loraband.Channel),
		RX2Frequency:          869525000,
		NbTrans:               1,
		EnabledUplinkChannels: []int{0, 1, 2},
	})

	ts.CreateGateway(storage.Gateway{
		GatewayID: lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
	})

	ts.RXInfo = gw.UplinkRXInfo{
		GatewayId: ts.Gateway.GatewayID[:],
		Location:  &common.Location{},
	}

	ts.TXInfo = gw.UplinkTXInfo{
		Frequency: 868100000,
	}
	assert.NoError(helpers.SetUplinkTXInfoDataRate(&ts.TXInfo, 0, band.Band()))
}

func (ts *RejoinTestSuite) SetupTest() {
	ts.IntegrationTestSuite.SetupTest()
	assert := require.New(ts.T())

	conf := test.GetConfig()
	conf.NetworkServer.NetworkSettings.RX2DR = 3
	conf.NetworkServer.NetworkSettings.RX1DROffset = 2
	conf.NetworkServer.NetworkSettings.RX1Delay = 1
	conf.JoinServer.KEK.Set = []struct {
		Label string `mapstructure:"label"`
		KEK   string `mapstructure:"kek"`
	}{
		{
			Label: "010203",
			KEK:   "00000000000000000000000000000000",
		},
	}

	assert.NoError(uplink.Setup(conf))
	assert.NoError(downlink.Setup(conf))

	band.Band().AddChannel(867100000, 0, 5)
	band.Band().AddChannel(867300000, 0, 5)
	band.Band().AddChannel(867500000, 0, 5)

}

func (ts *RejoinTestSuite) TestRejoinType0() {
	assert := require.New(ts.T())

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
	assert.NoError(err)

	jrPHY := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: lorawan.RejoinRequest,
			Major: lorawan.LoRaWANR1,
		},
		MACPayload: &lorawan.RejoinRequestType02Payload{
			RejoinType: lorawan.RejoinRequestType0,
			NetID:      lorawan.NetID{3, 2, 1},
			DevEUI:     ts.Device.DevEUI,
			RJCount0:   123,
		},
	}
	assert.NoError(jrPHY.SetUplinkJoinMIC(ts.DeviceSession.SNwkSIntKey))
	jrPHYBytes, err := jrPHY.MarshalBinary()
	assert.NoError(err)

	jaPHY := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: lorawan.JoinAccept,
			Major: lorawan.LoRaWANR1,
		},
		MACPayload: &lorawan.JoinAcceptPayload{},
	}
	assert.NoError(jaPHY.EncryptJoinAcceptPayload(ts.JoinAcceptKey))
	jaPHYBytes, err := jaPHY.MarshalBinary()
	assert.NoError(err)
	assert.NoError(jaPHY.DecryptJoinAcceptPayload(ts.JoinAcceptKey))

	tests := []RejoinTest{
		{
			Name:          "valid rejoin-request",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload:    jrPHY,
			JoinServerRejoinAnsPayload: backend.RejoinAnsPayload{
				PHYPayload: backend.HEXBytes(jaPHYBytes),
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
			Assert: []Assertion{
				AssertJSRejoinReqPayload(backend.RejoinReqPayload{
					BasePayload: backend.BasePayload{
						ProtocolVersion: backend.ProtocolVersion1_0,
						SenderID:        "030201",
						ReceiverID:      "0807060504030201",
						MessageType:     backend.RejoinReq,
					},
					MACVersion: "1.1.0",
					PHYPayload: backend.HEXBytes(jrPHYBytes),
					DevEUI:     ts.Device.DevEUI,
					DLSettings: lorawan.DLSettings{
						OptNeg:      true,
						RX2DataRate: 3,
						RX1DROffset: 2,
					},
					RxDelay: 1,
					CFList:  cFListB,
				}),
				AssertDownlinkFrame(ts.Gateway.GatewayID, gw.DownlinkTXInfo{
					Frequency:  868100000,
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
					Timing: gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(5 * time.Second),
						},
					},
				}, jaPHY),
				AssertDeviceSession(storage.DeviceSession{
					RoutingProfileID:      ts.RoutingProfile.ID,
					ServiceProfileID:      ts.ServiceProfile.ID,
					DeviceProfileID:       ts.DeviceProfile.ID,
					DevEUI:                ts.Device.DevEUI,
					JoinEUI:               lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
					SNwkSIntKey:           lorawan.AES128Key{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
					FNwkSIntKey:           lorawan.AES128Key{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 2},
					NwkSEncKey:            lorawan.AES128Key{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 3},
					MACVersion:            "1.1.0",
					ExtraUplinkChannels:   make(map[int]loraband.Channel),
					RX2Frequency:          869525000,
					NbTrans:               1,
					EnabledUplinkChannels: []int{0, 1, 2},
					RejoinCount0:          124,
					PendingRejoinDeviceSession: &storage.DeviceSession{
						RoutingProfileID: ts.RoutingProfile.ID,
						ServiceProfileID: ts.ServiceProfile.ID,
						DeviceProfileID:  ts.DeviceProfile.ID,
						DevEUI:           ts.Device.DevEUI,
						JoinEUI:          lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
						SNwkSIntKey:      lorawan.AES128Key{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 1},
						FNwkSIntKey:      lorawan.AES128Key{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2},
						NwkSEncKey:       lorawan.AES128Key{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 3},
						AppSKeyEvelope: &storage.KeyEnvelope{
							AESKey: []byte{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 4},
						},
						MACVersion: "1.1.0",
						ExtraUplinkChannels: map[int]loraband.Channel{
							3: {Frequency: 867100000, MaxDR: 5},
							4: {Frequency: 867300000, MaxDR: 5},
							5: {Frequency: 867500000, MaxDR: 5},
						},
						RX2Frequency:          869525000,
						NbTrans:               1,
						EnabledUplinkChannels: []int{0, 1, 2, 3, 4, 5},
						RXDelay:               1,
						RX1DROffset:           2,
						RX2DR:                 3,
						MACCommandErrorCount:  make(map[lorawan.CID]int),
					},
					MACCommandErrorCount: make(map[lorawan.CID]int),
				}),
				AssertNCHandleUplinkMetaDataRequest(nc.HandleUplinkMetaDataRequest{
					DevEui:              ts.Device.DevEUI[:],
					TxInfo:              &ts.TXInfo,
					RxInfo:              []*gw.UplinkRXInfo{&ts.RXInfo},
					MessageType:         nc.MType_REJOIN_REQUEST,
					PhyPayloadByteCount: 19,
				}),
			},
		},
		{
			Name:          "valid rejoin-request with KEK",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload:    jrPHY,
			JoinServerRejoinAnsPayload: backend.RejoinAnsPayload{
				PHYPayload: backend.HEXBytes(jaPHYBytes),
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
			Assert: []Assertion{
				AssertJSRejoinReqPayload(backend.RejoinReqPayload{
					BasePayload: backend.BasePayload{
						ProtocolVersion: backend.ProtocolVersion1_0,
						SenderID:        "030201",
						ReceiverID:      "0807060504030201",
						MessageType:     backend.RejoinReq,
					},
					MACVersion: "1.1.0",
					PHYPayload: backend.HEXBytes(jrPHYBytes),
					DevEUI:     ts.Device.DevEUI,
					DLSettings: lorawan.DLSettings{
						OptNeg:      true,
						RX2DataRate: 3,
						RX1DROffset: 2,
					},
					RxDelay: 1,
					CFList:  cFListB,
				}),
				AssertDownlinkFrame(ts.Gateway.GatewayID, gw.DownlinkTXInfo{
					Frequency:  868100000,
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
					Timing: gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(5 * time.Second),
						},
					},
				}, jaPHY),
				AssertDeviceSession(storage.DeviceSession{
					RoutingProfileID:      ts.RoutingProfile.ID,
					ServiceProfileID:      ts.ServiceProfile.ID,
					DeviceProfileID:       ts.DeviceProfile.ID,
					DevEUI:                ts.Device.DevEUI,
					JoinEUI:               lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
					SNwkSIntKey:           lorawan.AES128Key{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
					FNwkSIntKey:           lorawan.AES128Key{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 2},
					NwkSEncKey:            lorawan.AES128Key{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 3},
					MACVersion:            "1.1.0",
					ExtraUplinkChannels:   make(map[int]loraband.Channel),
					RX2Frequency:          869525000,
					NbTrans:               1,
					EnabledUplinkChannels: []int{0, 1, 2},
					RejoinCount0:          124,
					PendingRejoinDeviceSession: &storage.DeviceSession{
						RoutingProfileID: ts.RoutingProfile.ID,
						ServiceProfileID: ts.ServiceProfile.ID,
						DeviceProfileID:  ts.DeviceProfile.ID,
						DevEUI:           ts.Device.DevEUI,
						JoinEUI:          lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
						SNwkSIntKey:      lorawan.AES128Key{88, 148, 152, 153, 48, 146, 207, 219, 95, 210, 224, 42, 199, 81, 11, 241},
						FNwkSIntKey:      lorawan.AES128Key{83, 127, 138, 174, 137, 108, 121, 224, 21, 209, 2, 208, 98, 134, 53, 78},
						NwkSEncKey:       lorawan.AES128Key{152, 152, 40, 60, 79, 102, 235, 108, 111, 213, 22, 88, 130, 4, 108, 64},
						AppSKeyEvelope: &storage.KeyEnvelope{
							KEKLabel: "lora-app-server",
							AESKey:   []byte{248, 215, 201, 250, 55, 176, 209, 198, 53, 78, 109, 184, 225, 157, 157, 122, 180, 229, 199, 88, 30, 159, 30, 32},
						},
						MACVersion: "1.1.0",
						ExtraUplinkChannels: map[int]loraband.Channel{
							3: {Frequency: 867100000, MaxDR: 5},
							4: {Frequency: 867300000, MaxDR: 5},
							5: {Frequency: 867500000, MaxDR: 5},
						},
						RX2Frequency:          869525000,
						NbTrans:               1,
						EnabledUplinkChannels: []int{0, 1, 2, 3, 4, 5},
						RXDelay:               1,
						RX1DROffset:           2,
						RX2DR:                 3,
						MACCommandErrorCount:  make(map[lorawan.CID]int),
					},
					MACCommandErrorCount: make(map[lorawan.CID]int),
				}),
			},
		},
		{
			Name: "invalid rejoin-counter",
			BeforeFunc: func(tst *RejoinTest) error {
				tst.DeviceSession.RejoinCount0 = 124
				return nil
			},
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload:    jrPHY,
			ExpectedError: errors.New("invalid RJcount0"),
		},
		{
			Name:                            "join-server returns error",
			DeviceSession:                   *ts.DeviceSession,
			TXInfo:                          ts.TXInfo,
			RXInfo:                          ts.RXInfo,
			PHYPayload:                      jrPHY,
			JoinServerRejoinAnsPayloadError: errors.New("boom"),
			ExpectedError:                   errors.New("rejoin-request to join-server error: boom"),
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertRejoinTest(t, tst)
		})
	}
}

func (ts *RejoinTestSuite) TestRejoinType2() {
	assert := require.New(ts.T())
	conf := test.GetConfig()

	jrPHY := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: lorawan.RejoinRequest,
			Major: lorawan.LoRaWANR1,
		},
		MACPayload: &lorawan.RejoinRequestType02Payload{
			RejoinType: lorawan.RejoinRequestType2,
			NetID:      lorawan.NetID{3, 2, 1},
			DevEUI:     ts.Device.DevEUI,
			RJCount0:   123,
		},
	}
	assert.NoError(jrPHY.SetUplinkJoinMIC(ts.DeviceSession.SNwkSIntKey))
	jrPHYBytes, err := jrPHY.MarshalBinary()
	assert.NoError(err)

	jsIntKey := lorawan.AES128Key{8, 7, 6, 5, 4, 3, 2, 1, 8, 7, 6, 5, 4, 3, 2, 1}
	jaPHY := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: lorawan.JoinAccept,
			Major: lorawan.LoRaWANR1,
		},
		MACPayload: &lorawan.JoinAcceptPayload{
			JoinNonce: 12345,
			HomeNetID: conf.NetworkServer.NetID,
			DLSettings: lorawan.DLSettings{
				RX2DataRate: 2,
				RX1DROffset: 1,
			},
			DevAddr: lorawan.DevAddr{1, 2, 3, 4},
			RXDelay: 3,
		},
	}
	assert.NoError(jaPHY.SetDownlinkJoinMIC(lorawan.RejoinRequestType2, ts.Device.DevEUI, lorawan.DevNonce(123), jsIntKey))
	assert.NoError(jaPHY.EncryptJoinAcceptPayload(ts.JoinAcceptKey))
	jaBytes, err := jaPHY.MarshalBinary()
	assert.NoError(err)
	assert.NoError(jaPHY.DecryptJoinAcceptPayload(ts.JoinAcceptKey))

	tests := []RejoinTest{
		{
			Name:          "valid rejoin-request",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload:    jrPHY,
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
			Assert: []Assertion{
				AssertJSRejoinReqPayload(backend.RejoinReqPayload{
					BasePayload: backend.BasePayload{
						ProtocolVersion: backend.ProtocolVersion1_0,
						SenderID:        "030201",
						ReceiverID:      "0807060504030201",
						MessageType:     backend.RejoinReq,
					},
					MACVersion: "1.1.0",
					PHYPayload: backend.HEXBytes(jrPHYBytes),
					DevEUI:     ts.Device.DevEUI,
					DLSettings: lorawan.DLSettings{
						OptNeg:      true,
						RX2DataRate: 3,
						RX1DROffset: 2,
					},
					RxDelay: 1,
				}),
				AssertDownlinkFrame(ts.Gateway.GatewayID, gw.DownlinkTXInfo{
					Frequency:  868100000,
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
					Timing: gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(5 * time.Second),
						},
					},
				}, jaPHY),
				AssertDeviceSession(storage.DeviceSession{
					RoutingProfileID:      ts.RoutingProfile.ID,
					ServiceProfileID:      ts.ServiceProfile.ID,
					DeviceProfileID:       ts.DeviceProfile.ID,
					DevEUI:                ts.Device.DevEUI,
					JoinEUI:               lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
					SNwkSIntKey:           lorawan.AES128Key{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
					FNwkSIntKey:           lorawan.AES128Key{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 2},
					NwkSEncKey:            lorawan.AES128Key{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 3},
					MACVersion:            "1.1.0",
					ExtraUplinkChannels:   make(map[int]loraband.Channel),
					RX2Frequency:          869525000,
					NbTrans:               1,
					EnabledUplinkChannels: []int{0, 1, 2},
					RejoinCount0:          124,
					PendingRejoinDeviceSession: &storage.DeviceSession{
						RoutingProfileID: ts.RoutingProfile.ID,
						ServiceProfileID: ts.ServiceProfile.ID,
						DeviceProfileID:  ts.DeviceProfile.ID,
						DevEUI:           ts.Device.DevEUI,
						JoinEUI:          lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
						SNwkSIntKey:      lorawan.AES128Key{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 1},
						FNwkSIntKey:      lorawan.AES128Key{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2},
						NwkSEncKey:       lorawan.AES128Key{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 3},
						AppSKeyEvelope: &storage.KeyEnvelope{
							AESKey: []byte{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 4},
						},
						MACVersion:            "1.1.0",
						ExtraUplinkChannels:   make(map[int]loraband.Channel),
						RX2Frequency:          869525000,
						NbTrans:               1,
						EnabledUplinkChannels: []int{0, 1, 2},
						MACCommandErrorCount:  make(map[lorawan.CID]int),
					},
					MACCommandErrorCount: make(map[lorawan.CID]int),
				}),
			},
		},
		{
			Name:          "valid rejoin-request with KEK",
			DeviceSession: *ts.DeviceSession,
			TXInfo:        ts.TXInfo,
			RXInfo:        ts.RXInfo,
			PHYPayload:    jrPHY,
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
			Assert: []Assertion{
				AssertJSRejoinReqPayload(backend.RejoinReqPayload{
					BasePayload: backend.BasePayload{
						ProtocolVersion: backend.ProtocolVersion1_0,
						SenderID:        "030201",
						ReceiverID:      "0807060504030201",
						MessageType:     backend.RejoinReq,
					},
					MACVersion: "1.1.0",
					PHYPayload: backend.HEXBytes(jrPHYBytes),
					DevEUI:     ts.Device.DevEUI,
					DLSettings: lorawan.DLSettings{
						OptNeg:      true,
						RX2DataRate: 3,
						RX1DROffset: 2,
					},
					RxDelay: 1,
				}),
				AssertDownlinkFrame(ts.Gateway.GatewayID, gw.DownlinkTXInfo{
					Frequency:  868100000,
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
					Timing: gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(5 * time.Second),
						},
					},
				}, jaPHY),
				AssertDeviceSession(storage.DeviceSession{
					RoutingProfileID:      ts.RoutingProfile.ID,
					ServiceProfileID:      ts.ServiceProfile.ID,
					DeviceProfileID:       ts.DeviceProfile.ID,
					DevEUI:                ts.Device.DevEUI,
					JoinEUI:               lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
					SNwkSIntKey:           lorawan.AES128Key{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
					FNwkSIntKey:           lorawan.AES128Key{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 2},
					NwkSEncKey:            lorawan.AES128Key{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 3},
					MACVersion:            "1.1.0",
					ExtraUplinkChannels:   make(map[int]loraband.Channel),
					RX2Frequency:          869525000,
					NbTrans:               1,
					EnabledUplinkChannels: []int{0, 1, 2},
					RejoinCount0:          124,
					PendingRejoinDeviceSession: &storage.DeviceSession{
						RoutingProfileID: ts.RoutingProfile.ID,
						ServiceProfileID: ts.ServiceProfile.ID,
						DeviceProfileID:  ts.DeviceProfile.ID,
						DevEUI:           ts.Device.DevEUI,
						JoinEUI:          lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
						SNwkSIntKey:      lorawan.AES128Key{88, 148, 152, 153, 48, 146, 207, 219, 95, 210, 224, 42, 199, 81, 11, 241},
						FNwkSIntKey:      lorawan.AES128Key{83, 127, 138, 174, 137, 108, 121, 224, 21, 209, 2, 208, 98, 134, 53, 78},
						NwkSEncKey:       lorawan.AES128Key{152, 152, 40, 60, 79, 102, 235, 108, 111, 213, 22, 88, 130, 4, 108, 64},
						AppSKeyEvelope: &storage.KeyEnvelope{
							KEKLabel: "lora-app-server",
							AESKey:   []byte{248, 215, 201, 250, 55, 176, 209, 198, 53, 78, 109, 184, 225, 157, 157, 122, 180, 229, 199, 88, 30, 159, 30, 32},
						},
						MACVersion:            "1.1.0",
						RX2Frequency:          869525000,
						NbTrans:               1,
						EnabledUplinkChannels: []int{0, 1, 2},
						ExtraUplinkChannels:   make(map[int]loraband.Channel),
						MACCommandErrorCount:  make(map[lorawan.CID]int),
					},
					MACCommandErrorCount: make(map[lorawan.CID]int),
				}),
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertRejoinTest(t, tst)
		})
	}
}

func TestRejoinRequest(t *testing.T) {
	suite.Run(t, new(RejoinTestSuite))
}
