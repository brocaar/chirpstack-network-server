package azureiothub

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-amqp-common-go/cbs"
	"github.com/Azure/azure-amqp-common-go/sas"
	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/gofrs/uuid"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"pack.ag/amqp"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/backend/gateway"
	"github.com/brocaar/loraserver/internal/backend/gateway/marshaler"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/helpers"
	"github.com/brocaar/lorawan"
)

// Backend implement an Azure IoT Hub backend.
type Backend struct {
	sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
	closed bool

	uplinkFrameChan   chan gw.UplinkFrame
	gatewayStatsChan  chan gw.GatewayStats
	downlinkTxAckChan chan gw.DownlinkTXAck
	gatewayMarshaler  map[lorawan.EUI64]marshaler.Type

	queueName string
	ns        *servicebus.Namespace
	queue     *servicebus.Queue

	c2dConn            *amqp.Client
	c2dTokenProvider   *sas.TokenProvider
	c2dTokenExpiration time.Duration
	c2dHost            string
	c2dSession         *amqp.Session
	c2dSender          *amqp.Sender
}

// NewBackend creates a new Backend.
func NewBackend(c config.Config) (gateway.Gateway, error) {
	var err error
	var ok bool

	conf := c.NetworkServer.Gateway.Backend.AzureIoTHub

	b := Backend{
		uplinkFrameChan:   make(chan gw.UplinkFrame),
		gatewayStatsChan:  make(chan gw.GatewayStats),
		downlinkTxAckChan: make(chan gw.DownlinkTXAck),
		gatewayMarshaler:  make(map[lorawan.EUI64]marshaler.Type),

		ctx: context.Background(),

		c2dTokenExpiration: time.Hour,
	}

	b.ctx, b.cancel = context.WithCancel(b.ctx)

	// setup uplink
	connProperties, err := parseConnectionString(conf.EventsConnectionString)
	if err != nil {
		return nil, errors.Wrap(err, "parse connection-string error")
	}

	b.queueName, ok = connProperties["EntityPath"]
	if !ok {
		return nil, errors.New("connection-string does not contain 'EntityPath', please use the queue connection-string")
	}

	log.Info("gateway/azure_iot_hub: setting up service-bus namespace")
	b.ns, err = servicebus.NewNamespace(
		servicebus.NamespaceWithConnectionString(conf.EventsConnectionString),
	)
	if err != nil {
		return nil, errors.Wrap(err, "new namespace error")
	}

	b.queue, err = b.ns.NewQueue(b.queueName)
	if err != nil {
		return nil, errors.Wrap(err, "new queue client error")
	}

	go func() {
		log.WithField("queue", b.queueName).Info("gateway/azure_iot_hub: starting queue consumer")
		for {
			if b.closed {
				break
			}

			if err := b.queue.Receive(b.ctx, servicebus.HandlerFunc(b.eventHandler)); err != nil {
				log.WithError(err).Error("gateway/azure_iot_hub: receive from queue error, trying to recover")

				err := b.queue.Close(b.ctx)
				if err != nil {
					log.WithError(err).Error("gateway/azure_iot_hub: close queue error")
				}

				b.queue, err = b.ns.NewQueue(b.queueName)
				if err != nil {
					log.WithError(err).Error("gateway/azure_iot_hub: new queue client error")
				}
			}
		}
	}()

	// setup Cloud2Device messaging
	connProperties, err = parseConnectionString(conf.CommandsConnectionString)
	if err != nil {
		return nil, errors.Wrap(err, "parse connection-string error")
	}

	b.c2dHost, ok = connProperties["HostName"]
	if !ok {
		return nil, errors.New("connection-string does not contain 'HostName'")
	}

	if sak, ok := connProperties["SharedAccessKey"]; ok {
		bb, err := base64.StdEncoding.DecodeString(sak)
		if err != nil {
			return nil, errors.Wrap(err, "decode SharedAccessKey error")
		}

		b.c2dTokenProvider, err = sas.NewTokenProvider(sas.TokenProviderWithKey(connProperties["SharedAccessKeyName"], string(bb)))
		if err != nil {
			return nil, errors.Wrap(err, "new sas token error")
		}
	} else {
		return nil, errors.New("connection-string does not contain 'SharedAccessKey'")
	}

	if err := b.c2dNewSessionAndLink(); err != nil {
		return nil, errors.Wrap(err, "new c2d session and link error")
	}

	return &b, nil
}

