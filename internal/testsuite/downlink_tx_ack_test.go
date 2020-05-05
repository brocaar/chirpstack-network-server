package testsuite

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/chirpstack-api/go/v3/as"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
)

type DownlinkTXAckTestSuite struct {
	IntegrationTestSuite
}

func (ts *DownlinkTXAckTestSuite) TestDownlinkTXAck() {
	assert := require.New(ts.T())

	ts.CreateDevice(storage.Device{
		DevEUI: lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
	})

	var fPortOne uint8 = 1
	phy := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: lorawan.UnconfirmedDataUp,
			Major: lorawan.LoRaWANR1,
		},
		MACPayload: &lorawan.MACPayload{
			FHDR: lorawan.FHDR{
				DevAddr: lorawan.DevAddr{1, 2, 3, 4},
				FCnt:    7,
			},
			FPort: &fPortOne,
		},
		MIC: lorawan.MIC{48, 94, 26, 239},
	}
	phyB, err := phy.MarshalBinary()
	assert.NoError(err)

	phyNS := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: lorawan.UnconfirmedDataUp,
			Major: lorawan.LoRaWANR1,
		},
		MACPayload: &lorawan.MACPayload{
			FHDR: lorawan.FHDR{
				DevAddr: lorawan.DevAddr{1, 2, 3, 4},
				FCnt:    7,
			},
		},
		MIC: lorawan.MIC{48, 94, 26, 239},
	}
	phyNSB, err := phyNS.MarshalBinary()
	assert.NoError(err)

	tests := []DownlinkTXAckTest{
		{
			Name:   "positive ack for app data",
			DevEUI: ts.Device.DevEUI,
			DownlinkTXAck: gw.DownlinkTXAck{
				Token:     12345,
				GatewayId: []byte{8, 7, 6, 5, 4, 3, 2, 1},
				Items: []*gw.DownlinkTXAckItem{
					{
						Status: gw.TxAckStatus_OK,
					},
				},
			},
			DownlinkFrame: storage.DownlinkFrame{
				Token:            12345,
				DevEui:           []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
				RoutingProfileId: ts.RoutingProfile.ID.Bytes(),
				FCnt:             7,
				DownlinkFrame: &gw.DownlinkFrame{
					Token: 12345,
					Items: []*gw.DownlinkFrameItem{
						{
							TxInfo: &gw.DownlinkTXInfo{
								GatewayId: []byte{8, 7, 6, 5, 4, 3, 2, 1},
							},
							PhyPayload: phyB,
						},
						{
							TxInfo: &gw.DownlinkTXInfo{
								GatewayId: []byte{8, 7, 6, 5, 4, 3, 2, 1},
							},
							PhyPayload: phyB,
						},
					},
				},
			},
			Assert: []Assertion{
				AssertNoDownlinkFrame,
				AssertASHandleTxAckRequest(as.HandleTxAckRequest{
					DevEui: ts.Device.DevEUI[:],
					FCnt:   7,
				}),
			},
		},
		{
			Name:   "positive ack for ns data",
			DevEUI: ts.Device.DevEUI,
			DownlinkTXAck: gw.DownlinkTXAck{
				Token:     12345,
				GatewayId: []byte{8, 7, 6, 5, 4, 3, 2, 1},
				Items: []*gw.DownlinkTXAckItem{
					{
						Status: gw.TxAckStatus_OK,
					},
				},
			},
			DownlinkFrame: storage.DownlinkFrame{
				Token:            12345,
				DevEui:           []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
				RoutingProfileId: ts.RoutingProfile.ID.Bytes(),
				FCnt:             7,
				DownlinkFrame: &gw.DownlinkFrame{
					Token: 12345,
					Items: []*gw.DownlinkFrameItem{

						{
							TxInfo: &gw.DownlinkTXInfo{
								GatewayId: []byte{8, 7, 6, 5, 4, 3, 2, 1},
							},
							PhyPayload: phyNSB,
						},
						{
							TxInfo: &gw.DownlinkTXInfo{
								GatewayId: []byte{8, 7, 6, 5, 4, 3, 2, 1},
							},
							PhyPayload: phyNSB,
						},
					},
				},
			},
			Assert: []Assertion{
				AssertNoDownlinkFrame,
				AssertASNoHandleTxAckRequest(),
			},
		},
		{
			Name:   "negative ack for app data",
			DevEUI: ts.Device.DevEUI,
			DownlinkTXAck: gw.DownlinkTXAck{
				Token:     54321,
				GatewayId: []byte{8, 7, 6, 5, 4, 3, 2, 1},
				Items: []*gw.DownlinkTXAckItem{
					{
						Status: gw.TxAckStatus_TX_POWER,
					},
				},
			},
			DownlinkFrame: storage.DownlinkFrame{
				Token:            54321,
				DevEui:           ts.Device.DevEUI[:],
				RoutingProfileId: ts.RoutingProfile.ID.Bytes(),
				FCnt:             7,
				DownlinkFrame: &gw.DownlinkFrame{
					Token: 54321,
					Items: []*gw.DownlinkFrameItem{
						{
							TxInfo: &gw.DownlinkTXInfo{
								GatewayId: []byte{8, 7, 6, 5, 4, 3, 2, 1},
							},
							PhyPayload: phyB,
						},
					},
				},
			},
			Assert: []Assertion{
				AssertNoDownlinkFrame,
				AssertASHandleErrorRequest(as.HandleErrorRequest{
					DevEui: ts.Device.DevEUI[:],
					Type:   as.ErrorType_DATA_DOWN_GATEWAY,
					Error:  "TX_POWER",
					FCnt:   7,
				}),
			},
		},
		{
			Name:   "negative ack for ns data",
			DevEUI: ts.Device.DevEUI,
			DownlinkTXAck: gw.DownlinkTXAck{
				Token:     54321,
				GatewayId: []byte{8, 7, 6, 5, 4, 3, 2, 1},
				Items: []*gw.DownlinkTXAckItem{
					{
						Status: gw.TxAckStatus_TX_FREQ,
					},
				},
			},
			DownlinkFrame: storage.DownlinkFrame{
				Token:            54321,
				DevEui:           ts.Device.DevEUI[:],
				RoutingProfileId: ts.RoutingProfile.ID.Bytes(),
				FCnt:             7,
				DownlinkFrame: &gw.DownlinkFrame{
					Token: 54321,
					Items: []*gw.DownlinkFrameItem{
						{
							TxInfo: &gw.DownlinkTXInfo{
								GatewayId: []byte{8, 7, 6, 5, 4, 3, 2, 1},
							},
							PhyPayload: phyNSB,
						},
					},
				},
			},
			Assert: []Assertion{
				AssertNoDownlinkFrame,
				AssertASNoHandleErrorRequest(),
			},
		},
	}

	for _, tst := range tests[2:] {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertDownlinkTXAckTest(t, tst)
		})
	}
}

func TestDownlinkTXAck(t *testing.T) {
	suite.Run(t, new(DownlinkTXAckTestSuite))
}
