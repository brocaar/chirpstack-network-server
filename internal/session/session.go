package session

import (
	"bytes"
	"crypto/rand"
	"encoding/gob"
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/lorawan"
)

const (
	nodeSessionKeyTempl = "node_session_%s"
)

// GetRandomDevAddr returns a random free DevAddr. Note that the 7 MSB will be
// set to the NwkID (based on the configured NetID).
// TODO: handle collission with retry?
func GetRandomDevAddr(p *redis.Pool, netID lorawan.NetID) (lorawan.DevAddr, error) {
	var d lorawan.DevAddr
	b := make([]byte, len(d))
	if _, err := rand.Read(b); err != nil {
		return d, fmt.Errorf("could not read from random reader: %s", err)
	}
	copy(d[:], b)
	d[0] = d[0] & 1                    // zero out 7 msb
	d[0] = d[0] ^ (netID.NwkID() << 1) // set 7 msb to NwkID

	c := p.Get()
	defer c.Close()

	key := "node_session_" + d.String()
	val, err := redis.Int(c.Do("EXISTS", key))
	if err != nil {
		return lorawan.DevAddr{}, fmt.Errorf("test DevAddr %s exist error: %s", d, err)
	}
	if val == 1 {
		return lorawan.DevAddr{}, fmt.Errorf("DevAddr %s already exists", d)
	}
	return d, nil
}

// ValidateAndGetFullFCntUp validates if the given fCntUp is valid
// and returns the full 32 bit frame-counter.
// Note that the LoRaWAN packet only contains the 16 LSB, so in order
// to validate the MIC, the full 32 bit frame-counter needs to be set.
// After a succesful validation of the FCntUp and the MIC, don't forget
// to synchronize the Node FCntUp with the packet FCnt.
func ValidateAndGetFullFCntUp(n NodeSession, fCntUp uint32) (uint32, bool) {
	// we need to compare the difference of the 16 LSB
	gap := uint32(uint16(fCntUp) - uint16(n.FCntUp%65536))
	if gap < common.Band.MaxFCntGap {
		return n.FCntUp + gap, true
	}
	return 0, false
}

// CreateNodeSession does the same as saveNodeSession except that it does not
// overwrite an exisitng record.
func CreateNodeSession(p *redis.Pool, s NodeSession) error {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(s); err != nil {
		return fmt.Errorf("encode node-session for node %s error: %s", s.DevEUI, err)
	}

	c := p.Get()
	defer c.Close()

	exp := int64(common.NodeSessionTTL) / int64(time.Millisecond)

	if _, err := redis.String(c.Do("SET", fmt.Sprintf(nodeSessionKeyTempl, s.DevAddr), buf.Bytes(), "NX", "PX", exp)); err != nil {
		if err == redis.ErrNil {
			return fmt.Errorf("create node-session %s for node %s error: DevAddr already in use", s.DevAddr, s.DevEUI)
		} else {
			return fmt.Errorf("create node-session %s for node %s error: %s", s.DevAddr, s.DevEUI, err)
		}
	}
	// DevEUI -> DevAddr pointer
	if _, err := redis.String(c.Do("PSETEX", fmt.Sprintf(nodeSessionKeyTempl, s.DevEUI), exp, s.DevAddr.String())); err != nil {
		return fmt.Errorf("create pointer node %s -> DevAddr %s error: %s", s.DevEUI, s.DevAddr, err)
	}

	log.WithField("dev_addr", s.DevAddr).Info("node-session created")
	return nil
}

// SaveNodeSession saves the node session. Note that the session will automatically
// expire after NodeSessionTTL.
func SaveNodeSession(p *redis.Pool, s NodeSession) error {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(s); err != nil {
		return fmt.Errorf("encode node-session for node %s error: %s", s.DevEUI, err)
	}

	c := p.Get()
	defer c.Close()

	exp := int64(common.NodeSessionTTL) / int64(time.Millisecond)

	if _, err := redis.String(c.Do("PSETEX", fmt.Sprintf(nodeSessionKeyTempl, s.DevAddr), exp, buf.Bytes())); err != nil {
		return fmt.Errorf("save node-session %s for node %s error: %s", s.DevAddr, s.DevEUI, err)
	}
	// DevEUI -> DevAddr pointer
	if _, err := redis.String(c.Do("PSETEX", fmt.Sprintf(nodeSessionKeyTempl, s.DevEUI), exp, s.DevAddr.String())); err != nil {
		return fmt.Errorf("create pointer node %s -> DevAddr %s error: %s", s.DevEUI, s.DevAddr, err)
	}

	log.WithField("dev_addr", s.DevAddr).Info("node-session saved")
	return nil
}

// GetNodeSession returns the NodeSession for the given DevAddr.
func GetNodeSession(p *redis.Pool, devAddr lorawan.DevAddr) (NodeSession, error) {
	var ns NodeSession

	c := p.Get()
	defer c.Close()

	val, err := redis.Bytes(c.Do("GET", fmt.Sprintf(nodeSessionKeyTempl, devAddr)))
	if err != nil {
		return ns, fmt.Errorf("get node-session for DevAddr %s error: %s", devAddr, err)
	}

	err = gob.NewDecoder(bytes.NewReader(val)).Decode(&ns)
	if err != nil {
		return ns, fmt.Errorf("decode node-session %s error: %s", devAddr, err)
	}

	return ns, nil
}

// GetNodeSessionByDevEUI returns the NodeSession for the given DevEUI.
func GetNodeSessionByDevEUI(p *redis.Pool, devEUI lorawan.EUI64) (NodeSession, error) {
	var ns NodeSession

	c := p.Get()
	defer c.Close()

	devAddr, err := redis.String(c.Do("GET", fmt.Sprintf(nodeSessionKeyTempl, devEUI)))
	if err != nil {
		return ns, fmt.Errorf("get node-session pointer for node %s error: %s", devEUI, err)
	}

	val, err := redis.Bytes(c.Do("GET", fmt.Sprintf(nodeSessionKeyTempl, devAddr)))
	if err != nil {
		return ns, fmt.Errorf("get node-session for DevAddr %s error: %s", devAddr, err)
	}

	err = gob.NewDecoder(bytes.NewReader(val)).Decode(&ns)
	if err != nil {
		return ns, fmt.Errorf("decode node-session %s error: %s", devAddr, err)
	}

	return ns, nil
}

// DeleteNodeSession deletes the NodeSession matching the given DevAddr.
func DeleteNodeSession(p *redis.Pool, devAddr lorawan.DevAddr) error {
	c := p.Get()
	defer c.Close()

	val, err := redis.Int(c.Do("DEL", fmt.Sprintf(nodeSessionKeyTempl, devAddr)))
	if err != nil {
		return fmt.Errorf("delete node-session %s error: %s", devAddr, err)
	}
	if val == 0 {
		return fmt.Errorf("node-session %s does not exist", devAddr)
	}
	log.WithField("dev_addr", devAddr).Info("node-session deleted")
	return nil
}
