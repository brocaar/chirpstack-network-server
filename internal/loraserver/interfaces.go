package loraserver

import (
	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
)

// GatewayBackend is the interface of a gateway backend.
// A gateway backend is responsible for the communication with the gateway.
type GatewayBackend interface {
	Send(models.TXPacket) error         // send the given packet to the gateway
	RXPacketChan() chan models.RXPacket // channel containing the received packets
	Close() error                       // close the gateway backend.
}

// ApplicationBackend is the interface of an application backend.
// An application backend is responsible forwarding data to the application
// and receiving data that should be sent to the node.
type ApplicationBackend interface {
	Send(devEUI, appEUI lorawan.EUI64, payload models.RXPayload) error                           // send the given payload to the application
	Notify(devEUI, appEUI lorawan.EUI64, typ models.NotificationType, payload interface{}) error // send the given notification to the application
	TXPayloadChan() chan models.TXPayload                                                        // channel containing the received payloads from the application
	Close() error                                                                                // close the application backend
}
