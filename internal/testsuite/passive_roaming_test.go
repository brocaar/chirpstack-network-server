package testsuite

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofrs/uuid"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/chirpstack-api/go/v3/as"
	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	roamingapi "github.com/brocaar/chirpstack-network-server/v3/internal/api/roaming"
	"github.com/brocaar/chirpstack-network-server/v3/internal/backend/joinserver"
	"github.com/brocaar/chirpstack-network-server/v3/internal/band"
	"github.com/brocaar/chirpstack-network-server/v3/internal/config"
	"github.com/brocaar/chirpstack-network-server/v3/internal/helpers"
	"github.com/brocaar/chirpstack-network-server/v3/internal/roaming"
	"github.com/brocaar/chirpstack-network-server/v3/internal/storage"
	"github.com/brocaar/chirpstack-network-server/v3/internal/test"
	"github.com/brocaar/chirpstack-network-server/v3/internal/uplink"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
)

// PassiveRoamingFNSTestSuite contains the tests from the fNS POV.
// This tests the receiving of uplinks by gateways and forwarding these to the
// hNS of these devices. It also tests downlinks received by the hNS and
// forwarding these to the gateway.
type PassiveRoamingFNSTestSuite struct {
	IntegrationTestSuite

	hnsServer   *httptest.Server
	hnsRequest  [][]byte
	hnsResponse [][]byte

	jsServer   *httptest.Server
	jsRequest  [][]byte
	jsResponse [][]byte

	rxInfo gw.UplinkRXInfo
	txInfo gw.UplinkTXInfo
}

func (ts *PassiveRoamingFNSTestSuite) SetupTest() {
	ts.IntegrationTestSuite.SetupTest()

	ts.hnsRequest = nil
	ts.hnsResponse = nil
	ts.jsRequest = nil
	ts.jsResponse = nil
}

func (ts *PassiveRoamingFNSTestSuite) SetupSuite() {
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

	ts.rxInfo = gw.UplinkRXInfo{
		GatewayId: ts.Gateway.GatewayID[:],
		LoraSnr:   7,
		Rssi:      6,
		Location: &common.Location{
			Latitude:  1,
			Longitude: 2,
			Altitude:  3,
		},

		Context: []byte{1, 2, 3, 4},
	}

	ts.txInfo = gw.UplinkTXInfo{
		Frequency: 868100000,
	}
	assert.NoError(helpers.SetUplinkTXInfoDataRate(&ts.txInfo, 1, band.Band()))

	// setup hNS endpoint
	ts.hnsServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := ioutil.ReadAll(r.Body)
		ts.hnsRequest = append(ts.hnsRequest, b)
		w.Write(ts.hnsResponse[0])
		ts.hnsResponse = ts.hnsResponse[1:]
	}))

	// setup JS endpoint
	ts.jsServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := ioutil.ReadAll(r.Body)
		ts.jsRequest = append(ts.jsRequest, b)
		w.Write(ts.jsResponse[0])
	}))

	// configure default JS
	conf := test.GetConfig()
	conf.JoinServer.Default.Server = ts.jsServer.URL
	assert.NoError(joinserver.Setup(conf))

	// configure passive-roaming agreement
	conf.Roaming.Servers = []config.RoamingServer{
		{
			NetID:                  lorawan.NetID{6, 6, 6},
			Async:                  false,
			PassiveRoaming:         true,
			PassiveRoamingLifetime: time.Minute,
			Server:                 ts.hnsServer.URL,
		},
	}
	assert.NoError(roaming.Setup(conf))
}

func (ts *PassiveRoamingFNSTestSuite) TearDownSuite() {
	ts.jsServer.Close()
	ts.hnsServer.Close()
}

