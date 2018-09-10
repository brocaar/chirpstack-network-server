package storage

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/pkg/errors"

	"github.com/gofrs/uuid"
	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/internal/config"
)

// Templates used for generating Redis keys
const (
	ServiceProfileKeyTempl = "lora:ns:sp:%s"
)

// RatePolicy defines the RatePolicy type.
type RatePolicy string

// Available rate policies.
const (
	Drop RatePolicy = "Drop"
	Mark RatePolicy = "Mark"
)

// ServiceProfile defines the backend.ServiceProfile with some extra meta-data.
type ServiceProfile struct {
	CreatedAt              time.Time  `db:"created_at"`
	UpdatedAt              time.Time  `db:"updated_at"`
	ID                     uuid.UUID  `db:"service_profile_id"`
	ULRate                 int        `db:"ul_rate"`
	ULBucketSize           int        `db:"ul_bucket_size"`
	ULRatePolicy           RatePolicy `db:"ul_rate_policy"`
	DLRate                 int        `db:"dl_rate"`
	DLBucketSize           int        `db:"dl_bucket_size"`
	DLRatePolicy           RatePolicy `db:"dl_rate_policy"`
	AddGWMetadata          bool       `db:"add_gw_metadata"`
	DevStatusReqFreq       int        `db:"dev_status_req_freq"` // Unit: requests-per-day
	ReportDevStatusBattery bool       `db:"report_dev_status_battery"`
	ReportDevStatusMargin  bool       `db:"report_dev_status_margin"`
	DRMin                  int        `db:"dr_min"`
	DRMax                  int        `db:"dr_max"`
	ChannelMask            []byte     `db:"channel_mask"`
	PRAllowed              bool       `db:"pr_allowed"`
	HRAllowed              bool       `db:"hr_allowed"`
	RAAllowed              bool       `db:"ra_allowed"`
	NwkGeoLoc              bool       `db:"nwk_geo_loc"`
	TargetPER              int        `db:"target_per"` // Example: 10 indicates 10%
	MinGWDiversity         int        `db:"min_gw_diversity"`
}

// CreateServiceProfile creates the given service-profile.
func CreateServiceProfile(db sqlx.Execer, sp *ServiceProfile) error {
	now := time.Now()

	if sp.ID == uuid.Nil {
		var err error
		sp.ID, err = uuid.NewV4()
		if err != nil {
			return errors.Wrap(err, "new uuid v4 error")
		}
	}

	sp.CreatedAt = now
	sp.UpdatedAt = now

	_, err := db.Exec(`
		insert into service_profile (
			created_at,
			updated_at,

			service_profile_id,
			ul_rate,
			ul_bucket_size,
			ul_rate_policy,
			dl_rate,
			dl_bucket_size,
			dl_rate_policy,
			add_gw_metadata,
			dev_status_req_freq,
			report_dev_status_battery,
			report_dev_status_margin,
			dr_min,
			dr_max,
			channel_mask,
			pr_allowed,
			hr_allowed,
			ra_allowed,
			nwk_geo_loc,
			target_per,
			min_gw_diversity
		) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)`,
		sp.CreatedAt,
		sp.UpdatedAt,
		sp.ID,
		sp.ULRate,
		sp.ULBucketSize,
		sp.ULRatePolicy,
		sp.DLRate,
		sp.DLBucketSize,
		sp.DLRatePolicy,
		sp.AddGWMetadata,
		sp.DevStatusReqFreq,
		sp.ReportDevStatusBattery,
		sp.ReportDevStatusMargin,
		sp.DRMin,
		sp.DRMax,
		sp.ChannelMask,
		sp.PRAllowed,
		sp.HRAllowed,
		sp.RAAllowed,
		sp.NwkGeoLoc,
		sp.TargetPER,
		sp.MinGWDiversity,
	)
	if err != nil {
		return handlePSQLError(err, "insert error")
	}

	log.WithFields(log.Fields{
		"id": sp.ID,
	}).Info("service-profile created")

	return nil
}

// CreateServiceProfileCache caches the given service-profile into the Redis.
// This is used for faster lookups, but also in case of roaming where we
// only want to store the service-profile of a roaming device for a finite
// duration.
// The TTL of the service-profile is the same as that of the device-sessions.
func CreateServiceProfileCache(p *redis.Pool, sp ServiceProfile) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(sp); err != nil {
		return errors.Wrap(err, "gob encode service-profile error")
	}

	c := p.Get()
	defer c.Close()

	key := fmt.Sprintf(ServiceProfileKeyTempl, sp.ID)
	exp := int64(config.C.NetworkServer.DeviceSessionTTL) / int64(time.Millisecond)

	_, err := c.Do("PSETEX", key, exp, buf.Bytes())
	if err != nil {
		return errors.Wrap(err, "set service-profile error")
	}

	return nil
}

