package loraserver

import (
	"fmt"

	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
)

// sendRXInfoPayload sends the RXInfoPayload to the network-controller.
func sendRXInfoPayload(ctx Context, appEUI, devEUI lorawan.EUI64, rxPackets RXPackets) error {
	if len(rxPackets) == 0 {
		return fmt.Errorf("length of rx packets must be at least 1")
	}

	macPL, ok := rxPackets[0].PHYPayload.MACPayload.(*lorawan.MACPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.MACPayload, got: %T", rxPackets[0].PHYPayload.MACPayload)
	}

	var rxInfo []models.RXInfo
	for _, rxPacket := range rxPackets {
		rxInfo = append(rxInfo, rxPacket.RXInfo)
	}

	rxInfoPayload := models.RXInfoPayload{
		DevEUI: devEUI,
		ADR:    macPL.FHDR.FCtrl.ADR,
		FCnt:   macPL.FHDR.FCnt,
		RXInfo: rxInfo,
	}

	if err := ctx.Controller.SendRXInfoPayload(appEUI, devEUI, rxInfoPayload); err != nil {
		return fmt.Errorf("send rx info payload error: %s", err)
	}
	return nil
}
