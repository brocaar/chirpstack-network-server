package maccommand

import (
	"fmt"

	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
)

// Handle handles a MACCommand sent by a node.
func Handle(ds *storage.DeviceSession, dp storage.DeviceProfile, sp storage.ServiceProfile, asClient as.ApplicationServerServiceClient, block storage.MACCommandBlock, pending *storage.MACCommandBlock, rxPacket models.RXPacket) ([]storage.MACCommandBlock, error) {
	switch block.CID {
	case lorawan.LinkADRAns:
		return handleLinkADRAns(ds, block, pending)
	case lorawan.LinkCheckReq:
		return handleLinkCheckReq(ds, rxPacket)
	case lorawan.DevStatusAns:
		return handleDevStatusAns(ds, sp, asClient, block)
	case lorawan.PingSlotInfoReq:
		return handlePingSlotInfoReq(ds, block)
	case lorawan.PingSlotChannelAns:
		return handlePingSlotChannelAns(ds, block, pending)
	case lorawan.DeviceTimeReq:
		return handleDeviceTimeReq(ds, rxPacket)
	case lorawan.NewChannelAns:
		return handleNewChannelAns(ds, block, pending)
	case lorawan.RXParamSetupAns:
		return handleRXParamSetupAns(ds, block, pending)
	case lorawan.RXTimingSetupAns:
		return handleRXTimingSetupAns(ds, block, pending)
	case lorawan.RekeyInd:
		return handleRekeyInd(ds, block)
	case lorawan.ResetInd:
		return handleResetInd(ds, dp, block)
	case lorawan.RejoinParamSetupAns:
		return handleRejoinParamSetupAns(ds, block, pending)
	case lorawan.DeviceModeInd:
		return handleDeviceModeInd(ds, block)
	default:
		return nil, fmt.Errorf("undefined CID %d", block.CID)
	}
}
