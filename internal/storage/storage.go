package storage

import (
	"crypto/tls"
	"embed"
	"fmt"
	"net/http"
	"time"

	"github.com/go-redis/redis/v7"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/httpfs"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-network-server/internal/config"
	"github.com/brocaar/chirpstack-network-server/internal/migrations/code"
	codemig "github.com/brocaar/chirpstack-network-server/internal/storage/migrations/code"
)

// Migrations
//go:embed migrations/*
var migrations embed.FS

// deviceSessionTTL holds the device-session TTL.
var deviceSessionTTL time.Duration

// schedulerInterval holds the interval in which the Class-B and -C
// scheduler runs.
var schedulerInterval time.Duration

// keyPrefix for Redis.
var keyPrefix string

// Setup configures the storage backend.
func Setup(c config.Config) error {
	log.Info("storage: setting up storage module")

	deviceSessionTTL = c.NetworkServer.DeviceSessionTTL
	schedulerInterval = c.NetworkServer.Scheduler.SchedulerInterval
	keyPrefix = c.Redis.KeyPrefix

	log.Info("storage: setting up Redis client")
	if len(c.Redis.Servers) == 0 {
		return errors.New("at least one redis server must be configured")
	}

	var tlsConfig *tls.Config
	if c.Redis.TLSEnabled {
		tlsConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	if c.Redis.Cluster {
		redisClient = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:     c.Redis.Servers,
			PoolSize:  c.Redis.PoolSize,
			Password:  c.Redis.Password,
			TLSConfig: tlsConfig,
		})
	} else if c.Redis.MasterName != "" {
		redisClient = redis.NewFailoverClient(&redis.FailoverOptions{
			MasterName:       c.Redis.MasterName,
			SentinelAddrs:    c.Redis.Servers,
			SentinelPassword: c.Redis.Password,
			DB:               c.Redis.Database,
			PoolSize:         c.Redis.PoolSize,
			TLSConfig:        tlsConfig,
		})
	} else {
		redisClient = redis.NewClient(&redis.Options{
			Addr:      c.Redis.Servers[0],
			DB:        c.Redis.Database,
			Password:  c.Redis.Password,
			PoolSize:  c.Redis.PoolSize,
			TLSConfig: tlsConfig,
		})
	}

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

	if err := code.Migrate(db.DB, "migrate_to_cluster_keys", func(db sqlx.Ext) error {
		return codemig.MigrateToClusterKeys(RedisClient())
	}); err != nil {
		return err
	}

	if err := code.Migrate(db.DB, "migrate_to_golang_migrate", func(db sqlx.Ext) error {
		return codemig.MigrateToGolangMigrate(db)
	}); err != nil {
		return err
	}

	if c.PostgreSQL.Automigrate {
		if err := MigrateUp(d); err != nil {
			return err
		}
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

// MigrateUp configure postgres migration up
func MigrateUp(db *sqlx.DB) error {
	log.Info("storage: applying PostgreSQL data migrations")

	driver, err := postgres.WithInstance(db.DB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("storage: migrate postgres driver error: %w", err)
	}

	src, err := httpfs.New(http.FS(migrations), "migrations")
	if err != nil {
		return fmt.Errorf("new httpfs error: %w", err)
	}

	m, err := migrate.NewWithInstance("httpfs", src, "postgres", driver)
	if err != nil {
		return fmt.Errorf("storage: new migrate instance error: %w", err)
	}

	oldVersion, _, _ := m.Version()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("storage: migrate up error: %w", err)
	}

	newVersion, _, _ := m.Version()

	if oldVersion != newVersion {
		log.WithFields(log.Fields{
			"from_version": oldVersion,
			"to_version":   newVersion,
		}).Info("storage: PostgreSQL data migrations applied")
	}

	return nil
}

// MigrateDown configure postgres migration down
func MigrateDown(db *sqlx.DB) error {
	log.Info("storage: reverting PostgreSQL data migrations")

	driver, err := postgres.WithInstance(db.DB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("storage: migrate postgres driver error: %w", err)
	}

	src, err := httpfs.New(http.FS(migrations), "migrations")
	if err != nil {
		return fmt.Errorf("new httpfs error: %w", err)
	}

	m, err := migrate.NewWithInstance("httpfs", src, "postgres", driver)
	if err != nil {
		return fmt.Errorf("storage: new migrate instance error: %w", err)
	}

	oldVersion, _, _ := m.Version()

	if err := m.Down(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("storage: migrate down error: %w", err)
	}

	newVersion, _, _ := m.Version()

	if oldVersion != newVersion {
		log.WithFields(log.Fields{
			"from_version": oldVersion,
			"to_version":   newVersion,
		}).Info("storage: reverted PostgreSQL data migrations applied")
	}

	return nil
}

// GetRedisKey returns the Redis key given a template and parameters.
func GetRedisKey(tmpl string, params ...interface{}) string {
	return keyPrefix + fmt.Sprintf(tmpl, params...)
}
