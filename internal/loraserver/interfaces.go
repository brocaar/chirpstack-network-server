package loraserver

import (
	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
)

// GatewayBackend is the interface of a gateway backend.
// A gateway backend is responsible for the communication with the gateway.
type GatewayBackend interface {
	SendTXPacket(models.TXPacket) error // send the given packet to the gateway
	RXPacketChan() chan models.RXPacket // channel containing the received packets
	Close() error                       // close the gateway backend.
}

// ApplicationBackend is the interface of an application backend.
// An application backend is responsible for forwarding data to the application
// and receiving data that should be sent to the node.
type ApplicationBackend interface {
	SendRXPayload(appEUI, devEUI lorawan.EUI64, payload models.RXPayload) error                            // send the given payload to the application
	SendNotification(appEUI, devEUI lorawan.EUI64, typ models.NotificationType, payload interface{}) error // send the given notification to the application
	TXPayloadChan() chan models.TXPayload                                                                  // channel containing the received payloads from the application
	Close() error                                                                                          // close the application backend
}

// NetworkControllerBackend is the interface of a network-controller backend.
// A network-controller backend is responsible for forwarding RX info and MAC
// command data.
type NetworkControllerBackend interface {
	SendRXInfoPayload(appEUI, devEUI lorawan.EUI64, payload models.RXInfoPayload) error // send the given RXInfoPayload to the network-controller
	SendMACPayload(appEUI, devEUI lorawan.EUI64, payload models.MACPayload) error       // send the given MACPayload to the network-controller
	SendErrorPayload(appEUI, devEUI lorawan.EUI64, payload models.ErrorPayload) error   // send the given ErrorPayload to the network-controller
	TXMACPayloadChan() chan models.MACPayload                                           // returns channel MACPayload items to send to the nodes
	Close() error                                                                       // close the network-controller backend
}
