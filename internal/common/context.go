package common

import (
	"github.com/garyburd/redigo/redis"

	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/api/nc"
	"github.com/brocaar/loraserver/internal/backend"
	"github.com/brocaar/lorawan"
)

// Context holds the context of a loraserver instance
// (backends, db connections etc..)
type Context struct {
	RedisPool   *redis.Pool
	Gateway     backend.Gateway
	NetID       lorawan.NetID
	Application as.ApplicationServerClient
	Controller  nc.NetworkControllerClient
}
