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
	"github.com/Azure/azure-amqp-common-go/uuid"
	servicebus "github.com/Azure/azure-service-bus-go"
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
				log.WithError(err).Error("gateway/azure_iot_hub: receive from queue error")
				time.Sleep(time.Second * 2)
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

	// credentials are set by the negotiateClaimLoop method
	b.c2dConn, err = amqp.Dial(fmt.Sprintf("amqps://%s", b.c2dHost), amqp.ConnSASLAnonymous())
	if err != nil {
		return nil, errors.Wrap(err, "amqp dial error")
	}

	go b.negotiateClaimLoop()

	return &b, nil
}

func (b *Backend) SendTXPacket(pl gw.DownlinkFrame) error {
	if pl.TxInfo == nil {
		return errors.New("tx_info must not be nil")
	}

	gatewayID := helpers.GetGatewayID(pl.TxInfo)
	t := b.getGatewayMarshaler(gatewayID)

	bb, err := marshaler.MarshalDownlinkFrame(t, pl)
	if err != nil {
		return errors.Wrap(err, "marshal downlink frame error")
	}

	return b.publishCommand(gatewayID, "down", bb)
}

func (b *Backend) SendGatewayConfigPacket(pl gw.GatewayConfiguration) error {
	gatewayID := helpers.GetGatewayID(&pl)
	t := b.getGatewayMarshaler(gatewayID)

	bb, err := marshaler.MarshalGatewayConfiguration(t, pl)
	if err != nil {
		return errors.Wrap(err, "marshal gateway configuration error")
	}

	return b.publishCommand(gatewayID, "config", bb)
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

	log.WithFields(log.Fields{
		"gateway_id": gatewayID,
		"event":      event,
	}).Info("gateway/azure_iot_hub: event received from gateway")

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

	b.downlinkTxAckChan <- ack

	return nil
}

func (b *Backend) publishCommand(gatewayID lorawan.EUI64, command string, data []byte) error {
	// needed because of negotiateClaimLoop
	b.RLock()
	defer b.RUnlock()

	var err error

	// init (new) session
	if b.c2dSession == nil {
		b.c2dSession, err = b.c2dConn.NewSession()
		if err != nil {
			return errors.Wrap(err, "new amqp session error")
		}
	}

	// init (new) sender
	if b.c2dSender == nil {
		b.c2dSender, err = b.c2dSession.NewSender(
			amqp.LinkTargetAddress("/messages/devicebound"),
		)
		if err != nil {
			return errors.Wrap(err, "new amqp sender error")
		}
	}

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

	if err := b.c2dSender.Send(b.ctx, msg); err != nil {
		return errors.Wrap(err, "amqp send error")
	}

	log.WithFields(log.Fields{
		"gateway_id": gatewayID,
		"command":    command,
	}).Info("gateway/azure_iot_hub: gateway command published")

	return nil
}

func (b *Backend) negotiateClaimLoop() {
	ticker := time.NewTimer(0)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.Lock()

			// close the sender and session first
			// testing turned out that without this, Send calls would be rejected due to invalid token
			if b.c2dSender != nil {
				if err := b.c2dSender.Close(b.ctx); err != nil {
					log.WithError(err).Error("gateway/azure_iot_hub: close amqp sender error")
				}
				b.c2dSender = nil
			}

			if b.c2dSession != nil {
				if err := b.c2dSession.Close(b.ctx); err != nil {
					log.WithError(err).Error("gateway/azure_iot_hub: close amqp session error")
				}
				b.c2dSession = nil
			}

			// negotiate cbs claim
			log.Info("gateway/azure_iot_hub: negotiating amqp cbs claim")
			if err := cbs.NegotiateClaim(b.ctx, b.c2dHost, b.c2dConn, b.c2dTokenProvider); err != nil {
				log.WithError(err).Error("gateway/azure_iot_hub: negotiate amqp cbs claim error")
				ticker.Reset(2 * time.Second)
				continue
			}

			b.Unlock()
		case <-b.ctx.Done():
			return
		}

		ticker.Reset(b.c2dTokenExpiration)
	}

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
