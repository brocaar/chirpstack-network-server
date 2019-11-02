package maccommand

import (
	"context"
	"fmt"

	"github.com/brocaar/chirpstack-network-server/api/as"
	"github.com/brocaar/chirpstack-network-server/internal/models"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
)

// Handle handles a MACCommand sent by a node.
func Handle(ctx context.Context, ds *storage.DeviceSession, dp storage.DeviceProfile, sp storage.ServiceProfile, asClient as.ApplicationServerServiceClient, block storage.MACCommandBlock, pending *storage.MACCommandBlock, rxPacket models.RXPacket) ([]storage.MACCommandBlock, error) {
	switch block.CID {
	case lorawan.LinkADRAns:
		return handleLinkADRAns(ctx, ds, block, pending)
	case lorawan.LinkCheckReq:
		return handleLinkCheckReq(ctx, ds, rxPacket)
	case lorawan.DevStatusAns:
		return handleDevStatusAns(ctx, ds, sp, asClient, block)
	case lorawan.PingSlotInfoReq:
		return handlePingSlotInfoReq(ctx, ds, block)
	case lorawan.PingSlotChannelAns:
		return handlePingSlotChannelAns(ctx, ds, block, pending)
	case lorawan.DeviceTimeReq:
		return handleDeviceTimeReq(ctx, ds, rxPacket)
	case lorawan.NewChannelAns:
		return handleNewChannelAns(ctx, ds, block, pending)
	case lorawan.RXParamSetupAns:
		return handleRXParamSetupAns(ctx, ds, block, pending)
	case lorawan.TXParamSetupAns:
		return handleTXParamSetupAns(ctx, ds, block, pending)
	case lorawan.RXTimingSetupAns:
		return handleRXTimingSetupAns(ctx, ds, block, pending)
	case lorawan.RekeyInd:
		return handleRekeyInd(ctx, ds, block)
	case lorawan.ResetInd:
		return handleResetInd(ctx, ds, dp, block)
	case lorawan.RejoinParamSetupAns:
		return handleRejoinParamSetupAns(ctx, ds, block, pending)
	case lorawan.DeviceModeInd:
		return handleDeviceModeInd(ctx, ds, block)
	default:
		return nil, fmt.Errorf("undefined CID %d", block.CID)
	}
}
