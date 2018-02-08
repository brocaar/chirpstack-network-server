package gateway

import (
	"os"

	log "github.com/sirupsen/logrus"
)

func init() {
	log.SetLevel(log.ErrorLevel)
}

type c struct {
	Server   string
	Username string
	Password string
	RedisURL string
}

func getConfig() *c {
	c := &c{
		Server:   "tcp://127.0.0.1:1883",
		RedisURL: "redis://localhost:6379",
	}

	if v := os.Getenv("TEST_MQTT_SERVER"); v != "" {
		c.Server = v
	}

	if v := os.Getenv("TEST_MQTT_USERNAME"); v != "" {
		c.Username = v
	}

	if v := os.Getenv("TEST_MQTT_PASSWORD"); v != "" {
		c.Password = v
	}

	if v := os.Getenv("TEST_REDIS_URL"); v != "" {
		c.RedisURL = v
	}

	return c
}
