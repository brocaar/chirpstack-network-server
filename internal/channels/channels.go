package channels

import (
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
)

// HandleChannelReconfigure handles the reconfiguration of active channels
// on the node. This is needed in case only a sub-set of channels is used
// (e.g. for the US band) or when a reconfiguration of active channels
// happens.
func HandleChannelReconfigure(ds storage.DeviceSession) ([]storage.MACCommandBlock, error) {
	payloads := config.C.NetworkServer.Band.Band.GetLinkADRReqPayloadsForEnabledChannels(ds.EnabledChannels)
	if len(payloads) == 0 {
		return nil, nil
	}

	payloads[len(payloads)-1].TXPower = uint8(ds.TXPowerIndex)
	payloads[len(payloads)-1].DataRate = uint8(ds.DR)
	payloads[len(payloads)-1].Redundancy.NbRep = ds.NbTrans

	block := storage.MACCommandBlock{
		CID: lorawan.LinkADRReq,
	}
	for i := range payloads {
		block.MACCommands = append(block.MACCommands, lorawan.MACCommand{
			CID:     lorawan.LinkADRReq,
			Payload: &payloads[i],
		})
	}

	return []storage.MACCommandBlock{block}, nil
}