func (ts *PassiveRoamingFNSTestSuite) TestJoinRequest() {
	assert := require.New(ts.T())
	dlFreq1 := 868.1
	dlFreq2 := 868.2
	classMode := "A"
	dataRate1 := 1
	dataRate2 := 2
	rxDelay1 := 5
	lifetime := 60
	gwCnt := 1
	devEUI := lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1}

	// join-request phypayload
	phy := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: lorawan.JoinRequest,
			Major: lorawan.LoRaWANR1,
		},
		MACPayload: &lorawan.JoinRequestPayload{
			JoinEUI:  lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
			DevEUI:   devEUI,
			DevNonce: 123,
		},
	}
	phyB, err := phy.MarshalBinary()
	assert.NoError(err)

	// JS HomeNSAns
	homeNSAns := backend.HomeNSAnsPayload{
		BasePayloadResult: backend.BasePayloadResult{
			Result: backend.Result{
				ResultCode: backend.Success,
			},
		},
		HNetID: lorawan.NetID{6, 6, 6},
	}
	homeNSAnsB, err := json.Marshal(homeNSAns)
	assert.NoError(err)

	// ULToken
	ulTokenB, err := proto.Marshal(&ts.rxInfo)
	assert.NoError(err)

	// hNS PRStartAns
	prStartAns := backend.PRStartAnsPayload{
		BasePayloadResult: backend.BasePayloadResult{
			Result: backend.Result{
				ResultCode: backend.Success,
			},
		},
		PHYPayload: backend.HEXBytes{1, 2, 3, 4},
		Lifetime:   &lifetime,
		NwkSKey: &backend.KeyEnvelope{
			AESKey: backend.HEXBytes{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
		},
		DLMetaData: &backend.DLMetaData{
			DLFreq1:   &dlFreq1,
			DLFreq2:   &dlFreq2,
			RXDelay1:  &rxDelay1,
			ClassMode: &classMode,
			DataRate1: &dataRate1,
			DataRate2: &dataRate2,
			GWInfo: []backend.GWInfoElement{
				{
					ID:      backend.HEXBytes(ts.Gateway.GatewayID[:]),
					ULToken: backend.HEXBytes(ulTokenB),
				},
			},
		},
	}
	prStartAnsB, err := json.Marshal(prStartAns)
	assert.NoError(err)

	ts.T().Run("private gateway", func(t *testing.T) {
		assert := require.New(t)
		ts.ServiceProfile.GwsPrivate = true
		assert.NoError(storage.UpdateServiceProfile(context.Background(), storage.DB(), ts.ServiceProfile))

		// "send" uplink
		assert.NoError(uplink.HandleUplinkFrame(context.Background(), gw.UplinkFrame{
			RxInfo:     &ts.rxInfo,
			TxInfo:     &ts.txInfo,
			PhyPayload: phyB,
		}))

		// no requests are made
		assert.Len(ts.jsRequest, 0)

		ts.ServiceProfile.GwsPrivate = false
		assert.NoError(storage.UpdateServiceProfile(context.Background(), storage.DB(), ts.ServiceProfile))

		// Make sure that the de-duplication lock is flushed
		storage.RedisClient().FlushAll(context.Background())
	})

	ts.T().Run("success", func(t *testing.T) {
		assert := require.New(t)

		ts.jsResponse = [][]byte{homeNSAnsB}
		ts.hnsResponse = [][]byte{prStartAnsB}

		// "send" uplink
		assert.NoError(uplink.HandleUplinkFrame(context.Background(), gw.UplinkFrame{
			RxInfo:     &ts.rxInfo,
			TxInfo:     &ts.txInfo,
			PhyPayload: phyB,
		}))

		// validate JS HomeNSReq request
		rssi := 6
		snr := float64(7)
		lat := float64(1)
		lon := float64(2)
		var homeNSReq backend.HomeNSReqPayload
		assert.NoError(json.Unmarshal(ts.jsRequest[0], &homeNSReq))
		assert.NotEqual(0, homeNSReq.TransactionID)
		homeNSReq.TransactionID = 0
		assert.Equal(backend.HomeNSReqPayload{
			BasePayload: backend.BasePayload{
				ProtocolVersion: "1.0",
				SenderID:        "030201",
				ReceiverID:      "0102030405060708",
				MessageType:     backend.HomeNSReq,
			},
			DevEUI: lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
		}, homeNSReq)

		// validate hNS PRStartReq
		var prStartReq backend.PRStartReqPayload
		assert.NoError(json.Unmarshal(ts.hnsRequest[0], &prStartReq))
		assert.NotEqual(0, prStartReq.TransactionID)
		var nilTime time.Time
		assert.False(time.Time(prStartReq.ULMetaData.RecvTime).Equal(nilTime))
		prStartReq.ULMetaData.RecvTime = backend.ISO8601Time(nilTime)
		prStartReq.TransactionID = 0
		assert.Equal(backend.PRStartReqPayload{
			BasePayload: backend.BasePayload{
				ProtocolVersion: "1.0",
				SenderID:        "030201",
				ReceiverID:      "060606",
				MessageType:     backend.PRStartReq,
			},
			PHYPayload: backend.HEXBytes(phyB),
			ULMetaData: backend.ULMetaData{
				DevEUI:   &devEUI,
				DataRate: &dataRate1,
				RFRegion: "EU868",
				ULFreq:   &dlFreq1,
				GWCnt:    &gwCnt,
				GWInfo: []backend.GWInfoElement{
					{
						ID:        backend.HEXBytes(ts.Gateway.GatewayID[:]),
						RSSI:      &rssi,
						SNR:       &snr,
						Lat:       &lat,
						Lon:       &lon,
						ULToken:   backend.HEXBytes(ulTokenB),
						DLAllowed: true,
					},
				},
			},
		}, prStartReq)

		// validate published downlink
		downlinkFrame := <-ts.GWBackend.TXPacketChan
		assert.Equal(ts.Gateway.GatewayID[:], downlinkFrame.GetGatewayId())
		assert.Equal(&gw.DownlinkFrameItem{
			PhyPayload: []byte{1, 2, 3, 4},
			TxInfo: &gw.DownlinkTXInfo{
				Frequency:  868100000,
				Power:      14,
				Modulation: common.Modulation_LORA,
				ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
					LoraModulationInfo: &gw.LoRaModulationInfo{
						Bandwidth:             125,
						SpreadingFactor:       11,
						CodeRate:              "4/5",
						PolarizationInversion: true,
					},
				},
				Timing: gw.DownlinkTiming_DELAY,
				TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
					DelayTimingInfo: &gw.DelayTimingInfo{
						Delay: ptypes.DurationProto(time.Second * 5),
					},
				},
				Context: []byte{1, 2, 3, 4},
			},
		}, downlinkFrame.Items[0])
		assert.Equal(&gw.DownlinkFrameItem{
			PhyPayload: []byte{1, 2, 3, 4},
			TxInfo: &gw.DownlinkTXInfo{
				Frequency:  868200000,
				Power:      14,
				Modulation: common.Modulation_LORA,
				ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
					LoraModulationInfo: &gw.LoRaModulationInfo{
						Bandwidth:             125,
						SpreadingFactor:       10,
						CodeRate:              "4/5",
						PolarizationInversion: true,
					},
				},
				Timing: gw.DownlinkTiming_DELAY,
				TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
					DelayTimingInfo: &gw.DelayTimingInfo{
						Delay: ptypes.DurationProto(time.Second * 6),
					},
				},
				Context: []byte{1, 2, 3, 4},
			},
		}, downlinkFrame.Items[1])
	})
}

