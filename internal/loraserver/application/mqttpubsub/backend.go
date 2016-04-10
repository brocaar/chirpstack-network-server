package mqttpubsub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/brocaar/loraserver/internal/loraserver"
	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
	"github.com/eclipse/paho.mqtt.golang"
)

const txTopic = "application/+/node/+/tx"

var txTopicRegex = regexp.MustCompile(`application/(\w+)/node/(\w+)/tx`)

// Backend implements a MQTT pub-sub application backend.
type Backend struct {
	conn          mqtt.Client
	txPayloadChan chan models.TXPayload
	wg            sync.WaitGroup
}

// NewBackend creates a new Backend.
func NewBackend(server, username, password string) (loraserver.ApplicationBackend, error) {
	b := Backend{
		txPayloadChan: make(chan models.TXPayload),
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(server)
	opts.SetUsername(username)
	opts.SetPassword(password)
	opts.SetOnConnectHandler(b.onConnected)
	opts.SetConnectionLostHandler(b.onConnectionLost)

	log.WithField("server", server).Info("application/mqttpubsub: connecting to mqtt server")
	b.conn = mqtt.NewClient(opts)
	if token := b.conn.Connect(); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	return &b, nil
}

// Close closes the backend.
// Note that this closes the backend one-way (application to the backend).
// This makes it possible to perform a graceful shutdown (e.g. when there are
// still packets to send back to the application).
func (b *Backend) Close() error {
	log.Info("application/mqttpubsub: closing backend")
	log.WithField("topic", txTopic).Info("application/mqttpubsub: unsubscribing from tx topic")
	if token := b.conn.Unsubscribe(txTopic); token.Wait() && token.Error() != nil {
		return token.Error()
	}
	log.Info("application/mqttpubsub: handling last consumed messages")
	b.wg.Wait()
	close(b.txPayloadChan)
	return nil
}

// TXPayloadChan returns the TXPayload channel.
func (b *Backend) TXPayloadChan() chan models.TXPayload {
	return b.txPayloadChan
}

// Send sends the given (collected) RXPackets the application.
func (b *Backend) Send(devEUI, appEUI lorawan.EUI64, payload models.RXPayload) error {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	topic := fmt.Sprintf("application/%s/node/%s/rx", appEUI, devEUI)
	log.WithField("topic", topic).Info("application/mqttpubsub: publishing message")
	if token := b.conn.Publish(topic, 0, false, bytes); token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

func (b *Backend) txPayloadHandler(c mqtt.Client, msg mqtt.Message) {
	b.wg.Add(1)
	defer b.wg.Done()

	log.WithField("topic", msg.Topic()).Info("application/mqttpubsub: payload received")

	// get the DevEUI from the topic. with mqtt it is possible to perform
	// authorization on a per topic level. we need to be sure that the
	// topic DevEUI matches the payload DevEUI.
	match := txTopicRegex.FindStringSubmatch(msg.Topic())
	if len(match) != 3 {
		log.WithField("topic", msg.Topic()).Error("application/mqttpubsub: regex did not match")
		return
	}

	var txPayload models.TXPayload
	dec := json.NewDecoder(bytes.NewReader(msg.Payload()))
	if err := dec.Decode(&txPayload); err != nil {
		log.Errorf("application/mqttpubsub: could not decode ApplicationTXPayload: %s", err)
		return
	}

	if match[2] != txPayload.DevEUI.String() {
		log.WithFields(log.Fields{
			"topic_dev_eui":   match[2],
			"payload_dev_eui": txPayload.DevEUI,
		}).Warning("topic DevEUI did not match payload DevEUI")
		return
	}

	b.txPayloadChan <- txPayload
}

func (b *Backend) onConnected(c mqtt.Client) {
	log.Info("application/mqttpubsub: connected to mqtt server")
	for {
		log.WithField("topic", txTopic).Info("application/mqttpubsub: subscribing to tx topic")
		if token := b.conn.Subscribe(txTopic, 0, b.txPayloadHandler); token.Wait() && token.Error() != nil {
			log.WithField("topic", txTopic).Errorf("application/mqttpubsub: subscribe failed: %s", token.Error())
			time.Sleep(time.Second)
			continue
		}
		return
	}
}

func (b *Backend) onConnectionLost(c mqtt.Client, reason error) {
	log.Errorf("application/mqttpubsub: mqtt connection error: %s", reason)
}
