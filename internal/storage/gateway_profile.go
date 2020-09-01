package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/gofrs/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-network-server/internal/logging"
)

// Modulations
const (
	ModulationFSK  = "FSK"
	ModulationLoRa = "LORA"
)

// ExtraChannel defines an extra channel for the gateway-profile.
type ExtraChannel struct {
	Modulation       string  `db:"modulation"`
	Frequency        int     `db:"frequency"`
	Bandwidth        int     `db:"bandwidth"`
	Bitrate          int     `db:"bitrate"`
	SpreadingFactors []int64 `db:"spreading_factors"`
}

// GatewayProfile defines a gateway-profile.
type GatewayProfile struct {
	ID            uuid.UUID      `db:"gateway_profile_id"`
	CreatedAt     time.Time      `db:"created_at"`
	UpdatedAt     time.Time      `db:"updated_at"`
	Channels      []int64        `db:"channels"`
	StatsInterval time.Duration  `db:"stats_interval"`
	ExtraChannels []ExtraChannel `db:"-"`
}

// GetVersion returns the gateway-profile version.
func (p GatewayProfile) GetVersion() string {
	return fmt.Sprintf("%s-r%d", p.ID, p.UpdatedAt.Unix())
}

// CreateGatewayProfile creates the given gateway-profile.
// As this will execute multiple SQL statements, it is recommended to perform
// this within a transaction.
func CreateGatewayProfile(ctx context.Context, db sqlx.Execer, c *GatewayProfile) error {
	now := time.Now()
	c.CreatedAt = now
	c.UpdatedAt = now

	if c.ID == uuid.Nil {
		var err error
		c.ID, err = uuid.NewV4()
		if err != nil {
			return errors.Wrap(err, "new uuid v4 error")
		}
	}

	_, err := db.Exec(`
		insert into gateway_profile (
			gateway_profile_id,
			created_at,
			updated_at,
			channels,
			stats_interval
		) values ($1, $2, $3, $4, $5)`,
		c.ID,
		c.CreatedAt,
		c.UpdatedAt,
		pq.Array(c.Channels),
		c.StatsInterval,
	)
	if err != nil {
		return handlePSQLError(err, "insert error")
	}

	for _, ec := range c.ExtraChannels {
		_, err := db.Exec(`
			insert into gateway_profile_extra_channel (
				gateway_profile_id,
				modulation,
				frequency,
				bandwidth,
				bitrate,
				spreading_factors
			) values ($1, $2, $3, $4, $5, $6)`,
			c.ID,
			ec.Modulation,
			ec.Frequency,
			ec.Bandwidth,
			ec.Bitrate,
			pq.Array(ec.SpreadingFactors),
		)
		if err != nil {
			return handlePSQLError(err, "insert error")
		}
	}

	log.WithFields(log.Fields{
		"id":     c.ID,
		"ctx_id": ctx.Value(logging.ContextIDKey),
	}).Info("gateway-profile created")

	return nil
}

// GetGatewayProfile returns the gateway-profile matching the
// given ID.
func GetGatewayProfile(ctx context.Context, db sqlx.Queryer, id uuid.UUID) (GatewayProfile, error) {
	var c GatewayProfile
	err := db.QueryRowx(`
		select
			gateway_profile_id,
			created_at,
			updated_at,
			channels,
			stats_interval
		from gateway_profile
		where
			gateway_profile_id = $1`,
		id,
	).Scan(
		&c.ID,
		&c.CreatedAt,
		&c.UpdatedAt,
		pq.Array(&c.Channels),
		&c.StatsInterval,
	)
	if err != nil {
		return c, handlePSQLError(err, "select error")
	}

	rows, err := db.Query(`
		select
			modulation,
			frequency,
			bandwidth,
			bitrate,
			spreading_factors
		from gateway_profile_extra_channel
		where
			gateway_profile_id = $1
		order by id`,
		id,
	)
	if err != nil {
		return c, handlePSQLError(err, "select error")
	}
	defer rows.Close()

	for rows.Next() {
		var ec ExtraChannel
		err := rows.Scan(
			&ec.Modulation,
			&ec.Frequency,
			&ec.Bandwidth,
			&ec.Bitrate,
			pq.Array(&ec.SpreadingFactors),
		)
		if err != nil {
			return c, handlePSQLError(err, "select error")
		}
		c.ExtraChannels = append(c.ExtraChannels, ec)
	}

	return c, nil
}

// UpdateGatewayProfile updates the given gateway-profile.
// As this will execute multiple SQL statements, it is recommended to perform
// this within a transaction.
func UpdateGatewayProfile(ctx context.Context, db sqlx.Execer, c *GatewayProfile) error {
	c.UpdatedAt = time.Now()
	res, err := db.Exec(`
		update gateway_profile
		set
			updated_at = $2,
			channels = $3,
			stats_interval = $4
		where
			gateway_profile_id = $1`,
		c.ID,
		c.UpdatedAt,
		pq.Array(c.Channels),
		c.StatsInterval,
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

	// This could be optimized by creating a diff of the actual extra channels
	// and the wanted. As it is not likely that this data changes really often
	// the 'simple' solution of re-creating all the extra channels has been
	// implemented.
	_, err = db.Exec(`
		delete from gateway_profile_extra_channel
		where
			gateway_profile_id = $1`,
		c.ID,
	)
	if err != nil {
		return handlePSQLError(err, "delete error")
	}
	for _, ec := range c.ExtraChannels {
		_, err := db.Exec(`
			insert into gateway_profile_extra_channel (
				gateway_profile_id,
				modulation,
				frequency,
				bandwidth,
				bitrate,
				spreading_factors
			) values ($1, $2, $3, $4, $5, $6)`,
			c.ID,
			ec.Modulation,
			ec.Frequency,
			ec.Bandwidth,
			ec.Bitrate,
			pq.Array(ec.SpreadingFactors),
		)
		if err != nil {
			return handlePSQLError(err, "insert error")
		}
	}

	log.WithFields(log.Fields{
		"id":     c.ID,
		"ctx_id": ctx.Value(logging.ContextIDKey),
	}).Info("gateway-profile updated")

	return nil
}

// DeleteGatewayProfile deletes the gateway-profile matching the
// given ID.
func DeleteGatewayProfile(ctx context.Context, db sqlx.Execer, id uuid.UUID) error {
	res, err := db.Exec(`
		delete from gateway_profile
		where
			gateway_profile_id = $1`,
		id,
	)
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

	log.WithFields(log.Fields{
		"id":     id,
		"ctx_id": ctx.Value(logging.ContextIDKey),
	}).Info("gateway-profile deleted")

	return nil
}
