package mqttpubsub

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/brocaar/loraserver/internal/loraserver"
	"github.com/brocaar/loraserver/models"
	"github.com/eclipse/paho.mqtt.golang"
)

const rxTopic = "gateway/+/rx"

// Backend implements a MQTT pub-sub backend.
type Backend struct {
	conn         mqtt.Client
	rxPacketChan chan models.RXPacket
	wg           sync.WaitGroup
}

// NewBackend creates a new Backend.
func NewBackend(server, username, password string) (loraserver.GatewayBackend, error) {
	b := Backend{
		rxPacketChan: make(chan models.RXPacket),
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(server)
	opts.SetUsername(username)
	opts.SetPassword(password)
	opts.SetOnConnectHandler(b.onConnected)
	opts.SetConnectionLostHandler(b.onConnectionLost)

	log.WithField("server", server).Info("gateway/mqttpubsub: connecting to mqtt server")
	b.conn = mqtt.NewClient(opts)
	if token := b.conn.Connect(); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	return &b, nil
}

// Close closes the backend.
// Note that this closes the backend one-way (gateway to backend).
// This makes it possible to perform a graceful shutdown (e.g. when there are
// still packets to send back to the gateway).
func (b *Backend) Close() error {
	log.Info("gateway/mqttpubsub: closing backend")
	log.WithField("topic", rxTopic).Info("gateway/mqttpubsub: unsubscribing from rx topic")
	if token := b.conn.Unsubscribe(rxTopic); token.Wait() && token.Error() != nil {
		return token.Error()
	}
	log.Info("gateway/mqttpubsub: handling last consumed messages")
	b.wg.Wait()
	close(b.rxPacketChan)
	return nil
}

// RXPacketChan returns the RXPacket channel.
func (b *Backend) RXPacketChan() chan models.RXPacket {
	return b.rxPacketChan
}

// Send sends the given TXPacket to the gateway.
func (b *Backend) Send(txPacket models.TXPacket) error {
	bytes, err := json.Marshal(txPacket)
	if err != nil {
		return err
	}

	topic := fmt.Sprintf("gateway/%s/tx", txPacket.TXInfo.MAC)
	log.WithField("topic", topic).Info("gateway/mqttpubsub: publishing TXPacket")

	token := b.conn.Publish(topic, 0, false, bytes)
	token.Wait()
	return token.Error()
}

func (b *Backend) rxPacketHandler(c mqtt.Client, msg mqtt.Message) {
	b.wg.Add(1)
	defer b.wg.Done()

	log.Info("gateway/mqttpubsub: RXPacket received")

	var rxPacket models.RXPacket
	if err := json.Unmarshal(msg.Payload(), &rxPacket); err != nil {
		log.WithFields(log.Fields{
			"data_base64": base64.StdEncoding.EncodeToString(msg.Payload()),
		}).Errorf("gateway/mqttpubsub: could not decode RXPacket: %s", err)
		return
	}

	b.rxPacketChan <- rxPacket
}

func (b *Backend) onConnected(c mqtt.Client) {
	log.Info("gateway/mqttpubsub: connected to mqtt server")
	for {
		log.WithField("topic", rxTopic).Info("gateway/mqttpubsub: subscribing to rx topic")
		if token := b.conn.Subscribe(rxTopic, 2, b.rxPacketHandler); token.Wait() && token.Error() != nil {
			log.WithField("topic", rxTopic).Errorf("gateway/mqttpubsub: subscribe failed: %s", token.Error())
			time.Sleep(time.Second)
			continue
		}
		return
	}
}

func (b *Backend) onConnectionLost(c mqtt.Client, reason error) {
	log.Errorf("gateway/mqttpubsub: mqtt connection error: %s", reason)
}
