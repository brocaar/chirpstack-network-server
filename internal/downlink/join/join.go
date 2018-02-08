package join

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
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
	if len(ctx.DeviceSession.LastRXInfoSet) == 0 {
		return errors.New("empty LastRXInfoSet")
	}

	rxInfo := ctx.DeviceSession.LastRXInfoSet[0]

	ctx.TXInfo = gw.TXInfo{
		MAC:      rxInfo.MAC,
		CodeRate: ctx.RXPacket.TXInfo.CodeRate,
		Power:    common.Band.DefaultTXPower,
	}

	var timestamp uint32

	if ctx.DeviceSession.RXWindow == storage.RX1 {
		timestamp = rxInfo.Timestamp + uint32(common.Band.JoinAcceptDelay1/time.Microsecond)

		dr, err := common.Band.GetDataRate(ctx.RXPacket.TXInfo.DataRate)
		if err != nil {
			return errors.Wrap(err, "get data-rate error")
		}

		// get RX1 DR
		rx1DR, err := common.Band.GetRX1DataRate(dr, 0)
		if err != nil {
			return errors.Wrap(err, "get rx1 data-rate error")
		}
		ctx.TXInfo.DataRate = common.Band.DataRates[rx1DR]

		// get RX1 frequency
		ctx.TXInfo.Frequency, err = common.Band.GetRX1Frequency(ctx.RXPacket.TXInfo.Frequency)
		if err != nil {
			return errors.Wrap(err, "get rx1 frequency error")
		}
	} else if ctx.DeviceSession.RXWindow == storage.RX2 {
		timestamp = rxInfo.Timestamp + uint32(common.Band.JoinAcceptDelay2/time.Microsecond)
		ctx.TXInfo.DataRate = common.Band.DataRates[common.Band.RX2DataRate]
		ctx.TXInfo.Frequency = common.Band.RX2Frequency
	} else {
		return fmt.Errorf("unknown RXWindow defined %d", ctx.DeviceSession.RXWindow)
	}

	ctx.TXInfo.Timestamp = &timestamp

	return nil
}

func sendJoinAcceptResponse(ctx *joinContext) error {
	err := common.Gateway.SendTXPacket(gw.TXPacket{
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
	frameLog := framelog.DownlinkFrameLog{
		PHYPayload: ctx.PHYPayload,
		TXInfo:     ctx.TXInfo,
	}

	if err := framelog.LogDownlinkFrameForGateway(frameLog); err != nil {
		log.WithError(err).Error("log downlink frame for gateway error")
	}

	if err := framelog.LogDownlinkFrameForDevEUI(ctx.DeviceSession.DevEUI, frameLog); err != nil {
		log.WithError(err).Error("log downlink frame for device error")
	}

	return nil
}
