package gateway

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"sync"
	"text/template"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gomodule/redigo/redis"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/backend"
	"github.com/brocaar/loraserver/internal/backend/gateway/marshaler"
	"github.com/brocaar/loraserver/internal/helpers"
	"github.com/brocaar/lorawan"
)

const uplinkLockTTL = time.Millisecond * 500
const statsLockTTL = time.Millisecond * 500
const (
	marshalerV2JSON = iota
	marshalerProtobuf
	marshalerJSON
)

// MQTTBackendConfig holds the MQTT backend configuration.
type MQTTBackendConfig struct {
	Server                string
	Username              string
	Password              string
	QOS                   uint8  `mapstructure:"qos"`
	CleanSession          bool   `mapstructure:"clean_session"`
	ClientID              string `mapstructure:"client_id"`
	CACert                string `mapstructure:"ca_cert"`
	TLSCert               string `mapstructure:"tls_cert"`
	TLSKey                string `mapstructure:"tls_key"`
	UplinkTopicTemplate   string `mapstructure:"uplink_topic_template"`
	DownlinkTopicTemplate string `mapstructure:"downlink_topic_template"`
	StatsTopicTemplate    string `mapstructure:"stats_topic_template"`
	AckTopicTemplate      string `mapstructure:"ack_topic_template"`
	ConfigTopicTemplate   string `mapstructure:"config_topic_template"`
}

// MQTTBackend implements a MQTT pub-sub backend.
type MQTTBackend struct {
	sync.RWMutex

	wg     sync.WaitGroup
	config MQTTBackendConfig

	rxPacketChan    chan gw.UplinkFrame
	statsPacketChan chan gw.GatewayStats

	conn             mqtt.Client
	redisPool        *redis.Pool
	downlinkTemplate *template.Template
	configTemplate   *template.Template

	gatewayMarshaler map[lorawan.EUI64]marshaler.Type
}

// NewMQTTBackend creates a new Backend.
func NewMQTTBackend(redisPool *redis.Pool, c MQTTBackendConfig) (backend.Gateway, error) {
	var err error
	b := MQTTBackend{
		rxPacketChan:     make(chan gw.UplinkFrame),
		statsPacketChan:  make(chan gw.GatewayStats),
		gatewayMarshaler: make(map[lorawan.EUI64]marshaler.Type),
		redisPool:        redisPool,
		config:           c,
	}

	b.downlinkTemplate, err = template.New("downlink").Parse(b.config.DownlinkTopicTemplate)
	if err != nil {
		return nil, errors.Wrap(err, "parse downlink template error")
	}

	b.configTemplate, err = template.New("config").Parse(b.config.ConfigTopicTemplate)
	if err != nil {
		return nil, errors.Wrap(err, "parse config template error")
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(b.config.Server)
	opts.SetUsername(b.config.Username)
	opts.SetPassword(b.config.Password)
	opts.SetCleanSession(b.config.CleanSession)
	opts.SetClientID(b.config.ClientID)
	opts.SetOnConnectHandler(b.onConnected)
	opts.SetConnectionLostHandler(b.onConnectionLost)

	tlsconfig, err := newTLSConfig(b.config.CACert, b.config.TLSCert, b.config.TLSKey)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"ca_cert":  b.config.CACert,
			"tls_cert": b.config.TLSCert,
			"tls_key":  b.config.TLSKey,
		}).Fatal("backend/gateway: error loading mqtt certificate files")
	}
	if tlsconfig != nil {
		opts.SetTLSConfig(tlsconfig)
	}

	log.WithField("server", b.config.Server).Info("backend/gateway: connecting to mqtt broker")
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

// Close closes the backend.
// Note that this closes the backend one-way (gateway to backend).
// This makes it possible to perform a graceful shutdown (e.g. when there are
// still packets to send back to the gateway).
func (b *MQTTBackend) Close() error {
	log.Info("backend/gateway: closing backend")

	log.WithField("topic", b.config.UplinkTopicTemplate).Info("backend/gateway: unsubscribing from rx topic")
	if token := b.conn.Unsubscribe(b.config.UplinkTopicTemplate); token.Wait() && token.Error() != nil {
		return fmt.Errorf("backend/gateway: unsubscribe from %s error: %s", b.config.UplinkTopicTemplate, token.Error())
	}
	log.WithField("topic", b.config.StatsTopicTemplate).Info("backend/gateway: unsubscribing from stats topic")
	if token := b.conn.Unsubscribe(b.config.StatsTopicTemplate); token.Wait() && token.Error() != nil {
		return fmt.Errorf("backend/gateway: unsubscribe from %s error: %s", b.config.StatsTopicTemplate, token.Error())
	}
	log.Info("backend/gateway: handling last messages")
	b.wg.Wait()
	close(b.rxPacketChan)
	close(b.statsPacketChan)
	return nil
}

