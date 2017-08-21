package gateway

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/backend"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/lorawan"
	"github.com/eclipse/paho.mqtt.golang"
	"github.com/garyburd/redigo/redis"
	"github.com/pkg/errors"
)

const rxTopic = "gateway/+/rx"
const statsTopic = "gateway/+/stats"
const uplinkLockTTL = time.Millisecond * 500
const statsLockTTL = time.Millisecond * 500

// Backend implements a MQTT pub-sub backend.
type Backend struct {
	conn            mqtt.Client
	rxPacketChan    chan gw.RXPacket
	statsPacketChan chan gw.GatewayStatsPacket
	wg              sync.WaitGroup
}

// NewBackend creates a new Backend.
func NewBackend(server, username, password, cafile string) (backend.Gateway, error) {
	b := Backend{
		rxPacketChan:    make(chan gw.RXPacket),
		statsPacketChan: make(chan gw.GatewayStatsPacket),
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(server)
	opts.SetUsername(username)
	opts.SetPassword(password)
	opts.SetOnConnectHandler(b.onConnected)
	opts.SetConnectionLostHandler(b.onConnectionLost)

	if cafile != "" {
		tlsconfig, err := newTLSConfig(cafile)
		if err != nil {
			log.Fatalf("Error with the mqtt CA certificate: %s", err)
		} else {
			opts.SetTLSConfig(tlsconfig)
		}
	}

	log.WithField("server", server).Info("backend/gateway: connecting to mqtt broker")
	b.conn = mqtt.NewClient(opts)
	for {
		if token := b.conn.Connect(); token.Wait() && token.Error() != nil {
			log.Errorf("backend/gateway: connecting to mqtt broker failed, will retry in 2s: %s", token.Error())
			time.Sleep(2 * time.Second)
		} else {
			break
		}
	}

	return &b, nil
}

func newTLSConfig(cafile string) (*tls.Config, error) {
	// Import trusted certificates from CAfile.pem.

	cert, err := ioutil.ReadFile(cafile)
	if err != nil {
		log.Errorf("backend: couldn't load cafile: %s", err)
		return nil, err
	}

	certpool := x509.NewCertPool()
	certpool.AppendCertsFromPEM(cert)

	// Create tls.Config with desired tls properties
	return &tls.Config{
		// RootCAs = certs used to verify server cert.
		RootCAs: certpool,
	}, nil
}

// Close closes the backend.
// Note that this closes the backend one-way (gateway to backend).
// This makes it possible to perform a graceful shutdown (e.g. when there are
// still packets to send back to the gateway).
func (b *Backend) Close() error {
	log.Info("backend/gateway: closing backend")
	log.WithField("topic", rxTopic).Info("backend/gateway: unsubscribing from rx topic")
	if token := b.conn.Unsubscribe(rxTopic); token.Wait() && token.Error() != nil {
		return fmt.Errorf("backend/gateway: unsubscribe from %s error: %s", rxTopic, token.Error())
	}
	log.WithField("topic", statsTopic).Info("backend/gateway: unsubscribing from stats topic")
	if token := b.conn.Unsubscribe(statsTopic); token.Wait() && token.Error() != nil {
		return fmt.Errorf("backend/gateway: unsubscribe from %s error: %s", statsTopic, token.Error())
	}
	log.Info("backend/gateway: handling last messages")
	b.wg.Wait()
	close(b.rxPacketChan)
	close(b.statsPacketChan)
	return nil
}

// RXPacketChan returns the RXPacket channel.
func (b *Backend) RXPacketChan() chan gw.RXPacket {
	return b.rxPacketChan
}

// StatsPacketChan returns the gateway stats channel.
func (b *Backend) StatsPacketChan() chan gw.GatewayStatsPacket {
	return b.statsPacketChan
}

// SendTXPacket sends the given TXPacket to the gateway.
func (b *Backend) SendTXPacket(txPacket gw.TXPacket) error {
	phyB, err := txPacket.PHYPayload.MarshalBinary()
	if err != nil {
		return errors.Wrap(err, "marshal binary error")
	}
	bytes, err := json.Marshal(gw.TXPacketBytes{
		TXInfo:     txPacket.TXInfo,
		PHYPayload: phyB,
	})
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

	var phy lorawan.PHYPayload
	var rxPacketBytes gw.RXPacketBytes
	if err := json.Unmarshal(msg.Payload(), &rxPacketBytes); err != nil {
		log.WithFields(log.Fields{
			"data_base64": base64.StdEncoding.EncodeToString(msg.Payload()),
		}).Errorf("backend/gateway: unmarshal rx packet error: %s", err)
		return
	}

	if err := phy.UnmarshalBinary(rxPacketBytes.PHYPayload); err != nil {
		log.WithFields(log.Fields{
			"data_base64": base64.StdEncoding.EncodeToString(msg.Payload()),
		}).Errorf("backend/gateway: unmarshal phypayload error: %s", err)
	}

	// Since with MQTT all subscribers will receive the uplink messages sent
	// by all the gatewyas, the first instance receiving the message must lock it,
	// so that other instances can ignore the same message (from the same gw).
	// As an unique id, the gw mac + base64 encoded payload is used. This is because
	// we can't trust any of the data, as the MIC hasn't been validated yet.
	strB, err := phy.MarshalText()
	if err != nil {
		log.Errorf("backend/gateway: marshal text error: %s", err)
	}
	key := fmt.Sprintf("lora:ns:uplink:lock:%s:%s", rxPacketBytes.RXInfo.MAC, string(strB))
	redisConn := common.RedisPool.Get()
	defer redisConn.Close()

	_, err = redis.String(redisConn.Do("SET", key, "lock", "PX", int64(uplinkLockTTL/time.Millisecond), "NX"))
	if err != nil {
		if err == redis.ErrNil {
			// the payload is already being processed by an other instance
			return
		}
		log.Errorf("backend/gateway: acquire uplink payload lock error: %s", err)
		return
	}

	b.rxPacketChan <- gw.RXPacket{
		RXInfo:     rxPacketBytes.RXInfo,
		PHYPayload: phy,
	}
}

func (b *Backend) statsPacketHandler(c mqtt.Client, msg mqtt.Message) {
	b.wg.Add(1)
	defer b.wg.Done()

	var statsPacket gw.GatewayStatsPacket
	if err := json.Unmarshal(msg.Payload(), &statsPacket); err != nil {
		log.WithFields(log.Fields{
			"data_base64": base64.StdEncoding.EncodeToString(msg.Payload()),
		}).Errorf("backend/gateway: unmarshal stats packet error: %s", err)
		return
	}

	// Since with MQTT all subscribers will receive the uplink messages sent
	// by all the gatewyas, the first instance receiving the message must lock it,
	// so that other instances can ignore the same message (from the same gw).
	// As an unique id, the gw mac + base64 encoded payload is used. This is because
	// we can't trust any of the data, as the MIC hasn't been validated yet.
	key := fmt.Sprintf("lora:ns:stats:lock:%s", statsPacket.MAC)
	redisConn := common.RedisPool.Get()
	defer redisConn.Close()

	_, err := redis.String(redisConn.Do("SET", key, "lock", "PX", int64(statsLockTTL/time.Millisecond), "NX"))
	if err != nil {
		if err == redis.ErrNil {
			// the payload is already being processed by an other instance
			return
		}
		log.Errorf("backend/gateway: acquire stats lock error: %s", err)
		return
	}

	log.WithField("mac", statsPacket.MAC).Info("backend/gateway: gateway stats packet received")
	b.statsPacketChan <- statsPacket
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
		break
	}

	for {
		log.WithField("topic", statsTopic).Info("backend/gateway: subscribing to stats topic")
		if token := b.conn.Subscribe(statsTopic, 2, b.statsPacketHandler); token.Wait() && token.Error() != nil {
			log.WithField("topic", statsTopic).Errorf("backend/gateway: subscribe error: %s", token.Error())
			time.Sleep(time.Second)
			continue
		}
		break
	}
}

func (b *Backend) onConnectionLost(c mqtt.Client, reason error) {
	log.Errorf("backend/gateway: mqtt connection error: %s", reason)
}
