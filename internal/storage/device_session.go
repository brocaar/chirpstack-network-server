//go:generate protoc -I=/protobuf/src -I=/tmp/chirpstack-api/protobuf -I=. --go_out=. device_session.proto

package storage

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/go-redis/redis/v7"
	"github.com/gofrs/uuid"
	proto "github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-network-server/internal/band"
	"github.com/brocaar/chirpstack-network-server/internal/logging"
	"github.com/brocaar/lorawan"
	loraband "github.com/brocaar/lorawan/band"
)

const (
	devAddrKeyTempl                = "lora:ns:devaddr:%s"     // contains a set of DevEUIs using this DevAddr
	deviceSessionKeyTempl          = "lora:ns:device:%s"      // contains the session of a DevEUI
	deviceGatewayRXInfoSetKeyTempl = "lora:ns:device:%s:gwrx" // contains gateway meta-data from the last uplink
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

// DeviceGatewayRXInfoSet contains the rx-info set of the receiving gateways
// for the last uplink.
type DeviceGatewayRXInfoSet struct {
	DevEUI lorawan.EUI64
	DR     int
	Items  []DeviceGatewayRXInfo
}

// DeviceGatewayRXInfo holds the meta-data of a gateway receiving the last
// uplink message.
type DeviceGatewayRXInfo struct {
	GatewayID lorawan.EUI64
	RSSI      int
	LoRaSNR   float64
	Antenna   uint32
	Board     uint32
	Context   []byte
}

// UplinkHistory contains the meta-data of an uplink transmission.
type UplinkHistory struct {
	FCnt         uint32
	MaxSNR       float64
	TXPowerIndex int
	GatewayCount int
}

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

	// NbTrans defines the number of transmissions for each unconfirmed uplink
	// frame. In case of 0, the default value is used.
	// This value is controlled by the ADR engine.
	NbTrans uint8

	EnabledChannels       []int                    // deprecated, migrated by GetDeviceSession
	EnabledUplinkChannels []int                    // channels that are activated on the node
	ExtraUplinkChannels   map[int]loraband.Channel // extra uplink channels, configured by the user
	ChannelFrequencies    []int                    // frequency of each channel
	UplinkHistory         []UplinkHistory          // contains the last 20 transmissions

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

	// ReferenceAltitude holds the device reference altitude used for
	// geolocation.
	ReferenceAltitude float64

	// Uplink and Downlink dwell time limitations.
	UplinkDwellTime400ms   bool
	DownlinkDwellTime400ms bool

	// Max uplink EIRP limitation.
	UplinkMaxEIRPIndex uint8

	// Delayed mac-commands.
	MACCommandErrorCount map[lorawan.CID]int

	// Device is disabled.
	IsDisabled bool
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
	s.ExtraUplinkChannels = make(map[int]loraband.Channel)
	s.RXDelay = uint8(dp.RXDelay1)
	s.RX1DROffset = uint8(dp.RXDROffset1)
	s.RX2DR = uint8(dp.RXDataRate2)
	s.RX2Frequency = int(dp.RXFreq2)
	s.EnabledUplinkChannels = band.Band().GetStandardUplinkChannelIndices() // TODO: replace by ServiceProfile.ChannelMask?
	s.ChannelFrequencies = channelFrequencies
	s.PingSlotDR = dp.PingSlotDR
	s.PingSlotFrequency = int(dp.PingSlotFreq)
	s.NbTrans = 1

	if dp.PingSlotPeriod != 0 {
		s.PingSlotNb = (1 << 12) / dp.PingSlotPeriod
	}
}

// GetRandomDevAddr returns a random DevAddr, prefixed with NwkID based on the
// given NetID.
func GetRandomDevAddr(netID lorawan.NetID) (lorawan.DevAddr, error) {
	var d lorawan.DevAddr
	b := make([]byte, len(d))
	if _, err := rand.Read(b); err != nil {
		return d, errors.Wrap(err, "read random bytes error")
	}
	copy(d[:], b)
	d.SetAddrPrefix(netID)

	return d, nil
}