func (b *Backend) SendTXPacket(pl gw.DownlinkFrame) error {
	if pl.TxInfo == nil {
		return errors.New("tx_info must not be nil")
	}

	gatewayID := helpers.GetGatewayID(pl.TxInfo)
	downID := helpers.GetDownlinkID(&pl)
	t := b.getGatewayMarshaler(gatewayID)

	bb, err := marshaler.MarshalDownlinkFrame(t, pl)
	if err != nil {
		return errors.Wrap(err, "marshal downlink frame error")
	}

	return b.publishCommand(log.Fields{
		"downlink_id": downID,
	}, gatewayID, "down", bb)
}

func (b *Backend) SendGatewayConfigPacket(pl gw.GatewayConfiguration) error {
	gatewayID := helpers.GetGatewayID(&pl)
	t := b.getGatewayMarshaler(gatewayID)

	bb, err := marshaler.MarshalGatewayConfiguration(t, pl)
	if err != nil {
		return errors.Wrap(err, "marshal gateway configuration error")
	}

	return b.publishCommand(log.Fields{}, gatewayID, "config", bb)
}

func (b *Backend) RXPacketChan() chan gw.UplinkFrame {
	return b.uplinkFrameChan
}

func (b *Backend) StatsPacketChan() chan gw.GatewayStats {
	return b.gatewayStatsChan
}

func (b *Backend) DownlinkTXAckChan() chan gw.DownlinkTXAck {
	return b.downlinkTxAckChan
}

func (b *Backend) Close() error {
	log.Info("gateway/azure_iot_hub: closing backend")
	b.cancel()
	close(b.uplinkFrameChan)
	close(b.gatewayStatsChan)
	close(b.downlinkTxAckChan)
	b.queue.Close(context.Background())
	return nil
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

func (b *Backend) eventHandler(ctx context.Context, msg *servicebus.Message) error {
	if err := b.handleEventMessage(msg); err != nil {
		log.WithError(err).Error("gateway/azure_iot_hub: handle event error")
	}

	return msg.Complete(ctx)
}

func (b *Backend) handleEventMessage(msg *servicebus.Message) error {
	var gatewayID lorawan.EUI64

	// decode gateway id
	if gwID, ok := msg.UserProperties["iothub-connection-device-id"]; ok {
		gwIDStr, ok := gwID.(string)
		if !ok {
			return fmt.Errorf("expected 'iothub-connection-device-id' to be a string, got: %T", gwID)
		}

		if err := gatewayID.UnmarshalText([]byte(gwIDStr)); err != nil {
			return errors.Wrap(err, "unmarshal gateway id error")
		}

	} else {
		return errors.New("'iothub-connection-device-id' missing in UserProperties")
	}

	var event string
	var err error

	// get event type
	if _, ok := msg.UserProperties["up"]; ok {
		event = "up"
	}
	if _, ok := msg.UserProperties["ack"]; ok {
		event = "ack"
	}
	if _, ok := msg.UserProperties["stats"]; ok {
		event = "stats"
	}

	azureEventCounter(event).Inc()

	switch event {
	case "up":
		err = b.handleUplinkFrame(gatewayID, msg.Data)
	case "stats":
		err = b.handleGatewayStats(gatewayID, msg.Data)
	case "ack":
		err = b.handleDownlinkTXAck(gatewayID, msg.Data)
	default:
		log.WithFields(log.Fields{
			"gateway_id": gatewayID,
			"event":      event,
		}).Warning("gateway/azure_iot_hub: unexpected gateway event received")
	}

	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"gateway_id":  gatewayID,
			"event":       event,
			"data_base64": base64.StdEncoding.EncodeToString(msg.Data),
		}).Error("gateway/azure_iot_hub: handle gateway event error")
	}

	return nil
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

	uplinkID := helpers.GetUplinkID(uplinkFrame.RxInfo)

	log.WithFields(log.Fields{
		"gateway_id": gatewayID,
		"uplink_id":  uplinkID,
	}).Info("gateway/azure_iot_hub: uplink received from gateway")

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
	}).Info("gateway/azure_iot_hub: stats received from gateway")

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

	downlinkID := helpers.GetDownlinkID(&ack)

	log.WithFields(log.Fields{
		"gateway_id":  gatewayID,
		"downlink_id": downlinkID,
	}).Info("gateway/azure_iot_hub: ack received from gateway")

	b.downlinkTxAckChan <- ack

	return nil
}

