package downlink

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"

	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/maccommand"
	"github.com/brocaar/loraserver/internal/node"
	"github.com/brocaar/loraserver/internal/session"
	"github.com/brocaar/lorawan"
)

func getDataTXInfo(ctx *DataContext) error {
	if len(ctx.NodeSession.LastRXInfoSet) == 0 {
		return ErrNoLastRXInfoSet
	}
	rxInfo := ctx.NodeSession.LastRXInfoSet[0]
	var err error
	ctx.TXInfo, ctx.DataRate, err = getDataDownTXInfoAndDR(ctx.NodeSession, rxInfo)
	if err != nil {
		return errors.Wrap(err, "get data down tx-info error")
	}

	return nil
}

func getDataTXInfoForRX2(ctx *DataContext) error {
	if len(ctx.NodeSession.LastRXInfoSet) == 0 {
		return ErrNoLastRXInfoSet
	}
	rxInfo := ctx.NodeSession.LastRXInfoSet[0]

	if int(ctx.NodeSession.RX2DR) > len(common.Band.DataRates)-1 {
		return errors.Wrapf(ErrInvalidDataRate, "dr: %d (max dr: %d)", ctx.NodeSession.RX2DR, len(common.Band.DataRates)-1)
	}

	ctx.TXInfo = gw.TXInfo{
		MAC:         rxInfo.MAC,
		Immediately: true,
		Frequency:   int(common.Band.RX2Frequency),
		Power:       common.Band.DefaultTXPower,
		DataRate:    common.Band.DataRates[int(ctx.NodeSession.RX2DR)],
		CodeRate:    "4/5",
	}

	return nil
}

func setRemainingPayloadSize(ctx *DataContext) error {
	ctx.RemainingPayloadSize = common.Band.MaxPayloadSize[ctx.DataRate].N - len(ctx.Data)

	if ctx.RemainingPayloadSize < 0 {
		return ErrMaxPayloadSizeExceeded
	}

	return nil
}

func getDataDownFromApplicationServer(ctx *DataContext) error {
	txPayload := getDataDownFromApplication(ctx.NodeSession, ctx.DataRate)
	if txPayload == nil {
		return nil
	}

	ctx.RemainingPayloadSize = ctx.RemainingPayloadSize - len(txPayload.Data)
	ctx.Data = txPayload.Data
	ctx.Confirmed = txPayload.Confirmed
	ctx.MoreData = txPayload.MoreData
	ctx.FPort = uint8(txPayload.FPort)

	return nil
}

func getMACCommands(ctx *DataContext) error {
	allowEncryptedMACCommands := (ctx.FPort == 0)

	macBlocks, encryptMACCommands, pendingMACCommands, err := getAndFilterMACQueueItems(ctx.NodeSession, allowEncryptedMACCommands, ctx.RemainingPayloadSize)
	if err != nil {
		return errors.Wrap(err, "get mac-commands error")
	}

	for bi := range macBlocks {
		for mi := range macBlocks[bi].MACCommands {
			ctx.MACCommands = append(ctx.MACCommands, macBlocks[bi].MACCommands[mi])
		}
	}

	ctx.EncryptMACCommands = encryptMACCommands

	if pendingMACCommands {
		// note that MoreData might already be true
		ctx.MoreData = true
	}

	for _, block := range macBlocks {
		if err = maccommand.SetPending(common.RedisPool, ctx.NodeSession.DevEUI, block); err != nil {
			return errors.Wrap(err, "set mac-command block as pending error")
		}

		if err = maccommand.DeleteQueueItem(common.RedisPool, ctx.NodeSession.DevEUI, block); err != nil {
			return errors.Wrap(err, "delete mac-command block from queue error")
		}
	}

	return nil
}

func stopOnNothingToSend(ctx *DataContext) error {
	if ctx.FPort == 0 && len(ctx.MACCommands) == 0 && !ctx.ACK && !ctx.MustSend {
		// ErrAbort will not be handled as a real error
		return ErrAbort
	}

	return nil
}

