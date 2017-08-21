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
	"github.com/brocaar/loraserver/internal/maccommand"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/session"
	"github.com/brocaar/lorawan"
)

// collectJoinRequestPacket collects a single received RXPacket of type
// join-request.
func collectJoinRequestPacket(rxPacket gw.RXPacket) error {
	return collectAndCallOnce(common.RedisPool, rxPacket, func(rxPacket models.RXPacket) error {
		return handleCollectedJoinRequestPackets(rxPacket)
	})
}

// handleCollectedJoinRequestPackets handles the received join-requests.
func handleCollectedJoinRequestPackets(rxPacket models.RXPacket) error {
	var macs []string
	for _, p := range rxPacket.RXInfoSet {
		macs = append(macs, p.MAC.String())
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
		"gw_count": len(macs),
		"gw_macs":  strings.Join(macs, ", "),
		"mtype":    rxPacket.PHYPayload.MHDR.MType,
	}).Info("packet(s) collected")

	// get random DevAddr
	devAddr, err := session.GetRandomDevAddr(common.RedisPool, common.NetID)
	if err != nil {
		return fmt.Errorf("get random DevAddr error: %s", err)
	}

	cFList := common.Band.GetCFList()
	var cFListSlice []uint32
	if cFList != nil {
		for _, f := range cFList {
			cFListSlice = append(cFListSlice, f)
		}
	}
	joinResp, err := common.Application.JoinRequest(context.Background(), &as.JoinRequestRequest{
		PhyPayload: b,
		DevAddr:    devAddr[:],
		NetID:      common.NetID[:],
		CFList:     cFListSlice,
	})
	if err != nil {
		return fmt.Errorf("application server join-request error: %s", err)
	}

	// log the uplink frame
	// note we log it at this place to make sure the join-request has been
	// authenticated by the application-server
	logUplink(common.DB, jrPL.DevEUI, rxPacket)

	var downlinkPHY lorawan.PHYPayload
	if err = downlinkPHY.UnmarshalBinary(joinResp.PhyPayload); err != nil {
		errStr := fmt.Sprintf("downlink PHYPayload unmarshal error: %s", err)
		common.Application.HandleError(context.Background(), &as.HandleErrorRequest{
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
		DevAddr:            devAddr,
		AppEUI:             jrPL.AppEUI,
		DevEUI:             jrPL.DevEUI,
		NwkSKey:            nwkSKey,
		FCntUp:             0,
		FCntDown:           0,
		RelaxFCnt:          joinResp.DisableFCntCheck,
		RXWindow:           session.RXWindow(joinResp.RxWindow),
		RXDelay:            uint8(joinResp.RxDelay),
		RX1DROffset:        uint8(joinResp.Rx1DROffset),
		RX2DR:              uint8(joinResp.Rx2DR),
		EnabledChannels:    common.Band.GetUplinkChannels(),
		ADRInterval:        joinResp.AdrInterval,
		InstallationMargin: joinResp.InstallationMargin,
		LastRXInfoSet:      rxPacket.RXInfoSet,
	}

	if err = session.SaveNodeSession(common.RedisPool, ns); err != nil {
		return fmt.Errorf("save node-session error: %s", err)
	}

	if err = maccommand.FlushQueue(common.RedisPool, ns.DevEUI); err != nil {
		return fmt.Errorf("flush mac-command queue error: %s", err)
	}

	if err = downlink.SendJoinAcceptResponse(ns, rxPacket, downlinkPHY); err != nil {
		return fmt.Errorf("send join-accept response error: %s", err)
	}

	return nil
}
