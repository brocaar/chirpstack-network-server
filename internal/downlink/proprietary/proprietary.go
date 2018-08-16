package proprietary

import (
	"crypto/rand"
	"encoding/binary"

	"github.com/pkg/errors"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/helpers"
	"github.com/brocaar/lorawan"
)

const defaultCodeRate = "4/5"

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
	var txPower int
	if config.C.NetworkServer.NetworkSettings.DownlinkTXPower != -1 {
		txPower = config.C.NetworkServer.NetworkSettings.DownlinkTXPower
	} else {
		txPower = config.C.NetworkServer.Band.Band.GetDownlinkTXPower(ctx.Frequency)
	}

	phy := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			Major: lorawan.LoRaWANR1,
			MType: lorawan.Proprietary,
		},
		MACPayload: &lorawan.DataPayload{Bytes: ctx.MACPayload},
		MIC:        ctx.MIC,
	}
	phyB, err := phy.MarshalBinary()
	if err != nil {
		return errors.Wrap(err, "marshal phypayload error")
	}

	for _, mac := range ctx.GatewayMACs {
		txInfo := gw.DownlinkTXInfo{
			GatewayId:   mac[:],
			Immediately: true,
			Frequency:   uint32(ctx.Frequency),
			Power:       int32(txPower),
		}

		err = helpers.SetDownlinkTXInfoDataRate(&txInfo, ctx.DR, config.C.NetworkServer.Band.Band)
		if err != nil {
			return errors.Wrap(err, "set downlink tx-info data-rate error")
		}

		if err := config.C.NetworkServer.Gateway.Backend.Backend.SendTXPacket(gw.DownlinkFrame{
			Token:      uint32(ctx.Token),
			TxInfo:     &txInfo,
			PhyPayload: phyB,
		}); err != nil {
			return errors.Wrap(err, "send tx packet to gateway error")
		}
	}

	return nil
}
