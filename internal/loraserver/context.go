package loraserver

import (
	"github.com/brocaar/loraserver/internal/backend"
	"github.com/brocaar/lorawan"
	"github.com/garyburd/redigo/redis"
	"github.com/jmoiron/sqlx"
)

// Context holds the context of a loraserver instance
// (backends, db connections etc..)
type Context struct {
	DB          *sqlx.DB
	RedisPool   *redis.Pool
	Gateway     backend.Gateway
	Application backend.Application
	Controller  backend.NetworkController
	NetID       lorawan.NetID
}
