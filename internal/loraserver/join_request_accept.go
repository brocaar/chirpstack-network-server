package loraserver

import (
	"errors"
	"fmt"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
)

// validateAndCollectJoinRequestPacket validates and collects a single
// received RXPacket of type join-request.
func validateAndCollectJoinRequestPacket(ctx Context, rxPacket models.RXPacket) error {
	// MACPayload must be of type *lorawan.JoinRequestPayload
	jrPL, ok := rxPacket.PHYPayload.MACPayload.(*lorawan.JoinRequestPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.JoinRequestPayload, got: %T", rxPacket.PHYPayload.MACPayload)
	}

	// get node information for this DevEUI
	node, err := getNode(ctx.DB, jrPL.DevEUI)
	if err != nil {
		return err
	}

	// validate the MIC
	ok, err = rxPacket.PHYPayload.ValidateMIC(node.AppKey)
	if err != nil {
		return fmt.Errorf("validate MIC error: %s", err)
	}
	if !ok {
		return errors.New("invalid MIC")
	}

	return collectAndCallOnce(ctx.RedisPool, rxPacket, func(rxPackets RXPackets) error {
		return handleCollectedJoinRequestPackets(ctx, rxPackets)
	})

}

// handleCollectedJoinRequestPackets handles the received join-request.
func handleCollectedJoinRequestPackets(ctx Context, rxPackets RXPackets) error {
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

	log.WithFields(log.Fields{
		"dev_eui":  jrPL.DevEUI,
		"gw_count": len(rxPackets),
		"gw_macs":  strings.Join(macs, ", "),
		"mtype":    rxPackets[0].PHYPayload.MHDR.MType,
	}).Info("packet(s) collected")

	// get node information for this DevEUI
	node, err := getNode(ctx.DB, jrPL.DevEUI)
	if err != nil {
		return err
	}

	// validate the given nonce
	if !node.ValidateDevNonce(jrPL.DevNonce) {
		return fmt.Errorf("given dev-nonce %x has already been used before for node %s", jrPL.DevNonce, jrPL.DevEUI)
	}

	// get random (free) DevAddr
	devAddr, err := getRandomDevAddr(ctx.RedisPool, ctx.NetID)
	if err != nil {
		return fmt.Errorf("get random DevAddr error: %s", err)
	}

	// get app nonce
	appNonce, err := getAppNonce()
	if err != nil {
		return fmt.Errorf("get AppNonce error: %s", err)
	}

	// get keys
	nwkSKey, err := getNwkSKey(node.AppKey, ctx.NetID, appNonce, jrPL.DevNonce)
	if err != nil {
		return fmt.Errorf("get NwkSKey error: %s", err)
	}
	appSKey, err := getAppSKey(node.AppKey, ctx.NetID, appNonce, jrPL.DevNonce)
	if err != nil {
		return fmt.Errorf("get AppSKey error: %s", err)
	}

	ns := models.NodeSession{
		DevAddr:  devAddr,
		DevEUI:   jrPL.DevEUI,
		AppSKey:  appSKey,
		NwkSKey:  nwkSKey,
		FCntUp:   0,
		FCntDown: 0,

		AppEUI:  node.AppEUI,
		RXDelay: node.RXDelay,
	}
	if err = saveNodeSession(ctx.RedisPool, ns); err != nil {
		return fmt.Errorf("save node-session error: %s", err)
	}

	// update the node (with updated used dev-nonces)
	if err = updateNode(ctx.DB, node); err != nil {
		return fmt.Errorf("update node error: %s", err)
	}

	// construct the lorawan packet
	phy := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: lorawan.JoinAccept,
			Major: lorawan.LoRaWANR1,
		},
		MACPayload: &lorawan.JoinAcceptPayload{
			AppNonce: appNonce,
			NetID:    ctx.NetID,
			DevAddr:  ns.DevAddr,
			RXDelay:  ns.RXDelay,
		},
	}
	if err = phy.SetMIC(node.AppKey); err != nil {
		return fmt.Errorf("set MIC error: %s", err)
	}
	if err = phy.EncryptJoinAcceptPayload(node.AppKey); err != nil {
		return fmt.Errorf("encrypt join-accept error: %s", err)
	}

	// get TX DR
	uplinkDR, err := Band.GetDataRate(rxPacket.RXInfo.DataRate)
	if err != nil {
		return err
	}
	// get TX channel
	uplinkChannel, err := Band.GetChannel(rxPacket.RXInfo.Frequency, uplinkDR)
	if err != nil {
		return err
	}
	// get RX1 channel
	rx1Channel := Band.GetRX1Channel(uplinkChannel)
	// get RX1 DR
	rx1DR := Band.RX1DataRate[uplinkDR][0]

	txPacket := models.TXPacket{
		TXInfo: models.TXInfo{
			MAC:       rxPacket.RXInfo.MAC,
			Timestamp: rxPacket.RXInfo.Timestamp + uint32(Band.JoinAcceptDelay1/time.Microsecond),
			Frequency: Band.DownlinkChannels[rx1Channel].Frequency,
			Power:     Band.DefaultTXPower,
			DataRate:  Band.DataRates[rx1DR],
			CodeRate:  rxPacket.RXInfo.CodeRate,
		},
		PHYPayload: phy,
	}

	// window 1
	if err = ctx.Gateway.Send(txPacket); err != nil {
		return fmt.Errorf("send tx packet (rx window 1) to gateway error: %s", err)
	}

	// send a notification to the application that a node joined the network
	return ctx.Application.SendNotification(ns.DevEUI, ns.AppEUI, models.JoinNotificationType, models.JoinNotification{
		DevAddr: ns.DevAddr,
		DevEUI:  ns.DevEUI,
	})
}
