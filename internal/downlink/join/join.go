package join

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/framelog"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
)

var tasks = []func(*joinContext) error{
	setToken,
	getJoinAcceptTXInfo,
	sendJoinAcceptResponse,
	logDownlinkFrame,
}

type joinContext struct {
	Token         uint16
	DeviceSession storage.DeviceSession
	RXPacket      models.RXPacket
	TXInfo        gw.TXInfo
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

	ctx.TXInfo = gw.TXInfo{
		MAC:      rxInfo.MAC,
		CodeRate: ctx.RXPacket.TXInfo.CodeRate,
		Board:    rxInfo.Board,
		Antenna:  rxInfo.Antenna,
	}

	var timestamp uint32

	if ctx.DeviceSession.RXWindow == storage.RX1 {
		timestamp = rxInfo.Timestamp + uint32(config.C.NetworkServer.Band.Band.GetDefaults().JoinAcceptDelay1/time.Microsecond)

		// get uplink dr
		uplinkDR, err := config.C.NetworkServer.Band.Band.GetDataRateIndex(true, ctx.RXPacket.TXInfo.DataRate)
		if err != nil {
			return errors.Wrap(err, "get data-rate index error")
		}

		// get RX1 DR
		rx1DR, err := config.C.NetworkServer.Band.Band.GetRX1DataRateIndex(uplinkDR, 0)
		if err != nil {
			return errors.Wrap(err, "get rx1 data-rate index error")
		}
		ctx.TXInfo.DataRate, err = config.C.NetworkServer.Band.Band.GetDataRate(rx1DR)
		if err != nil {
			return errors.Wrap(err, "get data-rate error")
		}

		// get RX1 frequency
		ctx.TXInfo.Frequency, err = config.C.NetworkServer.Band.Band.GetRX1FrequencyForUplinkFrequency(ctx.RXPacket.TXInfo.Frequency)
		if err != nil {
			return errors.Wrap(err, "get rx1 frequency error")
		}

	} else if ctx.DeviceSession.RXWindow == storage.RX2 {
		var err error

		timestamp = rxInfo.Timestamp + uint32(config.C.NetworkServer.Band.Band.GetDefaults().JoinAcceptDelay2/time.Microsecond)
		ctx.TXInfo.Frequency = config.C.NetworkServer.Band.Band.GetDefaults().RX2Frequency
		ctx.TXInfo.DataRate, err = config.C.NetworkServer.Band.Band.GetDataRate(config.C.NetworkServer.Band.Band.GetDefaults().RX2DataRate)
		if err != nil {
			return errors.Wrap(err, "get data-rate error")
		}
	} else {
		return fmt.Errorf("unknown RXWindow defined %d", ctx.DeviceSession.RXWindow)
	}

	ctx.TXInfo.Timestamp = &timestamp
	if config.C.NetworkServer.NetworkSettings.DownlinkTXPower != -1 {
		ctx.TXInfo.Power = config.C.NetworkServer.NetworkSettings.DownlinkTXPower
	} else {
		ctx.TXInfo.Power = config.C.NetworkServer.Band.Band.GetDownlinkTXPower(ctx.TXInfo.Frequency)
	}

	return nil
}

func sendJoinAcceptResponse(ctx *joinContext) error {
	err := config.C.NetworkServer.Gateway.Backend.Backend.SendTXPacket(gw.TXPacket{
		Token:      ctx.Token,
		TXInfo:     ctx.TXInfo,
		PHYPayload: ctx.PHYPayload,
	})
	if err != nil {
		return errors.Wrap(err, "send tx-packet error")
	}

	return nil
}

func logDownlinkFrame(ctx *joinContext) error {
	downlinkFrame, err := framelog.CreateDownlinkFrame(ctx.Token, ctx.PHYPayload, ctx.TXInfo)
	if err != nil {
		return errors.Wrap(err, "create downlink frame error")
	}

	if err := framelog.LogDownlinkFrameForGateway(downlinkFrame); err != nil {
		log.WithError(err).Error("log downlink frame for gateway error")
	}

	if err := framelog.LogDownlinkFrameForDevEUI(ctx.DeviceSession.DevEUI, downlinkFrame); err != nil {
		log.WithError(err).Error("log downlink frame for device error")
	}

	return nil
}
