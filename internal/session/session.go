package session

import (
	"bytes"
	"crypto/rand"
	"encoding/gob"
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
	"github.com/pkg/errors"

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
		return d, errors.Wrap(err, "read random bytes error")
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
		return false, errors.Wrap(err, "get exists error")
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
		return errors.Wrap(err, "gob encode error")
	}

	c := p.Get()
	defer c.Close()
	exp := int64(common.NodeSessionTTL) / int64(time.Millisecond)

	c.Send("MULTI")
	c.Send("PSETEX", fmt.Sprintf(nodeSessionKeyTempl, s.DevEUI), exp, buf.Bytes())
	c.Send("SADD", fmt.Sprintf(devAddrKeyTempl, s.DevAddr), s.DevEUI[:])
	c.Send("PEXPIRE", fmt.Sprintf(devAddrKeyTempl, s.DevAddr), exp)

	if _, err := c.Do("EXEC"); err != nil {
		return errors.Wrap(err, "exec error")
	}

	log.WithFields(log.Fields{
		"dev_eui":  s.DevEUI,
		"dev_addr": s.DevAddr,
	}).Info("node-session saved")
	return nil
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
		if err == redis.ErrNil {
			return items, nil
		}
		return nil, errors.Wrap(err, "get members error")
	}

	for _, b := range devEUIs {
		var devEUI lorawan.EUI64
		copy(devEUI[:], b)

		ns, err := GetNodeSession(p, devEUI)
		if err != nil {
			// TODO: in case not found, remove the DevEUI from the list
			log.WithFields(log.Fields{
				"dev_addr": devAddr,
				"dev_eui":  devEUI,
			}).Warning("get node-sessions for dev_addr error: %s", err)
		}
		items = append(items, ns)
	}

	return items, nil
}

// GetNodeSessionForPHYPayload returns the node-session matching the given
// PHYPayload. This will fetch all node-sessions associated with the used
// DevAddr and based on FCnt and MIC decide which one to use.
func GetNodeSessionForPHYPayload(p *redis.Pool, phy lorawan.PHYPayload) (NodeSession, error) {
	// MACPayload must be of type *lorawan.MACPayload
	macPL, ok := phy.MACPayload.(*lorawan.MACPayload)
	if !ok {
		return NodeSession{}, fmt.Errorf("expected *lorawan.MACPayload, got: %T", phy.MACPayload)
	}
	originalFCnt := macPL.FHDR.FCnt

	sessions, err := GetNodeSessionsForDevAddr(p, macPL.FHDR.DevAddr)
	if err != nil {
		return NodeSession{}, err
	}

	for _, ns := range sessions {
		// reset to the original FCnt
		macPL.FHDR.FCnt = originalFCnt
		// get full FCnt
		fullFCnt, ok := ValidateAndGetFullFCntUp(ns, macPL.FHDR.FCnt)
		if !ok {
			// If RelaxFCnt is turned on, just trust the uplink FCnt
			// this is insecure, but has been requested by many people for
			// debugging purposes.
			// Note that we do not reset the FCntDown as this would reset the
			// downlink frame-counter on a re-transmit, which is not what we
			// want.
			if ns.RelaxFCnt {
				fullFCnt = macPL.FHDR.FCnt
				ns.FCntUp = macPL.FHDR.FCnt

				// validate if the mic is valid given the FCnt reset
				micOK, err := phy.ValidateMIC(ns.NwkSKey)
				if err != nil {
					return NodeSession{}, errors.Wrap(err, "validate mic error")
				}

				if micOK {
					// we need to update the NodeSession
					if err := SaveNodeSession(p, ns); err != nil {
						return NodeSession{}, err
					}
					log.WithFields(log.Fields{
						"dev_addr": macPL.FHDR.DevAddr,
						"dev_eui":  ns.DevEUI,
					}).Warning("frame counters reset")
					return ns, nil
				}
			}
			// try the next node-session
			continue
		}

		// the FCnt is valid, validate the MIC
		macPL.FHDR.FCnt = fullFCnt
		micOK, err := phy.ValidateMIC(ns.NwkSKey)
		if err != nil {
			return NodeSession{}, errors.Wrap(err, "validate mic error")
		}
		if micOK {
			return ns, nil
		}
	}

	return NodeSession{}, ErrDoesNotExistOrFCntOrMICInvalid
}

// GetNodeSession returns the NodeSession for the given DevEUI.
func GetNodeSession(p *redis.Pool, devEUI lorawan.EUI64) (NodeSession, error) {
	var ns NodeSession

	c := p.Get()
	defer c.Close()

	val, err := redis.Bytes(c.Do("GET", fmt.Sprintf(nodeSessionKeyTempl, devEUI)))
	if err != nil {
		if err == redis.ErrNil {
			return ns, ErrDoesNotExist
		}
		return ns, errors.Wrap(err, "get error")
	}

	err = gob.NewDecoder(bytes.NewReader(val)).Decode(&ns)
	if err != nil {
		return ns, errors.Wrap(err, "gob decode error")
	}

	return ns, nil
}

// DeleteNodeSession deletes the NodeSession matching the given DevEUI.
func DeleteNodeSession(p *redis.Pool, devEUI lorawan.EUI64) error {
	c := p.Get()
	defer c.Close()

	val, err := redis.Int(c.Do("DEL", fmt.Sprintf(nodeSessionKeyTempl, devEUI)))
	if err != nil {
		return errors.Wrap(err, "delete error")
	}
	if val == 0 {
		return ErrDoesNotExist
	}
	log.WithField("dev_eui", devEUI).Info("node-session deleted")
	return nil
}
