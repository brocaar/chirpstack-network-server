package amqp

import (
	"bytes"
	"regexp"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/streadway/amqp"

	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-network-server/internal/backend/gateway"
	"github.com/brocaar/chirpstack-network-server/internal/backend/gateway/marshaler"
	"github.com/brocaar/chirpstack-network-server/internal/config"
	"github.com/brocaar/chirpstack-network-server/internal/helpers"
	"github.com/brocaar/lorawan"
)

var gatewayIDRegexp = regexp.MustCompile(`([0-9a-fA-F]{16})`)

// Backend implements an AMQP backend.
type Backend struct {
	chPool *pool

	eventQueueName    string
	eventRoutingKey   string
	commandRoutingKey *template.Template

	uplinkFrameChan   chan gw.UplinkFrame
	gatewayStatsChan  chan gw.GatewayStats
	downlinkTXAckChan chan gw.DownlinkTXAck

	gatewayMarshalerMux sync.RWMutex
	gatewayMarshaler    map[lorawan.EUI64]marshaler.Type
	downMode            string
}

// NewBackend creates a new Backend.
func NewBackend(c config.Config) (gateway.Gateway, error) {
	var err error
	config := c.NetworkServer.Gateway.Backend.AMQP

	b := Backend{
		eventQueueName:    config.EventQueueName,
		eventRoutingKey:   config.EventRoutingKey,
		gatewayMarshaler:  make(map[lorawan.EUI64]marshaler.Type),
		uplinkFrameChan:   make(chan gw.UplinkFrame),
		gatewayStatsChan:  make(chan gw.GatewayStats),
		downlinkTXAckChan: make(chan gw.DownlinkTXAck),
		downMode:          c.NetworkServer.Gateway.Backend.MultiDownlinkFeature,
	}

	log.Info("gateway/amqp: connecting to AMQP server")
	b.chPool, err = newPool(10, config.URL)
	if err != nil {
		return nil, errors.Wrap(err, "new amqp channel pool error")
	}

	b.commandRoutingKey, err = template.New("command").Parse(config.CommandRoutingKeyTemplate)
	if err != nil {
		return nil, errors.Wrap(err, "gateway/amqp: parse command routing-key template error")
	}

	if err := b.setupQueue(); err != nil {
		return nil, errors.Wrap(err, "gateway/amqp: setup queue error")
	}

	go b.eventLoop()

	return &b, nil
}

func (b *Backend) SendTXPacket(pl gw.DownlinkFrame) error {
	gatewayID := helpers.GetGatewayID(&pl)
	downID := helpers.GetDownlinkID(&pl)
	t := b.getGatewayMarshaler(gatewayID)

	if err := gateway.UpdateDownlinkFrame(b.downMode, &pl); err != nil {
		return errors.Wrap(err, "set downlink compatibility mode error")
	}

	bb, err := marshaler.MarshalDownlinkFrame(t, pl)
	if err != nil {
		return errors.Wrap(err, "gateway/amqp: marshal downlink frame error")
	}

	return b.publishCommand(log.Fields{
		"downlink_id": downID,
	}, gatewayID, "down", t, bb)
}

func (b *Backend) SendGatewayConfigPacket(pl gw.GatewayConfiguration) error {
	gatewayID := helpers.GetGatewayID(&pl)
	t := b.getGatewayMarshaler(gatewayID)

	bb, err := marshaler.MarshalGatewayConfiguration(t, pl)
	if err != nil {
		return errors.Wrap(err, "gateway/amqp: marshal gateway configuration error")
	}

	return b.publishCommand(log.Fields{}, gatewayID, "config", t, bb)
}

func (b *Backend) RXPacketChan() chan gw.UplinkFrame {
	return b.uplinkFrameChan
}

func (b *Backend) StatsPacketChan() chan gw.GatewayStats {
	return b.gatewayStatsChan
}

func (b *Backend) DownlinkTXAckChan() chan gw.DownlinkTXAck {
	return b.downlinkTXAckChan
}

