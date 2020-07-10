package testsuite

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-network-server/internal/backend/joinserver"
	"github.com/brocaar/chirpstack-network-server/internal/band"
	"github.com/brocaar/chirpstack-network-server/internal/config"
	"github.com/brocaar/chirpstack-network-server/internal/helpers"
	"github.com/brocaar/chirpstack-network-server/internal/roaming"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/chirpstack-network-server/internal/test"
	"github.com/brocaar/chirpstack-network-server/internal/uplink"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

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
			CheckMIC:               true,
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
	ts.hnsResponse = [][]byte{prStartAnsB}

	ts.T().Run("passive-roaming start", func(t *testing.T) {
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

}

func (ts *PassiveRoamingFNSTestSuite) TestDataStatefull() {

}

func (ts *PassiveRoamingFNSTestSuite) TestDownlink() {

}

type PassiveRoamingSNSTestSuite struct {
	IntegrationTestSuite
}

func (ts *PassiveRoamingSNSTestSuite) TestPRStartAnsStateless() {

}

func (ts *PassiveRoamingSNSTestSuite) TestPRStartAnsStatefull() {

}

func (ts *PassiveRoamingSNSTestSuite) TestXmitDataReqUplinkNoDownlink() {

}

func (ts *PassiveRoamingSNSTestSuite) TestXmitDataReqUplinkDownlink() {

}

// TestPassiveRoamingFNS tests the passive-roaming from the fNS POV.
func TestPassiveRoamingFNS(t *testing.T) {
	suite.Run(t, new(PassiveRoamingFNSTestSuite))
}

// TestPassiveRoamingSNS tests the passive-roaming from the sNS POV.
func TestPassiveRoamingSNS(t *testing.T) {
	suite.Run(t, new(PassiveRoamingSNSTestSuite))
}
