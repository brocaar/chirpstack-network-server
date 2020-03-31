package code

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-network-server/internal/storage"
)

// MigrateToClusterKeys migrates the keys to Redis Cluster compatible keys.
func MigrateToClusterKeys() error {

	keys, err := storage.RedisClient().Keys("lora:ns:metrics:*").Result()
	if err != nil {
		return errors.Wrap(err, "get keys error")
	}

	for i, key := range keys {
		if err := migrateKey(key); err != nil {
			log.WithError(err).Error("migrations/code: migrate metrics key error")
		}

		if i > 0 && i%1000 == 0 {
			log.WithFields(log.Fields{
				"migrated":    i,
				"total_count": len(keys),
			}).Info("migrations/code: migrating metrics keys")
		}
	}

	return nil
}

func migrateKey(key string) error {
	keyParts := strings.Split(key, ":")
	if len(keyParts) < 6 {
		return fmt.Errorf("key %s is invalid", key)
	}

	ttlMap := map[string]time.Duration{
		"MINUTE": time.Hour * 2,
		"HOUR":   time.Hour * 48,
		"DAY":    time.Hour * 24 * 90,
		"MONTH":  time.Hour * 24 * 730,
	}

	ttl, ok := ttlMap[keyParts[len(keyParts)-2]]
	if !ok {
		return fmt.Errorf("key %s is invalid", key)
	}

	newKey := fmt.Sprintf("lora:ns:metrics:{%s}:%s", strings.Join(keyParts[3:len(keyParts)-2], ":"), strings.Join(keyParts[len(keyParts)-2:], ":"))

	val, err := storage.RedisClient().HGetAll(key).Result()
	if err != nil {
		return errors.Wrap(err, "hgetall error")
	}

	pipe := storage.RedisClient().TxPipeline()
	for k, v := range val {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return errors.Wrap(err, "parse float error")
		}

		pipe.HIncrByFloat(newKey, k, f)
	}
	pipe.PExpire(key, ttl)

	if _, err := pipe.Exec(); err != nil {
		return errors.Wrap(err, "exec error")
	}

	return nil
}
