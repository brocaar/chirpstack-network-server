package join

import (
	"context"
	"crypto/rand"
	"encoding/binary"

	"github.com/gofrs/uuid"
	"github.com/golang/protobuf/ptypes"
	"github.com/pkg/errors"

	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-network-server/internal/backend/gateway"
	"github.com/brocaar/chirpstack-network-server/internal/band"
	"github.com/brocaar/chirpstack-network-server/internal/config"
	dwngateway "github.com/brocaar/chirpstack-network-server/internal/downlink/gateway"
	"github.com/brocaar/chirpstack-network-server/internal/helpers"
	"github.com/brocaar/chirpstack-network-server/internal/logging"
	"github.com/brocaar/chirpstack-network-server/internal/models"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
)

var (
	rxWindow               int
	downlinkTXPower        int
	gatewayPreferMinMargin float64
)

var tasks = []func(*joinContext) error{
	setDeviceGatewayRXInfo,
	selectDownlinkGateway,
	setTXInfo,
	setToken,
	setDownlinkFrame,
	sendJoinAcceptResponse,
	saveDownlinkFrame,
}

type joinContext struct {
	ctx context.Context

	Token               uint16
	DeviceSession       storage.DeviceSession
	DeviceGatewayRXInfo []storage.DeviceGatewayRXInfo
	RXPacket            models.RXPacket
	PHYPayload          lorawan.PHYPayload
	DownlinkGateway     storage.DeviceGatewayRXInfo
	DownlinkFrame       gw.DownlinkFrame
}

// Setup sets up the join handler.
func Setup(conf config.Config) error {
	nsConfig := conf.NetworkServer.NetworkSettings
	rxWindow = nsConfig.RXWindow
	downlinkTXPower = nsConfig.DownlinkTXPower
	gatewayPreferMinMargin = nsConfig.GatewayPreferMinMargin

	return nil
}

// Handle handles a downlink join-response.
func Handle(ctx context.Context, ds storage.DeviceSession, rxPacket models.RXPacket, phy lorawan.PHYPayload) error {
	jctx := joinContext{
		ctx:           ctx,
		DeviceSession: ds,
		PHYPayload:    phy,
		RXPacket:      rxPacket,
	}

	for _, t := range tasks {
		if err := t(&jctx); err != nil {
			return err
		}
	}

	return nil
}

func setDeviceGatewayRXInfo(ctx *joinContext) error {
	for i := range ctx.RXPacket.RXInfoSet {
		ctx.DeviceGatewayRXInfo = append(ctx.DeviceGatewayRXInfo, storage.DeviceGatewayRXInfo{
			GatewayID: helpers.GetGatewayID(ctx.RXPacket.RXInfoSet[i]),
			RSSI:      int(ctx.RXPacket.RXInfoSet[i].Rssi),
			LoRaSNR:   ctx.RXPacket.RXInfoSet[i].LoraSnr,
			Board:     ctx.RXPacket.RXInfoSet[i].Board,
			Antenna:   ctx.RXPacket.RXInfoSet[i].Antenna,
			Context:   ctx.RXPacket.RXInfoSet[i].Context,
		})
	}

	// this should not happen
	if len(ctx.DeviceGatewayRXInfo) == 0 {
		return errors.New("DeviceGatewayRXInfo is empty!")
	}

	return nil
}

func selectDownlinkGateway(ctx *joinContext) error {
	var err error
	ctx.DownlinkGateway, err = dwngateway.SelectDownlinkGateway(gatewayPreferMinMargin, ctx.RXPacket.DR, ctx.DeviceGatewayRXInfo)
	if err != nil {
		return err
	}

	ctx.DownlinkFrame.GatewayId = ctx.DownlinkGateway.GatewayID[:]

	return nil
}

func setTXInfo(ctx *joinContext) error {
	if rxWindow == 0 || rxWindow == 1 {
		if err := setTXInfoForRX1(ctx); err != nil {
			return err
		}
	}

	if rxWindow == 0 || rxWindow == 2 {
		if err := setTXInfoForRX2(ctx); err != nil {
			return err
		}
	}

	return nil
}

