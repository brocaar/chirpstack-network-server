package loraserver

import (
	"errors"
	"fmt"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/brocaar/lorawan"
)

// Start starts the loraserver.
func Start(ctx Context) error {
	log.Info("starting loraserver")
	handleRXPackets(ctx)
	return nil
}

func handleRXPackets(ctx Context) {
	for rxPacket := range ctx.Gateway.RXPacketChan() {
		go func(rxPacket RXPacket) {
			if err := handleRXPacket(ctx, rxPacket); err != nil {
				log.Errorf("error while processing RXPacket: %s", err)
			}
		}(rxPacket)
	}
}

func handleRXPacket(ctx Context, rxPacket RXPacket) error {
	switch rxPacket.PHYPayload.MHDR.MType {
	case lorawan.JoinRequest:
		return errors.New("TODO: implement JoinRequest packet handling")
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
	ns, err := GetNodeSession(ctx.RedisPool, macPL.FHDR.DevAddr)
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

	if macPL.FPort == 0 {
		// decrypt FRMPayload with NwkSKey when FPort == 0
		if err := macPL.DecryptFRMPayload(ns.NwkSKey); err != nil {
			return err
		}
	} else {
		if err := macPL.DecryptFRMPayload(ns.AppSKey); err != nil {
			return err
		}
	}
	rxPacket.PHYPayload.MACPayload = macPL

	return CollectAndCallOnce(ctx.RedisPool, rxPacket, func(rxPackets RXPackets) error {
		return handleCollectedRXPackets(ctx, rxPackets)
	})
}

func handleCollectedRXPackets(ctx Context, rxPackets RXPackets) error {
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

	ns, err := GetNodeSession(ctx.RedisPool, macPL.FHDR.DevAddr)
	if err != nil {
		return fmt.Errorf("could not get node-session: %s", err)
	}

	if macPL.FPort == 0 {
		log.Warn("todo: implement FPort == 0 packets")
	} else {
		if err := ctx.Application.Send(ns.DevEUI, ns.AppEUI, rxPackets); err != nil {
			return fmt.Errorf("could not send RXPackets to application: %s", err)
		}
	}

	// increment counter
	ns.FCntUp++
	if err := SaveNodeSession(ctx.RedisPool, ns); err != nil {
		return fmt.Errorf("could not update node-session: %s", err)
	}

	return nil
}
