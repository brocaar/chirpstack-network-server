package storage

import (
	"bytes"
	"crypto/rand"
	"encoding/gob"
	"fmt"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/lorawan"
)

const (
	devAddrKeyTempl       = "lora:ns:devaddr:%s" // contains a set of DevEUIs using this DevAddr
	deviceSessionKeyTempl = "lora:ns:device:%s"  // contains the session of a DevEUI
)

// UplinkHistorySize contains the number of frames to store
const UplinkHistorySize = 20

// RXWindow defines the RX window option.
type RXWindow int8

// Available RX window options.
const (
	RX1 = iota
	RX2
)

// UplinkHistory contains the meta-data of an uplink transmission.
type UplinkHistory struct {
	FCnt         uint32
	MaxSNR       float64
	TXPowerIndex int
	GatewayCount int
}

// DeviceSession defines a device-session.
type DeviceSession struct {
	// profile ids
	DeviceProfileID  string
	ServiceProfileID string
	RoutingProfileID string

	// session data
	DevAddr  lorawan.DevAddr
	DevEUI   lorawan.EUI64
	JoinEUI  lorawan.EUI64 // TODO: remove?
	NwkSKey  lorawan.AES128Key
	FCntUp   uint32
	FCntDown uint32

	// Only used by ABP activation
	SkipFCntValidation bool

	RXWindow     RXWindow
	RXDelay      uint8
	RX1DROffset  uint8
	RX2DR        uint8
	RX2Frequency int

	// TXPowerIndex which the node is using. The possible values are defined
	// by the lorawan/band package and are region specific. By default it is
	// assumed that the node is using TXPower 0. This value is controlled by
	// the ADR engine.
	TXPowerIndex int

	// DR defines the (last known) data-rate at which the node is operating.
	// This value is controlled by the ADR engine.
	DR int

	// MaxSupportedTXPowerIndex defines the maximum supported tx-power index
	// by the node, or 0 when not set.
	MaxSupportedTXPowerIndex int

	// MaxSupportedDR defines the maximum supported DR index by the node,
	// or 0 when not set.
	MaxSupportedDR int

	// NbTrans defines the number of transmissions for each unconfirmed uplink
	// frame. In case of 0, the default value is used.
	// This value is controlled by the ADR engine.
	NbTrans uint8

	EnabledChannels    []int            // channels that are activated on the node
	ChannelFrequencies []int            // frequency of each channel
	UplinkHistory      []UplinkHistory  // contains the last 20 transmissions
	LastRXInfoSet      models.RXInfoSet // sorted set (best at index 0)

	// LastDevStatusRequest contains the timestamp when the last device-status
	// request was made.
	LastDevStatusRequested time.Time

	// LastDevStatusBattery contains the last received battery status.
	LastDevStatusBattery uint8

	// LastDevStatusMargin contains the last received margin status.
	LastDevStatusMargin int8

	// LastDownlinkTX contains the timestamp of the last downlink.
	LastDownlinkTX time.Time
}

// AppendUplinkHistory appends an UplinkHistory item and makes sure the list
// never exceeds 20 records. In case more records are present, only the most
// recent ones will be preserved. In case of a re-transmission, the record with
// the best MaxSNR is stored.
func (s *DeviceSession) AppendUplinkHistory(up UplinkHistory) {
	if count := len(s.UplinkHistory); count > 0 {
		// ignore re-transmissions we don't know the source of the
		// re-transmission (it might be a replay-attack)
		if s.UplinkHistory[count-1].FCnt == up.FCnt {
			return
		}
	}

	s.UplinkHistory = append(s.UplinkHistory, up)
	if count := len(s.UplinkHistory); count > UplinkHistorySize {
		s.UplinkHistory = s.UplinkHistory[count-UplinkHistorySize : count]
	}
}

// GetPacketLossPercentage returns the percentage of packet-loss over the
// records stored in UplinkHistory.
// Note it returns 0 when the uplink history table hasn't been filled yet
// to avoid reporting 33% for example when one of the first three uplinks
// was lost.
func (s DeviceSession) GetPacketLossPercentage() float64 {
	if len(s.UplinkHistory) < UplinkHistorySize {
		return 0
	}

	var lostPackets uint32
	var previousFCnt uint32

	for i, uh := range s.UplinkHistory {
		if i == 0 {
			previousFCnt = uh.FCnt
			continue
		}
		lostPackets += uh.FCnt - previousFCnt - 1 // there is always an expected difference of 1
		previousFCnt = uh.FCnt
	}

	return float64(lostPackets) / float64(len(s.UplinkHistory)) * 100
}

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
func ValidateAndGetFullFCntUp(s DeviceSession, fCntUp uint32) (uint32, bool) {
	// we need to compare the difference of the 16 LSB
	gap := uint32(uint16(fCntUp) - uint16(s.FCntUp%65536))
	if gap < config.C.NetworkServer.Band.Band.MaxFCntGap {
		return s.FCntUp + gap, true
	}
	return 0, false
}

// SaveDeviceSession saves the device-session. In case it doesn't exist yet
// it will be created.
func SaveDeviceSession(p *redis.Pool, s DeviceSession) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(s); err != nil {
		return errors.Wrap(err, "gob encode error")
	}

	c := p.Get()
	defer c.Close()
	exp := int64(config.C.NetworkServer.DeviceSessionTTL) / int64(time.Millisecond)

	c.Send("MULTI")
	c.Send("PSETEX", fmt.Sprintf(deviceSessionKeyTempl, s.DevEUI), exp, buf.Bytes())
	c.Send("SADD", fmt.Sprintf(devAddrKeyTempl, s.DevAddr), s.DevEUI[:])
	c.Send("PEXPIRE", fmt.Sprintf(devAddrKeyTempl, s.DevAddr), exp)

	if _, err := c.Do("EXEC"); err != nil {
		return errors.Wrap(err, "exec error")
	}

	log.WithFields(log.Fields{
		"dev_eui":  s.DevEUI,
		"dev_addr": s.DevAddr,
	}).Info("device-session saved")

	return nil
}