func (ts *PassiveRoamingFNSTestSuite) TestDataStateless() {
	assert := require.New(ts.T())

	dataRate1 := 1
	ulFreq := 868.1
	gwCnt := 1

	// uplink phypayload
	devAddr := lorawan.DevAddr{1, 2, 3, 4}
	devAddr.SetAddrPrefix(lorawan.NetID{6, 6, 6})
	phy := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: lorawan.UnconfirmedDataUp,
			Major: lorawan.LoRaWANR1,
		},
		MACPayload: &lorawan.MACPayload{
			FHDR: lorawan.FHDR{
				DevAddr: devAddr,
				FCnt:    10,
			},
		},
	}
	phyB, err := phy.MarshalBinary()
	assert.NoError(err)

	// hNS PRStartAns
	prStartAns := backend.PRStartAnsPayload{
		BasePayloadResult: backend.BasePayloadResult{
			Result: backend.Result{
				ResultCode: backend.Success,
			},
		},
	}
	prStartAnsB, err := json.Marshal(prStartAns)
	assert.NoError(err)

	// ulToken
	ulTokenB, err := proto.Marshal(&ts.rxInfo)
	assert.NoError(err)

	ts.T().Run("private gateway", func(t *testing.T) {
		assert := require.New(t)
		ts.ServiceProfile.GwsPrivate = true
		assert.NoError(storage.UpdateServiceProfile(context.Background(), storage.DB(), ts.ServiceProfile))

		// "send" uplink
		assert.NoError(uplink.HandleUplinkFrame(context.Background(), gw.UplinkFrame{
			RxInfo:     &ts.rxInfo,
			TxInfo:     &ts.txInfo,
			PhyPayload: phyB,
		}))

		// no requests are made
		assert.Len(ts.hnsRequest, 0)

		ts.ServiceProfile.GwsPrivate = false
		assert.NoError(storage.UpdateServiceProfile(context.Background(), storage.DB(), ts.ServiceProfile))

		// Make sure that the de-duplication lock is flushed
		storage.RedisClient().FlushAll(context.Background())
	})

	ts.T().Run("success", func(t *testing.T) {
		assert := require.New(t)

		ts.hnsResponse = [][]byte{prStartAnsB}

		// "send" uplink
		assert.NoError(uplink.HandleUplinkFrame(context.Background(), gw.UplinkFrame{
			RxInfo:     &ts.rxInfo,
			TxInfo:     &ts.txInfo,
			PhyPayload: phyB,
		}))

		// validate hNS PRStartReq
		rssi := 6
		snr := float64(7)
		lat := float64(1)
		lon := float64(2)
		var prStartReq backend.PRStartReqPayload
		assert.NoError(json.Unmarshal(ts.hnsRequest[0], &prStartReq))
		assert.NotEqual(0, prStartReq.TransactionID)
		prStartReq.TransactionID = 0
		var nilTime time.Time
		assert.False(time.Time(prStartReq.ULMetaData.RecvTime).Equal(nilTime))
		prStartReq.ULMetaData.RecvTime = backend.ISO8601Time(nilTime)
		assert.Equal(backend.PRStartReqPayload{
			BasePayload: backend.BasePayload{
				ProtocolVersion: "1.0",
				SenderID:        "030201",
				ReceiverID:      "060606",
				MessageType:     backend.PRStartReq,
			},
			PHYPayload: backend.HEXBytes(phyB),
			ULMetaData: backend.ULMetaData{
				DataRate: &dataRate1,
				RFRegion: "EU868",
				ULFreq:   &ulFreq,
				GWCnt:    &gwCnt,
				GWInfo: []backend.GWInfoElement{
					{
						ID:        backend.HEXBytes(ts.Gateway.GatewayID[:]),
						RSSI:      &rssi,
						SNR:       &snr,
						Lat:       &lat,
						Lon:       &lon,
						ULToken:   backend.HEXBytes(ulTokenB),
						DLAllowed: true,
					},
				},
			},
		}, prStartReq)

		// validate no session has been stored
		sess, err := storage.GetPassiveRoamingDeviceSessionsForDevAddr(context.Background(), devAddr)
		assert.NoError(err)
		assert.Len(sess, 0)
	})
}

