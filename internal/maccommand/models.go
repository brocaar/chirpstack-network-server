package maccommand

import "github.com/brocaar/lorawan"

// QueueItem contains data from a MAC command.
type QueueItem struct {
	FRMPayload bool // indicating if the mac command was or must be sent as a FRMPayload (and thus encrypted)
	DevEUI     lorawan.EUI64
	Data       []byte
}
