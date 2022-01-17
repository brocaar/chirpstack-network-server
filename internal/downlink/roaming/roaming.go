package roaming

import (
	"context"
	"encoding/binary"
	"sort"
	"time"

	"github.com/gofrs/uuid"
	"github.com/golang/protobuf/ptypes"
	"github.com/pkg/errors"

	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-network-server/v3/internal/backend/gateway"
	"github.com/brocaar/chirpstack-network-server/v3/internal/band"
	"github.com/brocaar/chirpstack-network-server/v3/internal/helpers"
	"github.com/brocaar/chirpstack-network-server/v3/internal/models"
	"github.com/brocaar/chirpstack-network-server/v3/internal/storage"
	"github.com/brocaar/lorawan/backend"
)

type bySignal []*gw.UplinkRXInfo

func (s bySignal) Len() int {
	return len(s)
}

func (s bySignal) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s bySignal) Less(i, j int) bool {
	if s[i].LoraSnr == s[j].LoraSnr {
		return s[i].Rssi > s[j].Rssi
	}

	return s[i].LoraSnr > s[j].LoraSnr
}

type emitPRDownlinkContext struct {
	ctx             context.Context
	dr              int
	rxPacket        models.RXPacket
	phyPayload      []byte
	dlMetaData      backend.DLMetaData
	downlinkGateway storage.DeviceGatewayRXInfo
	downlinkFrame   gw.DownlinkFrame
}

func EmitPRDownlink(ctx context.Context, rxPacket models.RXPacket, phy []byte, dlMetaData backend.DLMetaData) error {
	cctx := emitPRDownlinkContext{
		ctx:        ctx,
		rxPacket:   rxPacket,
		phyPayload: phy,
		dlMetaData: dlMetaData,
	}

	for _, f := range []func() error{
		cctx.setDownlinkGateway,
		cctx.setDownlinkFrame,
		cctx.saveDownlinkFrame,
		cctx.sendDownlinkFrame,
	} {
		if err := f(); err != nil {
			return err
		}
	}

	return nil
}

func (ctx *emitPRDownlinkContext) setDownlinkGateway() error {
	if len(ctx.rxPacket.RXInfoSet) == 0 {
		return errors.New("rx-info must not be empty")
	}

	sort.Sort(bySignal(ctx.rxPacket.RXInfoSet))
	ctx.downlinkGateway = storage.DeviceGatewayRXInfo{
		RSSI:    int(ctx.rxPacket.RXInfoSet[0].Rssi),
		LoRaSNR: ctx.rxPacket.RXInfoSet[0].LoraSnr,
		Antenna: ctx.rxPacket.RXInfoSet[0].Antenna,
		Board:   ctx.rxPacket.RXInfoSet[0].Board,
		Context: ctx.rxPacket.RXInfoSet[0].Context,
	}

	copy(ctx.downlinkGateway.GatewayID[:], ctx.rxPacket.RXInfoSet[0].GatewayId)

	return nil
}

func (ctx *emitPRDownlinkContext) setDownlinkFrame() error {
	id, err := uuid.NewV4()
	if err != nil {
		return errors.Wrap(err, "new uuid error")
	}

	ctx.downlinkFrame = gw.DownlinkFrame{
		DownlinkId: id[:],
		Token:      uint32(binary.BigEndian.Uint16(id[0:2])),
		GatewayId:  ctx.downlinkGateway.GatewayID[:],
	}

	if ctx.dlMetaData.ClassMode != nil && *ctx.dlMetaData.ClassMode == "A" {
		if ctx.dlMetaData.DLFreq1 != nil && ctx.dlMetaData.DataRate1 != nil && ctx.dlMetaData.RXDelay1 != nil {
			item := gw.DownlinkFrameItem{
				PhyPayload: ctx.phyPayload,
				TxInfo: &gw.DownlinkTXInfo{
					Frequency: uint32(*ctx.dlMetaData.DLFreq1 * 1000000),
					Timing:    gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Duration(*ctx.dlMetaData.RXDelay1) * time.Second),
						},
					},
					Context: ctx.downlinkGateway.Context,
				},
			}

			item.TxInfo.Power = int32(band.Band().GetDownlinkTXPower(item.TxInfo.Frequency))
			if err := helpers.SetDownlinkTXInfoDataRate(item.TxInfo, *ctx.dlMetaData.DataRate1, band.Band()); err != nil {
				return errors.Wrap(err, "set txinfo data-rate error")
			}

			ctx.downlinkFrame.Items = append(ctx.downlinkFrame.Items, &item)
		}

		if ctx.dlMetaData.DLFreq2 != nil && ctx.dlMetaData.DataRate2 != nil && ctx.dlMetaData.RXDelay1 != nil {
			item := gw.DownlinkFrameItem{
				PhyPayload: ctx.phyPayload,
				TxInfo: &gw.DownlinkTXInfo{
					Frequency: uint32(*ctx.dlMetaData.DLFreq2 * 1000000),
					Timing:    gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: ptypes.DurationProto(time.Duration(*ctx.dlMetaData.RXDelay1+1) * time.Second),
						},
					},
					Context: ctx.downlinkGateway.Context,
				},
			}

			item.TxInfo.Power = int32(band.Band().GetDownlinkTXPower(item.TxInfo.Frequency))
			if err := helpers.SetDownlinkTXInfoDataRate(item.TxInfo, *ctx.dlMetaData.DataRate2, band.Band()); err != nil {
				return errors.Wrap(err, "set txinfo data-rate error")
			}

			ctx.downlinkFrame.Items = append(ctx.downlinkFrame.Items, &item)
		}
	}

	return nil
}

func (ctx *emitPRDownlinkContext) saveDownlinkFrame() error {
	df := storage.DownlinkFrame{
		Token:         ctx.downlinkFrame.Token,
		DownlinkFrame: &ctx.downlinkFrame,
	}

	if err := storage.SaveDownlinkFrame(ctx.ctx, &df); err != nil {
		return errors.Wrap(err, "save downlink-frame error")
	}

	return nil
}

func (ctx *emitPRDownlinkContext) sendDownlinkFrame() error {
	if len(ctx.downlinkFrame.Items) == 0 {
		return nil
	}

	if err := gateway.Backend().SendTXPacket(ctx.downlinkFrame); err != nil {
		return errors.Wrap(err, "send downlink-frame to gateway error")
	}

	return nil
}
