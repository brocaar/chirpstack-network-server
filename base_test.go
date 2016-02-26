package loraserver

import (
	"io/ioutil"
	golog "log"
	"os"

	"github.com/DavidHuie/gomigrate"
	log "github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
	"github.com/jmoiron/sqlx"
)

func init() {
	log.SetLevel(log.ErrorLevel)
	golog.SetOutput(ioutil.Discard)
}

type config struct {
	RedisURL    string
	PostgresDSN string
}

func getConfig() *config {
	c := &config{
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

func mustFlushRedis(p *redis.Pool) {
	c := p.Get()
	defer c.Close()
	if _, err := c.Do("FLUSHALL"); err != nil {
		log.Fatal(err)
	}
}

func mustResetDB(db *sqlx.DB) {
	m, err := gomigrate.NewMigrator(db.DB, gomigrate.Postgres{}, "./migrations")
	if err != nil {
		log.Fatal(err)
	}
	if err := m.RollbackAll(); err != nil {
		log.Fatal(err)
	}
	if err := m.Migrate(); err != nil {
		log.Fatal(err)
	}
}
