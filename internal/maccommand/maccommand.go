package maccommand

import (
	"fmt"

	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
)

// Handle handles a MACCommand sent by a node.
func Handle(ds *storage.DeviceSession, block storage.MACCommandBlock, pending *storage.MACCommandBlock, rxPacket models.RXPacket) ([]storage.MACCommandBlock, error) {
	switch block.CID {
	case lorawan.LinkADRAns:
		return handleLinkADRAns(ds, block, pending)
	case lorawan.LinkCheckReq:
		return handleLinkCheckReq(ds, rxPacket)
	case lorawan.DevStatusAns:
		return handleDevStatusAns(ds, block)
	case lorawan.PingSlotInfoReq:
		return handlePingSlotInfoReq(ds, block)
	case lorawan.PingSlotChannelAns:
		return handlePingSlotChannelAns(ds, block, pending)
	case lorawan.DeviceTimeReq:
		return handleDeviceTimeReq(ds, rxPacket)
	default:
		return nil, fmt.Errorf("undefined CID %d", block.CID)

	}
}
