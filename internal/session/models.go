package session

import "github.com/brocaar/lorawan"

// RXWindow defines the RX window option.
type RXWindow int8

// Available RX window options.
const (
	RX1 = iota
	RX2
)

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

	CFList *lorawan.CFList
}