func (b *Backend) publishCommand(fields log.Fields, gatewayID lorawan.EUI64, command string, data []byte) error {
	msgID, err := uuid.NewV4()
	if err != nil {
		return errors.Wrap(err, "new uuid error")
	}

	msg := amqp.NewMessage(data)
	msg.Properties = &amqp.MessageProperties{
		MessageID: msgID.String(),
		To:        fmt.Sprintf("/devices/%s/messages/devicebound", gatewayID),
	}
	msg.ApplicationProperties = map[string]interface{}{
		"iothub-ack": "none",
		"command":    command,
	}

	retries := 0

	for {
		select {
		case <-b.ctx.Done():
			return nil
		default:
			// aquire a read-lock to make sure an other go routine isn't
			// recovering / re-connecting in case of an error.
			b.RLock()
			err = b.c2dSender.Send(b.ctx, msg)
			b.RUnlock()
			if err == nil {
				fields["gateway_id"] = gatewayID
				fields["command"] = command
				log.WithFields(fields).Info("gateway/azure_iot_hub: gateway command published")

				azureCommandCounter(command).Inc()

				return nil
			}

			if retries > 0 {
				log.WithError(err).Error("gateway/azure_iot_hub: send cloud to device message error")
			}

			// try to recover connection
			if err := b.c2dRecover(); err != nil {
				log.WithError(err).Error("gateway/azure_iot_hub: recover iot hub connection error, retry in 2 seconds")
				time.Sleep(2 * time.Second)
			}
		}

		retries++
	}
}

func (b *Backend) c2dNewSessionAndLink() error {
	var err error
	log.WithField("host", b.c2dHost).Info("gateway/azure_iot_hub: connecting to iot hub")
	b.c2dConn, err = amqp.Dial(fmt.Sprintf("amqps://%s", b.c2dHost), amqp.ConnSASLAnonymous())
	if err != nil {
		return errors.Wrap(err, "amqp dial error")
	}

	log.Info("gateway/azure_iot_hub: negotiating amqp cbs claim")
	if err := cbs.NegotiateClaim(b.ctx, b.c2dHost, b.c2dConn, b.c2dTokenProvider); err != nil {
		return errors.Wrap(err, "cbs negotiate claim error")
	}

	b.c2dSession, err = b.c2dConn.NewSession()
	if err != nil {
		return errors.Wrap(err, "new amqp session error")
	}

	b.c2dSender, err = b.c2dSession.NewSender(
		amqp.LinkTargetAddress("/messages/devicebound"),
	)
	if err != nil {
		return errors.Wrap(err, "new amqp sender error")
	}

	return nil
}

func (b *Backend) c2dRecover() error {
	// aquire a write-lock to make sure that no Send calls are made during the
	// connection recovery
	b.Lock()
	defer b.Unlock()

	azureConnectionRecoverCounter().Inc()

	log.Info("gateway/azure_iot_hub: re-connecting to iot hub")
	_ = b.c2dSender.Close(b.ctx)
	_ = b.c2dSession.Close(b.ctx)
	_ = b.c2dConn.Close()

	return b.c2dNewSessionAndLink()
}

func parseConnectionString(str string) (map[string]string, error) {
	out := make(map[string]string)
	pairs := strings.Split(str, ";")
	for _, pair := range pairs {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("expected two items in: %+v", kv)
		}

		out[kv[0]] = kv[1]
	}

	return out, nil
}
