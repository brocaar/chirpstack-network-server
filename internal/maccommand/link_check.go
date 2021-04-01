package maccommand

import (
	"context"
	"fmt"

	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-network-server/v3/internal/config"
	"github.com/brocaar/chirpstack-network-server/v3/internal/models"
	"github.com/brocaar/chirpstack-network-server/v3/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/pkg/errors"
)

func handleLinkCheckReq(ctx context.Context, ds *storage.DeviceSession, rxPacket models.RXPacket) ([]storage.MACCommandBlock, error) {
	if len(rxPacket.RXInfoSet) == 0 {
		return nil, errors.New("rx info-set contains zero items")
	}

	if rxPacket.TXInfo.Modulation != common.Modulation_LORA {
		return nil, fmt.Errorf("modulation %s not supported for LinkCheckReq mac-command", rxPacket.TXInfo.Modulation)
	}

	modInfo := rxPacket.TXInfo.GetLoraModulationInfo()
	if modInfo == nil {
		return nil, errors.New("lora_modulation_info must not be nil")
	}

	requiredSNR, ok := config.SpreadFactorToRequiredSNRTable[int(modInfo.SpreadingFactor)]
	if !ok {
		return nil, fmt.Errorf("sf %d not in sf to required snr table", modInfo.SpreadingFactor)
	}

	margin := rxPacket.RXInfoSet[0].LoraSnr - requiredSNR
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
