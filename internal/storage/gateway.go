package storage

import (
	"bytes"
	"database/sql/driver"
	"encoding/gob"
	"fmt"
	"strconv"
	"time"

	"github.com/gofrs/uuid"
	"github.com/gomodule/redigo/redis"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/lorawan"
)

// tempaltes used for generating Redis keys
const (
	gatewayKeyTempl = "lora:ns:gw:%s"
)

// GPSPoint contains a GPS point.
type GPSPoint struct {
	Latitude  float64
	Longitude float64
}

// Value implements the driver.Valuer interface.
func (l GPSPoint) Value() (driver.Value, error) {
	return fmt.Sprintf("(%s,%s)", strconv.FormatFloat(l.Latitude, 'f', -1, 64), strconv.FormatFloat(l.Longitude, 'f', -1, 64)), nil
}

// Scan implements the sql.Scanner interface.
func (l *GPSPoint) Scan(src interface{}) error {
	b, ok := src.([]byte)
	if !ok {
		return fmt.Errorf("expected []byte, got %T", src)
	}

	_, err := fmt.Sscanf(string(b), "(%f,%f)", &l.Latitude, &l.Longitude)
	return err
}

// Gateway represents a gateway.
type Gateway struct {
	GatewayID        lorawan.EUI64  `db:"gateway_id"`
	CreatedAt        time.Time      `db:"created_at"`
	UpdatedAt        time.Time      `db:"updated_at"`
	Location         GPSPoint       `db:"location"`
	Altitude         float64        `db:"altitude"`
	GatewayProfileID *uuid.UUID     `db:"gateway_profile_id"`
	Boards           []GatewayBoard `db:"-"`
}

// GatewayBoard holds the gateway board configuration.
type GatewayBoard struct {
	FPGAID           *lorawan.EUI64     `db:"fpga_id"`
	FineTimestampKey *lorawan.AES128Key `db:"fine_timestamp_key"`
}

// CreateGateway creates the given gateway.
func CreateGateway(db sqlx.Execer, gw *Gateway) error {
	now := time.Now()
	gw.CreatedAt = now
	gw.UpdatedAt = now

	_, err := db.Exec(`
		insert into gateway (
			gateway_id,
			created_at,
			updated_at,
			location,
			altitude,
			gateway_profile_id
		) values ($1, $2, $3, $4, $5, $6)`,
		gw.GatewayID[:],
		gw.CreatedAt,
		gw.UpdatedAt,
		gw.Location,
		gw.Altitude,
		gw.GatewayProfileID,
	)
	if err != nil {
		return handlePSQLError(err, "insert error")
	}

	for i, board := range gw.Boards {
		_, err := db.Exec(`
			insert into gateway_board (
				id,
				gateway_id,
				fpga_id,
				fine_timestamp_key
			) values ($1, $2, $3, $4)`,
			i,
			gw.GatewayID,
			board.FPGAID,
			board.FineTimestampKey,
		)
		if err != nil {
			return handlePSQLError(err, "insert error")
		}
	}

	log.WithField("gateway_id", gw.GatewayID).Info("gateway created")
	return nil
}

// CreateGatewayCache caches the given gateway in Redis.
// The TTL of the gateway is the same as that of the device-sessions.
func CreateGatewayCache(p *redis.Pool, gw Gateway) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(gw); err != nil {
		return errors.Wrap(err, "gob encode gateway error")
	}

	c := p.Get()
	defer c.Close()

	key := fmt.Sprintf(gatewayKeyTempl, gw.GatewayID)
	exp := int64(config.C.NetworkServer.DeviceSessionTTL) / int64(time.Millisecond)

	_, err := c.Do("PSETEX", key, exp, buf.Bytes())
	if err != nil {
		return errors.Wrap(err, "set gateway error")
	}

	return nil
}

// GetGatewayCache returns a cached gateway.
func GetGatewayCache(p *redis.Pool, gatewayID lorawan.EUI64) (Gateway, error) {
	var gw Gateway
	key := fmt.Sprintf(gatewayKeyTempl, gatewayID)

	c := p.Get()
	defer c.Close()

	val, err := redis.Bytes(c.Do("GET", key))
	if err != nil {
		if err == redis.ErrNil {
			return gw, ErrDoesNotExist
		}
		return gw, errors.Wrap(err, "get error")
	}

	err = gob.NewDecoder(bytes.NewReader(val)).Decode(&gw)
	if err != nil {
		return gw, errors.Wrap(err, "gob decode error")
	}

	return gw, nil
}

