package code

import (
	"fmt"

	"github.com/go-redis/redis/v7"
	"github.com/gofrs/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// FlushProfilesCache fixes an issue with the device-profile and service-profile
// cache in Redis. As the struct changed the cached value from ChirpStack Network Server v1
// can't be unmarshaled into the ChirpStack Network Server v2 struct and therefore we need
// to flush the cache.
func FlushProfilesCache(db sqlx.Queryer, redisClient redis.UniversalClient, deviceProfileKeyTempl, serviceProfileKeyTempl string) error {
	var uuids []uuid.UUID
	var keys []string

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
		keys = append(keys, fmt.Sprintf(deviceProfileKeyTempl, id))
	}

	if len(keys) != 0 {
		err = redisClient.Del(keys...).Err()
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
		keys = append(keys, fmt.Sprintf(serviceProfileKeyTempl, id))
	}

	if len(keys) != 0 {
		err = redisClient.Del(keys...).Err()
		if err != nil {
			return errors.Wrap(err, "delete service-profiles from cache error")
		}
	}

	log.Info("service-profile and device-profile redis cache flushed")

	return nil
}
