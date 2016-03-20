package mqttpubsub

import (
	"encoding/json"
	"errors"
	"fmt"

	"git.eclipse.org/gitroot/paho/org.eclipse.paho.mqtt.golang.git"
	log "github.com/Sirupsen/logrus"
	"github.com/brocaar/loraserver"
	"github.com/brocaar/lorawan"
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
func (b *Backend) Send(devEUI, appEUI lorawan.EUI64, rxPackets loraserver.RXPackets) error {
	if len(rxPackets) == 0 {
		return errors.New("application/mqttpubsub: at least one RXPacket must be given")
	}

	macPL, ok := rxPackets[0].PHYPayload.MACPayload.(*lorawan.MACPayload)
	if !ok {
		return errors.New("application/mqttpubsub: MACPayload must be of type *lorawan.MACPayload")
	}

	if len(macPL.FRMPayload) != 1 {
		return errors.New("application/mqttpubsub: FRMPayload must be of length 1")
	}

	dataPL, ok := macPL.FRMPayload[0].(*lorawan.DataPayload)
	if !ok {
		return errors.New("application/mqttpubsub: FRMPayload must be of type *lorawan.DataPayload")
	}

	pl := ApplicationRXPayload{
		MType:        rxPackets[0].PHYPayload.MHDR.MType,
		DevEUI:       devEUI,
		GatewayCount: uint8(len(rxPackets)),
		ACK:          macPL.FHDR.FCtrl.ACK,
		FPort:        macPL.FPort,
		Data:         dataPL.Bytes,
	}

	bytes, err := json.Marshal(pl)
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
