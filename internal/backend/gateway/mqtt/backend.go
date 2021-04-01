package mqtt

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"strings"
	"sync"
	"text/template"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-network-server/v3/internal/backend/gateway"
	"github.com/brocaar/chirpstack-network-server/v3/internal/backend/gateway/marshaler"
	"github.com/brocaar/chirpstack-network-server/v3/internal/config"
	"github.com/brocaar/chirpstack-network-server/v3/internal/helpers"
	"github.com/brocaar/chirpstack-network-server/v3/internal/storage"
	"github.com/brocaar/lorawan"
)

const deduplicationLockTTL = time.Millisecond * 500
const (
	marshalerV2JSON = iota
	marshalerProtobuf
	marshalerJSON
)

// Backend implements a MQTT pub-sub backend.
type Backend struct {
	sync.RWMutex

	wg sync.WaitGroup

	rxPacketChan      chan gw.UplinkFrame
	statsPacketChan   chan gw.GatewayStats
	downlinkTXAckChan chan gw.DownlinkTXAck

	conn                 paho.Client
	eventTopic           string
	commandTopicTemplate *template.Template
	qos                  uint8

	downMode         string
	gatewayMarshaler map[lorawan.EUI64]marshaler.Type
}

// NewBackend creates a new Backend.
func NewBackend(c config.Config) (gateway.Gateway, error) {
	conf := c.NetworkServer.Gateway.Backend.MQTT
	var err error

	b := Backend{
		rxPacketChan:      make(chan gw.UplinkFrame),
		statsPacketChan:   make(chan gw.GatewayStats),
		downlinkTXAckChan: make(chan gw.DownlinkTXAck),
		gatewayMarshaler:  make(map[lorawan.EUI64]marshaler.Type),
		eventTopic:        conf.EventTopic,
		qos:               conf.QOS,
		downMode:          c.NetworkServer.Gateway.Backend.MultiDownlinkFeature,
	}

	b.commandTopicTemplate, err = template.New("command").Parse(conf.CommandTopicTemplate)
	if err != nil {
		return nil, errors.Wrap(err, "gateway/mqtt: parse command topic template error")
	}

	opts := paho.NewClientOptions()
	opts.AddBroker(conf.Server)
	opts.SetUsername(conf.Username)
	opts.SetPassword(conf.Password)
	opts.SetCleanSession(conf.CleanSession)
	opts.SetClientID(conf.ClientID)
	opts.SetOnConnectHandler(b.onConnected)
	opts.SetConnectionLostHandler(b.onConnectionLost)
	opts.SetMaxReconnectInterval(conf.MaxReconnectInterval)

	tlsconfig, err := newTLSConfig(conf.CACert, conf.TLSCert, conf.TLSKey)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"ca_cert":  conf.CACert,
			"tls_cert": conf.TLSCert,
			"tls_key":  conf.TLSKey,
		}).Fatal("gateway/mqtt: error loading mqtt certificate files")
	}
	if tlsconfig != nil {
		opts.SetTLSConfig(tlsconfig)
	}

	log.WithField("server", conf.Server).Info("gateway/mqtt: connecting to mqtt broker")
	b.conn = paho.NewClient(opts)
	for {
		if token := b.conn.Connect(); token.Wait() && token.Error() != nil {
			log.Errorf("gateway/mqtt: connecting to mqtt broker failed, will retry in 2s: %s", token.Error())
			time.Sleep(2 * time.Second)
		} else {
			break
		}
	}

	return &b, nil
}

// Close closes the backend.
// Note that this closes the backend one-way (gateway to backend).
// This makes it possible to perform a graceful shutdown (e.g. when there are
// still packets to send back to the gateway).
func (b *Backend) Close() error {
	log.Info("gateway/mqtt: closing backend")

	log.WithField("topic", b.eventTopic).Info("gateway/mqtt: unsubscribing from event topic")
	if token := b.conn.Unsubscribe(b.eventTopic); token.Wait() && token.Error() != nil {
		return fmt.Errorf("gateway/mqtt: unsubscribe from %s error: %s", b.eventTopic, token.Error())
	}

	log.Info("backend/gateway: handling last messages")
	b.wg.Wait()
	close(b.rxPacketChan)
	close(b.statsPacketChan)
	close(b.downlinkTXAckChan)
	return nil
}

// RXPacketChan returns the uplink-frame channel.
func (b *Backend) RXPacketChan() chan gw.UplinkFrame {
	return b.rxPacketChan
}

// StatsPacketChan returns the gateway stats channel.
func (b *Backend) StatsPacketChan() chan gw.GatewayStats {
	return b.statsPacketChan
}

// DownlinkTXAckChan returns the downlink tx ack channel.
func (b *Backend) DownlinkTXAckChan() chan gw.DownlinkTXAck {
	return b.downlinkTXAckChan
}

