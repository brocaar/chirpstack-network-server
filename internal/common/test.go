package common

import (
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
	"github.com/jmoiron/sqlx"
	migrate "github.com/rubenv/sql-migrate"

	"github.com/brocaar/loraserver/internal/loraserver/migrations"
	"github.com/brocaar/lorawan/band"
)

func init() {
	var err error
	log.SetLevel(log.ErrorLevel)

	Band, err = band.GetConfig(band.EU_863_870)
	if err != nil {
		panic(err)
	}
}

// TestConfig contains the test configuration.
type TestConfig struct {
	RedisURL    string
	PostgresDSN string
}

// GetTestConfig returns the test configuration.
func GetTestConfig() *TestConfig {
	c := &TestConfig{
		RedisURL: "redis://localhost:6379",
	}

	if v := os.Getenv("TEST_REDIS_URL"); v != "" {
		c.RedisURL = v
	}

	if v := os.Getenv("TEST_POSTGRES_DSN"); v != "" {
		c.PostgresDSN = v
	}

	return c
}

// MustFlushRedis flushes the Redis storage.
func MustFlushRedis(p *redis.Pool) {
	c := p.Get()
	defer c.Close()
	if _, err := c.Do("FLUSHALL"); err != nil {
		log.Fatal(err)
	}
}

// MustResetDB re-applies all database migrations.
func MustResetDB(db *sqlx.DB) {
	m := &migrate.AssetMigrationSource{
		Asset:    migrations.Asset,
		AssetDir: migrations.AssetDir,
		Dir:      "",
	}
	if _, err := migrate.Exec(db.DB, "postgres", m, migrate.Down); err != nil {
		log.Fatal(err)
	}
	if _, err := migrate.Exec(db.DB, "postgres", m, migrate.Up); err != nil {
		log.Fatal(err)
	}
}