func (ts *PassiveRoamingFNSTestSuite) TestDataStatefull() {
	assert := require.New(ts.T())

	dataRate1 := 1
	ulFreq := 868.1
	gwCnt := 1
	lifetime := 300
	fCntUp := uint32(32)
	devEUI := lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1}

	// uplink phypayload
	devAddr := lorawan.DevAddr{1, 2, 3, 4}
	devAddr.SetAddrPrefix(lorawan.NetID{6, 6, 6})
	phy := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: lorawan.UnconfirmedDataUp,
			Major: lorawan.LoRaWANR1,
		},
		MACPayload: &lorawan.MACPayload{
			FHDR: lorawan.FHDR{
				DevAddr: devAddr,
				FCnt:    10,
			},
		},
	}
	phyB, err := phy.MarshalBinary()
	assert.NoError(err)

	// ulToken
	ulTokenB, err := proto.Marshal(&ts.rxInfo)
	assert.NoError(err)

	// hNS PRStartAns
	prStartAns := backend.PRStartAnsPayload{
		BasePayloadResult: backend.BasePayloadResult{
			Result: backend.Result{
				ResultCode: backend.Success,
			},
		},
		Lifetime: &lifetime,
		NwkSKey: &backend.KeyEnvelope{
			AESKey: backend.HEXBytes{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
		},
		DevEUI: &devEUI,
		FCntUp: &fCntUp,
	}
	prStartAnsB, err := json.Marshal(prStartAns)
	assert.NoError(err)

	// nNS XmitDataAns
	xmitDataAns := backend.XmitDataAnsPayload{
		BasePayloadResult: backend.BasePayloadResult{
			Result: backend.Result{
				ResultCode: backend.Success,
			},
		},
	}
	xmitDataAnsB, err := json.Marshal(xmitDataAns)
	assert.NoError(err)

	ts.T().Run("success", func(t *testing.T) {
		assert := require.New(t)

		ts.hnsResponse = [][]byte{prStartAnsB, xmitDataAnsB}

		// "send" uplink
		assert.NoError(uplink.HandleUplinkFrame(context.Background(), gw.UplinkFrame{
			RxInfo:     &ts.rxInfo,
			TxInfo:     &ts.txInfo,
			PhyPayload: phyB,
		}))

		// validate hNS PRStartReq
		rssi := 6
		snr := float64(7)
		lat := float64(1)
		lon := float64(2)
		var prStartReq backend.PRStartReqPayload
		assert.NoError(json.Unmarshal(ts.hnsRequest[0], &prStartReq))
		assert.NotEqual(0, prStartReq.TransactionID)
		prStartReq.TransactionID = 0
		var nilTime time.Time
		assert.False(time.Time(prStartReq.ULMetaData.RecvTime).Equal(nilTime))
		prStartReq.ULMetaData.RecvTime = backend.ISO8601Time(nilTime)
		assert.Equal(backend.PRStartReqPayload{
			BasePayload: backend.BasePayload{
				ProtocolVersion: "1.0",
				SenderID:        "030201",
				ReceiverID:      "060606",
				MessageType:     backend.PRStartReq,
			},
			PHYPayload: backend.HEXBytes(phyB),
			ULMetaData: backend.ULMetaData{
				DataRate: &dataRate1,
				RFRegion: "EU868",
				ULFreq:   &ulFreq,
				GWCnt:    &gwCnt,
				GWInfo: []backend.GWInfoElement{
					{
						ID:        backend.HEXBytes(ts.Gateway.GatewayID[:]),
						RSSI:      &rssi,
						SNR:       &snr,
						Lat:       &lat,
						Lon:       &lon,
						ULToken:   backend.HEXBytes(ulTokenB),
						DLAllowed: true,
					},
				},
			},
		}, prStartReq)

		// validate session has been stored
		sess, err := storage.GetPassiveRoamingDeviceSessionsForDevAddr(context.Background(), devAddr)
		assert.NoError(err)
		assert.Len(sess, 1)
		sess[0].SessionID = uuid.Nil
		assert.True(sess[0].Lifetime.Sub(time.Now()) > (4 * time.Minute))
		sess[0].Lifetime = time.Time{}
		assert.Equal(storage.PassiveRoamingDeviceSession{
			NetID:       lorawan.NetID{6, 6, 6},
			DevAddr:     devAddr,
			DevEUI:      devEUI,
			FNwkSIntKey: lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
			FCntUp:      33,
		}, sess[0])
	})
}

