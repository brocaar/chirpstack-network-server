package storage

import (
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/pkg/errors"
	"github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/lorawan"
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
	GatewayProfileID string         `db:"gateway_profile_id"`
	CreatedAt        time.Time      `db:"created_at"`
	UpdatedAt        time.Time      `db:"updated_at"`
	Channels         []int64        `db:"channels"`
	ExtraChannels    []ExtraChannel `db:"-"`
}

// GetVersion returns the gateway-profile version.
func (p GatewayProfile) GetVersion() string {
	return p.UpdatedAt.UTC().Format(time.RFC3339Nano)
}

// CreateGatewayProfile creates the given gateway-profile.
// As this will execute multiple SQL statements, it is recommended to perform
// this within a transaction.
func CreateGatewayProfile(db sqlx.Execer, c *GatewayProfile) error {
	now := time.Now()
	c.CreatedAt = now
	c.UpdatedAt = now

	if c.GatewayProfileID == "" {
		c.GatewayProfileID = uuid.NewV4().String()
	}

	_, err := db.Exec(`
		insert into gateway_profile (
			gateway_profile_id,
			created_at,
			updated_at,
			channels
		) values ($1, $2, $3, $4)`,
		c.GatewayProfileID,
		c.CreatedAt,
		c.UpdatedAt,
		pq.Array(c.Channels),
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
			c.GatewayProfileID,
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
		"gateway_profile_id": c.GatewayProfileID,
	}).Info("gateway-profile created")

	return nil
}

// GetGatewayProfile returns the gateway-profile matching the
// given ID.
func GetGatewayProfile(db sqlx.Queryer, id string) (GatewayProfile, error) {
	var c GatewayProfile
	err := db.QueryRowx(`
		select
			gateway_profile_id,
			created_at,
			updated_at,
			channels
		from gateway_profile
		where
			gateway_profile_id = $1`,
		id,
	).Scan(
		&c.GatewayProfileID,
		&c.CreatedAt,
		&c.UpdatedAt,
		pq.Array(&c.Channels),
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
func UpdateGatewayProfile(db sqlx.Execer, c *GatewayProfile) error {
	c.UpdatedAt = time.Now()
	res, err := db.Exec(`
		update gateway_profile
		set
			updated_at = $2,
			channels = $3
		where
			gateway_profile_id = $1`,
		c.GatewayProfileID,
		c.UpdatedAt,
		pq.Array(c.Channels),
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
		c.GatewayProfileID,
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
			c.GatewayProfileID,
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
		"gateway_profile_id": c.GatewayProfileID,
	}).Info("gateway-profile updated")

	return nil
}

// DeleteGatewayProfile deletes the gateway-profile matching the
// given ID.
func DeleteGatewayProfile(db sqlx.Execer, id string) error {
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
		"gateway_profile_id": id,
	}).Info("gateway-profile deleted")

	return nil
}

// MigratedGatewayProfile contains the channel-configuration to
// gateway-profile migration.
type MigratedGatewayProfile struct {
	GatewayProfileID string
	Gateways         []lorawan.EUI64
	Name             string
}

// MigrateChannelConfigurationToGatewayProfile migrates the channel configuration.
func MigrateChannelConfigurationToGatewayProfile(db sqlx.Ext) ([]MigratedGatewayProfile, error) {
	var out []MigratedGatewayProfile
	var configMigrate []struct {
		ID   int64  `db:"id"`
		Name string `db:"name"`
	}

	err := sqlx.Select(db, &configMigrate, `
		select
			id,
			name
		from channel_configuration`)
	if err != nil {
		return nil, handlePSQLError(err, "select error")
	}

	for _, cm := range configMigrate {
		gp, err := getChannelConfigurationToGatewayProfile(db, cm.ID)
		if err != nil {
			return nil, errors.Wrap(err, "get channel-configuration to gateway-profile error")
		}

		if err := CreateGatewayProfile(db, &gp); err != nil {
			return nil, errors.Wrap(err, "create gateway-profile error")
		}

		_, err = db.Exec(`
			update gateway
			set
				gateway_profile_id = $2,
				channel_configuration_id = null
			where
				channel_configuration_id = $1`,
			cm.ID,
			gp.GatewayProfileID,
		)
		if err != nil {
			return nil, handlePSQLError(err, "update error")
		}

		var gateways []lorawan.EUI64
		err = sqlx.Select(db, &gateways, `
			select
				mac
			from
				gateway
			where
				gateway_profile_id = $1`,
			gp.GatewayProfileID,
		)
		if err != nil {
			return nil, handlePSQLError(err, "select error")
		}

		out = append(out, MigratedGatewayProfile{
			Gateways:         gateways,
			GatewayProfileID: gp.GatewayProfileID,
			Name:             cm.Name,
		})
	}

	return out, nil
}

func getChannelConfigurationToGatewayProfile(db sqlx.Queryer, id int64) (GatewayProfile, error) {
	var gp GatewayProfile
	err := db.QueryRowx(`
		select
			channels
		from channel_configuration
		where
			id = $1`,
		id,
	).Scan(pq.Array(&gp.Channels))
	if err != nil {
		return gp, handlePSQLError(err, "select error")
	}

	rows, err := db.Query(`
		select
			modulation,
			frequency,
			bandwidth,
			bit_rate,
			spread_factors
		from extra_channel
		where
			channel_configuration_id = $1
		order by id`,
		id,
	)
	if err != nil {
		return gp, handlePSQLError(err, "select error")
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
			return gp, handlePSQLError(err, "scan error")
		}
		gp.ExtraChannels = append(gp.ExtraChannels, ec)
	}

	return gp, nil
}
