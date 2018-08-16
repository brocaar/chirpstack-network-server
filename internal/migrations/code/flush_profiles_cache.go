package code

import (
	"fmt"

	"github.com/gofrs/uuid"
	"github.com/gomodule/redigo/redis"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/internal/storage"
)

// FlushProfilesCache fixes an issue with the device-profile and service-profile
// cache in Redis. As the struct changed the cached value from LoRa Server v1
// can't be unmarshaled into the LoRa Server v2 struct and therefore we need
// to flush the cache.
func FlushProfilesCache(p *redis.Pool, db sqlx.Queryer) error {
	c := p.Get()
	defer c.Close()

	var uuids []uuid.UUID
	var keys []interface{}

	// device-profiles
	err := sqlx.Select(db, &uuids, `
		select
			device_profile_id
		from
			device_profile
	`)
	if err != nil {
		return errors.Wrap(err, "select device-profile ids error")
	}

	for _, id := range uuids {
		keys = append(keys, fmt.Sprintf(storage.DeviceProfileKeyTempl, id))
	}

	if len(keys) != 0 {
		_, err = redis.Int(c.Do("DEL", keys...))
		if err != nil {
			return errors.Wrap(err, "delete device-profiles from cache error")
		}
	}

	// service-profiles
	err = sqlx.Select(db, &uuids, `
		select
			service_profile_id
		from
			service_profile
	`)
	if err != nil {
		return errors.Wrap(err, "select service-profile ids error")
	}

	keys = nil
	for _, id := range uuids {
		keys = append(keys, fmt.Sprintf(storage.ServiceProfileKeyTempl, id))
	}

	if len(keys) != 0 {
		_, err = redis.Int(c.Do("DEL", keys...))
		if err != nil {
			return errors.Wrap(err, "delete service-profiles from cache error")
		}
	}

	log.Info("service-profile and device-profile redis cache flushed")

	return nil
}