func (ts *PassiveRoamingFNSTestSuite) TestDownlink() {
	assert := require.New(ts.T())
	config := test.GetConfig()
	api := roamingapi.NewAPI(config.NetworkServer.NetID)

	server := httptest.NewServer(api)
	defer server.Close()

	client, err := backend.NewClient(backend.ClientConfig{
		SenderID:   "060606",
		ReceiverID: config.NetworkServer.NetID.String(),
		Server:     server.URL,
	})
	assert.NoError(err)

	dlFreq1 := 868.1
	dlFreq2 := 868.2
	rxDelay1 := 1
	dataRate1 := 3
	dataRate2 := 2
	classMode := "A"

	ulRxInfo := gw.UplinkRXInfo{
		GatewayId: ts.Gateway.GatewayID[:],
		Rssi:      -10,
		LoraSnr:   3,
		Board:     1,
		Antenna:   0,
		Context:   []byte{1, 2, 3},
	}
	ulRxInfoB, err := proto.Marshal(&ulRxInfo)
	assert.NoError(err)

	req := backend.XmitDataReqPayload{
		PHYPayload: backend.HEXBytes{1, 2, 3},
		DLMetaData: &backend.DLMetaData{
			DLFreq1:   &dlFreq1,
			DLFreq2:   &dlFreq2,
			RXDelay1:  &rxDelay1,
			DataRate1: &dataRate1,
			DataRate2: &dataRate2,
			ClassMode: &classMode,
			GWInfo: []backend.GWInfoElement{
				{
					ULToken: backend.HEXBytes(ulRxInfoB),
				},
			},
		},
	}

	// perform API request
	resp, err := client.XmitDataReq(context.Background(), req)
	assert.NoError(err)

	// check that api returns success
	assert.Equal(backend.Success, resp.Result.ResultCode)

	// check that downlink was sent to the gateway
	frame := <-ts.GWBackend.TXPacketChan
	assert.Len(frame.DownlinkId, 16) // just check that the downlink id is set, we can't predict its value
	frame.DownlinkId = nil
	assert.Equal(gw.DownlinkFrame{
		GatewayId: ts.Gateway.GatewayID[:],
		Items: []*gw.DownlinkFrameItem{
			{
				PhyPayload: []byte{1, 2, 3},
				TxInfo: &gw.DownlinkTXInfo{
					Frequency:  868100000,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       9,
							CodeRate:              "4/5",
							PolarizationInversion: true,
						},
					},
					Board:   1,
					Antenna: 0,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second),
						},
					},
					Context: []byte{1, 2, 3},
				},
			},
			{
				PhyPayload: []byte{1, 2, 3},
				TxInfo: &gw.DownlinkTXInfo{
					Frequency:  868200000,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       10,
							CodeRate:              "4/5",
							PolarizationInversion: true,
						},
					},
					Board:   1,
					Antenna: 0,
					Timing:  gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Second * 2),
						},
					},
					Context: []byte{1, 2, 3},
				},
			},
		},
	}, frame)
}

// PassiveRoamingSNSTestSuite contains the tests from the hNS POV.
// This tests uplinks received from a fNS (through the roaming API) and
// forwarding these uplinks to the application-server. It also tests sending
// downloads back to the fNS (roaming API).
type PassiveRoamingSNSTestSuite struct {
	IntegrationTestSuite

	// mocked request / response for the fNS
	fnsServer   *httptest.Server
	fnsRequest  [][]byte
	fnsResponse [][]byte

	// hNS roaming API endpoint
	hnsServer *httptest.Server
}

func (ts *PassiveRoamingSNSTestSuite) SetupTest() {
	ts.IntegrationTestSuite.SetupTest()

	ts.fnsRequest = nil
	ts.fnsResponse = nil
}

func (ts *PassiveRoamingSNSTestSuite) SetupSuite() {
	ts.IntegrationTestSuite.SetupSuite()
	assert := require.New(ts.T())

	// fNS mock
	ts.fnsServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := ioutil.ReadAll(r.Body)
		ts.fnsRequest = append(ts.fnsRequest, b)
		w.Write(ts.fnsResponse[0])
		ts.fnsResponse = ts.fnsResponse[1:]
	}))

	// configuration
	conf := test.GetConfig()

	// configure hNS API
	api := roamingapi.NewAPI(conf.NetworkServer.NetID)
	ts.hnsServer = httptest.NewServer(api)

	// configure passive-roaming agreement with fNS
	conf.Roaming.Servers = []config.RoamingServer{
		{
			NetID:                  lorawan.NetID{6, 6, 6},
			Async:                  false,
			PassiveRoaming:         true,
			PassiveRoamingLifetime: time.Minute,
			Server:                 ts.fnsServer.URL,
		},
	}
	assert.NoError(roaming.Setup(conf))

	// create test device
	ts.CreateServiceProfile(storage.ServiceProfile{
		DRMax:         5,
		AddGWMetadata: true,
	})
	ts.CreateDevice(storage.Device{
		DevEUI: lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
	})
}

func (ts *PassiveRoamingSNSTestSuite) TearDownSuite() {
	ts.fnsServer.Close()
	ts.hnsServer.Close()
}

