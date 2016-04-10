package loraserver

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
)

var (
	errDoesNotExist = errors.New("object does not exist")
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
// server to complete the pending packets.
func (s *Server) Stop() error {
	if err := s.ctx.Gateway.Close(); err != nil {
		return err
	}
	if err := s.ctx.Application.Close(); err != nil {
		return err
	}

	log.Info("waiting for pending packets to complete")
	s.wg.Wait()
	return nil
}

func handleTXPayloads(ctx Context) {
	var wg sync.WaitGroup
	for txPayload := range ctx.Application.TXPayloadChan() {
		go func(txPayload TXPayload) {
			wg.Add(1)
			if err := addTXPayloadToQueue(ctx.RedisPool, txPayload); err != nil {
				log.Errorf("could not add TXPayload to queue: %s", err)
			}
			wg.Done()
		}(txPayload)
	}
	wg.Wait()
}

func handleRXPackets(ctx Context) {
	var wg sync.WaitGroup
	for rxPacket := range ctx.Gateway.RXPacketChan() {
		go func(rxPacket RXPacket) {
			wg.Add(1)
			if err := handleRXPacket(ctx, rxPacket); err != nil {
				log.Errorf("error while processing RXPacket: %s", err)
			}
			wg.Done()
		}(rxPacket)
	}
	wg.Wait()
}

func handleRXPacket(ctx Context, rxPacket RXPacket) error {
	switch rxPacket.PHYPayload.MHDR.MType {
	case lorawan.JoinRequest:
		return validateAndCollectJoinRequestPacket(ctx, rxPacket)
	case lorawan.UnconfirmedDataUp, lorawan.ConfirmedDataUp:
		return validateAndCollectDataUpRXPacket(ctx, rxPacket)
	default:
		return fmt.Errorf("unknown MType: %v", rxPacket.PHYPayload.MHDR.MType)
	}
}

func validateAndCollectDataUpRXPacket(ctx Context, rxPacket RXPacket) error {
	// MACPayload must be of type *lorawan.MACPayload
	macPL, ok := rxPacket.PHYPayload.MACPayload.(*lorawan.MACPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.MACPayload, got: %T", rxPacket.PHYPayload.MACPayload)
	}

	// get the session data
	ns, err := getNodeSession(ctx.RedisPool, macPL.FHDR.DevAddr)
	if err != nil {
		return fmt.Errorf("could not get node-session: %s", err)
	}

	// validate and get the full int32 FCnt
	fullFCnt, ok := ns.ValidateAndGetFullFCntUp(macPL.FHDR.FCnt)
	if !ok {
		log.WithFields(log.Fields{
			"packet_fcnt": macPL.FHDR.FCnt,
			"server_fcnt": ns.FCntUp,
		}).Warning("invalid FCnt")
		return errors.New("invalid FCnt or too many dropped frames")
	}
	macPL.FHDR.FCnt = fullFCnt

	// validate MIC
	micOK, err := rxPacket.PHYPayload.ValidateMIC(ns.NwkSKey)
	if err != nil {
		return err
	}
	if !micOK {
		return errors.New("invalid MIC")
	}

	if macPL.FPort != nil {
		if *macPL.FPort == 0 {
			// decrypt FRMPayload with NwkSKey when FPort == 0
			if err := macPL.DecryptFRMPayload(ns.NwkSKey); err != nil {
				return err
			}
		} else {
			if err := macPL.DecryptFRMPayload(ns.AppSKey); err != nil {
				return err
			}
		}
	}
	rxPacket.PHYPayload.MACPayload = macPL

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

	log.WithFields(log.Fields{
		"gw_count": len(rxPackets),
		"gw_macs":  strings.Join(macs, ", "),
		"mtype":    rxPackets[0].PHYPayload.MHDR.MType,
	}).Info("packet(s) collected")

	macPL, ok := rxPacket.PHYPayload.MACPayload.(*lorawan.MACPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.MACPayload, got: %T", rxPacket.PHYPayload.MACPayload)
	}

	ns, err := getNodeSession(ctx.RedisPool, macPL.FHDR.DevAddr)
	if err != nil {
		return fmt.Errorf("could not get node-session: %s", err)
	}

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

			err = ctx.Application.Send(ns.DevEUI, ns.AppEUI, RXPayload{
				DevEUI:       ns.DevEUI,
				GatewayCount: len(rxPackets),
				FPort:        *macPL.FPort,
				Data:         data,
			})
			if err != nil {
				return fmt.Errorf("could not send RXPacket to application: %s", err)
			}
		}
	}

	// sync counter with that of the device
	ns.FCntUp = macPL.FHDR.FCnt
	if err := saveNodeSession(ctx.RedisPool, ns); err != nil {
		return fmt.Errorf("could not update node-session: %s", err)
	}

	// handle downlink (ACK)
	time.Sleep(CollectDataDownWait)
	return handleDataDownReply(ctx, rxPacket, ns)
}