// SendTXPacket sends the given downlink-frame to the gateway.
func (b *Backend) SendTXPacket(txPacket gw.DownlinkFrame) error {
	gatewayID := helpers.GetGatewayID(&txPacket)
	downID := helpers.GetDownlinkID(&txPacket)

	if err := gateway.UpdateDownlinkFrame(b.downMode, &txPacket); err != nil {
		return errors.Wrap(err, "set downlink compatibility mode error")
	}

	return b.publishCommand(log.Fields{
		"downlink_id": downID,
	}, gatewayID, "down", &txPacket)
}

// SendGatewayConfigPacket sends the given GatewayConfigPacket to the gateway.
func (b *Backend) SendGatewayConfigPacket(configPacket gw.GatewayConfiguration) error {
	gatewayID := helpers.GetGatewayID(&configPacket)

	return b.publishCommand(log.Fields{}, gatewayID, "config", &configPacket)
}

func (b *Backend) publishCommand(fields log.Fields, gatewayID lorawan.EUI64, command string, msg proto.Message) error {
	t := b.getGatewayMarshaler(gatewayID)
	bb, err := marshaler.MarshalCommand(t, msg)
	if err != nil {
		return errors.Wrap(err, "gateway/mqtt: marshal gateway command error")
	}

	templateCtx := struct {
		GatewayID   lorawan.EUI64
		CommandType string
	}{gatewayID, command}
	topic := bytes.NewBuffer(nil)
	if err := b.commandTopicTemplate.Execute(topic, templateCtx); err != nil {
		return errors.Wrap(err, "execute command topic template error")
	}

	fields["gateway_id"] = gatewayID
	fields["command"] = command
	fields["qos"] = b.qos
	fields["topic"] = topic.String()

	log.WithFields(fields).Info("gateway/mqtt: publishing gateway command")

	mqttCommandCounter(command).Inc()

	if token := b.conn.Publish(topic.String(), b.qos, false, bb); token.Wait() && token.Error() != nil {
		return errors.Wrap(err, "gateway/mqtt: publish gateway command error")
	}

	return nil
}

func (b *Backend) eventHandler(c paho.Client, msg paho.Message) {
	b.wg.Add(1)
	defer b.wg.Done()

	if strings.HasSuffix(msg.Topic(), "up") {
		mqttEventCounter("up").Inc()
		b.rxPacketHandler(c, msg)
	} else if strings.HasSuffix(msg.Topic(), "ack") {
		mqttEventCounter("ack").Inc()
		b.ackPacketHandler(c, msg)
	} else if strings.HasSuffix(msg.Topic(), "stats") {
		mqttEventCounter("stats").Inc()
		b.statsPacketHandler(c, msg)
	}
}

func (b *Backend) rxPacketHandler(c paho.Client, msg paho.Message) {
	b.wg.Add(1)
	defer b.wg.Done()

	var uplinkFrame gw.UplinkFrame
	t, err := marshaler.UnmarshalUplinkFrame(msg.Payload(), &uplinkFrame)
	if err != nil {
		log.WithFields(log.Fields{
			"data_base64": base64.StdEncoding.EncodeToString(msg.Payload()),
		}).WithError(err).Error("gateway/mqtt: unmarshal uplink frame error")
		return
	}

	if uplinkFrame.TxInfo == nil {
		log.WithFields(log.Fields{
			"data_base64": base64.StdEncoding.EncodeToString(msg.Payload()),
		}).Error("gateway/mqtt: tx_info must not be nil")
		return
	}

	if uplinkFrame.RxInfo == nil {
		log.WithFields(log.Fields{
			"data_base64": base64.StdEncoding.EncodeToString(msg.Payload()),
		}).Error("gateway/mqtt: rx_info must not be nil")
		return
	}

	gatewayID := helpers.GetGatewayID(uplinkFrame.RxInfo)
	b.setGatewayMarshaler(gatewayID, t)
	uplinkID := helpers.GetUplinkID(uplinkFrame.RxInfo)

	log.WithFields(log.Fields{
		"uplink_id":  uplinkID,
		"gateway_id": gatewayID,
	}).Info("gateway/mqtt: uplink frame received")

	b.rxPacketChan <- uplinkFrame
}

