package storage

import (
	"fmt"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/jmoiron/sqlx"
	// register postgresql driver
	_ "github.com/lib/pq"
)

const (
	redisMaxIdle        = 3
	redisIdleTimeoutSec = 240
)

// OpenDatabase opens the database and performs a ping to make sure the
// database is up.
func OpenDatabase(dsn string) (*sqlx.DB, error) {
	db, err := sqlx.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("database connection error: %s", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database error: %s", err)
	}
	return db, nil
}

// NewRedisPool returns a new Redis connection pool.
func NewRedisPool(redisURL string) *redis.Pool {
	return &redis.Pool{
		MaxIdle:     redisMaxIdle,
		IdleTimeout: redisIdleTimeoutSec * time.Second,
		Dial: func() (redis.Conn, error) {
			c, err := redis.DialURL(redisURL)
			if err != nil {
				return nil, fmt.Errorf("redis connection error: %s", err)
			}
			return c, err
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			if err != nil {
				return fmt.Errorf("ping redis error: %s", err)
			}
			return nil
		},
	}
}
