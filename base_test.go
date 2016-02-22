package loraserver

import (
	"os"

	log "github.com/Sirupsen/logrus"
)

func init() {
	log.SetLevel(log.ErrorLevel)
}

type config struct {
	RedisURL string
}

func getConfig() *config {
	c := &config{
		RedisURL: "redis://localhost:6379",
	}

	if v := os.Getenv("TEST_REDIS_URL"); v != "" {
		c.RedisURL = v
	}

	return c
}