func (b *Backend) Close() error {
	b.chPool.close()
	close(b.uplinkFrameChan)
	close(b.gatewayStatsChan)
	close(b.downlinkTXAckChan)
	return b.chPool.close()
}

func (b *Backend) publishCommand(fields log.Fields, gatewayID lorawan.EUI64, command string, t marshaler.Type, data []byte) error {
	ch, err := b.chPool.get()
	if err != nil {
		return errors.Wrap(err, "get amqp channel from pool error")
	}
	defer ch.close()

	templateCtx := struct {
		GatewayID   lorawan.EUI64
		CommandType string
	}{gatewayID, command}
	topic := bytes.NewBuffer(nil)
	if err := b.commandRoutingKey.Execute(topic, templateCtx); err != nil {
		return errors.Wrap(err, "execute command topic error")
	}

	fields["gateway_id"] = gatewayID
	fields["command"] = command
	fields["routing_key"] = topic.String()

	var contentType string
	switch t {
	case marshaler.JSON:
		contentType = "application/json"
	case marshaler.Protobuf:
		contentType = "application/octet-stream"
	}

	amqpCommandCounter(command).Inc()

	err = ch.ch.Publish(
		"amq.topic",
		topic.String(),
		false,
		false,
		amqp.Publishing{
			ContentType: contentType,
			Body:        data,
		},
	)
	if err != nil {
		return errors.Wrap(err, "publish message error")
	}

	return nil
}

func (b *Backend) setupQueue() error {
	ch, err := b.chPool.get()
	if err != nil {
		return errors.Wrap(err, "open channel error")
	}
	defer ch.close()

	_, err = ch.ch.QueueDeclare(
		b.eventQueueName,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return errors.Wrap(err, "declare queue error")
	}

	err = ch.ch.QueueBind(
		b.eventQueueName,
		b.eventRoutingKey,
		"amq.topic",
		false,
		nil,
	)
	if err != nil {
		return errors.Wrap(err, "bind queue error")
	}

	return nil
}

func (b *Backend) eventLoop() {
	for {
		err := func() error {
			// borrow amqp channel from the pool
			ch, err := b.chPool.get()
			if err != nil {
				return errors.Wrap(err, "get amqp channel from pool error")
			}
			defer ch.close()

			log.Info("gateway/amqp: start consuming gateway events")

			// get message channel
			msgs, err := ch.ch.Consume(
				b.eventQueueName,
				"",
				true,
				false,
				false,
				false,
				nil,
			)
			if err != nil {
				ch.markUnusable()
				return errors.Wrap(err, "register consumer error")
			}

			// iterate over messages in message channel
			for msg := range msgs {
				var err error

				routing := strings.Split(msg.RoutingKey, ".")
				typ := routing[len(routing)-1]

				switch typ {
				case "up":
					amqpEventCounter("up").Inc()
					err = b.handleUplinkFrame(msg)
				case "ack":
					amqpEventCounter("ack").Inc()
					err = b.handleDownlinkTXAck(msg)
				case "stats":
					amqpEventCounter("stats").Inc()
					err = b.handleGatewayStats(msg)
				default:
					log.WithFields(log.Fields{
						"routing_key": msg.RoutingKey,
						"type":        typ,
					}).Warning("gateway/amqp: unexpected event type")
				}

				if err != nil {
					log.WithError(err).WithFields(log.Fields{
						"type":        typ,
						"routing_key": msg.RoutingKey,
					}).Error("gateway/amqp: handle event error")
				}
			}

			return nil
		}()
		if err != nil {
			// if errClosed, the channel pool was closed and we can break out
			// of the loop
			if errors.Cause(err) == errClosed {
				break
			}

			// in case of any other error, print log and start over again
			// (in the loop).
			log.WithError(err).Error("gateway/amqp: event loop error")
			time.Sleep(time.Second)
		}
	}
}

