package gateway

import (
	"os"

	log "github.com/Sirupsen/logrus"
)

func init() {
	log.SetLevel(log.ErrorLevel)
}

type config struct {
	Server   string
	Username string
	Password string
}

func getConfig() *config {
	c := &config{
		Server: "tcp://127.0.0.1:1883",
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

	return c
}