func (b *Backend) statsPacketHandler(c paho.Client, msg paho.Message) {
	b.wg.Add(1)
	defer b.wg.Done()

	var gatewayStats gw.GatewayStats
	t, err := marshaler.UnmarshalGatewayStats(msg.Payload(), &gatewayStats)
	if err != nil {
		log.WithFields(log.Fields{
			"data_base64": base64.StdEncoding.EncodeToString(msg.Payload()),
		}).WithError(err).Error("gateway/mqtt: unmarshal gateway stats error")
		return
	}

	gatewayID := helpers.GetGatewayID(&gatewayStats)
	statsID := helpers.GetStatsID(&gatewayStats)
	b.setGatewayMarshaler(gatewayID, t)

	// Since with MQTT all subscribers will receive the stats messages sent
	// by all the gateways, the first instance receiving the message must lock it,
	// so that other instances can ignore the same message (from the same gw).
	// As an unique id, the gw mac is used.
	key := storage.GetRedisKey("lora:ns:stats:lock:%s", gatewayID)
	if locked, err := b.isLocked(key); err != nil || locked {
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"key":      key,
				"stats_id": statsID,
			}).Error("gateway/mqtt: acquire lock error")
		}

		return
	}

	log.WithFields(log.Fields{
		"gateway_id": gatewayID,
		"stats_id":   statsID,
	}).Info("gateway/mqtt: gateway stats packet received")
	b.statsPacketChan <- gatewayStats
}

func (b *Backend) ackPacketHandler(c paho.Client, msg paho.Message) {
	b.wg.Add(1)
	defer b.wg.Done()

	var ack gw.DownlinkTXAck
	t, err := marshaler.UnmarshalDownlinkTXAck(msg.Payload(), &ack)
	if err != nil {
		log.WithFields(log.Fields{
			"data_base64": base64.StdEncoding.EncodeToString(msg.Payload()),
		}).WithError(err).Error("backend/gateway: unmarshal downlink tx ack error")
	}

	gatewayID := helpers.GetGatewayID(&ack)
	downlinkID := helpers.GetDownlinkID(&ack)
	b.setGatewayMarshaler(gatewayID, t)

	// Since with MQTT all subscribers will receive the ack messages sent
	// by all the gateways, the first instance receiving the message must lock it,
	// so that other instances can ignore the same message (from the same gw).
	// As an unique id, the gw mac is used.
	key := storage.GetRedisKey("lora:ns:ack:lock:%s", gatewayID)
	if locked, err := b.isLocked(key); err != nil || locked {
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"key":         key,
				"downlink_id": downlinkID,
			}).Error("gateway/mqtt: acquire lock error")
		}

		return
	}

	log.WithFields(log.Fields{
		"gateway_id":  gatewayID,
		"downlink_id": downlinkID,
	}).Info("backend/gateway: downlink tx acknowledgement received")
	b.downlinkTXAckChan <- ack
}

func (b *Backend) onConnected(c paho.Client) {
	log.Info("backend/gateway: connected to mqtt server")

	mqttConnectCounter().Inc()

	for {
		log.WithFields(log.Fields{
			"topic": b.eventTopic,
			"qos":   b.qos,
		}).Info("gateway/mqtt: subscribing to gateway event topic")
		if token := b.conn.Subscribe(b.eventTopic, b.qos, b.eventHandler); token.Wait() && token.Error() != nil {
			log.WithError(token.Error()).WithFields(log.Fields{
				"topic": b.eventTopic,
				"qos":   b.qos,
			}).Errorf("gateway/mqtt: subscribe error")
			time.Sleep(time.Second)
			continue
		}
		break
	}
}

func (b *Backend) onConnectionLost(c paho.Client, reason error) {
	log.Errorf("gateway/mqtt: mqtt connection error: %s", reason)
	mqttDisconnectCounter().Inc()
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

// isLocked returns if a lock exists for the given key, if false a lock is
// acquired.
func (b *Backend) isLocked(key string) (bool, error) {
	set, err := storage.RedisClient().SetNX(key, "lock", deduplicationLockTTL).Result()
	if err != nil {
		return false, errors.Wrap(err, "acquire lock error")
	}

	// Set is true when we were able to set the lock, we return true if it
	// was already locked.
	return !set, nil
}

func newTLSConfig(cafile, certFile, certKeyFile string) (*tls.Config, error) {
	if cafile == "" && certFile == "" && certKeyFile == "" {
		return nil, nil
	}

	tlsConfig := &tls.Config{}

	// Import trusted certificates from CAfile.pem.
	if cafile != "" {
		cacert, err := ioutil.ReadFile(cafile)
		if err != nil {
			log.WithError(err).Error("gateway/mqtt: could not load ca certificate")
			return nil, err
		}
		certpool := x509.NewCertPool()
		certpool.AppendCertsFromPEM(cacert)

		tlsConfig.RootCAs = certpool // RootCAs = certs used to verify server cert.
	}

	// Import certificate and the key
	if certFile != "" && certKeyFile != "" {
		kp, err := tls.LoadX509KeyPair(certFile, certKeyFile)
		if err != nil {
			log.WithError(err).Error("gateway/mqtt: could not load mqtt tls key-pair")
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{kp}
	}

	return tlsConfig, nil
}
