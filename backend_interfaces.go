package loraserver

import "github.com/brocaar/lorawan"

// GatewayBackend is the interface of a gateway backend.
// A gateway backend is responsible for the communication with the gateway.
type GatewayBackend interface {
	Send(TXPacket) error    // send the given packet to the gateway
	Receive() chan RXPacket // receive packets from the gateway
	Close() error           // close the gateway backend.
}

// ApplicationBackend is the interface of an application backend.
// An application backend is responsible forwarding data to the application
// and receiving data that should be sent to the node.
type ApplicationBackend interface {
	Send(lorawan.EUI64, RXPackets) error // send the payload of RXPackets to the application
	Receive() chan TXPacket              // receive packets from the application
	Close() error                        // close the application backend
}
