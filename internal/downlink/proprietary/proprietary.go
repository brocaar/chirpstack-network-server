package proprietary

import (
	"context"
	"encoding/binary"

	"github.com/gofrs/uuid"
	"github.com/pkg/errors"

	"github.com/brocaar/lorawan"
	"github.com/brocaar/chirpstack-api/go/common"
	"github.com/brocaar/chirpstack-api/go/gw"
	"github.com/brocaar/chirpstack-network-server/internal/backend/gateway"
	"github.com/brocaar/chirpstack-network-server/internal/band"
	"github.com/brocaar/chirpstack-network-server/internal/config"
	"github.com/brocaar/chirpstack-network-server/internal/helpers"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
)

const defaultCodeRate = "4/5"

var tasks = []func(*proprietaryContext) error{
	sendProprietaryDown,
	saveFrame,
}

type proprietaryContext struct {
	ctx context.Context

	MACPayload     []byte
	MIC            lorawan.MIC
	GatewayMACs    []lorawan.EUI64
	IPol           bool
	Frequency      int
	DR             int
	DownlinkFrames []gw.DownlinkFrame
}

var (
	downlinkTXPower int
)

// Setup configures the package.
func Setup(conf config.Config) error {
	downlinkTXPower = conf.NetworkServer.NetworkSettings.DownlinkTXPower

	return nil
}

// Handle handles a proprietary downlink.
func Handle(ctx context.Context, macPayload []byte, mic lorawan.MIC, gwMACs []lorawan.EUI64, iPol bool, frequency, dr int) error {
	pctx := proprietaryContext{
		ctx:         ctx,
		MACPayload:  macPayload,
		MIC:         mic,
		GatewayMACs: gwMACs,
		IPol:        iPol,
		Frequency:   frequency,
		DR:          dr,
	}

	for _, t := range tasks {
		if err := t(&pctx); err != nil {
			return err
		}
	}

	return nil
}

func sendProprietaryDown(ctx *proprietaryContext) error {
	var txPower int
	if downlinkTXPower != -1 {
		txPower = downlinkTXPower
	} else {
		txPower = band.Band().GetDownlinkTXPower(ctx.Frequency)
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
		downID, err := uuid.NewV4()
		if err != nil {
			return errors.Wrap(err, "new uuid error")
		}
		token := binary.BigEndian.Uint16(downID[0:2])

		txInfo := gw.DownlinkTXInfo{
			GatewayId: mac[:],
			Frequency: uint32(ctx.Frequency),
			Power:     int32(txPower),

			Timing: gw.DownlinkTiming_IMMEDIATELY,
			TimingInfo: &gw.DownlinkTXInfo_ImmediatelyTimingInfo{
				ImmediatelyTimingInfo: &gw.ImmediatelyTimingInfo{},
			},
		}

		err = helpers.SetDownlinkTXInfoDataRate(&txInfo, ctx.DR, band.Band())
		if err != nil {
			return errors.Wrap(err, "set downlink tx-info data-rate error")
		}

		// for LoRa, set the iPol value
		if txInfo.Modulation == common.Modulation_LORA {
			modInfo := txInfo.GetLoraModulationInfo()
			if modInfo != nil {
				modInfo.PolarizationInversion = ctx.IPol
			}
		}

		df := gw.DownlinkFrame{
			Token:      uint32(token),
			DownlinkId: downID[:],
			TxInfo:     &txInfo,
			PhyPayload: phyB,
		}

		ctx.DownlinkFrames = append(ctx.DownlinkFrames, df)

		if err := gateway.Backend().SendTXPacket(df); err != nil {
			return errors.Wrap(err, "send downlink frame to gateway error")
		}
	}

	return nil
}

func saveFrame(ctx *proprietaryContext) error {
	for _, df := range ctx.DownlinkFrames {
		if err := storage.SaveDownlinkFrames(ctx.ctx, storage.RedisPool(), storage.DownlinkFrames{
			Token:          df.Token,
			DownlinkFrames: []*gw.DownlinkFrame{&df},
		}); err != nil {
			return errors.Wrap(err, "save downlink-frames error")
		}
	}

	return nil
}
