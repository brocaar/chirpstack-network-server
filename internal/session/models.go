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
	DevAddr  lorawan.DevAddr   `json:"devAddr"`
	AppEUI   lorawan.EUI64     `json:"appEUI"`
	DevEUI   lorawan.EUI64     `json:"devEUI"`
	AppSKey  lorawan.AES128Key `json:"appSKey"`
	NwkSKey  lorawan.AES128Key `json:"nwkSKey"`
	FCntUp   uint32            `json:"fCntUp"`
	FCntDown uint32            `json:"fCntDown"`

	RXWindow    RXWindow `json:"rxWindow"`
	RXDelay     uint8    `json:"rxDelay"`
	RX1DROffset uint8    `json:"rx1DROffset"`
	RX2DR       uint8    `json:"rx2DR"`

	CFList *lorawan.CFList `json:"cFlist"`
}