// FlushGatewayCache deletes a cached gateway.
func FlushGatewayCache(p *redis.Pool, gatewayID lorawan.EUI64) error {
	key := fmt.Sprintf(gatewayKeyTempl, gatewayID)
	c := p.Get()
	defer c.Close()

	_, err := c.Do("DEL", key)
	if err != nil {
		return errors.Wrap(err, "delete error")
	}

	return nil
}

// GetAndCacheGateway returns a gateway from the cache in case it is available.
// In case the gateway is not cached, it will be retrieved from the database
// and then cached.
func GetAndCacheGateway(db sqlx.Queryer, p *redis.Pool, gatewayID lorawan.EUI64) (Gateway, error) {
	gw, err := GetGatewayCache(p, gatewayID)
	if err == nil {
		return gw, nil
	}

	if err != ErrDoesNotExist {
		log.WithFields(log.Fields{
			"gateway_id": gatewayID,
		}).WithError(err).Error("get gateway cache error")
		// we don't return the error as we can still fall-back onto db retrieval
	}

	gw, err = GetGateway(db, gatewayID)
	if err != nil {
		return gw, errors.Wrap(err, "get gateway error")
	}

	err = CreateGatewayCache(p, gw)
	if err != nil {
		log.WithFields(log.Fields{
			"gateway_id": gatewayID,
		}).WithError(err).Error("create gateway cache error")
	}

	return gw, nil
}

// GetGateway returns the gateway for the given Gateway ID.
func GetGateway(db sqlx.Queryer, id lorawan.EUI64) (Gateway, error) {
	var gw Gateway
	err := sqlx.Get(db, &gw, "select * from gateway where gateway_id = $1", id[:])
	if err != nil {
		return gw, handlePSQLError(err, "select error")
	}

	err = sqlx.Select(db, &gw.Boards, `
		select
			fpga_id,
			fine_timestamp_key
		from
			gateway_board
		where
			gateway_id = $1
		order by
			id
		`,
		id,
	)
	if err != nil {
		return gw, handlePSQLError(err, "select error")
	}

	return gw, nil
}

// UpdateGateway updates the given gateway.
func UpdateGateway(db sqlx.Execer, gw *Gateway) error {
	now := time.Now()
	gw.UpdatedAt = now

	res, err := db.Exec(`
		update gateway set
			updated_at = $2,
			location = $3,
			altitude = $4,
			gateway_profile_id = $5
		where gateway_id = $1`,
		gw.GatewayID[:],
		gw.UpdatedAt,
		gw.Location,
		gw.Altitude,
		gw.GatewayProfileID,
	)
	if err != nil {
		return handlePSQLError(err, "update error")
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "get rows affected error")
	}
	if ra == 0 {
		return ErrDoesNotExist
	}

	_, err = db.Exec(`
		delete from gateway_board where gateway_id = $1`,
		gw.GatewayID,
	)
	if err != nil {
		return handlePSQLError(err, "delete error")
	}

	for i, board := range gw.Boards {
		_, err := db.Exec(`
			insert into gateway_board (
				id,
				gateway_id,
				fpga_id,
				fine_timestamp_key
			) values ($1, $2, $3, $4)`,
			i,
			gw.GatewayID,
			board.FPGAID,
			board.FineTimestampKey,
		)
		if err != nil {
			return handlePSQLError(err, "insert error")
		}
	}

	log.WithField("gateway_id", gw.GatewayID).Info("gateway updated")
	return nil
}

// DeleteGateway deletes the gateway matching the given Gateway ID.
func DeleteGateway(db sqlx.Execer, id lorawan.EUI64) error {
	res, err := db.Exec("delete from gateway where gateway_id = $1", id[:])
	if err != nil {
		return handlePSQLError(err, "delete error")
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "get rows affected error")
	}
	if ra == 0 {
		return ErrDoesNotExist
	}
	log.WithField("gateway_id", id).Info("gateway deleted")
	return nil
}

// GetGatewaysForIDs returns a map of gateways given a slice of IDs.
func GetGatewaysForIDs(db sqlx.Queryer, ids []lorawan.EUI64) (map[lorawan.EUI64]Gateway, error) {
	out := make(map[lorawan.EUI64]Gateway)
	var idsB [][]byte
	for i := range ids {
		idsB = append(idsB, ids[i][:])
	}

	var gws []Gateway
	err := sqlx.Select(db, &gws, "select * from gateway where gateway_id = any($1)", pq.ByteaArray(idsB))
	if err != nil {
		return nil, handlePSQLError(err, "select error")
	}

	if len(gws) != len(ids) {
		return nil, fmt.Errorf("expected %d gateways, got %d", len(ids), len(out))
	}

	for i := range gws {
		out[gws[i].GatewayID] = gws[i]
	}

	return out, nil
}
