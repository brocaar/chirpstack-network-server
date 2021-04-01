package test

import (
	"os"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-network-server/v3/internal/band"
	"github.com/brocaar/chirpstack-network-server/v3/internal/config"
	"github.com/brocaar/lorawan"
	loraband "github.com/brocaar/lorawan/band"
)

func init() {
	log.SetLevel(log.ErrorLevel)

}

// GetConfig returns the test configuration.
func GetConfig() config.Config {
	log.SetLevel(log.FatalLevel)

	var c config.Config
	c.NetworkServer.Band.Name = loraband.EU868

	if err := band.Setup(c); err != nil {
		panic(err)
	}

	c.Redis.Servers = []string{"localhost:6379"}
	c.PostgreSQL.DSN = "postgres://localhost/chirpstack_ns_test?sslmode=disable"

	c.NetworkServer.NetID = lorawan.NetID{3, 2, 1}
	c.NetworkServer.DeviceSessionTTL = time.Hour
	c.NetworkServer.DeduplicationDelay = 5 * time.Millisecond
	c.NetworkServer.GetDownlinkDataDelay = 5 * time.Millisecond

	c.NetworkServer.NetworkSettings.RX2Frequency = band.Band().GetDefaults().RX2Frequency
	c.NetworkServer.NetworkSettings.RX2DR = band.Band().GetDefaults().RX2DataRate
	c.NetworkServer.NetworkSettings.RX1Delay = 0
	c.NetworkServer.NetworkSettings.DownlinkTXPower = -1
	c.NetworkServer.NetworkSettings.MaxMACCommandErrorCount = 3

	c.NetworkServer.Scheduler.SchedulerInterval = time.Second

	c.NetworkServer.Gateway.Backend.MultiDownlinkFeature = "multi_only"
	c.NetworkServer.Gateway.Backend.MQTT.Server = "tcp://127.0.0.1:1883"
	c.NetworkServer.Gateway.Backend.MQTT.CleanSession = true
	c.NetworkServer.Gateway.Backend.MQTT.EventTopic = "gateway/+/event/+"
	c.NetworkServer.Gateway.Backend.MQTT.CommandTopicTemplate = "gateway/{{ .GatewayID }}/command/{{ .CommandType }}"

	c.NetworkServer.Gateway.Backend.AMQP.EventQueueName = "gateway-events"
	c.NetworkServer.Gateway.Backend.AMQP.EventRoutingKey = "gateway.*.event.*"
	c.NetworkServer.Gateway.Backend.AMQP.CommandRoutingKeyTemplate = "gateway.{{ .GatewayID }}.command.{{ .CommandType }}"

	if v := os.Getenv("TEST_REDIS_SERVERS"); v != "" {
		c.Redis.Servers = strings.Split(v, ",")
	}
	if v := os.Getenv("TEST_POSTGRES_DSN"); v != "" {
		c.PostgreSQL.DSN = v
	}
	if v := os.Getenv("TEST_MQTT_SERVER"); v != "" {
		c.NetworkServer.Gateway.Backend.MQTT.Server = v
	}
	if v := os.Getenv("TEST_MQTT_USERNAME"); v != "" {
		c.NetworkServer.Gateway.Backend.MQTT.Username = v
	}
	if v := os.Getenv("TEST_MQTT_PASSWORD"); v != "" {
		c.NetworkServer.Gateway.Backend.MQTT.Password = v
	}
	if v := os.Getenv("TEST_RABBITMQ_URL"); v != "" {
		c.NetworkServer.Gateway.Backend.AMQP.URL = v
	}

	return c
}
