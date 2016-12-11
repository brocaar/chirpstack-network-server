package maccommand

import "github.com/brocaar/lorawan"

// QueueItem contains data from a MAC command.
type QueueItem struct {
	FRMPayload bool // indicating if the mac command was or must be sent as a FRMPayload (and thus encrypted)
	DevEUI     lorawan.EUI64
	Data       []byte
}

// PendingItem contains a pending MAC command. In some cases we need to wait
// for the node the ACK a change before we can change for example the session.
type PendingItem struct {
	CID     lorawan.CID
	Payload lorawan.MACCommandPayload
}
