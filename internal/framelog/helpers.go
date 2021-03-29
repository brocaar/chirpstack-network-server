package framelog

import (
	"github.com/pkg/errors"

	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-api/go/v3/ns"
	"github.com/brocaar/chirpstack-network-server/internal/models"
	"github.com/brocaar/lorawan"
)

// CreateUplinkFrameLog creates a UplinkFrameLog.
func CreateUplinkFrameLog(rxPacket models.RXPacket) (ns.UplinkFrameLog, error) {
	b, err := rxPacket.PHYPayload.MarshalBinary()
	if err != nil {
		return ns.UplinkFrameLog{}, errors.Wrap(err, "marshal phypayload error")
	}

	var protoMType common.MType
	switch rxPacket.PHYPayload.MHDR.MType {
	case lorawan.JoinRequest:
		protoMType = common.MType_JoinRequest
	case lorawan.RejoinRequest:
		protoMType = common.MType_RejoinRequest
	case lorawan.UnconfirmedDataUp:
		protoMType = common.MType_UnconfirmedDataUp
	case lorawan.ConfirmedDataUp:
		protoMType = common.MType_ConfirmedDataUp
	case lorawan.Proprietary:
		protoMType = common.MType_Proprietary
	}

	return ns.UplinkFrameLog{
		PhyPayload: b,
		TxInfo:     rxPacket.TXInfo,
		RxInfo:     rxPacket.RXInfoSet,
		MType:      protoMType,
	}, nil
}
