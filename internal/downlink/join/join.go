package join

import (
	"crypto/rand"
	"encoding/binary"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/framelog"
	"github.com/brocaar/loraserver/internal/helpers"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
)

var tasks = []func(*joinContext) error{
	setToken,
	getJoinAcceptTXInfo,
	setDownlinkFrame,
	sendJoinAcceptResponse,
	logDownlinkFrame,
}

type joinContext struct {
	Token         uint16
	DeviceSession storage.DeviceSession
	RXPacket      models.RXPacket
	TXInfo        gw.DownlinkTXInfo
	DownlinkFrame gw.DownlinkFrame
	PHYPayload    lorawan.PHYPayload
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

func setToken(ctx *joinContext) error {
	b := make([]byte, 2)
	_, err := rand.Read(b)
	if err != nil {
		return errors.Wrap(err, "read random error")
	}
	ctx.Token = binary.BigEndian.Uint16(b)
	return nil
}

func getJoinAcceptTXInfo(ctx *joinContext) error {
	if len(ctx.RXPacket.RXInfoSet) == 0 {
		return errors.New("empty RXInfoSet")
	}
	rxInfo := ctx.RXPacket.RXInfoSet[0]

	ctx.TXInfo = gw.DownlinkTXInfo{
		GatewayId: rxInfo.GatewayId,
		Board:     rxInfo.Board,
		Antenna:   rxInfo.Antenna,
	}

	timestamp := rxInfo.Timestamp + uint32(config.C.NetworkServer.Band.Band.GetDefaults().JoinAcceptDelay1/time.Microsecond)

	// get RX1 DR
	rx1DR, err := config.C.NetworkServer.Band.Band.GetRX1DataRateIndex(ctx.RXPacket.DR, 0)
	if err != nil {
		return errors.Wrap(err, "get rx1 data-rate index error")
	}

	// set data-rate
	err = helpers.SetDownlinkTXInfoDataRate(&ctx.TXInfo, rx1DR, config.C.NetworkServer.Band.Band)
	if err != nil {
		return errors.Wrap(err, "set downlink tx-info data-rate error")
	}

	// get RX1 frequency
	freq, err := config.C.NetworkServer.Band.Band.GetRX1FrequencyForUplinkFrequency(int(ctx.RXPacket.TXInfo.Frequency))
	if err != nil {
		return errors.Wrap(err, "get rx1 frequency error")
	}
	ctx.TXInfo.Frequency = uint32(freq)

	ctx.TXInfo.Timestamp = timestamp
	if config.C.NetworkServer.NetworkSettings.DownlinkTXPower != -1 {
		ctx.TXInfo.Power = int32(config.C.NetworkServer.NetworkSettings.DownlinkTXPower)
	} else {
		ctx.TXInfo.Power = int32(config.C.NetworkServer.Band.Band.GetDownlinkTXPower(freq))
	}

	return nil
}

func setDownlinkFrame(ctx *joinContext) error {
	phyB, err := ctx.PHYPayload.MarshalBinary()
	if err != nil {
		return errors.Wrap(err, "marshal phypayload error")
	}

	ctx.DownlinkFrame = gw.DownlinkFrame{
		Token:      uint32(ctx.Token),
		TxInfo:     &ctx.TXInfo,
		PhyPayload: phyB,
	}

	return nil
}

func sendJoinAcceptResponse(ctx *joinContext) error {
	err := config.C.NetworkServer.Gateway.Backend.Backend.SendTXPacket(ctx.DownlinkFrame)
	if err != nil {
		return errors.Wrap(err, "send downlink frame error")
	}

	return nil
}

func logDownlinkFrame(ctx *joinContext) error {
	if err := framelog.LogDownlinkFrameForGateway(ctx.DownlinkFrame); err != nil {
		log.WithError(err).Error("log downlink frame for gateway error")
	}

	if err := framelog.LogDownlinkFrameForDevEUI(ctx.DeviceSession.DevEUI, ctx.DownlinkFrame); err != nil {
		log.WithError(err).Error("log downlink frame for device error")
	}

	return nil
}
