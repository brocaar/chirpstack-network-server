package queue

import "github.com/brocaar/lorawan"

// TXPayload contains the payload to be sent to the node.
type TXPayload struct {
	Confirmed bool          // indicates if the packet needs to be confirmed with an ACK
	DevEUI    lorawan.EUI64 // the DevEUI of the node to send the payload to
	FCnt      uint32        // the frame-counter which will be used to transmit this payload
	FPort     uint8         // the FPort
	Data      []byte        // the data to send (encrypted by the application-server)
}

// MACPayload contains data from a MAC command.
type MACPayload struct {
	Reference  string // TODO remove?
	FRMPayload bool   // indicating if the mac command was or must be sent as a FRMPayload (and thus encrypted)
	DevEUI     lorawan.EUI64
	Data       []byte
}
