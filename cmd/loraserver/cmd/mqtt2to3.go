package cmd

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/brocaar/loraserver/internal/config"
)

var mqtt2to3 = &cobra.Command{
	Use:   "mqtt2to3",
	Short: "Re-publishes MQTT messages to the new LoRa Server v3 topic layout",
	RunE: func(cmd *cobra.Command, args []string) error {
		b, err := NewMQTT2To3Backend(config.C)
		if err != nil {
			return err
		}

		sigChan := make(chan os.Signal)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		log.WithField("signal", <-sigChan).Info("signal received")

		return b.Close()
	},
}

// MQTT2To3Backend implement a MQTT forwarder backend.
type MQTT2To3Backend struct {
	conn mqtt.Client
}

// NewMQTT2To3Backend creates a new Backend.
func NewMQTT2To3Backend(conf config.Config) (*MQTT2To3Backend, error) {
	c := conf.NetworkServer.Gateway.Backend.MQTT

	b := MQTT2To3Backend{}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(c.Server)
	opts.SetUsername(c.Username)
	opts.SetPassword(c.Password)
	opts.SetCleanSession(c.CleanSession)
	opts.SetClientID(c.ClientID)
	opts.SetOnConnectHandler(b.onConnected)

	tlsconfig, err := newTLSConfig(c.CACert, c.TLSCert, c.TLSKey)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"ca_cert":  c.CACert,
			"tls_cert": c.TLSCert,
			"tls_key":  c.TLSKey,
		}).Fatal("mqtt2to3: error loading mqtt certificate files")
	}
	if tlsconfig != nil {
		opts.SetTLSConfig(tlsconfig)
	}

	log.WithField("broker", c.Server).Info("mqtt2to3: connecting to mqtt broker")
	b.conn = mqtt.NewClient(opts)

	for {
		if token := b.conn.Connect(); token.Wait() && token.Error() != nil {
			log.WithError(err).Error("mqtt2to3: connecting to mqtt broker failed")
			time.Sleep(2 * time.Second)
		} else {
			break
		}
	}

	return &b, nil
}

func (b *MQTT2To3Backend) onConnected(c mqtt.Client) {
	log.Info("mqtt2to3: connected to mqtt broker")

	topics := map[string]byte{
		// old uplink (no region prefix)
		"gateway/+/rx":    0,
		"gateway/+/ack":   0,
		"gateway/+/stats": 0,

		// old uplink (region prefix)
		"+/gateway/+/rx":    0,
		"+/gateway/+/ack":   0,
		"+/gateway/+/stats": 0,

		// new downlink (no region prefix)
		"gateway/+/command/+": 0,

		// new downlink (no region prefix)
		"+/gateway/+/command/+": 0,
	}

	for {
		if token := b.conn.SubscribeMultiple(topics, b.messageHandler); token.Wait() && token.Error() != nil {
			log.WithError(token.Error()).Error("mqtt2to3: subscribe error")
			time.Sleep(time.Second)
			continue
		}
		break
	}
}

func (b *MQTT2To3Backend) Close() error {
	log.Info("mqtt2to3: closing backend")
	b.conn.Disconnect(250)
	return nil
}

func (b *MQTT2To3Backend) messageHandler(c mqtt.Client, msg mqtt.Message) {
	parts := strings.Split(msg.Topic(), "/")
	if len(parts) < 2 {
		log.WithField("topic", msg.Topic()).Error("mqtt2to3: topic must have at least two elements")
		return
	}

	var topic string
	var prefix string

	suffix := parts[len(parts)-1]

	if parts[len(parts)-2] == "command" {
		prefix = strings.Join(parts[:len(parts)-2], "/") // prefix without 'command'
	} else {
		prefix = strings.Join(parts[:len(parts)-1], "/")
	}

	switch suffix {
	// uplink (old to new)
	case "rx":
		topic = prefix + "/event/" + "up"
	case "ack":
		topic = prefix + "/event/" + "ack"
	case "stats":
		topic = prefix + "/event/" + "stats"
	// downlink (new to old)
	case "down":
		topic = prefix + "/" + "tx"
	case "config":
		topic = prefix + "/" + "config"
	default:
		return
	}

	if token := b.conn.Publish(topic, 0, false, msg.Payload()); token.Wait() && token.Error() != nil {
		log.WithError(token.Error()).WithField("topic", topic).Error("mqtt2to3: publish event error")
	}

	log.WithFields(log.Fields{
		"topic_from": msg.Topic(),
		"topic_to":   topic,
	}).Info("mqtt2to3: message forwarded")
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
