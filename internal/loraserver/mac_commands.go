package loraserver

import (
	log "github.com/Sirupsen/logrus"
	"github.com/brocaar/lorawan"
)

func handleMACCommands(ctx Context, appEUI, devEUI lorawan.EUI64, frmPayload bool, commands []lorawan.MACCommand) error {
	for _, cmd := range commands {
		log.WithFields(log.Fields{
			"dev_eui":     devEUI,
			"cid":         cmd.CID,
			"frm_payload": frmPayload,
		}).Info("mac command received")
	}
	return nil
}
