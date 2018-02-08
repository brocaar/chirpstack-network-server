package storage

import (
	"bytes"
	"encoding/gob"

	"github.com/garyburd/redigo/redis"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/lorawan"
)

// NodeSession contains the information of a node-session (an activated node).
type NodeSession struct {
	DevAddr   lorawan.DevAddr
	AppEUI    lorawan.EUI64
	DevEUI    lorawan.EUI64
	NwkSKey   lorawan.AES128Key
	FCntUp    uint32
	FCntDown  uint32
	RelaxFCnt bool

	RXWindow    RXWindow
	RXDelay     uint8
	RX1DROffset uint8
	RX2DR       uint8

	// ADRInterval controls the interval on which to send ADR mac-commands
	// (in case the data-rate / tx power of the node can be changed).
	// Setting this to 0 will disable ADR, 1 means to respond to every uplink
	// with ADR commands.
	ADRInterval uint32

	// Installation margin in dB, used for calculated the recommended DR (ADR).
	// A higher margin will lower the data-rate and therefore decrease
	// packet-loss.
	// A lower margin will increase the data-rate and therefore increase
	// possible packet-loss.
	InstallationMargin float64

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

	EnabledChannels []int           // channels that are activated on the node
	UplinkHistory   []UplinkHistory // contains the last 20 transmissions
	LastRXInfoSet   []gw.RXInfo     // sorted set (best at index 0)
}

// MigrateNodeToDeviceSession migrates the node-session to the device-session
// given a DevEUI.
func MigrateNodeToDeviceSession(p *redis.Pool, db sqlx.Ext, devEUI, joinEUI lorawan.EUI64, devNonces []lorawan.DevNonce) error {
	c := p.Get()
	defer c.Close()

	d, err := GetDevice(db, devEUI)
	if err != nil {
		return errors.Wrap(err, "get device error")
	}

	// migrate nonces
	for _, n := range devNonces {
		da := DeviceActivation{
			DevEUI:   devEUI,
			JoinEUI:  joinEUI,
			DevAddr:  lorawan.DevAddr{},   // empty DevAddr, we don't have this information
			NwkSKey:  lorawan.AES128Key{}, // empty AES key, we don't have this information
			DevNonce: n,
		}
		err = CreateDeviceActivation(db, &da)
		if err != nil {
			return errors.Wrap(err, "create device-activation error")
		}
	}

	var ns NodeSession
	val, err := redis.Bytes(c.Do("GET", "lora:ns:session:"+devEUI.String()))
	if err != nil {
		if err == redis.ErrNil {
			// the node-session does not exist, nothing to migrate
			return nil
		}
		return errors.Wrap(err, "get error")
	}

	err = gob.NewDecoder(bytes.NewReader(val)).Decode(&ns)
	if err != nil {
		return errors.Wrap(err, "gob decode error")
	}

	ds := DeviceSession{
		RoutingProfileID: d.RoutingProfileID,
		DeviceProfileID:  d.DeviceProfileID,
		ServiceProfileID: d.ServiceProfileID,

		DevAddr:  ns.DevAddr,
		DevEUI:   ns.DevEUI,
		JoinEUI:  ns.AppEUI,
		NwkSKey:  ns.NwkSKey,
		FCntUp:   ns.FCntUp,
		FCntDown: ns.FCntDown,

		SkipFCntValidation: ns.RelaxFCnt,

		RXWindow:     ns.RXWindow,
		RXDelay:      ns.RXDelay,
		RX1DROffset:  ns.RX1DROffset,
		RX2DR:        ns.RX2DR,
		RX2Frequency: common.Band.RX2Frequency,

		TXPowerIndex: ns.TXPowerIndex,
		DR:           ns.DR,
		MaxSupportedTXPowerIndex: ns.MaxSupportedTXPowerIndex,
		MaxSupportedDR:           ns.MaxSupportedDR,
		NbTrans:                  ns.NbTrans,
		EnabledChannels:          ns.EnabledChannels,
		UplinkHistory:            ns.UplinkHistory,
	}

	for _, c := range ns.EnabledChannels {
		if c < len(common.Band.UplinkChannels) {
			ds.ChannelFrequencies = append(ds.ChannelFrequencies, common.Band.UplinkChannels[c].Frequency)
		}
	}

	err = SaveDeviceSession(p, ds)
	if err != nil {
		return errors.Wrap(err, "save device-session error")
	}

	return nil
}
