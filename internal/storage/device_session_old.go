package storage

import (
	"time"

	"github.com/brocaar/loraserver/internal/band"
	"github.com/brocaar/lorawan"
	loraband "github.com/brocaar/lorawan/band"
	"github.com/gofrs/uuid"
)

// DeviceSessionOld defines the "old" device-session struct.
// This struct is needed to gob decode "old" device-sessions.
type DeviceSessionOld struct {
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

	// ADR defines if the device has ADR enabled.
	ADR bool

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

	// LastDevStatusBattery contains the last received battery status.
	LastDevStatusBattery uint8

	// LastDevStatusMargin contains the last received margin status.
	LastDevStatusMargin int8

	// LastDownlinkTX contains the timestamp of the last downlink.
	LastDownlinkTX time.Time

	// Class-B related configuration.
	BeaconLocked      bool
	PingSlotNb        int
	PingSlotDR        int
	PingSlotFrequency int
}

func migrateDeviceSessionOld(d DeviceSessionOld) DeviceSession {
	dpID, _ := uuid.FromString(d.DeviceProfileID)
	spID, _ := uuid.FromString(d.ServiceProfileID)
	rpID, _ := uuid.FromString(d.RoutingProfileID)

	out := DeviceSession{
		MACVersion: "1.0.2",

		DeviceProfileID:  dpID,
		ServiceProfileID: spID,
		RoutingProfileID: rpID,

		DevAddr:     d.DevAddr,
		DevEUI:      d.DevEUI,
		JoinEUI:     d.JoinEUI,
		FNwkSIntKey: d.NwkSKey,
		SNwkSIntKey: d.NwkSKey,
		NwkSEncKey:  d.NwkSKey,
		FCntUp:      d.FCntUp,
		NFCntDown:   d.FCntDown,

		SkipFCntValidation: d.SkipFCntValidation,

		RXDelay:      d.RXDelay,
		RX1DROffset:  d.RX1DROffset,
		RX2DR:        d.RX2DR,
		RX2Frequency: d.RX2Frequency,

		TXPowerIndex:             d.TXPowerIndex,
		DR:                       d.DR,
		ADR:                      d.ADR,
		MaxSupportedTXPowerIndex: d.MaxSupportedTXPowerIndex,
		NbTrans:                  d.NbTrans,
		EnabledChannels:          d.EnabledChannels,
		EnabledUplinkChannels:    d.EnabledUplinkChannels,
		ExtraUplinkChannels:      d.ExtraUplinkChannels,
		ChannelFrequencies:       d.ChannelFrequencies,
		UplinkHistory:            d.UplinkHistory,
		LastDevStatusRequested:   d.LastDevStatusRequested,
		LastDownlinkTX:           d.LastDownlinkTX,
		BeaconLocked:             d.BeaconLocked,
		PingSlotNb:               d.PingSlotNb,
		PingSlotDR:               d.PingSlotDR,
		PingSlotFrequency:        d.PingSlotFrequency,
	}

	if len(out.EnabledUplinkChannels) == 0 {
		out.EnabledUplinkChannels = out.EnabledChannels
	}

	if out.ExtraUplinkChannels == nil {
		out.ExtraUplinkChannels = make(map[int]loraband.Channel)
	}

	if out.RX2Frequency == 0 {
		out.RX2Frequency = band.Band().GetDefaults().RX2Frequency
	}

	return out
}