func (b *Backend) handleUplinkFrame(msg amqp.Delivery) error {
	var uplinkFrame gw.UplinkFrame
	t, err := marshaler.UnmarshalUplinkFrame(msg.Body, &uplinkFrame)
	if err != nil {
		return errors.Wrap(err, "unmarshal error")
	}

	if uplinkFrame.RxInfo == nil {
		return errors.New("rx_info must not be nil")
	}

	if uplinkFrame.TxInfo == nil {
		return errors.New("tx_info must not be nil")
	}

	gatewayID := helpers.GetGatewayID(uplinkFrame.GetRxInfo())
	if err := validateGatewayID(msg.RoutingKey, gatewayID); err != nil {
		return errors.Wrap(err, "validate gateway ID error")
	}

	b.setGatewayMarshaler(gatewayID, t)

	upID := helpers.GetUplinkID(uplinkFrame.GetRxInfo())

	log.WithFields(log.Fields{
		"gateway_id": gatewayID,
		"uplink_id":  upID,
	}).Info("gateway/amqp: uplink event received")

	b.uplinkFrameChan <- uplinkFrame

	return nil
}

func (b *Backend) handleDownlinkTXAck(msg amqp.Delivery) error {
	var downlinkTXAck gw.DownlinkTXAck
	t, err := marshaler.UnmarshalDownlinkTXAck(msg.Body, &downlinkTXAck)
	if err != nil {
		return errors.Wrap(err, "unmarshal error")
	}

	gatewayID := helpers.GetGatewayID(&downlinkTXAck)
	if err := validateGatewayID(msg.RoutingKey, gatewayID); err != nil {
		return errors.Wrap(err, "validate gateway ID error")
	}

	b.setGatewayMarshaler(gatewayID, t)

	downID := helpers.GetDownlinkID(&downlinkTXAck)

	log.WithFields(log.Fields{
		"gateway_id":  gatewayID,
		"downlink_id": downID,
	}).Info("gateway/amqp: ack event received")

	b.downlinkTXAckChan <- downlinkTXAck

	return nil
}

func (b *Backend) handleGatewayStats(msg amqp.Delivery) error {
	var gatewayStats gw.GatewayStats
	t, err := marshaler.UnmarshalGatewayStats(msg.Body, &gatewayStats)
	if err != nil {
		return errors.Wrap(err, "unmarshal error")
	}

	gatewayID := helpers.GetGatewayID(&gatewayStats)
	if err := validateGatewayID(msg.RoutingKey, gatewayID); err != nil {
		return errors.Wrap(err, "validate gateway ID error")
	}

	b.setGatewayMarshaler(gatewayID, t)

	statsID := helpers.GetStatsID(&gatewayStats)

	log.WithFields(log.Fields{
		"gateway_id": gatewayID,
		"stats_id":   statsID,
	}).Info("gateway/amqp: stats event received")

	b.gatewayStatsChan <- gatewayStats
	return nil
}

func (b *Backend) setGatewayMarshaler(gatewayID lorawan.EUI64, t marshaler.Type) {
	b.gatewayMarshalerMux.Lock()
	defer b.gatewayMarshalerMux.Unlock()

	b.gatewayMarshaler[gatewayID] = t
}

func (b *Backend) getGatewayMarshaler(gatewayID lorawan.EUI64) marshaler.Type {
	b.gatewayMarshalerMux.RLock()
	defer b.gatewayMarshalerMux.RUnlock()

	return b.gatewayMarshaler[gatewayID]
}

func validateGatewayID(routingKey string, gatewayID lorawan.EUI64) error {
	idStr := gatewayIDRegexp.FindString(routingKey)

	var id lorawan.EUI64
	if err := id.UnmarshalText([]byte(idStr)); err != nil {
		return errors.Wrap(err, "unmarshal gateway id error")
	}

	if gatewayID != id {
		return errors.New("message gateway ID does not match routing-key gateway ID!")
	}

	return nil
}
