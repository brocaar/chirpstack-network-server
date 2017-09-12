package downlink

import (
	"github.com/pkg/errors"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/lorawan"
)

func sendProprietaryDown(ctx *ProprietaryDownContext) error {
	if ctx.DR > len(common.Band.DataRates)-1 {
		return errors.Wrapf(ErrInvalidDataRate, "dr: %d (max dr: %d)", ctx.DR, len(common.Band.DataRates)-1)
	}

	phy := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			Major: lorawan.LoRaWANR1,
			MType: lorawan.Proprietary,
		},
		MACPayload: &lorawan.DataPayload{Bytes: ctx.MACPayload},
		MIC:        ctx.MIC,
	}

	for _, mac := range ctx.GatewayMACs {
		txInfo := gw.TXInfo{
			MAC:         mac,
			Immediately: true,
			Frequency:   ctx.Frequency,
			Power:       common.Band.DefaultTXPower,
			DataRate:    common.Band.DataRates[ctx.DR],
			CodeRate:    "4/5",
			IPol:        &ctx.IPol,
		}

		if err := common.Gateway.SendTXPacket(gw.TXPacket{
			TXInfo:     txInfo,
			PHYPayload: phy,
		}); err != nil {
			return errors.Wrap(err, "send tx packet to gateway error")
		}
	}

	return nil
}
