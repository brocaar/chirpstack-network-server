package mqttpubsub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"

	log "github.com/Sirupsen/logrus"
	"github.com/brocaar/loraserver"
	"github.com/brocaar/lorawan"
	"github.com/eclipse/paho.mqtt.golang"
)

var txTopicRegex = regexp.MustCompile(`application/(\w+)/node/(\w+)/tx`)

// Backend implements a MQTT pub-sub application backend.
type Backend struct {
	conn          *mqtt.Client
	txPayloadChan chan loraserver.TXPayload
}

// NewBackend creates a new Backend.
func NewBackend(server, username, password string) (loraserver.ApplicationBackend, error) {
	b := Backend{
		txPayloadChan: make(chan loraserver.TXPayload),
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(server)
	opts.SetUsername(username)
	opts.SetPassword(password)
	opts.SetOnConnectHandler(b.OnConnected)

	log.WithField("server", server).Info("application/mqttpubsub: connecting to mqtt server")
	b.conn = mqtt.NewClient(opts)
	if token := b.conn.Connect(); token.Wait() && token.Error() != nil {
		log.WithField("server", server).Info("application/mqttpubsub: connecting to mqtt server")
		return nil, token.Error()
	}

	return &b, nil
}

// Close closes the backend.
func (b *Backend) Close() error {
	b.conn.Disconnect(250)
	return nil
}

// TXPayloadChan returns the TXPayload channel.
func (b *Backend) TXPayloadChan() chan loraserver.TXPayload {
	return b.txPayloadChan
}

// Send sends the given (collected) RXPackets the application.
func (b *Backend) Send(devEUI, appEUI lorawan.EUI64, payload loraserver.RXPayload) error {
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

func (b *Backend) txPayloadHandler(c *mqtt.Client, msg mqtt.Message) {
	// get the DevEUI from the topic. with mqtt it is possible to perform
	// authorization on a per topic level. we need to be sure that the
	// topic DevEUI matches the payload DevEUI.
	match := txTopicRegex.FindStringSubmatch(msg.Topic())
	if len(match) != 3 {
		log.WithField("topic", msg.Topic()).Error("application/mqttpubsub: regex did not match")
		return
	}

	var txPayload loraserver.TXPayload
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

// Subscribe to Topic
func (b *Backend) OnConnected(c *mqtt.Client) {
	log.WithField("topic", "application/+/node/+/tx").Info("application/mqttpubsub: subscribing to tx topic")
	if token := b.conn.Subscribe("application/+/node/+/tx", 0, b.txPayloadHandler); token.Wait() && token.Error() != nil {
		log.WithField("error", "application/+/node/+/tx").Info("application/mqttpubsub: subscribing to tx topic error: %s", token.Error())
	}
}
