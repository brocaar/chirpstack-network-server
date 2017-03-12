package migration

import (
	"bytes"
	"encoding/gob"
	"fmt"

	log "github.com/Sirupsen/logrus"
	"github.com/brocaar/loraserver/internal/session"
	"github.com/garyburd/redigo/redis"
)

// MigrateNodeSessionDevAddrDevEUI migrates the node-sessions from DevAddr to
// DevEUI identifier.
func MigrateNodeSessionDevAddrDevEUI(p *redis.Pool) error {
	c := p.Get()
	defer c.Close()

	keys, err := redis.Strings(c.Do("KEYS", "node_session_????????")) // only match DevAddr (e.g. 01020304)
	if err != nil {
		return fmt.Errorf("get keys error: %s", err)
	}

	var errCount int

	for _, key := range keys {
		if err := migrateNodeSession(p, key); err != nil {
			log.WithField("key", key).Errorf("migrate node-session error: %s", err)
			errCount++
		}
	}

	log.WithFields(log.Fields{
		"migrated": len(keys) - errCount,
		"errors":   errCount,
	}).Infof("migrated node-sessions to new format")
	return nil
}

func migrateNodeSession(p *redis.Pool, key string) error {
	var ns session.NodeSession

	c := p.Get()
	defer c.Close()

	// get node-session from old key
	val, err := redis.Bytes(c.Do("GET", key))
	if err != nil {
		return fmt.Errorf("get key error: %s", err)
	}
	err = gob.NewDecoder(bytes.NewReader(val)).Decode(&ns)
	if err != nil {
		return fmt.Errorf("decode node-session error: %s", err)
	}

	// save node-session (using new key layout)
	err = session.SaveNodeSession(p, ns)
	if err != nil {
		return fmt.Errorf("save node-session error: %s", err)
	}

	// delete the old key
	_, err = redis.Int(c.Do("DEL", key))
	if err != nil {
		return fmt.Errorf("delete key error: %s", err)
	}

	return nil
}