// GetFullFCntUp returns the full 32bit frame-counter, given the fCntUp which
// has been truncated to the last 16 LSB.
// Notes:
// * After a succesful validation of the FCntUp and the MIC, don't forget
//   to synchronize the device FCntUp with the packet FCnt.
// * In case of a frame-counter rollover, the returned values will be less
//   than the given DeviceSession FCntUp. This must be validated outside this
//   function!
// * In case of a re-transmission, the returned frame-counter equals
//   DeviceSession.FCntUp - 1, as the FCntUp value holds the next expected
//   frame-counter, not the FCntUp which was last seen.
func GetFullFCntUp(nextExpectedFullFCnt, truncatedFCntUp uint32) uint32 {
	// Handle re-transmission.
	// Note: the s.FCntUp value holds the next expected uplink frame-counter,
	// therefore this function returns sFCntUp - 1 in case of a re-transmission.
	if truncatedFCntUp == uint32(uint16(nextExpectedFullFCnt%(1<<16))-1) {
		return nextExpectedFullFCnt - 1
	}
	gap := uint32(uint16(truncatedFCntUp) - uint16(nextExpectedFullFCnt%(1<<16)))
	return nextExpectedFullFCnt + gap
}

// SaveDeviceSession saves the device-session. In case it doesn't exist yet
// it will be created.
func SaveDeviceSession(ctx context.Context, s DeviceSession) error {
	devAddrKey := fmt.Sprintf(devAddrKeyTempl, s.DevAddr)
	devSessKey := fmt.Sprintf(deviceSessionKeyTempl, s.DevEUI)

	dsPB := deviceSessionToPB(s)
	b, err := proto.Marshal(&dsPB)
	if err != nil {
		return errors.Wrap(err, "protobuf encode error")
	}

	// Note that we must execute the DevAddr set related operations in multiple
	// tx pipelines in order to support Redis Cluster. It can not be guaranteed
	// that devAddrKey, pendingDevAddrKey and DevSessKey are on the same Cluster
	// shard.

	pipe := RedisClient().TxPipeline()
	pipe.SAdd(devAddrKey, s.DevEUI[:])
	pipe.PExpire(devAddrKey, deviceSessionTTL)
	if _, err := pipe.Exec(); err != nil {
		return errors.Wrap(err, "exec error")
	}

	if s.PendingRejoinDeviceSession != nil {
		pendingDevAddrKey := fmt.Sprintf(devAddrKeyTempl, s.PendingRejoinDeviceSession.DevAddr)

		pipe = RedisClient().TxPipeline()
		pipe.SAdd(pendingDevAddrKey, s.DevEUI[:])
		pipe.PExpire(pendingDevAddrKey, deviceSessionTTL)
		if _, err := pipe.Exec(); err != nil {
			return errors.Wrap(err, "exec error")
		}
	}

	err = RedisClient().Set(devSessKey, b, deviceSessionTTL).Err()
	if err != nil {
		return errors.Wrap(err, "set error")
	}

	log.WithFields(log.Fields{
		"dev_eui":  s.DevEUI,
		"dev_addr": s.DevAddr,
		"ctx_id":   ctx.Value(logging.ContextIDKey),
	}).Info("device-session saved")

	return nil
}

// GetDeviceSession returns the device-session for the given DevEUI.
func GetDeviceSession(ctx context.Context, devEUI lorawan.EUI64) (DeviceSession, error) {
	key := fmt.Sprintf(deviceSessionKeyTempl, devEUI)
	var dsPB DeviceSessionPB

	val, err := RedisClient().Get(key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return DeviceSession{}, ErrDoesNotExist
		}
		return DeviceSession{}, errors.Wrap(err, "get error")
	}

	err = proto.Unmarshal(val, &dsPB)
	if err != nil {
		return DeviceSession{}, errors.Wrap(err, "unmarshal protobuf error")
	}

	return deviceSessionFromPB(dsPB), nil
}