func handleDataDownReply(ctx Context, rxPacket RXPacket, ns NodeSession) error {
	macPL, ok := rxPacket.PHYPayload.MACPayload.(*lorawan.MACPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.MACPayload, got: %T", rxPacket.PHYPayload.MACPayload)
	}

	// the last payload was received by the node
	if macPL.FHDR.FCtrl.ACK {
		if err := clearInProcessTXPayload(ctx.RedisPool, ns.DevEUI); err != nil {
			return fmt.Errorf("could not clear in-process TXPayload: %s", err)
		}
		ns.FCntDown++
		if err := saveNodeSession(ctx.RedisPool, ns); err != nil {
			return fmt.Errorf("could not update node-session: %s", err)
		}
	}

	// check if there are payloads pending in the queue
	txPayload, remaining, err := getTXPayloadAndRemainingFromQueue(ctx.RedisPool, ns.DevEUI)

	// errDoesNotExist should not be handled as an error, it just means there
	// is no queue / the queue is empty
	if err != nil && err != errDoesNotExist {
		return fmt.Errorf("could not get TXPayload from queue: %s", err)
	}

	// nothing pending in the queue and no need to ACK RXPacket
	if rxPacket.PHYPayload.MHDR.MType != lorawan.ConfirmedDataUp && err == errDoesNotExist {
		return nil
	}

	phy := lorawan.NewPHYPayload(false)
	phy.MHDR = lorawan.MHDR{
		MType: lorawan.UnconfirmedDataDown,
		Major: lorawan.LoRaWANR1,
	}
	macPL = lorawan.NewMACPayload(false)
	macPL.FHDR = lorawan.FHDR{
		DevAddr: ns.DevAddr,
		FCtrl: lorawan.FCtrl{
			ACK: rxPacket.PHYPayload.MHDR.MType == lorawan.ConfirmedDataUp, // set ACK to true when received packet needs an ACK
		},
		FCnt: ns.FCntDown,
	}

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
		if err := macPL.EncryptFRMPayload(ns.AppSKey); err != nil {
			return fmt.Errorf("could not encrypt FRMPayload: %s", err)
		}

		// remove the payload from the queue when not confirmed
		if !txPayload.Confirmed {
			if err := clearInProcessTXPayload(ctx.RedisPool, ns.DevEUI); err != nil {
				return err
			}
		}
	}

	phy.MACPayload = macPL
	if err := phy.SetMIC(ns.NwkSKey); err != nil {
		return fmt.Errorf("could not set MIC: %s", err)
	}

	txPacket := TXPacket{
		TXInfo: TXInfo{
			MAC:       rxPacket.RXInfo.MAC,
			Timestamp: rxPacket.RXInfo.Timestamp + uint32(band.ReceiveDelay1/time.Microsecond),
			Frequency: rxPacket.RXInfo.Frequency,
			Power:     band.DefaultTXPower,
			DataRate:  rxPacket.RXInfo.DataRate,
			CodeRate:  rxPacket.RXInfo.CodeRate,
		},
		PHYPayload: phy,
	}

	// window 1
	if err := ctx.Gateway.Send(txPacket); err != nil {
		return fmt.Errorf("sending TXPacket to the gateway failed: %s", err)
	}

	// window 2
	txPacket.TXInfo.Timestamp = rxPacket.RXInfo.Timestamp + uint32(band.ReceiveDelay2/time.Microsecond)
	txPacket.TXInfo.Frequency = band.RX2Frequency
	txPacket.TXInfo.DataRate = band.DataRateConfiguration[band.RX2DataRate]
	if err := ctx.Gateway.Send(txPacket); err != nil {
		return fmt.Errorf("sending TXPacket to the gateway failed: %s", err)
	}

	// increment the FCntDown when MType != ConfirmedDataDown. In case of
	// ConfirmedDataDown we increment on ACK.
	if phy.MHDR.MType != lorawan.ConfirmedDataDown {
		ns.FCntDown++
		if err := saveNodeSession(ctx.RedisPool, ns); err != nil {
			return fmt.Errorf("could not update node-session: %s", err)
		}
	}

	return nil
}