// GetServiceProfileCache returns a cached service-profile.
func GetServiceProfileCache(p *redis.Pool, id uuid.UUID) (ServiceProfile, error) {
	var sp ServiceProfile
	key := fmt.Sprintf(ServiceProfileKeyTempl, id)

	c := p.Get()
	defer c.Close()

	val, err := redis.Bytes(c.Do("GET", key))
	if err != nil {
		if err == redis.ErrNil {
			return sp, ErrDoesNotExist
		}
		return sp, errors.Wrap(err, "get error")
	}

	err = gob.NewDecoder(bytes.NewReader(val)).Decode(&sp)
	if err != nil {
		return sp, errors.Wrap(err, "gob decode error")
	}

	return sp, nil
}

// FlushServiceProfileCache deletes a cached service-profile.
func FlushServiceProfileCache(p *redis.Pool, id uuid.UUID) error {
	key := fmt.Sprintf(ServiceProfileKeyTempl, id)
	c := p.Get()
	defer c.Close()

	_, err := c.Do("DEL", key)
	if err != nil {
		return errors.Wrap(err, "delete error")
	}
	return nil
}

// GetAndCacheServiceProfile returns the service-profile from cache in case
// available, else it will be retrieved from the database and then stored
// in cache.
func GetAndCacheServiceProfile(db sqlx.Queryer, p *redis.Pool, id uuid.UUID) (ServiceProfile, error) {
	sp, err := GetServiceProfileCache(p, id)
	if err == nil {
		return sp, nil
	}

	if err != ErrDoesNotExist {
		log.WithFields(log.Fields{
			"id": id,
		}).WithError(err).Error("get service-profile cache error")
		// we don't return as we can fall-back onto db retrieval
	}

	sp, err = GetServiceProfile(db, id)
	if err != nil {
		return ServiceProfile{}, errors.Wrap(err, "get service-profile-error")
	}

	err = CreateServiceProfileCache(p, sp)
	if err != nil {
		log.WithFields(log.Fields{
			"id": id,
		}).WithError(err).Error("create service-profile cache error")
	}

	return sp, nil
}

// GetServiceProfile returns the service-profile matching the given id.
func GetServiceProfile(db sqlx.Queryer, id uuid.UUID) (ServiceProfile, error) {
	var sp ServiceProfile
	err := sqlx.Get(db, &sp, "select * from service_profile where service_profile_id = $1", id)
	if err != nil {
		return sp, handlePSQLError(err, "select error")
	}

	return sp, nil
}

// UpdateServiceProfile updates the given service-profile.
func UpdateServiceProfile(db sqlx.Execer, sp *ServiceProfile) error {
	sp.UpdatedAt = time.Now()

	res, err := db.Exec(`
		update service_profile set
			updated_at = $2,

			ul_rate = $3,
			ul_bucket_size = $4,
			ul_rate_policy = $5,
			dl_rate = $6,
			dl_bucket_size = $7,
			dl_rate_policy = $8,
			add_gw_metadata = $9,
			dev_status_req_freq = $10,
			report_dev_status_battery = $11,
			report_dev_status_margin = $12,
			dr_min = $13,
			dr_max = $14,
			channel_mask = $15,
			pr_allowed = $16,
			hr_allowed = $17,
			ra_allowed = $18,
			nwk_geo_loc = $19,
			target_per = $20,
			min_gw_diversity = $21
		where
			service_profile_id = $1`,
		sp.ID,
		sp.UpdatedAt,
		sp.ULRate,
		sp.ULBucketSize,
		sp.ULRatePolicy,
		sp.DLRate,
		sp.DLBucketSize,
		sp.DLRatePolicy,
		sp.AddGWMetadata,
		sp.DevStatusReqFreq,
		sp.ReportDevStatusBattery,
		sp.ReportDevStatusMargin,
		sp.DRMin,
		sp.DRMax,
		sp.ChannelMask,
		sp.PRAllowed,
		sp.HRAllowed,
		sp.RAAllowed,
		sp.NwkGeoLoc,
		sp.TargetPER,
		sp.MinGWDiversity,
	)
	if err != nil {
		return handlePSQLError(err, "update error")
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return handlePSQLError(err, "get rows affected error")
	}
	if ra == 0 {
		return ErrDoesNotExist
	}

	log.WithField("id", sp.ID).Info("service-profile updated")
	return nil
}

// DeleteServiceProfile deletes the service-profile matching the given id.
func DeleteServiceProfile(db sqlx.Execer, id uuid.UUID) error {
	res, err := db.Exec("delete from service_profile where service_profile_id = $1", id)
	if err != nil {
		return handlePSQLError(err, "delete error")
	}

	ra, err := res.RowsAffected()
	if err != nil {
		return handlePSQLError(err, "get rows affected error")
	}
	if ra == 0 {
		return ErrDoesNotExist
	}

	log.WithField("id", id).Info("service-profile deleted")
	return nil
}
