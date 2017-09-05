package uplink

import (
	"context"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/pkg/errors"

	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/downlink"
	"github.com/brocaar/loraserver/internal/maccommand"
	"github.com/brocaar/loraserver/internal/session"
	"github.com/brocaar/lorawan"
)

func setContextFromJoinRequestPHYPayload(ctx *JoinRequestContext) error {
	jrPL, ok := ctx.RXPacket.PHYPayload.MACPayload.(*lorawan.JoinRequestPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.JoinRequestPayload, got: %T", ctx.RXPacket.PHYPayload.MACPayload)
	}
	ctx.JoinRequestPayload = jrPL

	return nil
}

func logJoinRequestFramesCollected(ctx *JoinRequestContext) error {
	var macs []string
	for _, p := range ctx.RXPacket.RXInfoSet {
		macs = append(macs, p.MAC.String())
	}

	log.WithFields(log.Fields{
		"dev_eui":  ctx.JoinRequestPayload.DevEUI,
		"gw_count": len(macs),
		"gw_macs":  strings.Join(macs, ", "),
		"mtype":    ctx.RXPacket.PHYPayload.MHDR.MType,
	}).Info("packet(s) collected")

	return nil
}

func getRandomDevAddr(ctx *JoinRequestContext) error {
	devAddr, err := session.GetRandomDevAddr(common.RedisPool, common.NetID)
	if err != nil {
		return errors.Wrap(err, "get random DevAddr error")
	}
	ctx.DevAddr = devAddr

	return nil
}

func getOptionalCFList(ctx *JoinRequestContext) error {
	cFList := common.Band.GetCFList()
	if cFList != nil {
		for _, f := range cFList {
			ctx.CFList = append(ctx.CFList, f)
		}
	}
	return nil
}

func getJoinAcceptFromAS(ctx *JoinRequestContext) error {
	b, err := ctx.RXPacket.PHYPayload.MarshalBinary()
	if err != nil {
		return errors.Wrap(err, "PHYPayload marshal binary error")
	}

	joinResp, err := common.Application.JoinRequest(context.Background(), &as.JoinRequestRequest{
		PhyPayload: b,
		DevAddr:    ctx.DevAddr[:],
		NetID:      common.NetID[:],
		CFList:     ctx.CFList,
	})
	if err != nil {
		return errors.Wrap(err, "application server join-request error")
	}

	ctx.JoinRequestResponse = joinResp

	return nil
}

func logJoinRequestFrame(ctx *JoinRequestContext) error {
	logUplink(common.DB, ctx.JoinRequestPayload.DevEUI, ctx.RXPacket)
	return nil
}

func createNodeSession(ctx *JoinRequestContext) error {
	var nwkSKey lorawan.AES128Key
	copy(nwkSKey[:], ctx.JoinRequestResponse.NwkSKey)

	ctx.NodeSession = session.NodeSession{
		DevAddr:            ctx.DevAddr,
		AppEUI:             ctx.JoinRequestPayload.AppEUI,
		DevEUI:             ctx.JoinRequestPayload.DevEUI,
		NwkSKey:            nwkSKey,
		FCntUp:             0,
		FCntDown:           0,
		RelaxFCnt:          ctx.JoinRequestResponse.DisableFCntCheck,
		RXWindow:           session.RXWindow(ctx.JoinRequestResponse.RxWindow),
		RXDelay:            uint8(ctx.JoinRequestResponse.RxDelay),
		RX1DROffset:        uint8(ctx.JoinRequestResponse.Rx1DROffset),
		RX2DR:              uint8(ctx.JoinRequestResponse.Rx2DR),
		EnabledChannels:    common.Band.GetUplinkChannels(),
		ADRInterval:        ctx.JoinRequestResponse.AdrInterval,
		InstallationMargin: ctx.JoinRequestResponse.InstallationMargin,
		LastRXInfoSet:      ctx.RXPacket.RXInfoSet,
	}

	if err := session.SaveNodeSession(common.RedisPool, ctx.NodeSession); err != nil {
		return errors.Wrap(err, "save node-session error")
	}

	if err := maccommand.FlushQueue(common.RedisPool, ctx.NodeSession.DevEUI); err != nil {
		return fmt.Errorf("flush mac-command queue error: %s", err)
	}

	return nil
}

func sendJoinAcceptDownlink(ctx *JoinRequestContext) error {
	var phy lorawan.PHYPayload
	if err := phy.UnmarshalBinary(ctx.JoinRequestResponse.PhyPayload); err != nil {
		errStr := fmt.Sprintf("downlink PHYPayload unmarshal error: %s", err)
		common.Application.HandleError(context.Background(), &as.HandleErrorRequest{
			AppEUI: ctx.JoinRequestPayload.AppEUI[:],
			DevEUI: ctx.JoinRequestPayload.DevEUI[:],
			Type:   as.ErrorType_OTAA,
			Error:  errStr,
		})
		return errors.New(errStr)
	}

	if err := downlink.Flow.RunJoinResponse(ctx.NodeSession, phy); err != nil {
		return errors.Wrap(err, "run join-response flow error")
	}

	return nil
}