func (ts *PassiveRoamingSNSTestSuite) TestPRStartReqStateless() {
	assert := require.New(ts.T())

	// setup stateless roaming-agreement
	conf := test.GetConfig()
	conf.Roaming.Servers = []config.RoamingServer{
		{
			NetID:                  lorawan.NetID{6, 6, 6},
			Async:                  false,
			PassiveRoaming:         true,
			PassiveRoamingLifetime: 0,
			Server:                 ts.fnsServer.URL,
		},
	}
	assert.NoError(roaming.Setup(conf))

	devAddr := lorawan.DevAddr{1, 2, 3, 4}
	devAddr.SetAddrPrefix(conf.NetworkServer.NetID)

	// setup device-session
	ts.CreateDeviceSession(storage.DeviceSession{
		JoinEUI:               lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
		DevAddr:               devAddr,
		FNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		NwkSEncKey:            [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		FCntUp:                20,
		NFCntDown:             10,
		EnabledUplinkChannels: []int{0, 1, 2},
		RX2Frequency:          869525000,
	})

	fPort := uint8(10)

	phy := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: lorawan.UnconfirmedDataUp,
			Major: lorawan.LoRaWANR1,
		},
		MACPayload: &lorawan.MACPayload{
			FHDR: lorawan.FHDR{
				DevAddr: devAddr,
				FCnt:    20,
			},
			FPort: &fPort,
		},
	}
	assert.NoError(phy.SetUplinkDataMIC(lorawan.LoRaWAN1_0, 0, 0, 0, ts.DeviceSession.FNwkSIntKey, ts.DeviceSession.SNwkSIntKey))
	phyB, err := phy.MarshalBinary()
	assert.NoError(err)

	// setup client
	client, err := backend.NewClient(backend.ClientConfig{
		SenderID:   "060606",
		ReceiverID: conf.NetworkServer.NetID.String(),
		Server:     ts.hnsServer.URL,
	})
	assert.NoError(err)

	// request
	ulFreq := 868.1
	dataRate := 3
	recvTime := time.Now().Round(time.Second)
	gwCnt := 1

	prStartReq := backend.PRStartReqPayload{
		PHYPayload: backend.HEXBytes(phyB),
		ULMetaData: backend.ULMetaData{
			ULFreq:   &ulFreq,
			DataRate: &dataRate,
			RecvTime: backend.ISO8601Time(recvTime),
			RFRegion: "EU868",
			GWCnt:    &gwCnt,
			GWInfo: []backend.GWInfoElement{
				{
					ID:        backend.HEXBytes{1, 2, 3, 4, 5, 6, 7, 8},
					DLAllowed: true,
				},
			},
		},
	}
	resp, err := client.PRStartReq(context.Background(), prStartReq)
	assert.NoError(err)
	assert.Equal(backend.Success, resp.Result.ResultCode)
	assert.Equal(0, *resp.Lifetime)

	// check downlink was sent to AS
	asReq := <-ts.ASClient.HandleDataUpChan
	assert.True(proto.Equal(&as.HandleUplinkDataRequest{
		DevEui:  ts.Device.DevEUI[:],
		JoinEui: ts.DeviceSession.JoinEUI[:],
		FCnt:    20,
		FPort:   10,
		Dr:      3,
		TxInfo: &gw.UplinkTXInfo{
			Frequency:  868100000,
			Modulation: common.Modulation_LORA,
			ModulationInfo: &gw.UplinkTXInfo_LoraModulationInfo{
				LoraModulationInfo: &gw.LoRaModulationInfo{
					SpreadingFactor:       9,
					Bandwidth:             125,
					CodeRate:              "4/5",
					PolarizationInversion: true,
				},
			},
		},
		RxInfo: []*gw.UplinkRXInfo{
			{
				GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
				CrcStatus: gw.CRCStatus_CRC_OK,
			},
		},
	}, &asReq))
}

func (ts *PassiveRoamingSNSTestSuite) TestPRStartReqStatefull() {
	assert := require.New(ts.T())

	// setup stateless roaming-agreement
	conf := test.GetConfig()
	conf.Roaming.Servers = []config.RoamingServer{
		{
			NetID:                  lorawan.NetID{6, 6, 6},
			Async:                  false,
			PassiveRoaming:         true,
			PassiveRoamingLifetime: time.Minute,
			Server:                 ts.fnsServer.URL,
		},
	}
	assert.NoError(roaming.Setup(conf))

	devAddr := lorawan.DevAddr{1, 2, 3, 4}
	devAddr.SetAddrPrefix(conf.NetworkServer.NetID)

	// setup device-session
	ts.CreateDeviceSession(storage.DeviceSession{
		JoinEUI:               lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
		DevAddr:               devAddr,
		FNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		NwkSEncKey:            [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		FCntUp:                20,
		NFCntDown:             10,
		EnabledUplinkChannels: []int{0, 1, 2},
		RX2Frequency:          869525000,
	})

	fPort := uint8(10)

	phy := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: lorawan.UnconfirmedDataUp,
			Major: lorawan.LoRaWANR1,
		},
		MACPayload: &lorawan.MACPayload{
			FHDR: lorawan.FHDR{
				DevAddr: devAddr,
				FCnt:    20,
			},
			FPort: &fPort,
		},
	}
	assert.NoError(phy.SetUplinkDataMIC(lorawan.LoRaWAN1_0, 0, 0, 0, ts.DeviceSession.FNwkSIntKey, ts.DeviceSession.SNwkSIntKey))
	phyB, err := phy.MarshalBinary()
	assert.NoError(err)

	// setup client
	client, err := backend.NewClient(backend.ClientConfig{
		SenderID:   "060606",
		ReceiverID: conf.NetworkServer.NetID.String(),
		Server:     ts.hnsServer.URL,
	})
	assert.NoError(err)

	// request
	ulFreq := 868.1
	dataRate := 3
	recvTime := time.Now().Round(time.Second)
	gwCnt := 1

	prStartReq := backend.PRStartReqPayload{
		PHYPayload: backend.HEXBytes(phyB),
		ULMetaData: backend.ULMetaData{
			ULFreq:   &ulFreq,
			DataRate: &dataRate,
			RecvTime: backend.ISO8601Time(recvTime),
			RFRegion: "EU868",
			GWCnt:    &gwCnt,
			GWInfo: []backend.GWInfoElement{
				{
					ID:        backend.HEXBytes{1, 2, 3, 4, 5, 6, 7, 8},
					DLAllowed: true,
				},
			},
		},
	}
	resp, err := client.PRStartReq(context.Background(), prStartReq)
	assert.NoError(err)
	assert.Equal(backend.Success, resp.Result.ResultCode)
	assert.Equal(60, *resp.Lifetime)

	// nothing sent as in case of statefull an XmitDataReq is expected
	// after the PRStartReq.
	assert.Equal(0, len(ts.ASClient.HandleDataUpChan))
}