func setTXInfoForRX1(ctx *joinContext) error {
	txInfo := gw.DownlinkTXInfo{
		Board:   ctx.DownlinkGateway.Board,
		Antenna: ctx.DownlinkGateway.Antenna,
		Context: ctx.DownlinkGateway.Context,
	}

	// get RX1 data-rate
	rx1DR, err := band.Band().GetRX1DataRateIndex(ctx.RXPacket.DR, 0)
	if err != nil {
		return errors.Wrap(err, "get rx1 data-rate index error")
	}

	// set data-rate
	err = helpers.SetDownlinkTXInfoDataRate(&txInfo, rx1DR, band.Band())
	if err != nil {
		return errors.Wrap(err, "set downlink tx-info data-rate error")
	}

	// set frequency
	freq, err := band.Band().GetRX1FrequencyForUplinkFrequency(int(ctx.RXPacket.TXInfo.Frequency))
	if err != nil {
		return errors.Wrap(err, "get rx1 frequency error")
	}
	txInfo.Frequency = uint32(freq)

	// set tx power
	if downlinkTXPower != -1 {
		txInfo.Power = int32(downlinkTXPower)
	} else {
		txInfo.Power = int32(band.Band().GetDownlinkTXPower(int(txInfo.Frequency)))
	}

	// set timestamp
	txInfo.Timing = gw.DownlinkTiming_DELAY
	txInfo.TimingInfo = &gw.DownlinkTXInfo_DelayTimingInfo{
		DelayTimingInfo: &gw.DelayTimingInfo{
			Delay: ptypes.DurationProto(band.Band().GetDefaults().JoinAcceptDelay1),
		},
	}

	// set downlink item
	ctx.DownlinkFrame.Items = append(ctx.DownlinkFrame.Items, &gw.DownlinkFrameItem{
		TxInfo: &txInfo,
	})

	return nil
}

func setTXInfoForRX2(ctx *joinContext) error {
	txInfo := gw.DownlinkTXInfo{
		Board:     ctx.DownlinkGateway.Board,
		Antenna:   ctx.DownlinkGateway.Antenna,
		Frequency: uint32(band.Band().GetDefaults().RX2Frequency),
		Context:   ctx.DownlinkGateway.Context,
	}

	// set data-rate
	err := helpers.SetDownlinkTXInfoDataRate(&txInfo, band.Band().GetDefaults().RX2DataRate, band.Band())
	if err != nil {
		return errors.Wrap(err, "set downlink tx-info data-rate error")
	}

	// set tx power
	if downlinkTXPower != -1 {
		txInfo.Power = int32(downlinkTXPower)
	} else {
		txInfo.Power = int32(band.Band().GetDownlinkTXPower(int(txInfo.Frequency)))
	}

	// set timestamp
	txInfo.Timing = gw.DownlinkTiming_DELAY
	txInfo.TimingInfo = &gw.DownlinkTXInfo_DelayTimingInfo{
		DelayTimingInfo: &gw.DelayTimingInfo{
			Delay: ptypes.DurationProto(band.Band().GetDefaults().JoinAcceptDelay2),
		},
	}

	ctx.DownlinkFrame.Items = append(ctx.DownlinkFrame.Items, &gw.DownlinkFrameItem{
		TxInfo: &txInfo,
	})

	return nil
}

func setToken(ctx *joinContext) error {
	b := make([]byte, 2)
	_, err := rand.Read(b)
	if err != nil {
		return errors.Wrap(err, "read random error")
	}

	var downID uuid.UUID
	if ctxID := ctx.ctx.Value(logging.ContextIDKey); ctxID != nil {
		if id, ok := ctxID.(uuid.UUID); ok {
			downID = id
		}
	}

	ctx.DownlinkFrame.Token = uint32(binary.BigEndian.Uint16(b))
	ctx.DownlinkFrame.DownlinkId = downID[:]

	return nil
}

func setDownlinkFrame(ctx *joinContext) error {
	phyB, err := ctx.PHYPayload.MarshalBinary()
	if err != nil {
		return errors.Wrap(err, "marshal phypayload error")
	}

	for i := range ctx.DownlinkFrame.Items {
		ctx.DownlinkFrame.Items[i].PhyPayload = phyB
	}

	return nil
}

func sendJoinAcceptResponse(ctx *joinContext) error {
	err := gateway.Backend().SendTXPacket(ctx.DownlinkFrame)
	if err != nil {
		return errors.Wrap(err, "send downlink frame error")
	}

	return nil
}

func saveDownlinkFrame(ctx *joinContext) error {
	df := storage.DownlinkFrame{
		DevEui:           ctx.DeviceSession.DevEUI[:],
		Token:            ctx.DownlinkFrame.Token,
		RoutingProfileId: ctx.DeviceSession.RoutingProfileID.Bytes(),
		DownlinkFrame:    &ctx.DownlinkFrame,
	}

	if err := storage.SaveDownlinkFrame(ctx.ctx, &df); err != nil {
		return errors.Wrap(err, "save downlink-frame error")
	}

	return nil
}
