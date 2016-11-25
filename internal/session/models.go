package session

import "github.com/brocaar/lorawan"

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

// NodeSession contains the informatio of a node-session (an activated node).
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

	UplinkHistory []UplinkHistory // contains the last 20 transmissions
	CFList        *lorawan.CFList
}

// AppendUplinkHistory appends an UplinkHistory item and makes sure the list
// never exceeds 20 records. In case more records are present, only the most
// recent ones will be preserved.
func (b *NodeSession) AppendUplinkHistory(up UplinkHistory) {
	b.UplinkHistory = append(b.UplinkHistory, up)
	if count := len(b.UplinkHistory); count > 20 {
		b.UplinkHistory = b.UplinkHistory[count-20 : count]
	}
}
