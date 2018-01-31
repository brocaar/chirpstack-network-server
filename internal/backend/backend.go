package backend

import "github.com/Frankz/loraserver/api/gw"

// Gateway is the interface of a gateway backend.
// A gateway backend is responsible for the communication with the gateway.
type Gateway interface {
	SendTXPacket(gw.TXPacket) error              // send the given packet to the gateway
	RXPacketChan() chan gw.RXPacket              // channel containing the received packets
	StatsPacketChan() chan gw.GatewayStatsPacket // channel containing the received gateway stats
	Close() error                                // close the gateway backend.
}