// RXPacketChan returns the uplink-frame channel.
func (b *MQTTBackend) RXPacketChan() chan gw.UplinkFrame {
	return b.rxPacketChan
}

// StatsPacketChan returns the gateway stats channel.
func (b *MQTTBackend) StatsPacketChan() chan gw.GatewayStats {
	return b.statsPacketChan
}

// SendTXPacket sends the given downlink-frame to the gateway.
func (b *MQTTBackend) SendTXPacket(txPacket gw.DownlinkFrame) error {
	if txPacket.TxInfo == nil {
		return errors.New("tx_info must not be nil")
	}

	gatewayID := helpers.GetGatewayID(txPacket.TxInfo)
	t := b.getGatewayMarshaler(gatewayID)

	bb, err := marshaler.MarshalDownlinkFrame(t, txPacket)
	if err != nil {
		return errors.Wrap(err, "backend/gateway: marshal downlink frame error")
	}

	topic := bytes.NewBuffer(nil)
	if err := b.downlinkTemplate.Execute(topic, struct{ MAC lorawan.EUI64 }{gatewayID}); err != nil {
		return errors.Wrap(err, "execute uplink template error")
	}
	log.WithFields(log.Fields{
		"topic": topic.String(),
		"qos":   b.config.QOS,
	}).Info("backend/gateway: publishing downlink frame")

	if token := b.conn.Publish(topic.String(), b.config.QOS, false, bb); token.Wait() && token.Error() != nil {
		return errors.Wrap(err, "backend/gateway: publish downlink frame error")
	}
	return nil
}

// SendGatewayConfigPacket sends the given GatewayConfigPacket to the gateway.
func (b *MQTTBackend) SendGatewayConfigPacket(configPacket gw.GatewayConfiguration) error {
	gatewayID := helpers.GetGatewayID(&configPacket)
	t := b.getGatewayMarshaler(gatewayID)

	bb, err := marshaler.MarshalGatewayConfiguration(t, configPacket)
	if err != nil {
		return errors.Wrap(err, "backend/gateway: marshal gateway configuration error")
	}

	topic := bytes.NewBuffer(nil)
	if err := b.configTemplate.Execute(topic, struct{ MAC lorawan.EUI64 }{gatewayID}); err != nil {
		return errors.Wrap(err, "backend/gateway: execute config template error")
	}
	log.WithFields(log.Fields{
		"topic": topic.String(),
		"qos":   b.config.QOS,
	}).Info("backend/gateway: publishing gateway configuration")

	if token := b.conn.Publish(topic.String(), b.config.QOS, false, bb); token.Wait() && token.Error() != nil {
		return errors.Wrap(err, "backend/gateway: publish gateway configuration error")
	}

	return nil
}

func (b *MQTTBackend) rxPacketHandler(c mqtt.Client, msg mqtt.Message) {
	b.wg.Add(1)
	defer b.wg.Done()

	log.Info("backend/gateway: uplink frame received")

	var uplinkFrame gw.UplinkFrame
	t, err := marshaler.UnmarshalUplinkFrame(msg.Payload(), &uplinkFrame)
	if err != nil {
		log.WithFields(log.Fields{
			"data_base64": base64.StdEncoding.EncodeToString(msg.Payload()),
		}).WithError(err).Error("backend/gateway: unmarshal uplink frame error")
		return
	}

	if uplinkFrame.TxInfo == nil {
		log.WithFields(log.Fields{
			"data_base64": base64.StdEncoding.EncodeToString(msg.Payload()),
		}).Error("backend/gateway: tx_info must not be nil")
		return
	}

	if uplinkFrame.RxInfo == nil {
		log.WithFields(log.Fields{
			"data_base64": base64.StdEncoding.EncodeToString(msg.Payload()),
		}).Error("backend/gateway: rx_info must not be nil")
		return
	}

	gatewayID := helpers.GetGatewayID(uplinkFrame.RxInfo)
	b.setGatewayMarshaler(gatewayID, t)

	// Since with MQTT all subscribers will receive the uplink messages sent
	// by all the gatewyas, the first instance receiving the message must lock it,
	// so that other instances can ignore the same message (from the same gw).
	// As an unique id, the gw mac + hex encoded payload is used. This is because
	// we can't trust any of the data, as the MIC hasn't been validated yet.
	key := fmt.Sprintf("lora:ns:uplink:lock:%s:%s", gatewayID, hex.EncodeToString(uplinkFrame.PhyPayload))
	redisConn := b.redisPool.Get()
	defer redisConn.Close()

	_, err = redis.String(redisConn.Do("SET", key, "lock", "PX", int64(uplinkLockTTL/time.Millisecond), "NX"))
	if err != nil {
		if err == redis.ErrNil {
			// the payload is already being processed by an other instance
			return
		}
		log.WithError(err).Error("backend/gateway: acquire uplink payload lock error")
		return
	}

	b.rxPacketChan <- uplinkFrame
}

