package maccommand

import (
	"fmt"

	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/pkg/errors"
)

func handleLinkCheckReq(ds *storage.DeviceSession, rxPacket models.RXPacket) ([]storage.MACCommandBlock, error) {
	if len(rxPacket.RXInfoSet) == 0 {
		return nil, errors.New("rx info-set contains zero items")
	}

	requiredSNR, ok := config.SpreadFactorToRequiredSNRTable[rxPacket.TXInfo.DataRate.SpreadFactor]
	if !ok {
		return nil, fmt.Errorf("sf %d not in sf to required snr table", rxPacket.TXInfo.DataRate.SpreadFactor)
	}

	margin := rxPacket.RXInfoSet[0].LoRaSNR - requiredSNR
	if margin < 0 {
		margin = 0
	}

	block := storage.MACCommandBlock{
		CID: lorawan.LinkCheckAns,
		MACCommands: storage.MACCommands{
			{
				CID: lorawan.LinkCheckAns,
				Payload: &lorawan.LinkCheckAnsPayload{
					Margin: uint8(margin),
					GwCnt:  uint8(len(rxPacket.RXInfoSet)),
				},
			},
		},
	}

	return []storage.MACCommandBlock{block}, nil
}
