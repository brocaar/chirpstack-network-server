package loraserver

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
)

// Server represents a LoRaWAN network-server.
type Server struct {
	ctx Context
	wg  sync.WaitGroup
}

// NewServer creates a new server.
func NewServer(ctx Context) *Server {
	return &Server{
		ctx: ctx,
	}
}

// Start starts the server.
func (s *Server) Start() error {
	log.WithFields(log.Fields{
		"net_id": s.ctx.NetID,
		"nwk_id": hex.EncodeToString([]byte{s.ctx.NetID.NwkID()}),
	}).Info("starting loraserver")

	go func() {
		s.wg.Add(1)
		handleRXPackets(s.ctx)
		s.wg.Done()
	}()
	go func() {
		s.wg.Add(1)
		handleTXPayloads(s.ctx)
		s.wg.Done()
	}()
	return nil
}

// Stop closes the gateway and application backends and waits for the
// server to complete the pending packets and actions.
func (s *Server) Stop() error {
	if err := s.ctx.Gateway.Close(); err != nil {
		return fmt.Errorf("close gateway backend error: %s", err)
	}
	if err := s.ctx.Application.Close(); err != nil {
		return fmt.Errorf("close application backend error: %s", err)
	}

	log.Info("waiting for pending packets to complete")
	s.wg.Wait()
	return nil
}

func handleTXPayloads(ctx Context) {
	var wg sync.WaitGroup
	for txPayload := range ctx.Application.TXPayloadChan() {
		go func(txPayload models.TXPayload) {
			wg.Add(1)
			if err := addTXPayloadToQueue(ctx.RedisPool, txPayload); err != nil {
				log.WithField("dev_eui", txPayload.DevEUI).Errorf("add tx payload to queue error: %s", err)
			}
			wg.Done()
		}(txPayload)
	}
	wg.Wait()
}

func handleRXPackets(ctx Context) {
	var wg sync.WaitGroup
	for rxPacket := range ctx.Gateway.RXPacketChan() {
		go func(rxPacket models.RXPacket) {
			wg.Add(1)
			if err := handleRXPacket(ctx, rxPacket); err != nil {
				data, _ := rxPacket.PHYPayload.MarshalText()
				log.WithField("data_base64", data).Errorf("processing rx packet error: %s", err)
			}
			wg.Done()
		}(rxPacket)
	}
	wg.Wait()
}

func handleRXPacket(ctx Context, rxPacket models.RXPacket) error {
	switch rxPacket.PHYPayload.MHDR.MType {
	case lorawan.JoinRequest:
		return validateAndCollectJoinRequestPacket(ctx, rxPacket)
	case lorawan.UnconfirmedDataUp, lorawan.ConfirmedDataUp:
		return validateAndCollectDataUpRXPacket(ctx, rxPacket)
	default:
		return fmt.Errorf("unknown MType: %v", rxPacket.PHYPayload.MHDR.MType)
	}
}

func validateAndCollectDataUpRXPacket(ctx Context, rxPacket models.RXPacket) error {
	// MACPayload must be of type *lorawan.MACPayload
	macPL, ok := rxPacket.PHYPayload.MACPayload.(*lorawan.MACPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.MACPayload, got: %T", rxPacket.PHYPayload.MACPayload)
	}

	// get the session data
	ns, err := getNodeSession(ctx.RedisPool, macPL.FHDR.DevAddr)
	if err != nil {
		return err
	}

	// validate and get the full int32 FCnt
	fullFCnt, ok := ns.ValidateAndGetFullFCntUp(macPL.FHDR.FCnt)
	if !ok {
		log.WithFields(log.Fields{
			"dev_addr":    macPL.FHDR.DevAddr,
			"dev_eui":     ns.DevEUI,
			"packet_fcnt": macPL.FHDR.FCnt,
			"server_fcnt": ns.FCntUp,
		}).Warning("invalid FCnt")
		return errors.New("invalid FCnt or too many dropped frames")
	}
	macPL.FHDR.FCnt = fullFCnt

	// validate MIC
	micOK, err := rxPacket.PHYPayload.ValidateMIC(ns.NwkSKey)
	if err != nil {
		return fmt.Errorf("validate MIC error: %s", err)
	}
	if !micOK {
		return errors.New("invalid MIC")
	}

	if macPL.FPort != nil {
		if *macPL.FPort == 0 {
			// decrypt FRMPayload with NwkSKey when FPort == 0
			if err := rxPacket.PHYPayload.DecryptFRMPayload(ns.NwkSKey); err != nil {
				return fmt.Errorf("decrypt FRMPayload error: %s", err)
			}
		} else {
			if err := rxPacket.PHYPayload.DecryptFRMPayload(ns.AppSKey); err != nil {
				return fmt.Errorf("decrypt FRMPayload error: %s", err)
			}
		}
	}

	return collectAndCallOnce(ctx.RedisPool, rxPacket, func(rxPackets RXPackets) error {
		return handleCollectedDataUpPackets(ctx, rxPackets)
	})
}

