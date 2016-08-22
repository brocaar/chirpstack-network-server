package common

import (
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"

	"github.com/brocaar/lorawan/band"
)

// TestConfig contains the test configuration.
type TestConfig struct {
	RedisURL string
}

// GetTestConfig returns the test configuration.
func GetTestConfig() *TestConfig {
	var err error
	log.SetLevel(log.ErrorLevel)

	Band, err = band.GetConfig(band.EU_863_870)
	if err != nil {
		panic(err)
	}

	c := &TestConfig{
		RedisURL: "redis://localhost:6379",
	}

	if v := os.Getenv("TEST_REDIS_URL"); v != "" {
		c.RedisURL = v
	}

	return c
}

// MustFlushRedis flushes the Redis storage.
func MustFlushRedis(p *redis.Pool) {
	c := p.Get()
	defer c.Close()
	if _, err := c.Do("FLUSHALL"); err != nil {
		log.Fatal(err)
	}
}
