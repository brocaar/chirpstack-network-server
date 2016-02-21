package loraserver

import "github.com/brocaar/lorawan"

// NodeSession contains the informatio of a node-session (an activated node).
type NodeSession struct {
	DevAddr  lorawan.DevAddr
	DevEUI   lorawan.EUI64
	AppSKey  lorawan.AES128Key
	NwkSKey  lorawan.AES128Key
	FCntUp   uint32
	FCntDown uint32
}