func validateAndCollectJoinRequestPacket(ctx Context, rxPacket RXPacket) error {
	// MACPayload must be of type *lorawan.JoinRequestPayload
	jrPL, ok := rxPacket.PHYPayload.MACPayload.(*lorawan.JoinRequestPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.JoinRequestPayload, got: %T", rxPacket.PHYPayload.MACPayload)
	}

	// get node information for this DevEUI
	node, err := getNode(ctx.DB, jrPL.DevEUI)
	if err != nil {
		return fmt.Errorf("could not get node: %s", err)
	}

	// validate the MIC
	ok, err = rxPacket.PHYPayload.ValidateMIC(node.AppKey)
	if err != nil {
		return fmt.Errorf("could not validate MIC: %s", err)
	}
	if !ok {
		return errors.New("invalid mic")
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

	log.WithFields(log.Fields{
		"gw_count": len(rxPackets),
		"gw_macs":  strings.Join(macs, ", "),
		"mtype":    rxPackets[0].PHYPayload.MHDR.MType,
	}).Info("packet(s) collected")

	// MACPayload must be of type *lorawan.JoinRequestPayload
	jrPL, ok := rxPacket.PHYPayload.MACPayload.(*lorawan.JoinRequestPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.JoinRequestPayload, got: %T", rxPacket.PHYPayload.MACPayload)
	}

	// get node information for this DevEUI
	node, err := getNode(ctx.DB, jrPL.DevEUI)
	if err != nil {
		return fmt.Errorf("could not get node: %s", err)
	}

	// validate the given nonce
	if !node.ValidateDevNonce(jrPL.DevNonce) {
		return errors.New("the given dev-nonce has already been used before")
	}

	// get random (free) DevAddr
	devAddr, err := getRandomDevAddr(ctx.RedisPool, ctx.NetID)
	if err != nil {
		return fmt.Errorf("could not get random DevAddr: %s", err)
	}

	// get app nonce
	appNonce, err := getAppNonce()
	if err != nil {
		return fmt.Errorf("could not get AppNonce: %s", err)
	}

	// get keys
	nwkSKey, err := getNwkSKey(node.AppKey, ctx.NetID, appNonce, jrPL.DevNonce)
	if err != nil {
		return fmt.Errorf("could not get NwkSKey: %s", err)
	}
	appSKey, err := getAppSKey(node.AppKey, ctx.NetID, appNonce, jrPL.DevNonce)
	if err != nil {
		return fmt.Errorf("could not get AppSKey: %s", err)
	}

	ns := NodeSession{
		DevAddr:  devAddr,
		DevEUI:   jrPL.DevEUI,
		AppSKey:  appSKey,
		NwkSKey:  nwkSKey,
		FCntUp:   0,
		FCntDown: 0,

		AppEUI: node.AppEUI,
	}
	if err = saveNodeSession(ctx.RedisPool, ns); err != nil {
		return fmt.Errorf("could not save node-session: %s", err)
	}

	// update the node (with updated used dev-nonces)
	if err = updateNode(ctx.DB, node); err != nil {
		return fmt.Errorf("could not update the node: %s", err)
	}

	// construct the lorawan packet
	phy := lorawan.NewPHYPayload(false)
	phy.MHDR = lorawan.MHDR{
		MType: lorawan.JoinAccept,
		Major: lorawan.LoRaWANR1,
	}
	phy.MACPayload = &lorawan.JoinAcceptPayload{
		AppNonce: appNonce,
		NetID:    ctx.NetID,
		DevAddr:  devAddr,
	}
	if err = phy.SetMIC(node.AppKey); err != nil {
		return fmt.Errorf("could not set MIC: %s", err)
	}
	if err = phy.EncryptJoinAcceptPayload(node.AppKey); err != nil {
		return fmt.Errorf("could not encrypt join-accept: %s", err)
	}

	txPacket := TXPacket{
		TXInfo: TXInfo{
			MAC:       rxPacket.RXInfo.MAC,
			Timestamp: rxPacket.RXInfo.Timestamp + uint32(band.JoinAcceptDelay1/time.Microsecond),
			Frequency: rxPacket.RXInfo.Frequency,
			Power:     band.DefaultTXPower,
			DataRate:  rxPacket.RXInfo.DataRate,
			CodeRate:  rxPacket.RXInfo.CodeRate,
		},
		PHYPayload: phy,
	}

	// window 1
	if err = ctx.Gateway.Send(txPacket); err != nil {
		return fmt.Errorf("sending TXPacket to the gateway failed: %s", err)
	}

	// window 2
	txPacket.TXInfo.Timestamp = rxPacket.RXInfo.Timestamp + uint32(band.JoinAcceptDelay2/time.Microsecond)
	txPacket.TXInfo.Frequency = band.RX2Frequency
	txPacket.TXInfo.DataRate = band.DataRateConfiguration[band.RX2DataRate]
	if err = ctx.Gateway.Send(txPacket); err != nil {
		return fmt.Errorf("sending TXPacket to the gateway failed: %s", err)
	}

	return nil
}
