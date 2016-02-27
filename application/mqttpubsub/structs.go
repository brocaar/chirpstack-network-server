package mqttpubsub

import (
	"github.com/brocaar/lorawan"
)

// ApplicationRXPayload contains the data sent to the application
// after a RXPacket was received by the server.
type ApplicationRXPayload struct {
	MType        lorawan.MType
	DevEUI       lorawan.EUI64
	ACK          bool
	FPort        uint8
	GatewayCount uint8

	Data []byte
}