// DeleteDeviceSession deletes the device-session matching the given DevEUI.
func DeleteDeviceSession(ctx context.Context, devEUI lorawan.EUI64) error {
	key := fmt.Sprintf(deviceSessionKeyTempl, devEUI)

	val, err := RedisClient().Del(key).Result()
	if err != nil {
		return errors.Wrap(err, "delete error")
	}
	if val == 0 {
		return ErrDoesNotExist
	}

	log.WithFields(log.Fields{
		"dev_eui": devEUI,
		"ctx_id":  ctx.Value(logging.ContextIDKey),
	}).Info("device-session deleted")
	return nil
}

// GetDeviceSessionsForDevAddr returns a slice of device-sessions using the
// given DevAddr. When no device-session is using the given DevAddr, this returns
// an empty slice.
func GetDeviceSessionsForDevAddr(ctx context.Context, devAddr lorawan.DevAddr) ([]DeviceSession, error) {
	var items []DeviceSession

	devEUIs, err := GetDevEUIsForDevAddr(ctx, devAddr)
	if err != nil {
		return nil, err
	}

	for _, devEUI := range devEUIs {
		s, err := GetDeviceSession(ctx, devEUI)
		if err != nil {
			// TODO: in case not found, remove the DevEUI from the list
			log.WithError(err).WithFields(log.Fields{
				"dev_addr": devAddr,
				"dev_eui":  devEUI,
				"ctx_id":   ctx.Value(logging.ContextIDKey),
			}).Warning("get device-session for devaddr error")
			continue
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

// GetDevEUIsForDevAddr returns the DevEUIs that are using the given DevAddr.
func GetDevEUIsForDevAddr(ctx context.Context, devAddr lorawan.DevAddr) ([]lorawan.EUI64, error) {
	key := fmt.Sprintf(devAddrKeyTempl, devAddr)

	val, err := RedisClient().SMembers(key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, errors.Wrap(err, "get deveuis for devaddr error")
	}

	var out []lorawan.EUI64
	for i := range val {
		var devEUI lorawan.EUI64
		copy(devEUI[:], []byte(val[i]))
		out = append(out, devEUI)
	}

	return out, nil
}

// GetDeviceSessionForPHYPayload returns the device-session matching the given
// PHYPayload. This will fetch all device-sessions associated with the used
// DevAddr and based on FCnt and MIC decide which one to use.
func GetDeviceSessionForPHYPayload(ctx context.Context, phy lorawan.PHYPayload, txDR, txCh int) (DeviceSession, error) {
	macPL, ok := phy.MACPayload.(*lorawan.MACPayload)
	if !ok {
		return DeviceSession{}, fmt.Errorf("expected *lorawan.MACPayload, got: %T", phy.MACPayload)
	}
	originalFCnt := macPL.FHDR.FCnt

	deviceSessions, err := GetDeviceSessionsForDevAddr(ctx, macPL.FHDR.DevAddr)
	if err != nil {
		return DeviceSession{}, err
	}
	if len(deviceSessions) == 0 {
		return DeviceSession{}, ErrDoesNotExist
	}

	for _, ds := range deviceSessions {
		// restore the original frame-counter
		macPL.FHDR.FCnt = originalFCnt

		// get the full frame-counter (from 16bit to 32bit)
		fullFCnt := GetFullFCntUp(ds.FCntUp, macPL.FHDR.FCnt)

		// Check both the full frame-counter and the received frame-counter
		// truncated to the 16LSB.
		// The latter is needed in case of a frame-counter reset as the
		// GetFullFCntUp will think the 16LSB has rolled over and will
		// increment the 16MSB bit.
		var micOK bool
		for _, fullFCnt := range []uint32{fullFCnt, originalFCnt} {
			macPL.FHDR.FCnt = fullFCnt
			micOK, err = phy.ValidateUplinkDataMIC(ds.GetMACVersion(), ds.ConfFCnt, uint8(txDR), uint8(txCh), ds.FNwkSIntKey, ds.SNwkSIntKey)
			if err != nil {
				// this is not related to a bad MIC, but is a software error
				return DeviceSession{}, errors.Wrap(err, "validate mic error")
			}

			if micOK {
				break
			}
		}

		if micOK {
			if macPL.FHDR.FCnt >= ds.FCntUp {
				// the frame-counter is greater than or equal to the expected value
				return ds, nil
			} else if ds.SkipFCntValidation {
				// re-transmission or frame-counter reset
				ds.FCntUp = 0
				ds.UplinkHistory = []UplinkHistory{}
				return ds, nil
			} else if macPL.FHDR.FCnt == (ds.FCntUp - 1) {
				// re-transmission, the frame-counter did not increment
				return ds, ErrFrameCounterRetransmission
			} else {
				// frame-counter reset or roll-over happened
				return ds, ErrFrameCounterReset
			}
		}
	}

	// Return the first device-session. We could not validate if this is the
	// device-session matching the uplink DevAddr, but it could still provide
	// value to forward the MIC error to the AS in the Routing Profile of the
	// device-session.
	return deviceSessions[0], ErrInvalidMIC
}

// DeviceSessionExists returns a bool indicating if a device session exist.
func DeviceSessionExists(ctx context.Context, devEUI lorawan.EUI64) (bool, error) {
	key := fmt.Sprintf(deviceSessionKeyTempl, devEUI)

	r, err := RedisClient().Exists(key).Result()
	if err != nil {
		return false, errors.Wrap(err, "get exists error")
	}
	if r == 1 {
		return true, nil
	}
	return false, nil
}

// SaveDeviceGatewayRXInfoSet saves the given DeviceGatewayRXInfoSet.
func SaveDeviceGatewayRXInfoSet(ctx context.Context, rxInfoSet DeviceGatewayRXInfoSet) error {
	key := fmt.Sprintf(deviceGatewayRXInfoSetKeyTempl, rxInfoSet.DevEUI)

	rxInfoSetPB := deviceGatewayRXInfoSetToPB(rxInfoSet)
	b, err := proto.Marshal(&rxInfoSetPB)
	if err != nil {
		return errors.Wrap(err, "protobuf encode error")
	}

	err = RedisClient().Set(key, b, deviceSessionTTL).Err()
	if err != nil {
		return errors.Wrap(err, "psetex error")
	}

	log.WithFields(log.Fields{
		"dev_eui": rxInfoSet.DevEUI,
		"ctx_id":  ctx.Value(logging.ContextIDKey),
	}).Info("device gateway rx-info meta-data saved")

	return nil
}

// DeleteDeviceGatewayRXInfoSet deletes the device gateway rx-info meta-data
// for the given Device EUI.
func DeleteDeviceGatewayRXInfoSet(ctx context.Context, devEUI lorawan.EUI64) error {
	key := fmt.Sprintf(deviceGatewayRXInfoSetKeyTempl, devEUI)

	val, err := RedisClient().Del(key).Result()
	if err != nil {
		return errors.Wrap(err, "delete error")
	}
	if val == 0 {
		return ErrDoesNotExist
	}

	log.WithFields(log.Fields{
		"dev_eui": devEUI,
		"ctx_id":  ctx.Value(logging.ContextIDKey),
	}).Info("device gateway rx-info meta-data deleted")
	return nil
}

// GetDeviceGatewayRXInfoSet returns the DeviceGatewayRXInfoSet for the given
// Device EUI.
func GetDeviceGatewayRXInfoSet(ctx context.Context, devEUI lorawan.EUI64) (DeviceGatewayRXInfoSet, error) {
	var rxInfoSetPB DeviceGatewayRXInfoSetPB
	key := fmt.Sprintf(deviceGatewayRXInfoSetKeyTempl, devEUI)

	val, err := RedisClient().Get(key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return DeviceGatewayRXInfoSet{}, ErrDoesNotExist
		}
		return DeviceGatewayRXInfoSet{}, errors.Wrap(err, "get error")
	}

	err = proto.Unmarshal(val, &rxInfoSetPB)
	if err != nil {
		return DeviceGatewayRXInfoSet{}, errors.Wrap(err, "protobuf unmarshal error")
	}

	return deviceGatewayRXInfoSetFromPB(rxInfoSetPB), nil
}

// GetDeviceGatewayRXInfoSetForDevEUIs returns the DeviceGatewayRXInfoSet
// objects for the given Device EUIs.
func GetDeviceGatewayRXInfoSetForDevEUIs(ctx context.Context, devEUIs []lorawan.EUI64) ([]DeviceGatewayRXInfoSet, error) {
	if len(devEUIs) == 0 {
		return nil, nil
	}

	var keys []string
	for _, d := range devEUIs {
		keys = append(keys, fmt.Sprintf(deviceGatewayRXInfoSetKeyTempl, d))
	}

	bs, err := RedisClient().MGet(keys...).Result()
	if err != nil {
		return nil, errors.Wrap(err, "get byte slices error")
	}

	var out []DeviceGatewayRXInfoSet
	for _, val := range bs {
		if val == nil {
			continue
		}

		b := []byte(val.(string))

		var rxInfoSetPB DeviceGatewayRXInfoSetPB
		if err = proto.Unmarshal(b, &rxInfoSetPB); err != nil {
			log.WithError(err).WithFields(log.Fields{
				"ctx_id": ctx.Value(logging.ContextIDKey),
			}).Error("protobuf unmarshal error")
			continue
		}

		out = append(out, deviceGatewayRXInfoSetFromPB(rxInfoSetPB))
	}

	return out, nil
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

		Dr:                       uint32(d.DR),
		Adr:                      d.ADR,
		MinSupportedTxPowerIndex: uint32(d.MinSupportedTXPowerIndex),
		MaxSupportedTxPowerIndex: uint32(d.MaxSupportedTXPowerIndex),
		NbTrans:                  uint32(d.NbTrans),

		ExtraUplinkChannels: make(map[uint32]*DeviceSessionPBChannel),

		LastDeviceStatusRequestTimeUnixNs: d.LastDevStatusRequested.UnixNano(),

		LastDownlinkTxTimestampUnixNs: d.LastDownlinkTX.UnixNano(),
		BeaconLocked:                  d.BeaconLocked,
		PingSlotNb:                    uint32(d.PingSlotNb),
		PingSlotDr:                    uint32(d.PingSlotDR),
		PingSlotFrequency:             uint32(d.PingSlotFrequency),

		RejoinRequestEnabled:   d.RejoinRequestEnabled,
		RejoinRequestMaxCountN: uint32(d.RejoinRequestMaxCountN),
		RejoinRequestMaxTimeN:  uint32(d.RejoinRequestMaxTimeN),

		RejoinCount_0:     uint32(d.RejoinCount0),
		ReferenceAltitude: d.ReferenceAltitude,

		UplinkDwellTime_400Ms:   d.UplinkDwellTime400ms,
		DownlinkDwellTime_400Ms: d.DownlinkDwellTime400ms,
		UplinkMaxEirpIndex:      uint32(d.UplinkMaxEIRPIndex),

		MacCommandErrorCount: make(map[uint32]uint32),

		IsDisabled: d.IsDisabled,
	}

	if d.AppSKeyEvelope != nil {
		out.AppSKeyEnvelope = &common.KeyEnvelope{
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

	if d.PendingRejoinDeviceSession != nil {
		dsPB := deviceSessionToPB(*d.PendingRejoinDeviceSession)
		b, err := proto.Marshal(&dsPB)
		if err != nil {
			log.WithField("dev_eui", d.DevEUI).WithError(err).Error("protobuf encode error")
		}

		out.PendingRejoinDeviceSession = b
	}

	for k, v := range d.MACCommandErrorCount {
		out.MacCommandErrorCount[uint32(k)] = uint32(v)
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

		DR:                       int(d.Dr),
		ADR:                      d.Adr,
		MinSupportedTXPowerIndex: int(d.MinSupportedTxPowerIndex),
		MaxSupportedTXPowerIndex: int(d.MaxSupportedTxPowerIndex),
		NbTrans:                  uint8(d.NbTrans),

		ExtraUplinkChannels: make(map[int]loraband.Channel),

		BeaconLocked:      d.BeaconLocked,
		PingSlotNb:        int(d.PingSlotNb),
		PingSlotDR:        int(d.PingSlotDr),
		PingSlotFrequency: int(d.PingSlotFrequency),

		RejoinRequestEnabled:   d.RejoinRequestEnabled,
		RejoinRequestMaxCountN: int(d.RejoinRequestMaxCountN),
		RejoinRequestMaxTimeN:  int(d.RejoinRequestMaxTimeN),

		RejoinCount0:      uint16(d.RejoinCount_0),
		ReferenceAltitude: d.ReferenceAltitude,

		UplinkDwellTime400ms:   d.UplinkDwellTime_400Ms,
		DownlinkDwellTime400ms: d.DownlinkDwellTime_400Ms,
		UplinkMaxEIRPIndex:     uint8(d.UplinkMaxEirpIndex),

		MACCommandErrorCount: make(map[lorawan.CID]int),

		IsDisabled: d.IsDisabled,
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
		out.ExtraUplinkChannels[int(i)] = loraband.Channel{
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

	if len(d.PendingRejoinDeviceSession) != 0 {
		var dsPB DeviceSessionPB
		if err := proto.Unmarshal(d.PendingRejoinDeviceSession, &dsPB); err != nil {
			log.WithField("dev_eui", out.DevEUI).WithError(err).Error("decode pending rejoin device-session error")
		} else {
			ds := deviceSessionFromPB(dsPB)
			out.PendingRejoinDeviceSession = &ds
		}
	}

	for k, v := range d.MacCommandErrorCount {
		out.MACCommandErrorCount[lorawan.CID(k)] = int(v)
	}

	return out
}

func deviceGatewayRXInfoSetToPB(d DeviceGatewayRXInfoSet) DeviceGatewayRXInfoSetPB {
	out := DeviceGatewayRXInfoSetPB{
		DevEui: d.DevEUI[:],
		Dr:     uint32(d.DR),
	}

	for i := range d.Items {
		out.Items = append(out.Items, &DeviceGatewayRXInfoPB{
			GatewayId: d.Items[i].GatewayID[:],
			Rssi:      int32(d.Items[i].RSSI),
			LoraSnr:   d.Items[i].LoRaSNR,
			Board:     d.Items[i].Board,
			Antenna:   d.Items[i].Antenna,
			Context:   d.Items[i].Context,
		})
	}

	return out
}

func deviceGatewayRXInfoSetFromPB(d DeviceGatewayRXInfoSetPB) DeviceGatewayRXInfoSet {
	out := DeviceGatewayRXInfoSet{
		DR: int(d.Dr),
	}
	copy(out.DevEUI[:], d.DevEui)

	for i := range d.Items {
		var id lorawan.EUI64
		copy(id[:], d.Items[i].GatewayId)
		out.Items = append(out.Items, DeviceGatewayRXInfo{
			GatewayID: id,
			RSSI:      int(d.Items[i].Rssi),
			LoRaSNR:   d.Items[i].LoraSnr,
			Board:     d.Items[i].Board,
			Antenna:   d.Items[i].Antenna,
			Context:   d.Items[i].Context,
		})
	}

	return out
}