func (ts *PassiveRoamingSNSTestSuite) TestXmitDataReqUplinkNoDownlink() {
	assert := require.New(ts.T())

	// setup stateless roaming-agreement
	conf := test.GetConfig()
	conf.Roaming.Servers = []config.RoamingServer{
		{
			NetID:                  lorawan.NetID{6, 6, 6},
			Async:                  false,
			PassiveRoaming:         true,
			PassiveRoamingLifetime: time.Minute,
			Server:                 ts.fnsServer.URL,
		},
	}
	assert.NoError(roaming.Setup(conf))

	devAddr := lorawan.DevAddr{1, 2, 3, 4}
	devAddr.SetAddrPrefix(conf.NetworkServer.NetID)

	// setup device-session
	ts.CreateDeviceSession(storage.DeviceSession{
		JoinEUI:               lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
		DevAddr:               devAddr,
		FNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		NwkSEncKey:            [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		FCntUp:                20,
		NFCntDown:             10,
		EnabledUplinkChannels: []int{0, 1, 2},
		RX2Frequency:          869525000,
	})

	fPort := uint8(10)

	phy := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: lorawan.UnconfirmedDataUp,
			Major: lorawan.LoRaWANR1,
		},
		MACPayload: &lorawan.MACPayload{
			FHDR: lorawan.FHDR{
				DevAddr: devAddr,
				FCnt:    20,
			},
			FPort: &fPort,
		},
	}
	assert.NoError(phy.SetUplinkDataMIC(lorawan.LoRaWAN1_0, 0, 0, 0, ts.DeviceSession.FNwkSIntKey, ts.DeviceSession.SNwkSIntKey))
	phyB, err := phy.MarshalBinary()
	assert.NoError(err)

	// setup client
	client, err := backend.NewClient(backend.ClientConfig{
		SenderID:   "060606",
		ReceiverID: conf.NetworkServer.NetID.String(),
		Server:     ts.hnsServer.URL,
	})
	assert.NoError(err)

	// request
	ulFreq := 868.1
	dataRate := 3
	recvTime := time.Now().Round(time.Second)
	gwCnt := 1

	xmitDataReq := backend.XmitDataReqPayload{
		PHYPayload: backend.HEXBytes(phyB),
		ULMetaData: &backend.ULMetaData{
			ULFreq:   &ulFreq,
			DataRate: &dataRate,
			RecvTime: backend.ISO8601Time(recvTime),
			RFRegion: "EU868",
			GWCnt:    &gwCnt,
			GWInfo: []backend.GWInfoElement{
				{
					ID:        backend.HEXBytes{1, 2, 3, 4, 5, 6, 7, 8},
					DLAllowed: true,
				},
			},
		},
	}
	resp, err := client.XmitDataReq(context.Background(), xmitDataReq)
	assert.NoError(err)
	assert.Equal(backend.Success, resp.Result.ResultCode)

	// check downlink was sent to AS
	asReq := <-ts.ASClient.HandleDataUpChan
	assert.True(proto.Equal(&as.HandleUplinkDataRequest{
		DevEui:  ts.Device.DevEUI[:],
		JoinEui: ts.DeviceSession.JoinEUI[:],
		FCnt:    20,
		FPort:   10,
		Dr:      3,
		TxInfo: &gw.UplinkTXInfo{
			Frequency:  868100000,
			Modulation: common.Modulation_LORA,
			ModulationInfo: &gw.UplinkTXInfo_LoraModulationInfo{
				LoraModulationInfo: &gw.LoRaModulationInfo{
					SpreadingFactor:       9,
					Bandwidth:             125,
					CodeRate:              "4/5",
					PolarizationInversion: true,
				},
			},
		},
		RxInfo: []*gw.UplinkRXInfo{
			{
				GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
				CrcStatus: gw.CRCStatus_CRC_OK,
			},
		},
	}, &asReq))

	// assert that no request was sent to the fNS
	assert.Len(ts.fnsRequest, 0)
}