func handleCollectedDataUpPackets(ctx Context, rxPackets RXPackets) error {
	if len(rxPackets) == 0 {
		return errors.New("packet collector returned 0 packets")
	}
	rxPacket := rxPackets[0]

	var macs []string
	for _, p := range rxPackets {
		macs = append(macs, p.RXInfo.MAC.String())
	}

	macPL, ok := rxPacket.PHYPayload.MACPayload.(*lorawan.MACPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.MACPayload, got: %T", rxPacket.PHYPayload.MACPayload)
	}

	ns, err := getNodeSession(ctx.RedisPool, macPL.FHDR.DevAddr)
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"dev_eui":  ns.DevEUI,
		"gw_count": len(rxPackets),
		"gw_macs":  strings.Join(macs, ", "),
		"mtype":    rxPackets[0].PHYPayload.MHDR.MType,
	}).Info("packet(s) collected")

	if macPL.FPort != nil {
		if *macPL.FPort == 0 {
			log.Warn("todo: implement FPort == 0 packets")
		} else {
			var data []byte

			// it is possible that the FRMPayload is empty, in this case only
			// the FPort will be send
			if len(macPL.FRMPayload) == 1 {
				dataPL, ok := macPL.FRMPayload[0].(*lorawan.DataPayload)
				if !ok {
					return errors.New("FRMPayload must be of type *lorawan.DataPayload")
				}
				data = dataPL.Bytes
			}

			err = ctx.Application.Send(ns.DevEUI, ns.AppEUI, models.RXPayload{
				DevEUI:       ns.DevEUI,
				GatewayCount: len(rxPackets),
				FPort:        *macPL.FPort,
				RSSI:         rxPacket.RXInfo.RSSI,
				Data:         data,
			})
			if err != nil {
				return fmt.Errorf("send rx payload to application error: %s", err)
			}
		}
	}

	// sync counter with that of the device
	ns.FCntUp = macPL.FHDR.FCnt
	if err := saveNodeSession(ctx.RedisPool, ns); err != nil {
		return err
	}

	// handle downlink (ACK)
	time.Sleep(CollectDataDownWait)
	if err := handleDataDownReply(ctx, rxPacket, ns); err != nil {
		return fmt.Errorf("handling downlink data for node %s failed: %s", ns.DevEUI, err)
	}

	return nil
}

