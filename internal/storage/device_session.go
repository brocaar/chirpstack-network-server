//go:generate protoc -I=. -I=$GOPATH/src --go_out=plugins=grpc:. device_session.proto

package storage

import (
	"bytes"
	"crypto/rand"
	"encoding/gob"
	"fmt"
	"strings"
	"time"

	"github.com/gofrs/uuid"
	proto "github.com/golang/protobuf/proto"
	"github.com/gomodule/redigo/redis"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	commonPB "github.com/brocaar/loraserver/api/common"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
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

// UplinkGatewayHistory contains the uplink gateway history meta-data.
// This is used for Class-B and Class-C downlinks.
type UplinkGatewayHistory struct{}

// KeyEnvelope defined a key-envelope.
type KeyEnvelope struct {
	KEKLabel string
	AESKey   []byte
}

// DeviceSession defines a device-session.
type DeviceSession struct {
	// MAC version
	MACVersion string

	// profile ids
	DeviceProfileID  uuid.UUID
	ServiceProfileID uuid.UUID
	RoutingProfileID uuid.UUID

	// session data
	DevAddr        lorawan.DevAddr
	DevEUI         lorawan.EUI64
	JoinEUI        lorawan.EUI64
	FNwkSIntKey    lorawan.AES128Key
	SNwkSIntKey    lorawan.AES128Key
	NwkSEncKey     lorawan.AES128Key
	AppSKeyEvelope *KeyEnvelope
	FCntUp         uint32
	NFCntDown      uint32
	AFCntDown      uint32
	ConfFCnt       uint32

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

	// ADR defines if the device has ADR enabled.
	ADR bool

	// MinSupportedTXPowerIndex defines the minimum supported tx-power index
	// by the node (default 0).
	MinSupportedTXPowerIndex int

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

	EnabledChannels       []int                // deprecated, migrated by GetDeviceSession
	EnabledUplinkChannels []int                // channels that are activated on the node
	ExtraUplinkChannels   map[int]band.Channel // extra uplink channels, configured by the user
	ChannelFrequencies    []int                // frequency of each channel
	UplinkHistory         []UplinkHistory      // contains the last 20 transmissions
	UplinkGatewayHistory  map[lorawan.EUI64]UplinkGatewayHistory

	// LastDevStatusRequest contains the timestamp when the last device-status
	// request was made.
	LastDevStatusRequested time.Time

	// LastDownlinkTX contains the timestamp of the last downlink.
	LastDownlinkTX time.Time

	// Class-B related configuration.
	BeaconLocked      bool
	PingSlotNb        int
	PingSlotDR        int
	PingSlotFrequency int

	// RejoinRequestEnabled defines if the rejoin-request is enabled on the
	// device.
	RejoinRequestEnabled bool

	// RejoinRequestMaxCountN defines the 2^(C+4) uplink message interval for
	// the rejoin-request.
	RejoinRequestMaxCountN int

	// RejoinRequestMaxTimeN defines the 2^(T+10) time interval (seconds)
	// for the rejoin-request.
	RejoinRequestMaxTimeN int

	RejoinCount0               uint16
	PendingRejoinDeviceSession *DeviceSession
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

// GetMACVersion returns the LoRaWAN mac version.
func (s DeviceSession) GetMACVersion() lorawan.MACVersion {
	if strings.HasPrefix(s.MACVersion, "1.1") {
		return lorawan.LoRaWAN1_1
	}

	return lorawan.LoRaWAN1_0
}

// ResetToBootParameters resets the device-session to the device boo
// parameters as defined by the given device-profile.
func (s *DeviceSession) ResetToBootParameters(dp DeviceProfile) {
	if dp.SupportsJoin {
		return
	}

	var channelFrequencies []int
	for _, f := range dp.FactoryPresetFreqs {
		channelFrequencies = append(channelFrequencies, int(f))
	}

	s.TXPowerIndex = 0
	s.MinSupportedTXPowerIndex = 0
	s.MaxSupportedTXPowerIndex = 0
	s.ExtraUplinkChannels = make(map[int]band.Channel)
	s.RXDelay = uint8(dp.RXDelay1)
	s.RX1DROffset = uint8(dp.RXDROffset1)
	s.RX2DR = uint8(dp.RXDataRate2)
	s.RX2Frequency = int(dp.RXFreq2)
	s.EnabledUplinkChannels = config.C.NetworkServer.Band.Band.GetStandardUplinkChannelIndices() // TODO: replace by ServiceProfile.ChannelMask?
	s.ChannelFrequencies = channelFrequencies
	s.PingSlotDR = dp.PingSlotDR
	s.PingSlotFrequency = int(dp.PingSlotFreq)
	s.NbTrans = 1

	if dp.PingSlotPeriod != 0 {
		s.PingSlotNb = (1 << 12) / dp.PingSlotPeriod
	}
}

// GetDownlinkGatewayMAC returns the gateway MAC of the gateway close to the
// device.
func (s DeviceSession) GetDownlinkGatewayMAC() (lorawan.EUI64, error) {
	for mac := range s.UplinkGatewayHistory {
		return mac, nil
	}

	return lorawan.EUI64{}, errors.New("uplink gateway-history is empty")
}

// GetRandomDevAddr returns a random DevAddr, prefixed with NwkID based on the
// given NetID.
func GetRandomDevAddr(p *redis.Pool, netID lorawan.NetID) (lorawan.DevAddr, error) {
	var d lorawan.DevAddr
	b := make([]byte, len(d))
	if _, err := rand.Read(b); err != nil {
		return d, errors.Wrap(err, "read random bytes error")
	}
	copy(d[:], b)
	d.SetAddrPrefix(netID)

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
	if gap < config.C.NetworkServer.Band.Band.GetDefaults().MaxFCntGap {
		return s.FCntUp + gap, true
	}
	return 0, false
}

// SaveDeviceSession saves the device-session. In case it doesn't exist yet
// it will be created.
func SaveDeviceSession(p *redis.Pool, s DeviceSession) error {
	dsPB := deviceSessionToPB(s)
	b, err := proto.Marshal(&dsPB)
	if err != nil {
		return errors.Wrap(err, "protobuf encode error")
	}

	c := p.Get()
	defer c.Close()
	exp := int64(config.C.NetworkServer.DeviceSessionTTL) / int64(time.Millisecond)

	c.Send("MULTI")
	c.Send("PSETEX", fmt.Sprintf(deviceSessionKeyTempl, s.DevEUI), exp, b)
	c.Send("SADD", fmt.Sprintf(devAddrKeyTempl, s.DevAddr), s.DevEUI[:])
	c.Send("PEXPIRE", fmt.Sprintf(devAddrKeyTempl, s.DevAddr), exp)
	if s.PendingRejoinDeviceSession != nil {
		c.Send("SADD", fmt.Sprintf(devAddrKeyTempl, s.PendingRejoinDeviceSession.DevAddr), s.DevEUI[:])
		c.Send("PEXPIRE", fmt.Sprintf(devAddrKeyTempl, s.PendingRejoinDeviceSession.DevAddr), exp)
	}
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
	var dsPB DeviceSessionPB

	c := p.Get()
	defer c.Close()

	val, err := redis.Bytes(c.Do("GET", fmt.Sprintf(deviceSessionKeyTempl, devEUI)))
	if err != nil {
		if err == redis.ErrNil {
			return DeviceSession{}, ErrDoesNotExist
		}
		return DeviceSession{}, errors.Wrap(err, "get error")
	}

	err = proto.Unmarshal(val, &dsPB)
	if err != nil {
		// fallback on old gob encoding
		var dsOld DeviceSessionOld
		err = gob.NewDecoder(bytes.NewReader(val)).Decode(&dsOld)
		if err != nil {
			return DeviceSession{}, errors.Wrap(err, "gob decode error")
		}

		return migrateDeviceSessionOld(dsOld), nil
	}

	return deviceSessionFromPB(dsPB), nil
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

		// It is possible that the "main" device-session maps to a different
		// devAddr as the PendingRejoinDeviceSession is set (using the devAddr
		// that is used for the lookup).
		if s.DevAddr == devAddr {
			items = append(items, s)
		}

		// When a pending rejoin device-session context is set and it has
		// the given devAddr, add it to the items list.
		if s.PendingRejoinDeviceSession != nil && s.PendingRejoinDeviceSession.DevAddr == devAddr {
			items = append(items, *s.PendingRejoinDeviceSession)
		}
	}

	return items, nil
}

// GetDeviceSessionForPHYPayload returns the device-session matching the given
// PHYPayload. This will fetch all device-sessions associated with the used
// DevAddr and based on FCnt and MIC decide which one to use.
func GetDeviceSessionForPHYPayload(p *redis.Pool, phy lorawan.PHYPayload, txDR, txCh int) (DeviceSession, error) {
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
				// note that we can always set the ConfFCnt as the validation
				// function will only use it when the ACK bit is set
				micOK, err := phy.ValidateUplinkDataMIC(s.GetMACVersion(), s.ConfFCnt, uint8(txDR), uint8(txCh), s.FNwkSIntKey, s.SNwkSIntKey)
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
		micOK, err := phy.ValidateUplinkDataMIC(s.GetMACVersion(), s.AFCntDown, uint8(txDR), uint8(txCh), s.FNwkSIntKey, s.SNwkSIntKey)
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

func deviceSessionToPB(d DeviceSession) DeviceSessionPB {
	out := DeviceSessionPB{
		MacVersion: d.MACVersion,

		DeviceProfileId:  d.DeviceProfileID.String(),
		ServiceProfileId: d.ServiceProfileID.String(),
		RoutingProfileId: d.RoutingProfileID.String(),

		DevAddr:     d.DevAddr[:],
		DevEui:      d.DevEUI[:],
		JoinEui:     d.JoinEUI[:],
		FNwkSIntKey: d.FNwkSIntKey[:],
		SNwkSIntKey: d.SNwkSIntKey[:],
		NwkSEncKey:  d.NwkSEncKey[:],

		FCntUp:        d.FCntUp,
		NFCntDown:     d.NFCntDown,
		AFCntDown:     d.AFCntDown,
		ConfFCnt:      d.ConfFCnt,
		SkipFCntCheck: d.SkipFCntValidation,

		RxDelay:      uint32(d.RXDelay),
		Rx1DrOffset:  uint32(d.RX1DROffset),
		Rx2Dr:        uint32(d.RX2DR),
		Rx2Frequency: uint32(d.RX2Frequency),
		TxPowerIndex: uint32(d.TXPowerIndex),

		Dr:  uint32(d.DR),
		Adr: d.ADR,
		MinSupportedTxPowerIndex: uint32(d.MinSupportedTXPowerIndex),
		MaxSupportedTxPowerIndex: uint32(d.MaxSupportedTXPowerIndex),
		MaxSupportedDr:           uint32(d.MaxSupportedDR),
		NbTrans:                  uint32(d.NbTrans),

		ExtraUplinkChannels:  make(map[uint32]*DeviceSessionPBChannel),
		UplinkGatewayHistory: make(map[string]*DeviceSessionPBUplinkGatewayHistory),

		LastDeviceStatusRequestTimeUnixNs: d.LastDevStatusRequested.UnixNano(),

		LastDownlinkTxTimestampUnixNs: d.LastDownlinkTX.UnixNano(),
		BeaconLocked:                  d.BeaconLocked,
		PingSlotNb:                    uint32(d.PingSlotNb),
		PingSlotDr:                    uint32(d.PingSlotDR),
		PingSlotFrequency:             uint32(d.PingSlotFrequency),

		RejoinRequestEnabled:   d.RejoinRequestEnabled,
		RejoinRequestMaxCountN: uint32(d.RejoinRequestMaxCountN),
		RejoinRequestMaxTimeN:  uint32(d.RejoinRequestMaxTimeN),

		RejoinCount_0: uint32(d.RejoinCount0),
	}

	if d.AppSKeyEvelope != nil {
		out.AppSKeyEnvelope = &commonPB.KeyEnvelope{
			KekLabel: d.AppSKeyEvelope.KEKLabel,
			AesKey:   d.AppSKeyEvelope.AESKey,
		}
	}

	for _, c := range d.EnabledUplinkChannels {
		out.EnabledUplinkChannels = append(out.EnabledUplinkChannels, uint32(c))
	}

	for i, c := range d.ExtraUplinkChannels {
		out.ExtraUplinkChannels[uint32(i)] = &DeviceSessionPBChannel{
			Frequency: uint32(c.Frequency),
			MinDr:     uint32(c.MinDR),
			MaxDr:     uint32(c.MaxDR),
		}
	}

	for _, c := range d.ChannelFrequencies {
		out.ChannelFrequencies = append(out.ChannelFrequencies, uint32(c))
	}

	for _, h := range d.UplinkHistory {
		out.UplinkAdrHistory = append(out.UplinkAdrHistory, &DeviceSessionPBUplinkADRHistory{
			FCnt:         h.FCnt,
			MaxSnr:       float32(h.MaxSNR),
			TxPowerIndex: uint32(h.TXPowerIndex),
			GatewayCount: uint32(h.GatewayCount),
		})
	}

	for mac := range d.UplinkGatewayHistory {
		out.UplinkGatewayHistory[mac.String()] = nil
	}

	if d.PendingRejoinDeviceSession != nil {
		dsPB := deviceSessionToPB(*d.PendingRejoinDeviceSession)
		b, err := proto.Marshal(&dsPB)
		if err != nil {
			log.WithField("dev_eui", d.DevEUI).WithError(err).Error("protobuf encode error")
		}

		out.PendingRejoinDeviceSession = b
	}

	return out
}

func deviceSessionFromPB(d DeviceSessionPB) DeviceSession {
	dpID, _ := uuid.FromString(d.DeviceProfileId)
	rpID, _ := uuid.FromString(d.RoutingProfileId)
	spID, _ := uuid.FromString(d.ServiceProfileId)

	out := DeviceSession{
		MACVersion: d.MacVersion,

		DeviceProfileID:  dpID,
		ServiceProfileID: spID,
		RoutingProfileID: rpID,

		FCntUp:             d.FCntUp,
		NFCntDown:          d.NFCntDown,
		AFCntDown:          d.AFCntDown,
		ConfFCnt:           d.ConfFCnt,
		SkipFCntValidation: d.SkipFCntCheck,

		RXDelay:      uint8(d.RxDelay),
		RX1DROffset:  uint8(d.Rx1DrOffset),
		RX2DR:        uint8(d.Rx2Dr),
		RX2Frequency: int(d.Rx2Frequency),
		TXPowerIndex: int(d.TxPowerIndex),

		DR:  int(d.Dr),
		ADR: d.Adr,
		MinSupportedTXPowerIndex: int(d.MinSupportedTxPowerIndex),
		MaxSupportedTXPowerIndex: int(d.MaxSupportedTxPowerIndex),
		MaxSupportedDR:           int(d.MaxSupportedDr),
		NbTrans:                  uint8(d.NbTrans),

		ExtraUplinkChannels:  make(map[int]band.Channel),
		UplinkGatewayHistory: make(map[lorawan.EUI64]UplinkGatewayHistory),

		BeaconLocked:      d.BeaconLocked,
		PingSlotNb:        int(d.PingSlotNb),
		PingSlotDR:        int(d.PingSlotDr),
		PingSlotFrequency: int(d.PingSlotFrequency),

		RejoinRequestEnabled:   d.RejoinRequestEnabled,
		RejoinRequestMaxCountN: int(d.RejoinRequestMaxCountN),
		RejoinRequestMaxTimeN:  int(d.RejoinRequestMaxTimeN),

		RejoinCount0: uint16(d.RejoinCount_0),
	}

	if d.LastDeviceStatusRequestTimeUnixNs > 0 {
		out.LastDevStatusRequested = time.Unix(0, d.LastDeviceStatusRequestTimeUnixNs)
	}

	if d.LastDownlinkTxTimestampUnixNs > 0 {
		out.LastDownlinkTX = time.Unix(0, d.LastDownlinkTxTimestampUnixNs)
	}

	copy(out.DevAddr[:], d.DevAddr)
	copy(out.DevEUI[:], d.DevEui)
	copy(out.JoinEUI[:], d.JoinEui)
	copy(out.FNwkSIntKey[:], d.FNwkSIntKey)
	copy(out.SNwkSIntKey[:], d.SNwkSIntKey)
	copy(out.NwkSEncKey[:], d.NwkSEncKey)

	if d.AppSKeyEnvelope != nil {
		out.AppSKeyEvelope = &KeyEnvelope{
			KEKLabel: d.AppSKeyEnvelope.KekLabel,
			AESKey:   d.AppSKeyEnvelope.AesKey,
		}
	}

	for _, c := range d.EnabledUplinkChannels {
		out.EnabledUplinkChannels = append(out.EnabledUplinkChannels, int(c))
	}

	for i, c := range d.ExtraUplinkChannels {
		out.ExtraUplinkChannels[int(i)] = band.Channel{
			Frequency: int(c.Frequency),
			MinDR:     int(c.MinDr),
			MaxDR:     int(c.MaxDr),
		}
	}

	for _, c := range d.ChannelFrequencies {
		out.ChannelFrequencies = append(out.ChannelFrequencies, int(c))
	}

	for _, h := range d.UplinkAdrHistory {
		out.UplinkHistory = append(out.UplinkHistory, UplinkHistory{
			FCnt:         h.FCnt,
			MaxSNR:       float64(h.MaxSnr),
			TXPowerIndex: int(h.TxPowerIndex),
			GatewayCount: int(h.GatewayCount),
		})
	}

	for macStr := range d.UplinkGatewayHistory {
		var mac lorawan.EUI64
		if err := mac.UnmarshalText([]byte(macStr)); err != nil {
			continue
		}
		out.UplinkGatewayHistory[mac] = UplinkGatewayHistory{}
	}

	if len(d.PendingRejoinDeviceSession) != 0 {
		var dsPB DeviceSessionPB
		if err := proto.Unmarshal(d.PendingRejoinDeviceSession, &dsPB); err != nil {
			log.WithField("dev_eui", out.DevEUI).WithError(err).Error("decode pending rejoin device-session error")
		} else {
			ds := deviceSessionFromPB(dsPB)
			out.PendingRejoinDeviceSession = &ds
		}
	}

	return out
}
