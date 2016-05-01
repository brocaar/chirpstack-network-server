package loraserver

import (
	"fmt"

	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
)

// sendRXInfoNotification sends the RXInfoNotification to the network-controller.
func sendRXInfoNotification(ctx Context, appEUI, devEUI lorawan.EUI64, rxPackets RXPackets) error {
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

	rxInfoNotification := models.RXInfoNotification{
		DevEUI: devEUI,
		ADR:    macPL.FHDR.FCtrl.ADR,
		FCnt:   macPL.FHDR.FCnt,
		RXInfo: rxInfo,
	}

	// TODO: should the backend be more generic, or should we create a separate
	// network-controller backend? For now we're using the Application backend
	// to send out the notification.
	if err := ctx.Application.SendNotification(devEUI, appEUI, models.RXInfoNotificationType, rxInfoNotification); err != nil {
		return fmt.Errorf("send rx info notification error: %s", err)
	}
	return nil
}