// GetDeviceSession returns the device-session for the given DevEUI.
func GetDeviceSession(p *redis.Pool, devEUI lorawan.EUI64) (DeviceSession, error) {
	var s DeviceSession

	c := p.Get()
	defer c.Close()

	val, err := redis.Bytes(c.Do("GET", fmt.Sprintf(deviceSessionKeyTempl, devEUI)))
	if err != nil {
		if err == redis.ErrNil {
			return s, ErrDoesNotExist
		}
		return s, errors.Wrap(err, "get error")
	}

	err = gob.NewDecoder(bytes.NewReader(val)).Decode(&s)
	if err != nil {
		return s, errors.Wrap(err, "gob decode error")
	}

	return s, nil
}

// DeleteDeviceSession deletes the device-session matching the given DevEUI.
func DeleteDeviceSession(p *redis.Pool, devEUI lorawan.EUI64) error {
	c := p.Get()
	defer c.Close()

	val, err := redis.Int(c.Do("DEL", fmt.Sprintf(deviceSessionKeyTempl, devEUI)))
	if err != nil {
		return errors.Wrap(err, "delete error")
	}
	if val == 0 {
		return ErrDoesNotExist
	}
	log.WithField("dev_eui", devEUI).Info("device-session deleted")
	return nil
}

// GetDeviceSessionsForDevAddr returns a slice of device-sessions using the
// given DevAddr. When no device-session is using the given DevAddr, this returns
// an empty slice.
func GetDeviceSessionsForDevAddr(p *redis.Pool, devAddr lorawan.DevAddr) ([]DeviceSession, error) {
	var items []DeviceSession

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

		s, err := GetDeviceSession(p, devEUI)
		if err != nil {
			// TODO: in case not found, remove the DevEUI from the list
			log.WithFields(log.Fields{
				"dev_addr": devAddr,
				"dev_eui":  devEUI,
			}).Warningf("get device-sessions for dev_addr error: %s", err)
		}
		items = append(items, s)
	}

	return items, nil
}

// GetDeviceSessionForPHYPayload returns the device-session matching the given
// PHYPayload. This will fetch all device-sessions associated with the used
// DevAddr and based on FCnt and MIC decide which one to use.
func GetDeviceSessionForPHYPayload(p *redis.Pool, phy lorawan.PHYPayload) (DeviceSession, error) {
	macPL, ok := phy.MACPayload.(*lorawan.MACPayload)
	if !ok {
		return DeviceSession{}, fmt.Errorf("expected *lorawan.MACPayload, got: %T", phy.MACPayload)
	}
	originalFCnt := macPL.FHDR.FCnt

	sessions, err := GetDeviceSessionsForDevAddr(p, macPL.FHDR.DevAddr)
	if err != nil {
		return DeviceSession{}, err
	}

	for _, s := range sessions {
		// reset to the original FCnt
		macPL.FHDR.FCnt = originalFCnt
		// get full FCnt
		fullFCnt, ok := ValidateAndGetFullFCntUp(s, macPL.FHDR.FCnt)
		if !ok {
			// If RelaxFCnt is turned on, just trust the uplink FCnt
			// this is insecure, but has been requested by many people for
			// debugging purposes.
			// Note that we do not reset the FCntDown as this would reset the
			// downlink frame-counter on a re-transmit, which is not what we
			// want.
			if s.SkipFCntValidation {
				fullFCnt = macPL.FHDR.FCnt
				s.FCntUp = macPL.FHDR.FCnt
				s.UplinkHistory = []UplinkHistory{}

				// validate if the mic is valid given the FCnt reset
				micOK, err := phy.ValidateMIC(s.NwkSKey)
				if err != nil {
					return DeviceSession{}, errors.Wrap(err, "validate mic error")
				}

				if micOK {
					// we need to update the NodeSession
					if err := SaveDeviceSession(p, s); err != nil {
						return DeviceSession{}, err
					}
					log.WithFields(log.Fields{
						"dev_addr": macPL.FHDR.DevAddr,
						"dev_eui":  s.DevEUI,
					}).Warning("frame counters reset")
					return s, nil
				}
			}
			// try the next node-session
			continue
		}

		// the FCnt is valid, validate the MIC
		macPL.FHDR.FCnt = fullFCnt
		micOK, err := phy.ValidateMIC(s.NwkSKey)
		if err != nil {
			return DeviceSession{}, errors.Wrap(err, "validate mic error")
		}
		if micOK {
			return s, nil
		}
	}

	return DeviceSession{}, ErrDoesNotExistOrFCntOrMICInvalid
}

// DeviceSessionExists returns a bool indicating if a device session exist.
func DeviceSessionExists(p *redis.Pool, devEUI lorawan.EUI64) (bool, error) {
	c := p.Get()
	defer c.Close()

	r, err := redis.Int(c.Do("EXISTS", fmt.Sprintf(deviceSessionKeyTempl, devEUI)))
	if err != nil {
		return false, errors.Wrap(err, "get exists error")
	}
	if r == 1 {
		return true, nil
	}
	return false, nil
}