func handleDataDownReply(ctx Context, rxPacket models.RXPacket, ns models.NodeSession) error {
	macPL, ok := rxPacket.PHYPayload.MACPayload.(*lorawan.MACPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.MACPayload, got: %T", rxPacket.PHYPayload.MACPayload)
	}

	// the last payload was received by the node
	if macPL.FHDR.FCtrl.ACK {
		if err := clearInProcessTXPayload(ctx.RedisPool, ns.DevEUI); err != nil {
			return err
		}
		ns.FCntDown++
		if err := saveNodeSession(ctx.RedisPool, ns); err != nil {
			return err
		}
	}

	// check if there are payloads pending in the queue
	txPayload, remaining, err := getTXPayloadAndRemainingFromQueue(ctx.RedisPool, ns.DevEUI)

	// errEmptyQueue should not be handled as an error, it just means there
	// is no queue / the queue is empty
	if err != nil && err != errEmptyQueue {
		return err
	}

	// nothing pending in the queue and no need to ACK RXPacket
	if rxPacket.PHYPayload.MHDR.MType != lorawan.ConfirmedDataUp && err == errEmptyQueue {
		return nil
	}

	phy := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: lorawan.UnconfirmedDataDown,
			Major: lorawan.LoRaWANR1,
		},
	}
	macPL = &lorawan.MACPayload{
		FHDR: lorawan.FHDR{
			DevAddr: ns.DevAddr,
			FCtrl: lorawan.FCtrl{
				ACK: rxPacket.PHYPayload.MHDR.MType == lorawan.ConfirmedDataUp, // set ACK to true when received packet needs an ACK
			},
			FCnt: ns.FCntDown,
		},
	}
	phy.MACPayload = macPL

	// add the payload from the queue
	if err == nil {
		macPL.FHDR.FCtrl.FPending = remaining

		if txPayload.Confirmed {
			phy.MHDR.MType = lorawan.ConfirmedDataDown
		}
		macPL.FPort = &txPayload.FPort
		macPL.FRMPayload = []lorawan.Payload{
			&lorawan.DataPayload{Bytes: txPayload.Data},
		}
		if err := phy.EncryptFRMPayload(ns.AppSKey); err != nil {
			return fmt.Errorf("encrypt FRMPayload error: %s", err)
		}

		// remove the payload from the queue when not confirmed
		if !txPayload.Confirmed {
			if err := clearInProcessTXPayload(ctx.RedisPool, ns.DevEUI); err != nil {
				return err
			}
		}
	}

	if err := phy.SetMIC(ns.NwkSKey); err != nil {
		return fmt.Errorf("set MIC error: %s", err)
	}

	dr, err := band.GetDataRate(rxPacket.RXInfo.DataRate)
	if err != nil {
		return err
	}
	rx1Frequency, err := band.GetRX1Frequency(rxPacket.RXInfo.Frequency, dr)
	if err != nil {
		return err
	}

	txPacket := models.TXPacket{
		TXInfo: models.TXInfo{
			MAC:       rxPacket.RXInfo.MAC,
			Timestamp: rxPacket.RXInfo.Timestamp + uint32(band.ReceiveDelay1/time.Microsecond),
			Frequency: rx1Frequency,
			Power:     band.DefaultTXPower,
			DataRate:  band.DataRateConfiguration[band.RX1DROffsetConfiguration[dr][0]], // currently offset is not configurable
			CodeRate:  rxPacket.RXInfo.CodeRate,
		},
		PHYPayload: phy,
	}

	// window 1
	if err := ctx.Gateway.Send(txPacket); err != nil {
		return fmt.Errorf("send tx packet (rx window 1) to gateway error: %s", err)
	}

	// window 2
	txPacket.TXInfo.Timestamp = rxPacket.RXInfo.Timestamp + uint32(band.ReceiveDelay2/time.Microsecond)
	txPacket.TXInfo.Frequency = band.RX2Frequency
	txPacket.TXInfo.DataRate = band.DataRateConfiguration[band.RX2DataRate]
	if err := ctx.Gateway.Send(txPacket); err != nil {
		return fmt.Errorf("send tx packet (rx window 2) to gateway error: %s", err)
	}

	// increment the FCntDown when MType != ConfirmedDataDown. In case of
	// ConfirmedDataDown we increment on ACK.
	if phy.MHDR.MType != lorawan.ConfirmedDataDown {
		ns.FCntDown++
		if err := saveNodeSession(ctx.RedisPool, ns); err != nil {
			return err
		}
	}

	return nil
}

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

		AppEUI: node.AppEUI,
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
			DevAddr:  devAddr,
		},
	}
	if err = phy.SetMIC(node.AppKey); err != nil {
		return fmt.Errorf("set MIC error: %s", err)
	}
	if err = phy.EncryptJoinAcceptPayload(node.AppKey); err != nil {
		return fmt.Errorf("encrypt join-accept error: %s", err)
	}

	dr, err := band.GetDataRate(rxPacket.RXInfo.DataRate)
	if err != nil {
		return err
	}
	rx1Frequency, err := band.GetRX1Frequency(rxPacket.RXInfo.Frequency, dr)
	if err != nil {
		return err
	}

	txPacket := models.TXPacket{
		TXInfo: models.TXInfo{
			MAC:       rxPacket.RXInfo.MAC,
			Timestamp: rxPacket.RXInfo.Timestamp + uint32(band.JoinAcceptDelay1/time.Microsecond),
			Frequency: rx1Frequency,
			Power:     band.DefaultTXPower,
			DataRate:  band.DataRateConfiguration[band.RX1DROffsetConfiguration[dr][0]], // currently offset is not configurable
			CodeRate:  rxPacket.RXInfo.CodeRate,
		},
		PHYPayload: phy,
	}

	// window 1
	if err = ctx.Gateway.Send(txPacket); err != nil {
		return fmt.Errorf("send tx packet (rx window 1) to gateway error: %s", err)
	}

	// window 2
	txPacket.TXInfo.Timestamp = rxPacket.RXInfo.Timestamp + uint32(band.JoinAcceptDelay2/time.Microsecond)
	txPacket.TXInfo.Frequency = band.RX2Frequency
	txPacket.TXInfo.DataRate = band.DataRateConfiguration[band.RX2DataRate]
	if err = ctx.Gateway.Send(txPacket); err != nil {
		return fmt.Errorf("send tx packet (rx window 2) to gateway error: %s", err)
	}

	// send a notification to the application that a node joined the network
	return ctx.Application.Notify(ns.DevEUI, ns.AppEUI, models.JoinNotification, models.JoinNotificationPayload{
		DevEUI: ns.DevEUI,
		Time:   time.Now(),
	})
}
