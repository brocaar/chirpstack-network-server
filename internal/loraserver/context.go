package loraserver

import (
	"github.com/brocaar/lorawan"
	"github.com/garyburd/redigo/redis"
)

// Context holds the context of a loraserver instance
// (backends, db connections etc..)
type Context struct {
	NodeManager    NodeManager
	NodeAppManager NodeApplicationsManager
	RedisPool      *redis.Pool
	Gateway        GatewayBackend
	Application    ApplicationBackend
	NetID          lorawan.NetID
}
