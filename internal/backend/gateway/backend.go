package gateway

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/backend"
	"github.com/eclipse/paho.mqtt.golang"
)

const rxTopic = "gateway/+/rx"

// Backend implements a MQTT pub-sub backend.
type Backend struct {
	conn         mqtt.Client
	rxPacketChan chan gw.RXPacket
	wg           sync.WaitGroup
}

// NewBackend creates a new Backend.
func NewBackend(server, username, password string) (backend.Gateway, error) {
	b := Backend{
		rxPacketChan: make(chan gw.RXPacket),
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(server)
	opts.SetUsername(username)
	opts.SetPassword(password)
	opts.SetOnConnectHandler(b.onConnected)
	opts.SetConnectionLostHandler(b.onConnectionLost)

	log.WithField("server", server).Info("backend/gateway: connecting to mqtt broker")
	b.conn = mqtt.NewClient(opts)
	if token := b.conn.Connect(); token.Wait() && token.Error() != nil {
		return nil, fmt.Errorf("backend/gateway: connecting to broker failed: %s", token.Error())
	}

	return &b, nil
}

// Close closes the backend.
// Note that this closes the backend one-way (gateway to backend).
// This makes it possible to perform a graceful shutdown (e.g. when there are
// still packets to send back to the gateway).
func (b *Backend) Close() error {
	log.Info("backend/gateway: closing backend")
	log.WithField("topic", rxTopic).Info("backend/gateway: unsubscribing from rx topic")
	if token := b.conn.Unsubscribe(rxTopic); token.Wait() && token.Error() != nil {
		return fmt.Errorf("backend/gateway: unsubscribe from %s failed: %s", rxTopic, token.Error())
	}
	log.Info("backend/gateway: handling last messages")
	b.wg.Wait()
	close(b.rxPacketChan)
	return nil
}

// RXPacketChan returns the RXPacket channel.
func (b *Backend) RXPacketChan() chan gw.RXPacket {
	return b.rxPacketChan
}

// SendTXPacket sends the given TXPacket to the gateway.
func (b *Backend) SendTXPacket(txPacket gw.TXPacket) error {
	bytes, err := json.Marshal(txPacket)
	if err != nil {
		return fmt.Errorf("backend/gateway: tx packet marshal error: %s", err)
	}

	topic := fmt.Sprintf("gateway/%s/tx", txPacket.TXInfo.MAC)
	log.WithField("topic", topic).Info("backend/gateway: publishing tx packet")

	if token := b.conn.Publish(topic, 0, false, bytes); token.Wait() && token.Error() != nil {
		return fmt.Errorf("backend/gateway: publish tx packet failed: %s", token.Error())
	}
	return nil
}

func (b *Backend) rxPacketHandler(c mqtt.Client, msg mqtt.Message) {
	b.wg.Add(1)
	defer b.wg.Done()

	log.Info("backend/gateway: rx packet received")

	var rxPacket gw.RXPacket
	if err := json.Unmarshal(msg.Payload(), &rxPacket); err != nil {
		log.WithFields(log.Fields{
			"data_base64": base64.StdEncoding.EncodeToString(msg.Payload()),
		}).Errorf("backend/gateway: unmarshal rx packet error: %s", err)
		return
	}

	b.rxPacketChan <- rxPacket
}

func (b *Backend) onConnected(c mqtt.Client) {
	log.Info("backend/gateway: connected to mqtt server")
	for {
		log.WithField("topic", rxTopic).Info("backend/gateway: subscribing to rx topic")
		if token := b.conn.Subscribe(rxTopic, 2, b.rxPacketHandler); token.Wait() && token.Error() != nil {
			log.WithField("topic", rxTopic).Errorf("backend/gateway: subscribe error: %s", token.Error())
			time.Sleep(time.Second)
			continue
		}
		return
	}
}

func (b *Backend) onConnectionLost(c mqtt.Client, reason error) {
	log.Errorf("backend/gateway: mqtt connection error: %s", reason)
}
