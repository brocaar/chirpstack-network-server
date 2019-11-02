package gcppubsub

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/option"

	"github.com/brocaar/chirpstack-network-server/api/gw"
	"github.com/brocaar/chirpstack-network-server/internal/backend/gateway"
	"github.com/brocaar/chirpstack-network-server/internal/backend/gateway/marshaler"
	"github.com/brocaar/chirpstack-network-server/internal/config"
	"github.com/brocaar/chirpstack-network-server/internal/helpers"
	"github.com/brocaar/lorawan"
)

const uplinkSubscriptionTmpl = "%s-chirpstack"

// Backend implements a Google Cloud Pub/Sub backend.
type Backend struct {
	sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc

	client             *pubsub.Client
	downlinkTopic      *pubsub.Topic
	uplinkTopic        *pubsub.Topic
	uplinkSubscription *pubsub.Subscription

	uplinkFrameChan   chan gw.UplinkFrame
	gatewayStatsChan  chan gw.GatewayStats
	downlinkTXAckChan chan gw.DownlinkTXAck
	gatewayMarshaler  map[lorawan.EUI64]marshaler.Type
}

// NewBackend creates a new Backend.
func NewBackend(c config.Config) (gateway.Gateway, error) {
	conf := c.NetworkServer.Gateway.Backend.GCPPubSub

	b := Backend{
		gatewayMarshaler:  make(map[lorawan.EUI64]marshaler.Type),
		uplinkFrameChan:   make(chan gw.UplinkFrame),
		gatewayStatsChan:  make(chan gw.GatewayStats),
		downlinkTXAckChan: make(chan gw.DownlinkTXAck),
		ctx:               context.Background(),
	}
	var err error
	var o []option.ClientOption

	b.ctx, b.cancel = context.WithCancel(b.ctx)

	if conf.CredentialsFile != "" {
		o = append(o, option.WithCredentialsFile(conf.CredentialsFile))
	}

	log.Info("gateway/gcp_pub_sub: setting up client")
	b.client, err = pubsub.NewClient(b.ctx, conf.ProjectID, o...)
	if err != nil {
		return nil, errors.Wrap(err, "gateway/gcp_pub_sub: new pubsub client error")
	}

	log.WithField("topic", conf.DownlinkTopicName).Info("gateway/gcp_pub_sub: setup downlink topic")
	b.downlinkTopic = b.client.Topic(conf.DownlinkTopicName)
	ok, err := b.downlinkTopic.Exists(b.ctx)
	if err != nil {
		return nil, errors.Wrap(err, "gateway/gcp_pub_sub: topic exists error")
	}
	if !ok {
		return nil, fmt.Errorf("gateway/gcp_pub_sub: downlink topic '%s' does not exist", conf.DownlinkTopicName)
	}

	log.WithField("topic", conf.UplinkTopicName).Info("gateway/gcp_pub_sub: setup uplink topic")
	b.uplinkTopic = b.client.Topic(conf.UplinkTopicName)
	ok, err = b.uplinkTopic.Exists(b.ctx)
	if err != nil {
		return nil, errors.Wrap(err, "gateway/gcp_pub_sub: topic exists error")
	}
	if !ok {
		return nil, fmt.Errorf("gateway/gcp_pub_sub: uplink topic '%s' does not exist", conf.UplinkTopicName)
	}

	upSubName := fmt.Sprintf(uplinkSubscriptionTmpl, conf.UplinkTopicName)

	log.WithField("subscription", upSubName).Info("gateway/gcp_pub_sub: check if uplink subscription exists")
	b.uplinkSubscription = b.client.Subscription(upSubName)
	ok, err = b.uplinkSubscription.Exists(b.ctx)
	if err != nil {
		return nil, errors.Wrap(err, "gateway/gcp_pub_sub: subscription exists error")
	}

	// try to create the subscription if it doesn't exist
	if !ok {
		log.WithField("subscription", upSubName).Info("gateway/gcp_pub_sub: create uplink subscription")
		b.uplinkSubscription, err = b.client.CreateSubscription(b.ctx, upSubName, pubsub.SubscriptionConfig{
			Topic:             b.uplinkTopic,
			RetentionDuration: conf.UplinkRetentionDuration,
		})
	}

	// consume uplink frames
	go func() {
		for {
			err := b.uplinkSubscription.Receive(b.ctx, b.receiveFunc)
			if err != nil {
				log.WithError(err).Error("gateway/gcp_pub_sub: receive error")
				time.Sleep(time.Second * 2)
				continue
			}

			break
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
	downID := helpers.GetDownlinkID(&pl)
	t := b.getGatewayMarshaler(gatewayID)

	bb, err := marshaler.MarshalDownlinkFrame(t, pl)
	if err != nil {
		return errors.Wrap(err, "gateway/gcp_pub_sub: marshal downlink frame error")
	}

	return b.publishCommand(log.Fields{
		"downlink_id": downID,
	}, gatewayID, "down", bb)
}

// SendGatewayConfigPacket sends the given gateway configuration to the gateway.
func (b *Backend) SendGatewayConfigPacket(pl gw.GatewayConfiguration) error {
	gatewayID := helpers.GetGatewayID(&pl)
	t := b.getGatewayMarshaler(gatewayID)

	bb, err := marshaler.MarshalGatewayConfiguration(t, pl)
	if err != nil {
		return errors.Wrap(err, "gateway/gcp_pub_sub: marshal gateway configuration error")
	}

	return b.publishCommand(log.Fields{}, gatewayID, "config", bb)
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
	log.Info("gateway/gcp_pub_sub: closing backend")
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

func (b *Backend) publishCommand(fields log.Fields, gatewayID lorawan.EUI64, command string, data []byte) error {
	start := time.Now()

	res := b.downlinkTopic.Publish(b.ctx, &pubsub.Message{
		Data: data,
		Attributes: map[string]string{
			"deviceId":  "gw-" + gatewayID.String(),
			"subFolder": command,
		},
	})
	if _, err := res.Get(b.ctx); err != nil {
		return errors.Wrap(err, "get publish result error")
	}

	fields["duration"] = time.Now().Sub(start)
	fields["gateway_id"] = gatewayID
	fields["command"] = command

	log.WithFields(fields).Info("gateway/gcp_pub_sub: message published")

	gcpCommandCounter(command).Inc()

	return nil
}

func (b *Backend) receiveFunc(ctx context.Context, msg *pubsub.Message) {
	msg.Ack()

	var gatewayID lorawan.EUI64

	gatewayIDStr, ok := msg.Attributes["deviceId"]
	if !ok {
		log.Error("gateway/gcp_pub_sub: received message does not contain 'deviceId' attribute")
	}

	typ, ok := msg.Attributes["subFolder"]
	if !ok {
		log.Error("gateway/gcp_pub_sub: received message does not contain 'subFolder' attribute")
	}

	gatewayIDStr = strings.Replace(gatewayIDStr, "gw-", "", 1)
	if err := gatewayID.UnmarshalText([]byte(gatewayIDStr)); err != nil {
		log.WithError(err).Error("gateway/gcp_pub_sub: unmarshal gateway id error")
	}

	gcpEventCounter(typ).Inc()

	var err error

	switch typ {
	case "up":
		err = b.handleUplinkFrame(gatewayID, msg.Data)
	case "stats":
		err = b.handleGatewayStats(gatewayID, msg.Data)
	case "ack":
		err = b.handleDownlinkTXAck(gatewayID, msg.Data)
	default:
		log.WithFields(log.Fields{
			"gateway_id": gatewayID,
			"type":       typ,
		}).Warning("gateway/gcp_pub_sub: unexpected message type")
	}

	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"gateway_id":  gatewayID,
			"type":        typ,
			"data_base64": base64.StdEncoding.EncodeToString(msg.Data),
		}).Error("gateway/gcp_pub_sub: handle received message error")
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

	upID := helpers.GetUplinkID(uplinkFrame.RxInfo)

	log.WithFields(log.Fields{
		"gateway_id": gatewayID,
		"uplink_id":  upID,
	}).Info("gateway/gcp_pub_sub: uplink event received")

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

	statsID := helpers.GetStatsID(&gatewayStats)

	log.WithFields(log.Fields{
		"gateway_id": gatewayID,
		"stats_id":   statsID,
	}).Info("gateway/gcp_pub_sub: stats event received")

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

	downID := helpers.GetDownlinkID(&ack)

	log.WithFields(log.Fields{
		"gateway_id":  gatewayID,
		"downlink_id": downID,
	}).Info("gateway/gcp_pub_sub: ack event received")

	b.downlinkTXAckChan <- ack

	return nil
}
