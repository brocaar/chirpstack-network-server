package backend

import "github.com/brocaar/loraserver/api/gw"

// Gateway is the interface of a gateway backend.
// A gateway backend is responsible for the communication with the gateway.
type Gateway interface {
	SendTXPacket(gw.DownlinkFrame) error                   // send the given packet to the gateway
	SendGatewayConfigPacket(gw.GatewayConfiguration) error // SendGatewayConfigPacket sends the given GatewayConfigPacket to the gateway.
	RXPacketChan() chan gw.UplinkFrame                     // channel containing the received packets
	StatsPacketChan() chan gw.GatewayStats                 // channel containing the received gateway stats
	Close() error                                          // close the gateway backend.
}
