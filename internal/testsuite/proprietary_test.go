package testsuite

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/chirpstack-api/go/v3/as"
	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-api/go/v3/ns"
	"github.com/brocaar/chirpstack-network-server/internal/downlink"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/chirpstack-network-server/internal/test"
	"github.com/brocaar/lorawan"
)

type ProprietaryTestCase struct {
	IntegrationTestSuite
}

func (ts *ProprietaryTestCase) SetupTest() {
	ts.IntegrationTestSuite.SetupTest()
	assert := require.New(ts.T())

	conf := test.GetConfig()
	assert.NoError(downlink.Setup(conf))
}

func (ts *ProprietaryTestCase) TestDownlink() {
	gatewayID := lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1}

	tests := []DownlinkProprietaryTest{
		{
			Name: "send proprietary payload (iPol true)",
			SendProprietaryPayloadRequest: ns.SendProprietaryPayloadRequest{
				MacPayload:            []byte{1, 2, 3, 4},
				Mic:                   []byte{5, 6, 7, 8},
				GatewayMacs:           [][]byte{{8, 7, 6, 5, 4, 3, 2, 1}},
				PolarizationInversion: true,
				Frequency:             868100000,
				Dr:                    5,
			},

			Assert: []Assertion{
				AssertDownlinkFrame(gatewayID, gw.DownlinkTXInfo{
					Frequency:  868100000,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       7,
							CodeRate:              "4/5",
							PolarizationInversion: true,
						},
					},
					Timing: gw.DownlinkTiming_IMMEDIATELY,
					TimingInfo: &gw.DownlinkTXInfo_ImmediatelyTimingInfo{
						ImmediatelyTimingInfo: &gw.ImmediatelyTimingInfo{},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						Major: lorawan.LoRaWANR1,
						MType: lorawan.Proprietary,
					},
					MACPayload: &lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}},
					MIC:        lorawan.MIC{5, 6, 7, 8},
				}),
			},
		},
		{
			Name: "send proprietary payload (iPol false)",
			SendProprietaryPayloadRequest: ns.SendProprietaryPayloadRequest{
				MacPayload:            []byte{1, 2, 3, 4},
				Mic:                   []byte{5, 6, 7, 8},
				GatewayMacs:           [][]byte{{8, 7, 6, 5, 4, 3, 2, 1}},
				PolarizationInversion: false,
				Frequency:             868100000,
				Dr:                    5,
			},

			Assert: []Assertion{
				AssertDownlinkFrame(gatewayID, gw.DownlinkTXInfo{
					Frequency:  868100000,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       7,
							CodeRate:              "4/5",
							PolarizationInversion: false,
						},
					},
					Timing: gw.DownlinkTiming_IMMEDIATELY,
					TimingInfo: &gw.DownlinkTXInfo_ImmediatelyTimingInfo{
						ImmediatelyTimingInfo: &gw.ImmediatelyTimingInfo{},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						Major: lorawan.LoRaWANR1,
						MType: lorawan.Proprietary,
					},
					MACPayload: &lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}},
					MIC:        lorawan.MIC{5, 6, 7, 8},
				}),
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertDownlinkProprietaryTest(t, tst)
		})
	}
}

func (ts *ProprietaryTestCase) TestUplink() {
	// the routing profile is needed as the ns will send the proprietary
	// frame to all application-servers.
	ts.CreateRoutingProfile(storage.RoutingProfile{})

	ts.CreateGateway(storage.Gateway{
		GatewayID: lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
		Location: storage.GPSPoint{
			Latitude:  1.1234,
			Longitude: 2.345,
		},
		Altitude: 10,
	})

	tests := []UplinkProprietaryTest{
		{
			Name: "uplink proprietary payload",
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					Major: lorawan.LoRaWANR1,
					MType: lorawan.Proprietary,
				},
				MACPayload: &lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}},
				MIC:        lorawan.MIC{5, 6, 7, 8},
			},
			TXInfo: gw.UplinkTXInfo{
				Frequency:  868100000,
				Modulation: common.Modulation_LORA,
				ModulationInfo: &gw.UplinkTXInfo_LoraModulationInfo{
					LoraModulationInfo: &gw.LoRaModulationInfo{
						Bandwidth:       125,
						CodeRate:        "4/5",
						SpreadingFactor: 12,
					},
				},
			},
			RXInfo: gw.UplinkRXInfo{
				GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
				Rssi:      -10,
				LoraSnr:   5,
			},
			Assert: []Assertion{
				AssertASHandleProprietaryUplinkRequest(as.HandleProprietaryUplinkRequest{
					MacPayload: []byte{1, 2, 3, 4},
					Mic:        []byte{5, 6, 7, 8},
					TxInfo: &gw.UplinkTXInfo{
						Frequency:  868100000,
						Modulation: common.Modulation_LORA,
						ModulationInfo: &gw.UplinkTXInfo_LoraModulationInfo{
							LoraModulationInfo: &gw.LoRaModulationInfo{
								Bandwidth:       125,
								SpreadingFactor: 12,
								CodeRate:        "4/5",
							},
						},
					},
					RxInfo: []*gw.UplinkRXInfo{
						{
							GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
							Rssi:      -10,
							LoraSnr:   5,
							Location: &common.Location{
								Latitude:  1.1234,
								Longitude: 2.345,
								Altitude:  10,
							},
						},
					},
				}),
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertUplinkProprietaryTest(t, tst)
		})
	}
}

func TestProprietary(t *testing.T) {
	suite.Run(t, new(ProprietaryTestCase))
}
