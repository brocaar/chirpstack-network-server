package gcppubsub

import (
	"testing"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/backend/gateway/marshaler"
	"github.com/brocaar/lorawan"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type BackendTestSuite struct {
	suite.Suite
	backend *Backend
}

func (ts *BackendTestSuite) SetupSuite() {
	ts.backend = &Backend{
		gatewayMarshaler:  make(map[lorawan.EUI64]marshaler.Type),
		uplinkFrameChan:   make(chan gw.UplinkFrame, 10),
		gatewayStatsChan:  make(chan gw.GatewayStats, 10),
		downlinkTXAckChan: make(chan gw.DownlinkTXAck, 10),
	}
}

func (ts *BackendTestSuite) TestHandleUplinkFrame() {
	assert := require.New(ts.T())
	gatewayID := lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}

	testTable := []struct {
		UplinkFrame   gw.UplinkFrame
		ExpectedError error
	}{
		{
			UplinkFrame: gw.UplinkFrame{
				RxInfo: nil,
			},
			ExpectedError: errors.New("rx_info must not be nil"),
		},
		{
			UplinkFrame: gw.UplinkFrame{
				RxInfo: &gw.UplinkRXInfo{},
			},
			ExpectedError: errors.New("tx_info must not be nil"),
		},
		{
			UplinkFrame: gw.UplinkFrame{
				RxInfo: &gw.UplinkRXInfo{},
				TxInfo: &gw.UplinkTXInfo{},
			},
			ExpectedError: errors.New("gateway_id is not equal to expected gateway_id"),
		},
		{
			UplinkFrame: gw.UplinkFrame{
				RxInfo: &gw.UplinkRXInfo{
					GatewayId: gatewayID[:],
				},
				TxInfo: &gw.UplinkTXInfo{},
			},
		},
	}

	for _, test := range testTable {
		b, err := proto.Marshal(&test.UplinkFrame)
		assert.NoError(err)

		err = ts.backend.handleUplinkFrame(gatewayID, b)
		if err != nil {
			assert.EqualError(err, test.ExpectedError.Error())
		} else {
			assert.NoError(test.ExpectedError)
		}

		assert.Equal(marshaler.Protobuf, ts.backend.gatewayMarshaler[gatewayID])

		if err != nil {
			continue
		}

		rec := <-ts.backend.RXPacketChan()
		proto.Equal(&test.UplinkFrame, &rec)
	}
}

func (ts *BackendTestSuite) TestHandleGatewayStats() {
	assert := require.New(ts.T())
	gatewayID := lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}

	testTable := []struct {
		GatewayStats  gw.GatewayStats
		ExpectedError error
	}{
		{
			GatewayStats:  gw.GatewayStats{},
			ExpectedError: errors.New("gateway_id is not equal to expected gateway_id"),
		},
		{
			GatewayStats: gw.GatewayStats{
				GatewayId: gatewayID[:],
			},
		},
	}

	for _, test := range testTable {
		b, err := proto.Marshal(&test.GatewayStats)
		assert.NoError(err)

		err = ts.backend.handleGatewayStats(gatewayID, b)
		if err != nil {
			assert.EqualError(err, test.ExpectedError.Error())
		} else {
			assert.NoError(test.ExpectedError)
		}

		assert.Equal(marshaler.Protobuf, ts.backend.gatewayMarshaler[gatewayID])

		if err != nil {
			continue
		}

		rec := <-ts.backend.StatsPacketChan()
		proto.Equal(&test.GatewayStats, &rec)
	}
}

func (ts *BackendTestSuite) TestHandleDownlinkTXAck() {
	assert := require.New(ts.T())
	gatewayID := lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}

	testTable := []struct {
		DownlinkTXAck gw.DownlinkTXAck
		ExpectedError error
	}{
		{
			DownlinkTXAck: gw.DownlinkTXAck{},
			ExpectedError: errors.New("gateway_id is not equal to expected gateway_id"),
		},
		{
			DownlinkTXAck: gw.DownlinkTXAck{
				GatewayId: gatewayID[:],
			},
		},
	}

	for _, test := range testTable {
		b, err := proto.Marshal(&test.DownlinkTXAck)
		assert.NoError(err)

		err = ts.backend.handleDownlinkTXAck(gatewayID, b)
		if err != nil {
			assert.EqualError(err, test.ExpectedError.Error())
		} else {
			assert.NoError(test.ExpectedError)
		}

		assert.Equal(marshaler.Protobuf, ts.backend.gatewayMarshaler[gatewayID])

		if err != nil {
			continue
		}

		rec := <-ts.backend.DownlinkTXAckChan()
		proto.Equal(&test.DownlinkTXAck, &rec)
	}

}

func TestBackend(t *testing.T) {
	suite.Run(t, new(BackendTestSuite))
}