func (ts *PassiveRoamingSNSTestSuite) TestXmitDataReqUplinkDownlink() {
	assert := require.New(ts.T())

	// setup stateless roaming-agreement
	conf := test.GetConfig()
	conf.Roaming.Servers = []config.RoamingServer{
		{
			NetID:                  lorawan.NetID{6, 6, 6},
			Async:                  false,
			PassiveRoaming:         true,
			PassiveRoamingLifetime: time.Minute,
			Server:                 ts.fnsServer.URL,
		},
	}
	assert.NoError(roaming.Setup(conf))

	devAddr := lorawan.DevAddr{1, 2, 3, 4}
	devAddr.SetAddrPrefix(conf.NetworkServer.NetID)

	// setup device-session
	ts.CreateDeviceSession(storage.DeviceSession{
		JoinEUI:               lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
		DevAddr:               devAddr,
		FNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		NwkSEncKey:            [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		FCntUp:                20,
		NFCntDown:             10,
		EnabledUplinkChannels: []int{0, 1, 2},
		RX2Frequency:          869525000,
	})

	fPort := uint8(10)

	phy := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: lorawan.ConfirmedDataUp, // confirmed triggers a downlink
			Major: lorawan.LoRaWANR1,
		},
		MACPayload: &lorawan.MACPayload{
			FHDR: lorawan.FHDR{
				DevAddr: devAddr,
				FCnt:    20,
			},
			FPort: &fPort,
		},
	}
	assert.NoError(phy.SetUplinkDataMIC(lorawan.LoRaWAN1_0, 0, 0, 0, ts.DeviceSession.FNwkSIntKey, ts.DeviceSession.SNwkSIntKey))
	phyB, err := phy.MarshalBinary()
	assert.NoError(err)

	// setup client
	client, err := backend.NewClient(backend.ClientConfig{
		SenderID:   "060606",
		ReceiverID: conf.NetworkServer.NetID.String(),
		Server:     ts.hnsServer.URL,
	})
	assert.NoError(err)

	// mock XmitDataAns from fNS (for downlink)
	xmitDataAns := backend.XmitDataAnsPayload{
		BasePayloadResult: backend.BasePayloadResult{
			Result: backend.Result{
				ResultCode: backend.Success,
			},
		},
	}
	xmitDataAnsB, err := json.Marshal(xmitDataAns)
	assert.NoError(err)
	ts.fnsResponse = append(ts.fnsResponse, xmitDataAnsB)

	// request
	ulFreq := 868.1
	dataRate := 3
	recvTime := time.Now().Round(time.Second)
	gwCnt := 1

	xmitDataReq := backend.XmitDataReqPayload{
		PHYPayload: backend.HEXBytes(phyB),
		ULMetaData: &backend.ULMetaData{
			ULFreq:   &ulFreq,
			DataRate: &dataRate,
			RecvTime: backend.ISO8601Time(recvTime),
			RFRegion: "EU868",
			GWCnt:    &gwCnt,
			GWInfo: []backend.GWInfoElement{
				{
					ID:        backend.HEXBytes{1, 2, 3, 4, 5, 6, 7, 8},
					ULToken:   backend.HEXBytes{3, 2, 1},
					DLAllowed: true,
				},
			},
		},
	}
	resp, err := client.XmitDataReq(context.Background(), xmitDataReq)
	assert.NoError(err)
	assert.Equal(backend.Success, resp.Result.ResultCode)

	// check downlink was sent to AS
	asReq := <-ts.ASClient.HandleDataUpChan
	assert.True(proto.Equal(&as.HandleUplinkDataRequest{
		DevEui:          ts.Device.DevEUI[:],
		JoinEui:         ts.DeviceSession.JoinEUI[:],
		FCnt:            20,
		FPort:           10,
		Dr:              3,
		ConfirmedUplink: true,
		TxInfo: &gw.UplinkTXInfo{
			Frequency:  868100000,
			Modulation: common.Modulation_LORA,
			ModulationInfo: &gw.UplinkTXInfo_LoraModulationInfo{
				LoraModulationInfo: &gw.LoRaModulationInfo{
					SpreadingFactor:       9,
					Bandwidth:             125,
					CodeRate:              "4/5",
					PolarizationInversion: true,
				},
			},
		},
		RxInfo: []*gw.UplinkRXInfo{
			{
				GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
				CrcStatus: gw.CRCStatus_CRC_OK,
				Context:   []byte{3, 2, 1},
			},
		},
	}, &asReq))

	phy = lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: lorawan.UnconfirmedDataDown,
			Major: lorawan.LoRaWANR1,
		},
		MACPayload: &lorawan.MACPayload{
			FHDR: lorawan.FHDR{
				DevAddr: devAddr,
				FCnt:    10,
				FCtrl: lorawan.FCtrl{
					ADR: true,
					ACK: true,
				},
			},
		},
	}
	assert.NoError(phy.SetDownlinkDataMIC(lorawan.LoRaWAN1_0, 0, ts.DeviceSession.SNwkSIntKey))
	phyB, err = phy.MarshalBinary()
	assert.NoError(err)

	// assert downlink XmitDataReq
	assert.Len(ts.fnsRequest, 1)
	var fNSReq backend.XmitDataReqPayload
	assert.NoError(json.Unmarshal(ts.fnsRequest[0], &fNSReq))
	assert.NotEqual(0, fNSReq.TransactionID)
	fNSReq.TransactionID = 0

	dlFreq1 := 868.1
	dlFreq2 := 869.525
	rxDelay1 := 0
	classMode := "A"
	dataRate1 := 3
	dataRate2 := 0

	assert.Equal(backend.XmitDataReqPayload{
		BasePayload: backend.BasePayload{
			ProtocolVersion: "1.0",
			SenderID:        "030201",
			ReceiverID:      "060606",
			MessageType:     backend.XmitDataReq,
		},
		PHYPayload: backend.HEXBytes(phyB),
		DLMetaData: &backend.DLMetaData{
			DevEUI:    &ts.Device.DevEUI,
			DLFreq1:   &dlFreq1,
			DLFreq2:   &dlFreq2,
			RXDelay1:  &rxDelay1,
			ClassMode: &classMode,
			DataRate1: &dataRate1,
			DataRate2: &dataRate2,
			GWInfo: []backend.GWInfoElement{
				{
					ULToken: backend.HEXBytes{3, 2, 1},
				},
			},
		},
	}, fNSReq)
}

// TestPassiveRoamingFNS tests the passive-roaming from the fNS POV.
func TestPassiveRoamingFNS(t *testing.T) {
	suite.Run(t, new(PassiveRoamingFNSTestSuite))
}

// TestPassiveRoamingSNS tests the passive-roaming from the sNS POV.
func TestPassiveRoamingSNS(t *testing.T) {
	suite.Run(t, new(PassiveRoamingSNSTestSuite))
}
