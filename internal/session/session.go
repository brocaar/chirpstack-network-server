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

// TODO: implement migration tool to migrate from old to new data structure!
const (
	devAddrKeyTempl     = "lora:ns:devaddr:%s" // contains a set of DevEUIs using this DevAddr
	nodeSessionKeyTempl = "lora:ns:session:%s" // contains the session of a DevEUI
)

// GetRandomDevAddr returns a random free DevAddr. Note that the 7 MSB will be
// set to the NwkID (based on the configured NetID).
func GetRandomDevAddr(p *redis.Pool, netID lorawan.NetID) (lorawan.DevAddr, error) {
	var d lorawan.DevAddr
	b := make([]byte, len(d))
	if _, err := rand.Read(b); err != nil {
		return d, fmt.Errorf("could not read from random reader: %s", err)
	}
	copy(d[:], b)
	d[0] = d[0] & 1                    // zero out 7 msb
	d[0] = d[0] ^ (netID.NwkID() << 1) // set 7 msb to NwkID

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

// NodeSessionExists returns a bool indicating if a node session exist.
func NodeSessionExists(p *redis.Pool, devEUI lorawan.EUI64) (bool, error) {
	c := p.Get()
	defer c.Close()

	r, err := redis.Int(c.Do("EXISTS", fmt.Sprintf(nodeSessionKeyTempl, devEUI)))
	if err != nil {
		return false, fmt.Errorf("get node-session key exists error: %s", err)
	}
	if r == 1 {
		return true, nil
	}
	return false, nil
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

	c.Send("MULTI")
	c.Send("PSETEX", fmt.Sprintf(nodeSessionKeyTempl, s.DevEUI), exp, buf.Bytes())
	c.Send("SADD", fmt.Sprintf(devAddrKeyTempl, s.DevAddr), s.DevEUI[:])
	c.Send("PEXPIRE", fmt.Sprintf(devAddrKeyTempl, s.DevAddr), exp)

	if _, err := c.Do("EXEC"); err != nil {
		return fmt.Errorf("save node %s error: %s", s.DevEUI, err)
	}

	log.WithFields(log.Fields{
		"dev_eui":  s.DevEUI,
		"dev_addr": s.DevAddr,
	}).Info("node-session saved")
	return nil
}

// GetNodeSession returns the NodeSession for the given DevAddr.
// TODO: remove this method
func GetNodeSession(p *redis.Pool, devAddr lorawan.DevAddr) (NodeSession, error) {
	var ns NodeSession

	c := p.Get()
	defer c.Close()

	// get DevEUI set for DevAddr
	val, err := redis.ByteSlices(c.Do("SMEMBERS", fmt.Sprintf(devAddrKeyTempl, devAddr)))
	if err != nil {
		return ns, fmt.Errorf("get DevEUI set for DevAddr %s error: %s", devAddr, err)
	}

	if len(val) == 0 {
		return ns, fmt.Errorf("node-session for %s does not exist", devAddr)
	}

	var devEUI lorawan.EUI64
	copy(devEUI[:], val[0])

	return GetNodeSessionByDevEUI(p, devEUI)
}

// GetNodeSessionsForDevAddr returns a slice of NodeSession items using the
// given DevAddr. When no NodeSession is using the given DevAddr, this returns
// an empty slice without error.
func GetNodeSessionsForDevAddr(p *redis.Pool, devAddr lorawan.DevAddr) ([]NodeSession, error) {
	var items []NodeSession

	c := p.Get()
	defer c.Close()

	devEUIs, err := redis.ByteSlices(c.Do("SMEMBERS", fmt.Sprintf(devAddrKeyTempl, devAddr)))
	if err != nil {
		return nil, fmt.Errorf("get DevEUI set for DevAddr %s error: %s", devAddr, err)
	}

	for _, b := range devEUIs {
		var devEUI lorawan.EUI64
		copy(devEUI[:], b)

		ns, err := GetNodeSessionByDevEUI(p, devEUI)
		if err != nil {
			// TODO: in case not found, remove the DevEUI from the list
			log.WithFields(log.Fields{
				"dev_addr": devAddr,
				"dev_eui":  devEUI,
			}).Warning("get node-sessions for devaddr error: %s", err)
		}
		items = append(items, ns)
	}

	return items, nil
}

// GetNodeSessionByDevEUI returns the NodeSession for the given DevEUI.
func GetNodeSessionByDevEUI(p *redis.Pool, devEUI lorawan.EUI64) (NodeSession, error) {
	var ns NodeSession

	c := p.Get()
	defer c.Close()

	val, err := redis.Bytes(c.Do("GET", fmt.Sprintf(nodeSessionKeyTempl, devEUI)))
	if err != nil {
		return ns, fmt.Errorf("get node-session %s error: %s", devEUI, err)
	}

	err = gob.NewDecoder(bytes.NewReader(val)).Decode(&ns)
	if err != nil {
		return ns, fmt.Errorf("decode node-session %s error: %s", devEUI, err)
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
