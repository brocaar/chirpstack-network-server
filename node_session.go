package loraserver

import (
	"bytes"
	"encoding/gob"
	"time"

	"github.com/brocaar/lorawan"
	"github.com/garyburd/redigo/redis"
	"github.com/jmoiron/sqlx"
)

// NodeSession related constants
const (
	NodeSessionTTL = time.Hour * 24 * 5
)

// NodeSession contains the informatio of a node-session (an activated node).
type NodeSession struct {
	DevAddr  lorawan.DevAddr   `db:"dev_addr"`
	DevEUI   lorawan.EUI64     `db:"dev_eui"`
	AppSKey  lorawan.AES128Key `db:"app_s_key"`
	NwkSKey  lorawan.AES128Key `db:"nwk_s_key"`
	FCntUp   uint32            `db:"fcnt_up"`   // the next expected value
	FCntDown uint32            `db:"fcnt_down"` // the next expected value

	AppEUI lorawan.EUI64     `db:"app_eui"`
	AppKey lorawan.AES128Key `db:"app_key"`
}

// ValidateAndGetFullFCntUp validates if the given fCntUp is valid
// and returns the full 32 bit frame-counter.
// Note that the LoRaWAN packet only contains the 16 LSB, so in order
// to validate the MIC, the full 32 bit frame-counter needs to be set.
// After a succesful validation of the FCntUP and the MIC, don't forget
// to increment the Node FCntUp by 1.
func (n NodeSession) ValidateAndGetFullFCntUp(fCntUp uint32) (uint32, bool) {
	// we need to compare the difference of the 16 LSB
	gap := uint32(uint16(fCntUp) - uint16(n.FCntUp%65536))
	if gap < lorawan.MaxFCntGap {
		return n.FCntUp + gap, true
	}
	return 0, false
}

// CreateNodeSession does the same as SaveNodeSession except that it does not
// overwrite an exisitng record.
func CreateNodeSession(p *redis.Pool, s NodeSession) (bool, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(s); err != nil {
		return false, err
	}

	c := p.Get()
	defer c.Close()

	key := "node_session_" + s.DevAddr.String()
	_, err := redis.String(c.Do("SET", key, buf.Bytes(), "NX", "PX", int64(NodeSessionTTL)/int64(time.Millisecond)))
	if err != nil {
		if err == redis.ErrNil {
			return false, nil
		}
		return false, err
	}
	return true, nil
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

// NewNodeSessionsFromABP creates new node sessions for the stored
// NodeABP records. Note that this will not overwrite existing
// node sessions.
func NewNodeSessionsFromABP(db *sqlx.DB, p *redis.Pool) error {
	rows, err := db.Queryx(`
		select
			node_abp.dev_addr,
			node_abp.dev_eui,
			node_abp.app_s_key,
			node_abp.nwk_s_key,
			node_abp.fcnt_up,
			node_abp.fcnt_down,
			node.app_eui,
			node.app_key
		from
			node_abp
		inner join node on
			node_abp.dev_eui = node.dev_eui
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var ns NodeSession
	for rows.Next() {
		if err := rows.StructScan(&ns); err != nil {
			return err
		}
		if _, err := CreateNodeSession(p, ns); err != nil {
			return err
		}
	}
	return nil
}
