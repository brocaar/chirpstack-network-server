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

const txTopic = "application/+/node/+/tx"

// DownlinkLockTTL defines the downlink lock ttl.
const DownlinkLockTTL = time.Millisecond * 100

var txTopicRegex = regexp.MustCompile(`application/(\w+)/node/(\w+)/tx`)

// Backend implements a MQTT pub-sub application backend.
type Backend struct {
	conn          mqtt.Client
	txPayloadChan chan models.TXPayload
	wg            sync.WaitGroup
	redisPool     *redis.Pool
}

// NewBackend creates a new Backend.
func NewBackend(p *redis.Pool, server, username, password string) (loraserver.ApplicationBackend, error) {
	b := Backend{
		txPayloadChan: make(chan models.TXPayload),
		redisPool:     p,
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(server)
	opts.SetUsername(username)
	opts.SetPassword(password)
	opts.SetOnConnectHandler(b.onConnected)
	opts.SetConnectionLostHandler(b.onConnectionLost)

	log.WithField("server", server).Info("application/mqttpubsub: connecting to mqtt broker")
	b.conn = mqtt.NewClient(opts)
	if token := b.conn.Connect(); token.Wait() && token.Error() != nil {
		return nil, fmt.Errorf("application/mqttpubsub: connecting to broker failed: %s", token.Error())
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
		return fmt.Errorf("application/mqttpubsub: unsubscribe from %s failed: %s", txTopic, token.Error())
	}
	log.Info("application/mqttpubsub: handling last messages")
	b.wg.Wait()
	close(b.txPayloadChan)
	return nil
}

// TXPayloadChan returns the TXPayload channel.
func (b *Backend) TXPayloadChan() chan models.TXPayload {
	return b.txPayloadChan
}

// SendRXPayload sends the given RXPayload to the application.
func (b *Backend) SendRXPayload(appEUI, devEUI lorawan.EUI64, payload models.RXPayload) error {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("application/mqttpubsub: rx payload marshal error: %s", err)
	}

	topic := fmt.Sprintf("application/%s/node/%s/rx", appEUI, devEUI)
	log.WithField("topic", topic).Info("application/mqttpubsub: publishing rx payload")
	if token := b.conn.Publish(topic, 0, false, bytes); token.Wait() && token.Error() != nil {
		return fmt.Errorf("application/mqttpubsub: publish rx payload failed: %s", token.Error())
	}
	return nil
}

// SendNotification sends the given notification to the application.
func (b *Backend) SendNotification(appEUI, devEUI lorawan.EUI64, typ models.NotificationType, payload interface{}) error {
	var topicSuffix string
	switch typ {
	case models.JoinNotificationType:
		topicSuffix = "join"
	case models.ErrorNotificationType:
		topicSuffix = "error"
	case models.ACKNotificationType:
		topicSuffix = "ack"
	default:
		return fmt.Errorf("application/mqttpubsub: notification type unknown: %s", typ)
	}

	bytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("application/mqttpubsub: notification marshal error: %s", err)
	}

	topic := fmt.Sprintf("application/%s/node/%s/%s", appEUI, devEUI, topicSuffix)
	log.WithFields(log.Fields{
		"topic": topic,
	}).Info("application/mqttpubsub: publishing notification")
	if token := b.conn.Publish(topic, 0, false, bytes); token.Wait() && token.Error() != nil {
		return fmt.Errorf("application/mqttpubsub: publish notification failed: %s", token.Error())
	}

	return nil
}

func (b *Backend) txPayloadHandler(c mqtt.Client, msg mqtt.Message) {
	b.wg.Add(1)
	defer b.wg.Done()

	log.WithField("topic", msg.Topic()).Info("application/mqttpubsub: tx payload received")

	// get the DevEUI from the topic. with mqtt it is possible to perform
	// authorization on a per topic level. we need to be sure that the
	// topic DevEUI matches the payload DevEUI.
	match := txTopicRegex.FindStringSubmatch(msg.Topic())
	if len(match) != 3 {
		log.WithField("topic", msg.Topic()).Error("application/mqttpubsub: topic regex match error")
		return
	}

	var txPayload models.TXPayload
	dec := json.NewDecoder(bytes.NewReader(msg.Payload()))
	if err := dec.Decode(&txPayload); err != nil {
		log.WithFields(log.Fields{
			"data_base64": base64.StdEncoding.EncodeToString(msg.Payload()),
		}).Errorf("application/mqttpubsub: tx payload unmarshal error: %s", err)
		return
	}

	if match[2] != txPayload.DevEUI.String() {
		log.WithFields(log.Fields{
			"topic_dev_eui":   match[2],
			"payload_dev_eui": txPayload.DevEUI,
		}).Warning("application/mqttpubsub: topic DevEUI must match payload DevEUI")
		return
	}

	// Since with MQTT all subscribers will receive the downlink messages sent
	// by the application, the first loraserver receiving the message must lock it,
	// so that other instances can ignore the message.
	// As an unique id, the Reference field is used.
	key := fmt.Sprintf("app_backend_%s_%s_lock", txPayload.DevEUI, txPayload.Reference)
	redisConn := b.redisPool.Get()
	defer redisConn.Close()

	_, err := redis.String(redisConn.Do("SET", key, "lock", "PX", int64(DownlinkLockTTL/time.Millisecond), "NX"))
	if err != nil {
		if err == redis.ErrNil {
			// the payload is already being processed by an other instance
			// of Lora Server.
			return
		}
		log.Errorf("application/mqttpubsub: acquire downlink payload lock error: %s", err)
		return
	}

	b.txPayloadChan <- txPayload
}

func (b *Backend) onConnected(c mqtt.Client) {
	log.Info("application/mqttpubsub: connected to mqtt server")
	for {
		log.WithField("topic", txTopic).Info("application/mqttpubsub: subscribing to tx topic")
		if token := b.conn.Subscribe(txTopic, 2, b.txPayloadHandler); token.Wait() && token.Error() != nil {
			log.WithField("topic", txTopic).Errorf("application/mqttpubsub: subscribe error: %s", token.Error())
			time.Sleep(time.Second)
			continue
		}
		return
	}
}

func (b *Backend) onConnectionLost(c mqtt.Client, reason error) {
	log.Errorf("application/mqttpubsub: mqtt connection error: %s", reason)
}
