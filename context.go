package loraserver

import (
	"github.com/brocaar/lorawan"
	"github.com/garyburd/redigo/redis"
	"github.com/jmoiron/sqlx"
)

// Context holds the context of a loraserver instance
// (backends, db connections etc..)
type Context struct {
	DB          *sqlx.DB
	RedisPool   *redis.Pool
	Gateway     GatewayBackend
	Application ApplicationBackend
	NetID       lorawan.NetID
}
