package gateway

import "github.com/brocaar/chirpstack-api/go/gw"

var backend Gateway

// Backend returns the gateway backend.
func Backend() Gateway {
	return backend
}

// SetBackend sets the given gateway backend.
func SetBackend(b Gateway) {
	backend = b
}

// Gateway is the interface of a gateway backend.
// A gateway backend is responsible for the communication with the gateway.
type Gateway interface {
	SendTXPacket(gw.DownlinkFrame) error                   // send the given packet to the gateway
	SendGatewayConfigPacket(gw.GatewayConfiguration) error // SendGatewayConfigPacket sends the given GatewayConfigPacket to the gateway.
	RXPacketChan() chan gw.UplinkFrame                     // channel containing the received packets
	StatsPacketChan() chan gw.GatewayStats                 // channel containing the received gateway stats
	DownlinkTXAckChan() chan gw.DownlinkTXAck              // channel containing the downlink tx acknowledgements
	Close() error                                          // close the gateway backend.
}
