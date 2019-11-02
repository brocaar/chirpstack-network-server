package framelog

import (
	"github.com/pkg/errors"

	"github.com/brocaar/chirpstack-network-server/api/gw"
	"github.com/brocaar/chirpstack-network-server/internal/models"
)

// CreateUplinkFrameSet creates a UplinkFrameSet.
func CreateUplinkFrameSet(rxPacket models.RXPacket) (gw.UplinkFrameSet, error) {
	b, err := rxPacket.PHYPayload.MarshalBinary()
	if err != nil {
		return gw.UplinkFrameSet{}, errors.Wrap(err, "marshal phypayload error")
	}

	return gw.UplinkFrameSet{
		PhyPayload: b,
		TxInfo:     rxPacket.TXInfo,
		RxInfo:     rxPacket.RXInfoSet,
	}, nil
}
