package join

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/node"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
)

var tasks = []func(*joinContext) error{
	setToken,
	getJoinAcceptTXInfo,
	logJoinAcceptFrame,
	sendJoinAcceptResponse,
}

type joinContext struct {
	Token         uint16
	DeviceSession storage.DeviceSession
	TXInfo        gw.TXInfo
	PHYPayload    lorawan.PHYPayload
}

// Handle handles a downlink join-response.
func Handle(ds storage.DeviceSession, phy lorawan.PHYPayload) error {
	ctx := joinContext{
		DeviceSession: ds,
		PHYPayload:    phy,
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
		CodeRate: rxInfo.CodeRate,
		Power:    common.Band.DefaultTXPower,
	}

	var timestamp uint32

	if ctx.DeviceSession.RXWindow == storage.RX1 {
		timestamp = rxInfo.Timestamp + uint32(common.Band.JoinAcceptDelay1/time.Microsecond)

		// get uplink dr
		uplinkDR, err := common.Band.GetDataRate(rxInfo.DataRate)
		if err != nil {
			return errors.Wrap(err, "get data-rate error")
		}

		// get RX1 DR
		rx1DR, err := common.Band.GetRX1DataRate(uplinkDR, 0)
		if err != nil {
			return errors.Wrap(err, "get rx1 data-rate error")
		}
		ctx.TXInfo.DataRate = common.Band.DataRates[rx1DR]

		// get RX1 frequency
		ctx.TXInfo.Frequency, err = common.Band.GetRX1Frequency(rxInfo.Frequency)
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

func logJoinAcceptFrame(ctx *joinContext) error {
	logDownlink(common.DB, ctx.DeviceSession.DevEUI, ctx.PHYPayload, ctx.TXInfo)
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

func logDownlink(db *sqlx.DB, devEUI lorawan.EUI64, phy lorawan.PHYPayload, txInfo gw.TXInfo) {
	if !common.LogNodeFrames {
		return
	}

	phyB, err := phy.MarshalBinary()
	if err != nil {
		log.Errorf("marshal phypayload to binary error: %s", err)
		return
	}

	txB, err := json.Marshal(txInfo)
	if err != nil {
		log.Errorf("marshal tx-info to json error: %s", err)
	}

	fl := node.FrameLog{
		DevEUI:     devEUI,
		TXInfo:     &txB,
		PHYPayload: phyB,
	}
	err = node.CreateFrameLog(db, &fl)
	if err != nil {
		log.Errorf("create frame-log error: %s", err)
	}
}
