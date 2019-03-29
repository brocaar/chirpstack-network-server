package azureiothub

import (
	"bytes"
	"context"
	"encoding/base64"
	"strings"
	"sync"
	"time"

	"github.com/amenzhinsky/iothub/common"
	"github.com/amenzhinsky/iothub/iotservice"

	"github.com/apex/log"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/backend/gateway"
	"github.com/brocaar/loraserver/internal/backend/gateway/marshaler"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/helpers"
	"github.com/brocaar/lorawan"
	"github.com/pkg/errors"
)

const uplinkSubscriptionTmpl = "%s-loraserver"
const (
	marshalerV2JSON = iota
	marshalerProtobuf
	marshalerJSON
)

// Backend implements a Azure IoT Hub backend.
type Backend struct {
	sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc

	client             *iotservice.Client
	downlinkTopic      string
	uplinkTopic        string
	uplinkSubscription string

	uplinkFrameChan   chan gw.UplinkFrame
	gatewayStatsChan  chan gw.GatewayStats
	downlinkTXAckChan chan gw.DownlinkTXAck
	gatewayMarshaler  map[lorawan.EUI64]marshaler.Type
}

// NewBackend creates a new Backend.
func NewBackend(c config.Config) (gateway.Gateway, error) {
	conf := c.NetworkServer.Gateway.Backend.AzureIoTHub

	b := Backend{
		gatewayMarshaler:  make(map[lorawan.EUI64]marshaler.Type),
		uplinkFrameChan:   make(chan gw.UplinkFrame),
		gatewayStatsChan:  make(chan gw.GatewayStats),
		downlinkTXAckChan: make(chan gw.DownlinkTXAck),
		ctx:               context.Background(),
	}

	_client, err := iotservice.NewClient(
		iotservice.WithConnectionString(conf.ConnectionString),
	)
	if err != nil {
		return nil, errors.Wrap(err, "gateway/azure_iot_hub: new hub client error")
	}

	b.client = _client

	//consume rx packets
	go func() {
		for {
			b.client.SubscribeEvents(b.ctx, b.receivehandler)
			time.Sleep(time.Second * 2)
			continue
		}
	}()

	return &b, nil
}

// SendTXPacket sends the given downlink frame to the gateway.
func (b *Backend) SendTXPacket(pl gw.DownlinkFrame) error {
	if pl.TxInfo == nil {
		return errors.New("tx_info must not be nil")
	}

	gatewayID := helpers.GetGatewayID(pl.TxInfo)
	t := b.getGatewayMarshaler(gatewayID)

	bb, err := marshaler.MarshalDownlinkFrame(t, pl)
	if err != nil {
		return errors.Wrap(err, "gateway/azure_iot_hub: marshal downlink frame error")
	}

	return b.publishCommand(gatewayID, "down", bb)
}

// SendGatewayConfigPacket sends the given gateway configuration to the gateway.
func (b *Backend) SendGatewayConfigPacket(pl gw.GatewayConfiguration) error {
	gatewayID := helpers.GetGatewayID(&pl)
	t := b.getGatewayMarshaler(gatewayID)

	bb, err := marshaler.MarshalGatewayConfiguration(t, pl)
	if err != nil {
		return errors.Wrap(err, "gateway/azure_iot_hub: marshal gateway configuration error")
	}

	return b.publishCommand(gatewayID, "config", bb)
}

// RXPacketChan returns the channel to which uplink frames are published.
func (b *Backend) RXPacketChan() chan gw.UplinkFrame {
	return b.uplinkFrameChan
}

// StatsPacketChan returns the channel to which gateway stats are published.
func (b *Backend) StatsPacketChan() chan gw.GatewayStats {
	return b.gatewayStatsChan
}

// DownlinkTXAckChan returns the downlink tx ack channel.
func (b *Backend) DownlinkTXAckChan() chan gw.DownlinkTXAck {
	return b.downlinkTXAckChan
}

// Close closes the backend.
func (b *Backend) Close() error {
	log.Info("gateway/azure_iot_hub: closing backend")
	b.cancel()
	close(b.uplinkFrameChan)
	close(b.gatewayStatsChan)
	close(b.downlinkTXAckChan)
	return b.client.Close()
}

func (b *Backend) setGatewayMarshaler(gatewayID lorawan.EUI64, t marshaler.Type) {
	b.Lock()
	defer b.Unlock()

	b.gatewayMarshaler[gatewayID] = t
}

func (b *Backend) getGatewayMarshaler(gatewayID lorawan.EUI64) marshaler.Type {
	b.RLock()
	defer b.RUnlock()

	return b.gatewayMarshaler[gatewayID]
}

