package storage

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"time"

	"github.com/gofrs/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/lorawan"
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
	MAC              lorawan.EUI64 `db:"mac"`
	CreatedAt        time.Time     `db:"created_at"`
	UpdatedAt        time.Time     `db:"updated_at"`
	FirstSeenAt      *time.Time    `db:"first_seen_at"`
	LastSeenAt       *time.Time    `db:"last_seen_at"`
	Location         GPSPoint      `db:"location"`
	Altitude         float64       `db:"altitude"`
	GatewayProfileID *uuid.UUID    `db:"gateway_profile_id"`
}

// Validate validates the data of the gateway.
func (g Gateway) Validate() error {
	return nil
}

// CreateGateway creates the given gateway.
func CreateGateway(db sqlx.Execer, gw *Gateway) error {
	if err := gw.Validate(); err != nil {
		return errors.Wrap(err, "validate error")
	}

	now := time.Now()
	_, err := db.Exec(`
		insert into gateway (
			mac,
			created_at,
			updated_at,
			first_seen_at,
			last_seen_at,
			location,
			altitude,
			gateway_profile_id
		) values ($1, $2, $3, $4, $5, $6, $7, $8)`,
		gw.MAC[:],
		now,
		now,
		gw.FirstSeenAt,
		gw.LastSeenAt,
		gw.Location,
		gw.Altitude,
		gw.GatewayProfileID,
	)
	if err != nil {
		return handlePSQLError(err, "insert error")
	}
	gw.CreatedAt = now
	gw.UpdatedAt = now
	log.WithField("mac", gw.MAC).Info("gateway created")
	return nil
}

// GetGateway returns the gateway for the given MAC.
func GetGateway(db sqlx.Queryer, mac lorawan.EUI64) (Gateway, error) {
	var gw Gateway
	err := sqlx.Get(db, &gw, "select * from gateway where mac = $1", mac[:])
	if err != nil {
		return gw, handlePSQLError(err, "select error")
	}
	return gw, nil
}

// UpdateGateway updates the given gateway.
func UpdateGateway(db sqlx.Execer, gw *Gateway) error {
	if err := gw.Validate(); err != nil {
		return errors.Wrap(err, "validate error")
	}

	now := time.Now()
	res, err := db.Exec(`
		update gateway set
			updated_at = $2,
			first_seen_at = $3,
			last_seen_at = $4,
			location = $5,
			altitude = $6,
			gateway_profile_id = $7
		where mac = $1`,
		gw.MAC[:],
		now,
		gw.FirstSeenAt,
		gw.LastSeenAt,
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
	gw.UpdatedAt = now
	log.WithField("mac", gw.MAC).Info("gateway updated")
	return nil
}

// DeleteGateway deletes the gateway matching the given MAC.
func DeleteGateway(db sqlx.Execer, mac lorawan.EUI64) error {
	res, err := db.Exec("delete from gateway where mac = $1", mac[:])
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
	log.WithField("mac", mac).Info("gateway deleted")
	return nil
}

// GetGatewaysForMACs returns a map of gateways given a slice of MACs.
func GetGatewaysForMACs(db sqlx.Queryer, macs []lorawan.EUI64) (map[lorawan.EUI64]Gateway, error) {
	out := make(map[lorawan.EUI64]Gateway)
	var macsB [][]byte
	for i := range macs {
		macsB = append(macsB, macs[i][:])
	}

	var gws []Gateway
	err := sqlx.Select(db, &gws, "select * from gateway where mac = any($1)", pq.ByteaArray(macsB))
	if err != nil {
		return nil, handlePSQLError(err, "select error")
	}

	if len(gws) != len(macs) {
		return nil, fmt.Errorf("expected %d gateways, got %d", len(macs), len(out))
	}

	for i := range gws {
		out[gws[i].MAC] = gws[i]
	}

	return out, nil
}
