package testsuite

import (
	"testing"

	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
)

type DownlinkTXAckTestSuite struct {
	IntegrationTestSuite
}

func (ts *DownlinkTXAckTestSuite) TestDownlinkTXAck() {
	assert := require.New(ts.T())

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

	tests := []DownlinkTXAckTest{
		{
			Name:   "positive ack",
			DevEUI: lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
			DownlinkTXAck: gw.DownlinkTXAck{
				Token:     12345,
				GatewayId: []byte{8, 7, 6, 5, 4, 3, 2, 1},
			},
			DownlinkFrames: storage.DownlinkFrames{
				Token:  12345,
				DevEui: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
				DownlinkFrames: []*gw.DownlinkFrame{
					{
						Token: 12345,
						TxInfo: &gw.DownlinkTXInfo{
							GatewayId: []byte{8, 7, 6, 5, 4, 3, 2, 1},
						},
						PhyPayload: []byte{1, 2},
					},
					{
						Token: 12345,
						TxInfo: &gw.DownlinkTXInfo{
							GatewayId: []byte{8, 7, 6, 5, 4, 3, 2, 1},
						},
						PhyPayload: []byte{1, 2, 3},
					},
				},
			},
			Assert: []Assertion{
				AssertNoDownlinkFrame,
			},
		},
		{
			Name:   "negative ack",
			DevEUI: lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
			DownlinkTXAck: gw.DownlinkTXAck{
				Token:     12345,
				GatewayId: []byte{8, 7, 6, 5, 4, 3, 2, 1},
				Error:     "BOOM",
			},
			DownlinkFrames: storage.DownlinkFrames{
				Token:  12345,
				DevEui: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
				DownlinkFrames: []*gw.DownlinkFrame{
					// the one that was previously "sent"
					{
						Token: 12345,
						TxInfo: &gw.DownlinkTXInfo{
							GatewayId: []byte{8, 7, 6, 5, 4, 3, 2, 1},
						},
						PhyPayload: []byte{1, 2},
					},
					// the next one in the queue
					{
						Token: 12345,
						TxInfo: &gw.DownlinkTXInfo{
							GatewayId: []byte{8, 7, 6, 5, 4, 3, 2, 1},
						},
						PhyPayload: phyB,
					},
				},
			},
			Assert: []Assertion{
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId: []byte{8, 7, 6, 5, 4, 3, 2, 1},
				}, phy),
				AssertDownlinkFrameSaved(lorawan.EUI64{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}, uuid.Nil, gw.DownlinkTXInfo{
					GatewayId: []byte{8, 7, 6, 5, 4, 3, 2, 1},
				}, phy),
			},
		},
		{
			Name:   "negative ack, no saved downlink-frame",
			DevEUI: lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
			DownlinkTXAck: gw.DownlinkTXAck{
				Token:     54321,
				GatewayId: []byte{8, 7, 6, 5, 4, 3, 2, 1},
				Error:     "BOOM",
			},
			DownlinkFrames: storage.DownlinkFrames{
				Token:  54321,
				DevEui: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
				DownlinkFrames: []*gw.DownlinkFrame{
					{
						Token: 54321,
						TxInfo: &gw.DownlinkTXInfo{
							GatewayId: []byte{8, 7, 6, 5, 4, 3, 2, 1},
						},
						PhyPayload: phyB,
					},
				},
			},
			Assert: []Assertion{
				AssertNoDownlinkFrame,
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertDownlinkTXAckTest(t, tst)
		})
	}
}

func TestDownlinkTXAck(t *testing.T) {
	suite.Run(t, new(DownlinkTXAckTestSuite))
}
