package session

import (
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/lorawan"
)

// RXWindow defines the RX window option.
type RXWindow int8

// Available RX window options.
const (
	RX1 = iota
	RX2
)

// UplinkHistory contains meta-data of a transmission.
type UplinkHistory struct {
	FCnt         uint32
	MaxSNR       float64
	GatewayCount int
}

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

	// NbTrans defines the number of transmissions for each unconfirmed uplink
	// frame. In case of 0, the default value is used.
	// This value is controlled by the ADR engine.
	NbTrans uint8

	EnabledChannels []int           // channels that are activated on the node
	UplinkHistory   []UplinkHistory // contains the last 20 transmissions
	LastRXInfoSet   []gw.RXInfo     // sorted set (best at index 0)
}

// AppendUplinkHistory appends an UplinkHistory item and makes sure the list
// never exceeds 20 records. In case more records are present, only the most
// recent ones will be preserved. In case of a re-transmission, the record with
// the best MaxSNR is stored.
func (b *NodeSession) AppendUplinkHistory(up UplinkHistory) {
	if count := len(b.UplinkHistory); count > 0 {
		// ignore re-transmissions we don't know the source of the
		// re-transmission (it might be a replay-attack)
		if b.UplinkHistory[count-1].FCnt == up.FCnt {
			return
		}
	}

	b.UplinkHistory = append(b.UplinkHistory, up)
	if count := len(b.UplinkHistory); count > 20 {
		b.UplinkHistory = b.UplinkHistory[count-20 : count]
	}
}

// GetPacketLossPercentage returns the percentage of packet-loss over the
// records stored in UplinkHistory.
func (b NodeSession) GetPacketLossPercentage() float64 {
	var lostPackets uint32
	var previousFCnt uint32

	for i, uh := range b.UplinkHistory {
		if i == 0 {
			previousFCnt = uh.FCnt
			continue
		}
		lostPackets += uh.FCnt - previousFCnt - 1 // there is always an expected difference of 1
		previousFCnt = uh.FCnt
	}

	return float64(lostPackets) / float64(len(b.UplinkHistory)) * 100
}
