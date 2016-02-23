package loraserver

import (
	"bytes"
	"encoding/gob"
	"time"

	"github.com/brocaar/lorawan"
	"github.com/garyburd/redigo/redis"
)

// NodeSession related constants
const (
	NodeSessionTTL = time.Hour * 24 * 5
)

// NodeSession contains the informatio of a node-session (an activated node).
type NodeSession struct {
	DevAddr  lorawan.DevAddr
	DevEUI   lorawan.EUI64
	AppSKey  lorawan.AES128Key
	NwkSKey  lorawan.AES128Key
	FCntUp   uint32
	FCntDown uint32

	AppEUI lorawan.EUI64
	AppKey lorawan.AES128Key
}

// SaveNodeSession saves the node session. Note that the session will automatically
// expire after NodeSessionTTL.
func SaveNodeSession(p *redis.Pool, s NodeSession) error {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(s); err != nil {
		return err
	}

	c := p.Get()
	defer c.Close()

	key := "node_session_" + s.DevAddr.String()
	_, err := c.Do("PSETEX", key, int64(NodeSessionTTL)/int64(time.Millisecond), buf.Bytes())
	return err
}

// GetNodeSession returns the NodeSession for the given DevAddr.
func GetNodeSession(p *redis.Pool, devAddr lorawan.DevAddr) (NodeSession, error) {
	var ns NodeSession

	c := p.Get()
	defer c.Close()

	key := "node_session_" + devAddr.String()
	val, err := redis.Bytes(c.Do("GET", key))
	if err != nil {
		return ns, err
	}

	err = gob.NewDecoder(bytes.NewReader(val)).Decode(&ns)
	return ns, err
}
