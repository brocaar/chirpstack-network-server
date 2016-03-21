package mqttpubsub

import (
	"encoding/json"
	"fmt"

	log "github.com/Sirupsen/logrus"
	"github.com/brocaar/loraserver"
	"github.com/brocaar/lorawan"
	"github.com/eclipse/paho.mqtt.golang"
)

// Backend implements a MQTT pub-sub application backend.
type Backend struct {
	conn *mqtt.Client
}

// NewBackend creates a new Backend.
func NewBackend(server, username, password string) (loraserver.ApplicationBackend, error) {
	b := Backend{}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(server)
	opts.SetUsername(username)
	opts.SetPassword(password)

	log.WithField("server", server).Info("application/mqttpubsub: connecting to mqtt server")
	b.conn = mqtt.NewClient(opts)
	if token := b.conn.Connect(); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	return &b, nil
}

// Close closes the backend.
func (b *Backend) Close() error {
	b.conn.Disconnect(250)
	return nil
}

// Send sends the given (collected) RXPackets the application.
func (b *Backend) Send(devEUI, appEUI lorawan.EUI64, p loraserver.ApplicationRXPacket) error {
func (b *Backend) Send(devEUI, appEUI lorawan.EUI64, p loraserver.ApplicationRXPayload) error {
	bytes, err := json.Marshal(p)
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
