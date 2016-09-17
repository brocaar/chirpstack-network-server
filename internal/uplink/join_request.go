package uplink

import (
	"context"
	"errors"
	"fmt"
	"strings"

	log "github.com/Sirupsen/logrus"

	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/downlink"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/session"
	"github.com/brocaar/lorawan"
)

// collectJoinRequestPacket collects a single received RXPacket of type
// join-request.
func collectJoinRequestPacket(ctx common.Context, rxPacket gw.RXPacket) error {
	return collectAndCallOnce(ctx.RedisPool, rxPacket, func(rxPackets models.RXPackets) error {
		return handleCollectedJoinRequestPackets(ctx, rxPackets)
	})
}

// handleCollectedJoinRequestPackets handles the received join-requests.
func handleCollectedJoinRequestPackets(ctx common.Context, rxPackets models.RXPackets) error {
	if len(rxPackets) == 0 {
		return errors.New("packet collector returned 0 packets")
	}
	rxPacket := rxPackets[0]

	var macs []string
	for _, p := range rxPackets {
		macs = append(macs, p.RXInfo.MAC.String())
	}

	// MACPayload must be of type *lorawan.JoinRequestPayload
	jrPL, ok := rxPacket.PHYPayload.MACPayload.(*lorawan.JoinRequestPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.JoinRequestPayload, got: %T", rxPacket.PHYPayload.MACPayload)
	}

	b, err := rxPacket.PHYPayload.MarshalBinary()
	if err != nil {
		return fmt.Errorf("phypayload marshal binary error: %s", err)
	}

	log.WithFields(log.Fields{
		"dev_eui":  jrPL.DevEUI,
		"gw_count": len(rxPackets),
		"gw_macs":  strings.Join(macs, ", "),
		"mtype":    rxPackets[0].PHYPayload.MHDR.MType,
	}).Info("packet(s) collected")

	// get random (free) DevAddr
	devAddr, err := session.GetRandomDevAddr(ctx.RedisPool, ctx.NetID)
	if err != nil {
		return fmt.Errorf("get random DevAddr error: %s", err)
	}

	joinResp, err := ctx.Application.JoinRequest(context.Background(), &as.JoinRequestRequest{
		PhyPayload: b,
		DevAddr:    devAddr[:],
		NetID:      ctx.NetID[:],
	})
	if err != nil {
		return fmt.Errorf("application server join-request error: %s", err)
	}

	var cFList lorawan.CFList
	if len(joinResp.CFList) > len(cFList) {
		errStr := fmt.Sprintf("max CFlist size %d, got %d", len(cFList), len(joinResp.CFList))
		ctx.Application.HandleError(context.Background(), &as.HandleErrorRequest{
			AppEUI: jrPL.AppEUI[:],
			DevEUI: jrPL.DevEUI[:],
			Type:   as.ErrorType_OTAA,
			Error:  errStr,
		})
		return errors.New(errStr)
	}
	for i, cf := range joinResp.CFList {
		cFList[i] = cf
	}

	var downlinkPHY lorawan.PHYPayload
	if err = downlinkPHY.UnmarshalBinary(joinResp.PhyPayload); err != nil {
		errStr := fmt.Sprintf("downlink PHYPayload unmarshal error: %s", err)
		ctx.Application.HandleError(context.Background(), &as.HandleErrorRequest{
			AppEUI: jrPL.AppEUI[:],
			DevEUI: jrPL.DevEUI[:],
			Type:   as.ErrorType_OTAA,
			Error:  errStr,
		})
		return errors.New(errStr)
	}

	var nwkSKey lorawan.AES128Key
	copy(nwkSKey[:], joinResp.NwkSKey)

	ns := session.NodeSession{
		DevAddr:     devAddr,
		AppEUI:      jrPL.AppEUI,
		DevEUI:      jrPL.DevEUI,
		NwkSKey:     nwkSKey,
		FCntUp:      0,
		FCntDown:    0,
		RXWindow:    session.RXWindow(joinResp.RxWindow),
		RXDelay:     uint8(joinResp.RxDelay),
		RX1DROffset: uint8(joinResp.Rx1DROffset),
		RX2DR:       uint8(joinResp.Rx2DR),
		CFList:      &cFList,
	}

	if err = session.CreateNodeSession(ctx.RedisPool, ns); err != nil {
		return fmt.Errorf("create node-session error: %s", err)
	}

	if err = downlink.SendJoinAcceptResponse(ctx, ns, rxPackets, downlinkPHY); err != nil {
		return fmt.Errorf("send join-accept response error: %s", err)
	}

	return nil
}