func (b *Backend) publishCommand(gatewayID lorawan.EUI64, command string, data []byte) error {
	start := time.Now()

	expiryTime := time.Time{}
	gatewayIDStr := "eui-" + gatewayID.String()
	// IoT Hub Cloud-to-Device command topic is slighty different
	// than most general purpose MQTT broker. Hence, we need to add the "down" or "config"
	// topic pattern to the IoT Hub properties map
	err := b.client.SendEvent(
		b.ctx,
		gatewayIDStr,
		data,
		iotservice.WithSendAck(""),
		iotservice.WithSentExpiryTime(expiryTime),
		iotservice.WithSendProperty("cmd", "down"),
	)
	if err != nil {
		return errors.Wrap(err, "got azure_iot_hub publish result error")
	}

	log.WithFields(log.Fields{
		"duration":   time.Now().Sub(start),
		"gateway_id": gatewayID,
		"command":    command,
	}).Info("gateway/azure_iot_hub: message published")

	return nil
}

func (b *Backend) receivehandler(msg *common.Message) {

	var gatewayID lorawan.EUI64
	var typ string

	log.WithFields(log.Fields{
		"props":    msg.Properties,
		"deviceId": msg.ConnectionDeviceID,
	}).Info("gateway/azure_iot_hub: message received")

	var gatewayIDStr = msg.ConnectionDeviceID

	gatewayIDStr = strings.Replace(gatewayIDStr, "eui-", "", 1)
	if err := gatewayID.UnmarshalText([]byte(gatewayIDStr)); err != nil {
		log.WithError(err).Error("gateway/azure_iot_hub: unmarshal gateway id error")
	}

	log.WithFields(log.Fields{
		"gateway_id": gatewayID,
	}).Info("gateway/azure_iot_hub: message received")

	var err error
	// A little weird, however, IoT Hub topics place
	// extra parameters on their Properties
	if _, ok := msg.Properties["stats"]; ok {
		err = b.handleGatewayStats(gatewayID, msg.Payload)
	} else if _, ok := msg.Properties["up"]; ok {
		err = b.handleUplinkFrame(gatewayID, msg.Payload)
	} else if _, ok := msg.Properties["ack"]; ok {
		err = b.handleDownlinkTXAck(gatewayID, msg.Payload)
	} else {
		log.WithFields(log.Fields{
			"gateway_id": gatewayID,
			"type":       typ,
		}).Warn("gateway/azure_iot_hub: unexpected message type")
	}

	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"gateway_id":  gatewayID,
			"type":        typ,
			"data_base64": base64.StdEncoding.EncodeToString(msg.Payload),
		}).Error("gateway/azure_iot_hub: handle received message error")
	}
}

func (b *Backend) handleUplinkFrame(gatewayID lorawan.EUI64, data []byte) error {
	var uplinkFrame gw.UplinkFrame
	t, err := marshaler.UnmarshalUplinkFrame(data, &uplinkFrame)
	if err != nil {
		return errors.Wrap(err, "unmarshal error")
	}

	b.setGatewayMarshaler(gatewayID, t)

	if uplinkFrame.RxInfo == nil {
		return errors.New("rx_info must not be nil")
	}

	if uplinkFrame.TxInfo == nil {
		return errors.New("tx_info must not be nil")
	}

	// make sure that the registered gateway is not using a different gateway_id
	// than the ID used during the registration in Cloud IoT Core.
	if !bytes.Equal(uplinkFrame.RxInfo.GatewayId, gatewayID[:]) {
		return errors.New("gateway_id is not equal to expected gateway_id")
	}

	b.uplinkFrameChan <- uplinkFrame

	return nil
}

func (b *Backend) handleGatewayStats(gatewayID lorawan.EUI64, data []byte) error {
	var gatewayStats gw.GatewayStats
	t, err := marshaler.UnmarshalGatewayStats(data, &gatewayStats)
	if err != nil {
		return errors.Wrap(err, "unmarshal error")
	}

	b.setGatewayMarshaler(gatewayID, t)

	// make sure that the registered gateway is not using a different gateway_id
	// than the ID used during the registration in Cloud IoT Core.
	if !bytes.Equal(gatewayStats.GatewayId, gatewayID[:]) {
		return errors.New("gateway_id is not equal to expected gateway_id")
	}

	b.gatewayStatsChan <- gatewayStats

	return nil
}

func (b *Backend) handleDownlinkTXAck(gatewayID lorawan.EUI64, data []byte) error {
	var ack gw.DownlinkTXAck
	t, err := marshaler.UnmarshalDownlinkTXAck(data, &ack)
	if err != nil {
		return errors.Wrap(err, "unmarshal error")
	}

	b.setGatewayMarshaler(gatewayID, t)

	// make sure that the registered gateway is not using a different gateway_id
	// than the ID used during the registration in Cloud IoT Core.
	if !bytes.Equal(ack.GatewayId, gatewayID[:]) {
		return errors.New("gateway_id is not equal to expected gateway_id")
	}

	b.downlinkTXAckChan <- ack

	return nil
}
