package channels

import (
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/maccommand"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/pkg/errors"
)

// HandleChannelReconfigure handles the reconfiguration of active channels
// on the node. This is needed in case only a sub-set of channels is used
// (e.g. for the US band) or when a reconfiguration of active channels
// happens.
func HandleChannelReconfigure(ds storage.DeviceSession, rxPacket models.RXPacket) error {
	payloads := config.C.NetworkServer.Band.Band.GetLinkADRReqPayloadsForEnabledChannels(ds.EnabledChannels)
	if len(payloads) == 0 {
		return nil
	}

	// set the current tx-power, data-rate and nbrep on the last payload
	currentDR, err := config.C.NetworkServer.Band.Band.GetDataRate(rxPacket.TXInfo.DataRate)
	if err != nil {
		return errors.Wrap(err, "get data-rate error")
	}

	payloads[len(payloads)-1].TXPower = uint8(ds.TXPowerIndex)
	payloads[len(payloads)-1].DataRate = uint8(currentDR)
	payloads[len(payloads)-1].Redundancy.NbRep = ds.NbTrans

	// when reconfiguring the channels requires more than 3 commands, we must
	// send these as FRMPayload as the FOpts has a max of 15 bytes and each
	// command requires 5 bytes.
	var frmPayload bool
	if len(payloads) > 3 {
		frmPayload = true
	}

	block := maccommand.Block{
		CID:        lorawan.LinkADRReq,
		FRMPayload: frmPayload,
	}
	for i := range payloads {
		block.MACCommands = append(block.MACCommands, lorawan.MACCommand{
			CID:     lorawan.LinkADRReq,
			Payload: &payloads[i],
		})
	}

	if err = maccommand.AddQueueItem(config.C.Redis.Pool, ds.DevEUI, block); err != nil {
		return errors.Wrap(err, "add mac-command block to queue error")
	}

	return nil
}
