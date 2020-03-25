package storage

import (
	"time"

	"github.com/go-redis/redis/v7"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	migrate "github.com/rubenv/sql-migrate"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-network-server/internal/config"
	"github.com/brocaar/chirpstack-network-server/internal/migrations"
)

// deviceSessionTTL holds the device-session TTL.
var deviceSessionTTL time.Duration

// schedulerInterval holds the interval in which the Class-B and -C
// scheduler runs.
var schedulerInterval time.Duration

// Setup configures the storage backend.
func Setup(c config.Config) error {
	log.Info("storage: setting up storage module")

	deviceSessionTTL = c.NetworkServer.DeviceSessionTTL
	schedulerInterval = c.NetworkServer.Scheduler.SchedulerInterval

	log.Info("storage: setting up Redis client")
	opt, err := redis.ParseURL(c.Redis.URL)
	if err != nil {
		return errors.Wrap(err, "parse redis url error")
	}
	opt.PoolSize = c.Redis.PoolSize
	redisClient = redis.NewClient(opt)

	log.Info("storage: connecting to PostgreSQL")
	d, err := sqlx.Open("postgres", c.PostgreSQL.DSN)
	if err != nil {
		return errors.Wrap(err, "storage: PostgreSQL connection error")
	}
	d.SetMaxOpenConns(c.PostgreSQL.MaxOpenConnections)
	d.SetMaxIdleConns(c.PostgreSQL.MaxIdleConnections)
	for {
		if err := d.Ping(); err != nil {
			log.WithError(err).Warning("storage: ping PostgreSQL database error, will retry in 2s")
			time.Sleep(2 * time.Second)
		} else {
			break
		}
	}

	db = &DBLogger{d}

	if c.PostgreSQL.Automigrate {
		log.Info("storage: applying PostgreSQL data migrations")
		m := &migrate.AssetMigrationSource{
			Asset:    migrations.Asset,
			AssetDir: migrations.AssetDir,
			Dir:      "",
		}
		n, err := migrate.Exec(db.DB.DB, "postgres", m, migrate.Up)
		if err != nil {
			return errors.Wrap(err, "storage: applying PostgreSQL data migrations error")
		}
		log.WithField("count", n).Info("storage: PostgreSQL data migrations applied")
	}

	return nil
}

// Transaction wraps the given function in a transaction. In case the given
// functions returns an error, the transaction will be rolled back.
func Transaction(f func(tx sqlx.Ext) error) error {
	tx, err := db.Beginx()
	if err != nil {
		return errors.Wrap(err, "storage: begin transaction error")
	}

	err = f(tx)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return errors.Wrap(rbErr, "storage: transaction rollback error")
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "storage: transaction commit error")
	}
	return nil
}