func (b *MQTTBackend) statsPacketHandler(c mqtt.Client, msg mqtt.Message) {
	b.wg.Add(1)
	defer b.wg.Done()

	var gatewayStats gw.GatewayStats
	t, err := marshaler.UnmarshalGatewayStats(msg.Payload(), &gatewayStats)
	if err != nil {
		log.WithFields(log.Fields{
			"data_base64": base64.StdEncoding.EncodeToString(msg.Payload()),
		}).WithError(err).Error("backend/gateway: unmarshal gateway stats error")
		return
	}

	gatewayID := helpers.GetGatewayID(&gatewayStats)
	b.setGatewayMarshaler(gatewayID, t)

	// Since with MQTT all subscribers will receive the uplink messages sent
	// by all the gatewyas, the first instance receiving the message must lock it,
	// so that other instances can ignore the same message (from the same gw).
	// As an unique id, the gw mac is used.
	key := fmt.Sprintf("lora:ns:stats:lock:%s", gatewayID)
	redisConn := b.redisPool.Get()
	defer redisConn.Close()

	_, err = redis.String(redisConn.Do("SET", key, "lock", "PX", int64(statsLockTTL/time.Millisecond), "NX"))
	if err != nil {
		if err == redis.ErrNil {
			// the payload is already being processed by an other instance
			return
		}
		log.Errorf("backend/gateway: acquire stats lock error: %s", err)
		return
	}

	log.WithField("gateway_id", gatewayID).Info("backend/gateway: gateway stats packet received")
	b.statsPacketChan <- gatewayStats
}

func (b *MQTTBackend) onConnected(c mqtt.Client) {
	log.Info("backend/gateway: connected to mqtt server")

	for {
		log.WithFields(log.Fields{
			"topic": b.config.UplinkTopicTemplate,
			"qos":   b.config.QOS,
		}).Info("backend/gateway: subscribing to rx topic")
		if token := b.conn.Subscribe(b.config.UplinkTopicTemplate, b.config.QOS, b.rxPacketHandler); token.Wait() && token.Error() != nil {
			log.WithFields(log.Fields{
				"topic": b.config.UplinkTopicTemplate,
				"qos":   b.config.QOS,
			}).Errorf("backend/gateway: subscribe error: %s", token.Error())
			time.Sleep(time.Second)
			continue
		}
		break
	}

	for {
		log.WithFields(log.Fields{
			"topic": b.config.StatsTopicTemplate,
			"qos":   b.config.QOS,
		}).Info("backend/gateway: subscribing to stats topic")
		if token := b.conn.Subscribe(b.config.StatsTopicTemplate, b.config.QOS, b.statsPacketHandler); token.Wait() && token.Error() != nil {
			log.WithFields(log.Fields{
				"topic": b.config.StatsTopicTemplate,
				"qos":   b.config.QOS,
			}).Errorf("backend/gateway: subscribe error: %s", token.Error())
			time.Sleep(time.Second)
			continue
		}
		break
	}
}

func (b *MQTTBackend) onConnectionLost(c mqtt.Client, reason error) {
	log.Errorf("backend/gateway: mqtt connection error: %s", reason)
}

func (b *MQTTBackend) setGatewayMarshaler(gatewayID lorawan.EUI64, t marshaler.Type) {
	b.Lock()
	defer b.Unlock()

	b.gatewayMarshaler[gatewayID] = t
}

func (b *MQTTBackend) getGatewayMarshaler(gatewayID lorawan.EUI64) marshaler.Type {
	b.RLock()
	defer b.RUnlock()

	return b.gatewayMarshaler[gatewayID]
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
			log.WithError(err).Error("backend/gateway: could not load ca certificate")
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
			log.WithError(err).Error("backend/gateway: could not load mqtt tls key-pair")
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{kp}
	}

	return tlsConfig, nil
}
