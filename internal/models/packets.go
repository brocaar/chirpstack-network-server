package models

import "github.com/brocaar/loraserver/api/gw"

// RXPackets is a slice of RXPacket. It implements sort.Interface
// to sort the slice of packets by signal strength so that the
// packet received with the strongest signal will be at index 0
// of RXPackets.
type RXPackets []gw.RXPacket

// Len is part of sort.Interface.
func (p RXPackets) Len() int {
	return len(p)
}

// Swap is part of sort.Interface.
func (p RXPackets) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

// Less is part of sort.Interface.
func (p RXPackets) Less(i, j int) bool {
	return p[i].RXInfo.RSSI > p[j].RXInfo.RSSI
}
