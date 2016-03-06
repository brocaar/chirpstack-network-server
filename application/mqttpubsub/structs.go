package mqttpubsub

import (
	"github.com/brocaar/lorawan"
)

// ApplicationRXPayload contains the data sent to the application
// after a RXPacket was received by the server.
type ApplicationRXPayload struct {
	MType        lorawan.MType `json:"mType"`
	DevEUI       lorawan.EUI64 `json:"devEUI"`
	ACK          bool          `json:"ack"`
	FPort        uint8         `json:"fPort"`
	GatewayCount uint8         `json:"gatewayCount"`
	Data         []byte        `json:"data"`
}
