package mqttpubsub

import (
	"bytes"
	"encoding/base64"
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
	"github.com/garyburd/redigo/redis"
)

const txTopic = "application/+/node/+/mac/tx"

// DownlinkLockTTL defines the downlink lock ttl.
const DownlinkLockTTL = time.Millisecond * 100

var txTopicRegex = regexp.MustCompile(`application/(\w+)/node/(\w+)/mac/tx`)

// Backend implements a MQTT pub-sub network-controller backend.
type Backend struct {
	conn             mqtt.Client
	txMACPayloadChan chan models.MACPayload
	wg               sync.WaitGroup
	redisPool        *redis.Pool
}

// NewBackend creates a new Backend.
func NewBackend(p *redis.Pool, server, username, password string) (loraserver.NetworkControllerBackend, error) {
	b := Backend{
		txMACPayloadChan: make(chan models.MACPayload),
		redisPool:        p,
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(server)
	opts.SetUsername(username)
	opts.SetPassword(password)
	opts.SetOnConnectHandler(b.onConnected)
	opts.SetConnectionLostHandler(b.onConnectionLost)

	log.WithField("server", server).Info("controller/mqttpubsub: connecting to mqtt broker")
	b.conn = mqtt.NewClient(opts)
	if token := b.conn.Connect(); token.Wait() && token.Error() != nil {
		return nil, fmt.Errorf("controller/mqttpubsub: connecting to broker failed: %s", token.Error())
	}

	return &b, nil
}

// SendMACPayload sends the given MACPayload to the network-controller.
func (b *Backend) SendMACPayload(appEUI, devEUI lorawan.EUI64, pl models.MACPayload) error {
	bytes, err := json.Marshal(pl)
	if err != nil {
		return fmt.Errorf("controller/mqttpubsub: rx mac-payload marshal error: %s", err)
	}

	topic := fmt.Sprintf("application/%s/node/%s/mac/rx", appEUI, devEUI)
	log.WithField("topic", topic).Info("controller/mqttpubsub: publishing rx mac-payload")
	if token := b.conn.Publish(topic, 0, false, bytes); token.Wait() && token.Error() != nil {
		return fmt.Errorf("controller/mqttpubsub: publish rx mac-payload failed: %s", token.Error())
	}
	return nil
}

// SendRXInfoPayload sends the given RXInfoPayload to the network-controller.
func (b *Backend) SendRXInfoPayload(appEUI, devEUI lorawan.EUI64, payload models.RXInfoPayload) error {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("controller/mqttpubsub: rxinfo payload marshal error: %s", err)
	}

	topic := fmt.Sprintf("application/%s/node/%s/rxinfo", appEUI, devEUI)
	log.WithField("topic", topic).Info("controller/mqttpubsub: publishing rxinfo payload")
	if token := b.conn.Publish(topic, 0, false, bytes); token.Wait() && token.Error() != nil {
		return fmt.Errorf("controller/mqttpubsub: publish rxinfo payload failed: %s", token.Error())
	}
	return nil
}

// SendErrorPayload sends the given ErrorPayload to the network-controller.
func (b *Backend) SendErrorPayload(appEUI, devEUI lorawan.EUI64, payload models.ErrorPayload) error {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("controller/mqttpubsub: error payload marshal error: %s", err)
	}
	topic := fmt.Sprintf("application/%s/node/%s/mac/error", appEUI, devEUI)
	log.WithField("topic", topic).Info("controller/mqttpubsub: publishing error payload")
	if token := b.conn.Publish(topic, 0, false, bytes); token.Wait() && token.Error() != nil {
		return fmt.Errorf("controller/mqttpubsub: publish error payload failed: %s", token.Error())
	}
	return nil
}

// Close closes the backend.
// Note that this closes the backend one-way (network-controller to the backend).
// This makes it possible to perform a graceful shutdown (e.g. when there are
// still packets to send back to the network-controller).
func (b *Backend) Close() error {
	log.Info("controller/mqttpubsub: closing backend")
	log.WithField("topic", txTopic).Info("controller/mqttpubsub: unsubscribing from tx topic")
	if token := b.conn.Unsubscribe(txTopic); token.Wait() && token.Error() != nil {
		return fmt.Errorf("controller/mqttpubsub: unsubscribe from %s failed: %s", txTopic, token.Error())
	}
	log.Info("controller/mqttpubsub: handling last packets")
	b.wg.Wait()
	close(b.txMACPayloadChan)
	return nil
}

// TXMACPayloadChan returns the channel of MACPayload items to send.
func (b *Backend) TXMACPayloadChan() chan models.MACPayload {
	return b.txMACPayloadChan
}

func (b *Backend) onConnected(c mqtt.Client) {
	log.Info("controller/mqttpubsub: connected to mqtt broker")
	for {
		log.WithField("topic", txTopic).Info("controller/mqttpubsub: subscribing to tx topic")
		if token := b.conn.Subscribe(txTopic, 2, b.txMACPayloadHandler); token.Wait() && token.Error() != nil {
			log.WithField("topic", txTopic).Errorf("controller/mqttpubsub: subscribe error: %s", token.Error())
			time.Sleep(time.Second)
			continue
		}
		return
	}
}

func (b *Backend) onConnectionLost(c mqtt.Client, reason error) {
	log.Errorf("controller/mqttpubsub: mqtt connection error: %s", reason)
}

func (b *Backend) txMACPayloadHandler(c mqtt.Client, msg mqtt.Message) {
	b.wg.Add(1)
	defer b.wg.Done()

	log.WithField("topic", msg.Topic()).Info("controller/mqttpubsub: tx mac-payload received")

	// get the DevEUI from the topic. with mqtt it is possible to perform
	// authorization on a per topic level. we need to be sure that the
	// topic DevEUI matches the payload DevEUI.
	match := txTopicRegex.FindStringSubmatch(msg.Topic())
	if len(match) != 3 {
		log.WithField("topic", msg.Topic()).Error("controller/mqttpubsub: topic regex match error")
		return
	}

	var pl models.MACPayload
	dec := json.NewDecoder(bytes.NewReader(msg.Payload()))
	if err := dec.Decode(&pl); err != nil {
		log.WithFields(log.Fields{
			"data_base64": base64.StdEncoding.EncodeToString(msg.Payload()),
		}).Errorf("controller/mqttpubsub: tx mac-payload unmarshal error: %s", err)
		return
	}

	if match[2] != pl.DevEUI.String() {
		log.WithFields(log.Fields{
			"topic_dev_eui": match[2],
			"mac_dev_eui":   pl.DevEUI,
		}).Warning("controller/mqttpubsub: topic DevEUI must match mac-payload DevEUI")
		return
	}

	// Since with MQTT all subscribers will receive the downlink messages sent
	// by the application, the first loraserver receiving the message must lock it,
	// so that other instances can ignore the message.
	// As an unique id, the Reference field is used.
	key := fmt.Sprintf("controller_backend_%s_%s_lock", pl.DevEUI, pl.Reference)
	redisConn := b.redisPool.Get()
	defer redisConn.Close()

	_, err := redis.String(redisConn.Do("SET", key, "lock", "PX", int64(DownlinkLockTTL/time.Millisecond), "NX"))
	if err != nil {
		if err == redis.ErrNil {
			// the payload is already being processed by an other instance
			// of Lora Server.
			return
		}
		log.Errorf("controller/mqttpubsub: acquire tx mac-payload lock error: %s", err)
		return
	}

	b.txMACPayloadChan <- pl
}
