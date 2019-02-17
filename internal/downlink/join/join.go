package join

import (
	"crypto/rand"
	"encoding/binary"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/backend/gateway"
	"github.com/brocaar/loraserver/internal/band"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/framelog"
	"github.com/brocaar/loraserver/internal/helpers"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
)

var (
	rxWindow        int
	downlinkTXPower int
)

var tasks = []func(*joinContext) error{
	setTXInfo,
	setToken,
	setDownlinkFrame,
	sendJoinAcceptResponse,
	saveRemainingFrames,
}

type joinContext struct {
	Token         uint16
	DeviceSession storage.DeviceSession
	RXPacket      models.RXPacket
	PHYPayload    lorawan.PHYPayload

	// Downlink frames to be emitted (this can be a slice e.g. to first try
	// using RX1 parameters, failing that RX2 parameters).
	// Only the first item will be emitted, the other(s) will be enqueued
	// and emitted on a scheduling error.
	DownlinkFrames []gw.DownlinkFrame
}

// Setup sets up the join handler.
func Setup(conf config.Config) error {
	nsConfig := conf.NetworkServer.NetworkSettings
	rxWindow = nsConfig.RXWindow
	downlinkTXPower = nsConfig.DownlinkTXPower

	return nil
}

// Handle handles a downlink join-response.
func Handle(ds storage.DeviceSession, rxPacket models.RXPacket, phy lorawan.PHYPayload) error {
	ctx := joinContext{
		DeviceSession: ds,
		PHYPayload:    phy,
		RXPacket:      rxPacket,
	}

	for _, t := range tasks {
		if err := t(&ctx); err != nil {
			return err
		}
	}

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
	if len(ctx.RXPacket.RXInfoSet) == 0 {
		return errors.New("empty RXInfoSet")
	}

	rxInfo := ctx.RXPacket.RXInfoSet[0]
	txInfo := gw.DownlinkTXInfo{
		GatewayId: rxInfo.GatewayId,
		Board:     rxInfo.Board,
		Antenna:   rxInfo.Antenna,
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
	txInfo.Timestamp = rxInfo.Timestamp + uint32(band.Band().GetDefaults().JoinAcceptDelay1/time.Microsecond)

	ctx.DownlinkFrames = append(ctx.DownlinkFrames, gw.DownlinkFrame{
		TxInfo: &txInfo,
	})

	return nil
}

func setTXInfoForRX2(ctx *joinContext) error {
	if len(ctx.RXPacket.RXInfoSet) == 0 {
		return errors.New("empty RXInfoSet")
	}

	rxInfo := ctx.RXPacket.RXInfoSet[0]
	txInfo := gw.DownlinkTXInfo{
		GatewayId: rxInfo.GatewayId,
		Board:     rxInfo.Board,
		Antenna:   rxInfo.Antenna,
		Frequency: uint32(band.Band().GetDefaults().RX2Frequency),
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
	txInfo.Timestamp = rxInfo.Timestamp + uint32(band.Band().GetDefaults().JoinAcceptDelay2/time.Microsecond)

	ctx.DownlinkFrames = append(ctx.DownlinkFrames, gw.DownlinkFrame{
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

	for i := range ctx.DownlinkFrames {
		ctx.DownlinkFrames[i].Token = uint32(binary.BigEndian.Uint16(b))
	}
	return nil
}

func setDownlinkFrame(ctx *joinContext) error {
	phyB, err := ctx.PHYPayload.MarshalBinary()
	if err != nil {
		return errors.Wrap(err, "marshal phypayload error")
	}

	for i := range ctx.DownlinkFrames {
		ctx.DownlinkFrames[i].PhyPayload = phyB
	}

	return nil
}

func sendJoinAcceptResponse(ctx *joinContext) error {
	if len(ctx.DownlinkFrames) == 0 {
		return nil
	}

	err := gateway.Backend().SendTXPacket(ctx.DownlinkFrames[0])
	if err != nil {
		return errors.Wrap(err, "send downlink frame error")
	}

	// log frame
	if err := framelog.LogDownlinkFrameForGateway(storage.RedisPool(), ctx.DownlinkFrames[0]); err != nil {
		log.WithError(err).Error("log downlink frame for gateway error")
	}

	if err := framelog.LogDownlinkFrameForDevEUI(storage.RedisPool(), ctx.DeviceSession.DevEUI, ctx.DownlinkFrames[0]); err != nil {
		log.WithError(err).Error("log downlink frame for device error")
	}

	return nil
}

func saveRemainingFrames(ctx *joinContext) error {
	if len(ctx.DownlinkFrames) < 2 {
		return nil
	}

	if err := storage.SaveDownlinkFrames(storage.RedisPool(), ctx.DeviceSession.DevEUI, ctx.DownlinkFrames[1:]); err != nil {
		return errors.Wrap(err, "save downlink-frames error")
	}

	return nil
}