func sendDataDown(ctx *DataContext) error {
	if err := ctx.Validate(); err != nil {
		return errors.Wrap(err, "validation error")
	}

	phy := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: lorawan.UnconfirmedDataDown,
			Major: lorawan.LoRaWANR1,
		},
	}
	if ctx.Confirmed {
		phy.MHDR.MType = lorawan.ConfirmedDataDown
	}

	macPL := &lorawan.MACPayload{
		FHDR: lorawan.FHDR{
			DevAddr: ctx.NodeSession.DevAddr,
			FCtrl: lorawan.FCtrl{
				ADR:      ctx.NodeSession.ADRInterval != 0,
				ACK:      ctx.ACK,
				FPending: ctx.MoreData,
			},
			FCnt: ctx.NodeSession.FCntDown,
		},
	}
	phy.MACPayload = macPL

	if len(ctx.MACCommands) > 0 {
		if ctx.EncryptMACCommands {
			var frmPayload []lorawan.Payload
			for i := range ctx.MACCommands {
				frmPayload = append(frmPayload, &ctx.MACCommands[i])
			}
			macPL.FPort = &ctx.FPort
			macPL.FRMPayload = frmPayload

			// encrypt the FRMPayload with the NwkSKey
			if err := phy.EncryptFRMPayload(ctx.NodeSession.NwkSKey); err != nil {
				return errors.Wrap(err, "encrypt FRMPayload error")
			}
		} else {
			macPL.FHDR.FOpts = ctx.MACCommands
		}
	}

	if ctx.FPort > 0 {
		macPL.FPort = &ctx.FPort
		macPL.FRMPayload = []lorawan.Payload{
			&lorawan.DataPayload{Bytes: ctx.Data},
		}
	}

	if err := phy.SetMIC(ctx.NodeSession.NwkSKey); err != nil {
		return errors.Wrap(err, "set MIC error")
	}

	logDownlink(common.DB, ctx.NodeSession.DevEUI, phy, ctx.TXInfo)

	// send the packet to the gateway
	if err := common.Gateway.SendTXPacket(gw.TXPacket{
		TXInfo:     ctx.TXInfo,
		PHYPayload: phy,
	}); err != nil {
		return errors.Wrap(err, "send tx packet to gateway error")
	}

	// increment downlink framecounter
	ctx.NodeSession.FCntDown++

	return nil
}

func saveNodeSession(ctx *DataContext) error {
	if err := session.SaveNodeSession(common.RedisPool, ctx.NodeSession); err != nil {
		return errors.Wrap(err, "save node-session error")
	}
	return nil
}

func getDataDownTXInfoAndDR(ns session.NodeSession, rxInfo gw.RXInfo) (gw.TXInfo, int, error) {
	var dr int
	txInfo := gw.TXInfo{
		MAC:      rxInfo.MAC,
		CodeRate: rxInfo.CodeRate,
		Power:    common.Band.DefaultTXPower,
	}

	if ns.RXWindow == session.RX1 {
		uplinkDR, err := common.Band.GetDataRate(rxInfo.DataRate)
		if err != nil {
			return txInfo, dr, err
		}

		// get rx1 dr
		dr, err = common.Band.GetRX1DataRate(uplinkDR, int(ns.RX1DROffset))
		if err != nil {
			return txInfo, dr, err
		}
		txInfo.DataRate = common.Band.DataRates[dr]

		// get rx1 frequency
		txInfo.Frequency, err = common.Band.GetRX1Frequency(rxInfo.Frequency)
		if err != nil {
			return txInfo, dr, err
		}

		// get timestamp
		txInfo.Timestamp = rxInfo.Timestamp + uint32(common.Band.ReceiveDelay1/time.Microsecond)
		if ns.RXDelay > 0 {
			txInfo.Timestamp = rxInfo.Timestamp + uint32(time.Duration(ns.RXDelay)*time.Second/time.Microsecond)
		}
	} else if ns.RXWindow == session.RX2 {
		// rx2 dr
		dr = int(ns.RX2DR)
		if dr > len(common.Band.DataRates)-1 {
			return txInfo, 0, fmt.Errorf("invalid rx2 dr: %d (max dr: %d)", dr, len(common.Band.DataRates)-1)
		}
		txInfo.DataRate = common.Band.DataRates[dr]

		// rx2 frequency
		txInfo.Frequency = common.Band.RX2Frequency

		// rx2 timestamp (rx1 + 1 sec)
		txInfo.Timestamp = rxInfo.Timestamp + uint32(common.Band.ReceiveDelay1/time.Microsecond)
		if ns.RXDelay > 0 {
			txInfo.Timestamp = rxInfo.Timestamp + uint32(time.Duration(ns.RXDelay)*time.Second/time.Microsecond)
		}
		txInfo.Timestamp = txInfo.Timestamp + uint32(time.Second/time.Microsecond)
	} else {
		return txInfo, dr, fmt.Errorf("unknown RXWindow option %d", ns.RXWindow)
	}

	return txInfo, dr, nil
}

