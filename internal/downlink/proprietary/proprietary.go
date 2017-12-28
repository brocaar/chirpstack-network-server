package proprietary

import (
	"crypto/rand"
	"encoding/binary"

	"github.com/pkg/errors"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/lorawan"
)

var tasks = []func(*proprietaryContext) error{
	setToken,
	sendProprietaryDown,
}

type proprietaryContext struct {
	Token       uint16
	MACPayload  []byte
	MIC         lorawan.MIC
	GatewayMACs []lorawan.EUI64
	IPol        bool
	Frequency   int
	DR          int
}

// Handle handles a proprietary downlink.
func Handle(macPayload []byte, mic lorawan.MIC, gwMACs []lorawan.EUI64, iPol bool, frequency, dr int) error {
	ctx := proprietaryContext{
		MACPayload:  macPayload,
		MIC:         mic,
		GatewayMACs: gwMACs,
		IPol:        iPol,
		Frequency:   frequency,
		DR:          dr,
	}

	for _, t := range tasks {
		if err := t(&ctx); err != nil {
			return err
		}
	}

	return nil
}

func setToken(ctx *proprietaryContext) error {
	b := make([]byte, 2)
	_, err := rand.Read(b)
	if err != nil {
		return errors.Wrap(err, "read random erro")
	}
	ctx.Token = binary.BigEndian.Uint16(b)
	return nil
}

func sendProprietaryDown(ctx *proprietaryContext) error {
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
			Token:      ctx.Token,
			TXInfo:     txInfo,
			PHYPayload: phy,
		}); err != nil {
			return errors.Wrap(err, "send tx packet to gateway error")
		}
	}

	return nil
}
