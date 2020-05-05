package amqp

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/streadway/amqp"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-network-server/internal/backend/gateway"
	"github.com/brocaar/chirpstack-network-server/internal/backend/gateway/marshaler"
	"github.com/brocaar/chirpstack-network-server/internal/test"
	"github.com/brocaar/lorawan"
)

type BackendTestSuite struct {
	suite.Suite

	gatewayID lorawan.EUI64
	backend   gateway.Gateway

	amqpConn        *amqp.Connection
	amqpChannel     *amqp.Channel
	amqpCommandChan <-chan amqp.Delivery
}

func (ts *BackendTestSuite) SetupSuite() {
	var err error
	assert := require.New(ts.T())
	conf := test.GetConfig()

	ts.gatewayID = lorawan.EUI64{0x01, 0x02, 0x03, 0x043, 0x05, 0x06, 0x07, 0x08}

	ts.backend, err = NewBackend(conf)
	assert.NoError(err)

	ts.backend.(*Backend).setGatewayMarshaler(ts.gatewayID, marshaler.Protobuf)

	ts.amqpConn, err = amqp.Dial(conf.NetworkServer.Gateway.Backend.AMQP.URL)
	assert.NoError(err)

	ts.amqpChannel, err = ts.amqpConn.Channel()
	assert.NoError(err)

	_, err = ts.amqpChannel.QueueDeclare(
		"test-command-queue",
		true,
		false,
		false,
		false,
		nil,
	)
	assert.NoError(err)

	err = ts.amqpChannel.QueueBind(
		"test-command-queue",
		"gateway.*.command.*",
		"amq.topic",
		false,
		nil,
	)
	assert.NoError(err)

	ts.amqpCommandChan, err = ts.amqpChannel.Consume(
		"test-command-queue",
		"",
		true,
		false,
		false,
		false,
		nil,
	)
	assert.NoError(err)
}

func (ts *BackendTestSuite) TearDownSuite() {
	assert := require.New(ts.T())

	assert.NoError(ts.amqpConn.Close())
	assert.NoError(ts.backend.Close())
}

func (ts *BackendTestSuite) TestDownlinkCommand() {
	assert := require.New(ts.T())

	pl := gw.DownlinkFrame{
		GatewayId: ts.gatewayID[:],
		Items: []*gw.DownlinkFrameItem{
			{
				PhyPayload: []byte{0x01, 0x02, 0x03, 0x04},
				TxInfo:     &gw.DownlinkTXInfo{},
			},
		},
	}
	assert.NoError(ts.backend.SendTXPacket(pl))

	received := <-ts.amqpCommandChan
	assert.Equal("gateway.0102034305060708.command.down", received.RoutingKey)
	assert.Equal("application/octet-stream", received.ContentType)

	var receivedPL gw.DownlinkFrame
	assert.NoError(proto.Unmarshal(received.Body, &receivedPL))

	if !proto.Equal(&pl, &receivedPL) {
		assert.Equal(pl, receivedPL)
	}
}

func (ts *BackendTestSuite) TestUplinkEvent() {
	assert := require.New(ts.T())

	up := gw.UplinkFrame{
		PhyPayload: []byte{0x01, 0x02, 0x03, 0x04},
		RxInfo: &gw.UplinkRXInfo{
			GatewayId: ts.gatewayID[:],
		},
		TxInfo: &gw.UplinkTXInfo{
			Frequency: 868100000,
		},
	}
	b, err := proto.Marshal(&up)
	assert.NoError(err)

	err = ts.amqpChannel.Publish(
		"amq.topic",
		"gateway.0102034305060708.event.up",
		false,
		false,
		amqp.Publishing{
			ContentType: "application/octet-stream",
			Body:        b,
		},
	)
	assert.NoError(err)

	upReceived := <-ts.backend.RXPacketChan()

	if !proto.Equal(&up, &upReceived) {
		assert.Equal(up, upReceived)
	}
}

func (ts *BackendTestSuite) TestGatewayStats() {
	assert := require.New(ts.T())

	stats := gw.GatewayStats{
		GatewayId: ts.gatewayID[:],
	}
	b, err := proto.Marshal(&stats)
	assert.NoError(err)

	err = ts.amqpChannel.Publish(
		"amq.topic",
		"gateway.0102034305060708.event.stats",
		false,
		false,
		amqp.Publishing{
			ContentType: "application/octet-stream",
			Body:        b,
		},
	)
	assert.NoError(err)

	statsReceived := <-ts.backend.StatsPacketChan()

	if !proto.Equal(&stats, &statsReceived) {
		assert.Equal(stats, statsReceived)
	}
}

func (ts *BackendTestSuite) TestDownlinkTXAck() {
	assert := require.New(ts.T())

	txAck := gw.DownlinkTXAck{
		GatewayId: ts.gatewayID[:],
	}
	b, err := proto.Marshal(&txAck)
	assert.NoError(err)

	err = ts.amqpChannel.Publish(
		"amq.topic",
		"gateway.0102034305060708.event.ack",
		false,
		false,
		amqp.Publishing{
			ContentType: "application/octet-stream",
			Body:        b,
		},
	)
	assert.NoError(err)

	txAckReceived := <-ts.backend.DownlinkTXAckChan()

	if !proto.Equal(&txAck, &txAckReceived) {
		assert.Equal(txAck, txAckReceived)
	}
}

func TestBackend(t *testing.T) {
	suite.Run(t, new(BackendTestSuite))
}
