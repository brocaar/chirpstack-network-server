package queue

import "github.com/brocaar/lorawan"

// MACPayload contains data from a MAC command.
type MACPayload struct {
	FRMPayload bool // indicating if the mac command was or must be sent as a FRMPayload (and thus encrypted)
	DevEUI     lorawan.EUI64
	Data       []byte
}