// getDataDownFromApplication gets the downlink data from the application
// (if any). On error the error is logged.
func getDataDownFromApplication(ns session.NodeSession, dr int) *as.GetDataDownResponse {
	resp, err := common.Application.GetDataDown(context.Background(), &as.GetDataDownRequest{
		AppEUI:         ns.AppEUI[:],
		DevEUI:         ns.DevEUI[:],
		MaxPayloadSize: uint32(common.Band.MaxPayloadSize[dr].N),
		FCnt:           ns.FCntDown,
	})
	if err != nil {
		log.WithFields(log.Fields{
			"dev_eui": ns.DevEUI,
			"fcnt":    ns.FCntDown,
		}).Errorf("get data down from application error: %s", err)
		return nil
	}

	if resp == nil || resp.FPort == 0 {
		return nil
	}

	if len(resp.Data) > common.Band.MaxPayloadSize[dr].N {
		log.WithFields(log.Fields{
			"dev_eui":          ns.DevEUI,
			"size":             len(resp.Data),
			"max_payload_size": common.Band.MaxPayloadSize[dr].N,
			"dr":               dr,
		}).Warning("data down from application exceeds max payload size")
		return nil
	}

	log.WithFields(log.Fields{
		"dev_eui":     ns.DevEUI,
		"fcnt":        ns.FCntDown,
		"data_base64": base64.StdEncoding.EncodeToString(resp.Data),
		"confirmed":   resp.Confirmed,
		"more_data":   resp.MoreData,
	}).Info("received data down from application")

	return resp
}

// getAndFilterMACQueueItems returns the mac-commands to send, based on the constraints:
// - allowEncrypted: when set to true, the FRMPayload may be used for
//   (encrypted) mac-commands, else only FOpt mac-commands will be returned
// - remainingPayloadSize: the number of bytes that are left for mac-commands
// It returns:
// - a slice of mac-command queue items
// - if the mac-commands must be put into FRMPayload
// - if there are remaining mac-commands in the queue
func getAndFilterMACQueueItems(ns session.NodeSession, allowEncrypted bool, remainingPayloadSize int) ([]maccommand.Block, bool, bool, error) {
	var encrypted bool
	var blocks []maccommand.Block

	// read the mac payload queue
	allBlocks, err := maccommand.ReadQueueItems(common.RedisPool, ns.DevEUI)
	if err != nil {
		return nil, false, false, errors.Wrap(err, "read mac-command queue error")
	}

	// nothing to do
	if len(allBlocks) == 0 {
		return nil, false, false, nil
	}

	// encrypted mac-commands are allowed and the first mac-command in the
	// queue is marked to be encrypted
	if allowEncrypted && allBlocks[0].FRMPayload {
		encrypted = true
		blocks, err = maccommand.FilterItems(allBlocks, true, remainingPayloadSize)
		if err != nil {
			return nil, false, false, errors.Wrap(err, "filter mac-command blocks error")
		}
	} else {
		maxFOptsLen := remainingPayloadSize
		// the LoRaWAN specs define 15 to be the max FOpts size
		if maxFOptsLen > 15 {
			maxFOptsLen = 15
		}
		blocks, err = maccommand.FilterItems(allBlocks, false, maxFOptsLen)
		if err != nil {
			return nil, false, false, errors.Wrap(err, "filter mac-command blocks error")
		}
	}

	return blocks, encrypted, len(allBlocks) != len(blocks), nil
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
