package downlink

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/maccommand"
	"github.com/brocaar/loraserver/internal/node"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
)

func requestDevStatus(ctx *DataContext) error {
	if ctx.ServiceProfile.DevStatusReqFreq == 0 {
		return nil
	}

	reqInterval := 24 * time.Hour / time.Duration(ctx.ServiceProfile.DevStatusReqFreq)
	curInterval := time.Now().Sub(ctx.DeviceSession.LastDevStatusRequested)

	if curInterval >= reqInterval {
		err := maccommand.RequestDevStatus(&ctx.DeviceSession)
		if err != nil {
			log.WithError(err).Error("request device-status error")
		}
	}

	return nil
}

func getDataTXInfo(ctx *DataContext) error {
	if len(ctx.DeviceSession.LastRXInfoSet) == 0 {
		return ErrNoLastRXInfoSet
	}
	rxInfo := ctx.DeviceSession.LastRXInfoSet[0]
	var err error
	ctx.TXInfo, ctx.DataRate, err = getDataDownTXInfoAndDR(ctx.DeviceSession, rxInfo)
	if err != nil {
		return errors.Wrap(err, "get data down tx-info error")
	}

	return nil
}

func getDataTXInfoForRX2(ctx *DataContext) error {
	if len(ctx.DeviceSession.LastRXInfoSet) == 0 {
		return ErrNoLastRXInfoSet
	}
	rxInfo := ctx.DeviceSession.LastRXInfoSet[0]

	if int(ctx.DeviceSession.RX2DR) > len(common.Band.DataRates)-1 {
		return errors.Wrapf(ErrInvalidDataRate, "dr: %d (max dr: %d)", ctx.DeviceSession.RX2DR, len(common.Band.DataRates)-1)
	}

	ctx.TXInfo = gw.TXInfo{
		MAC:         rxInfo.MAC,
		Immediately: true,
		Frequency:   int(common.Band.RX2Frequency),
		Power:       common.Band.DefaultTXPower,
		DataRate:    common.Band.DataRates[int(ctx.DeviceSession.RX2DR)],
		CodeRate:    "4/5",
	}
	ctx.DataRate = int(ctx.DeviceSession.RX2DR)

	return nil
}

func setRemainingPayloadSize(ctx *DataContext) error {
	ctx.RemainingPayloadSize = common.Band.MaxPayloadSize[ctx.DataRate].N - len(ctx.Data)

	if ctx.RemainingPayloadSize < 0 {
		return ErrMaxPayloadSizeExceeded
	}

	return nil
}

func getNextDeviceQueueItem(ctx *DataContext) error {
	qi, err := storage.GetNextDeviceQueueItemForDevEUIMaxPayloadSizeAndFCnt(common.DB, ctx.DeviceSession.DevEUI, ctx.RemainingPayloadSize, ctx.DeviceSession.FCntDown, ctx.DeviceSession.RoutingProfileID)
	if err != nil {
		if errors.Cause(err) == storage.ErrDoesNotExist {
			return nil
		}
		return errors.Wrap(err, "get next device-queue item for max payload error")
	}

	ctx.Confirmed = qi.Confirmed
	ctx.Data = qi.FRMPayload
	ctx.FPort = qi.FPort
	ctx.RemainingPayloadSize = ctx.RemainingPayloadSize - len(ctx.Data)

	items, err := storage.GetDeviceQueueItemsForDevEUI(common.DB, ctx.DeviceSession.DevEUI)
	if err != nil {
		return errors.Wrap(err, "get device-queue items error")
	}
	ctx.MoreData = len(items) > 1 // more than only the current frame

	// Set the device-session fCnt (down). We might have discarded one or
	// multiple frames (payload size) or the application-server might have
	// incremented the counter incorrectly. This is important since it is
	// used for decrypting the payload by the device!!
	ctx.DeviceSession.FCntDown = qi.FCnt

	// delete when not confirmed
	if !qi.Confirmed {
		if err := storage.DeleteDeviceQueueItem(common.DB, qi.ID); err != nil {
			return errors.Wrap(err, "delete device-queue item error")
		}
	} else {
		// mark as pending and set timeout
		timeout := time.Now()
		if ctx.DeviceProfile.SupportsClassC {
			timeout = timeout.Add(time.Duration(ctx.DeviceProfile.ClassCTimeout) * time.Second)
		}
		if ctx.DeviceProfile.SupportsClassB {
			timeout = timeout.Add(time.Duration(ctx.DeviceProfile.ClassBTimeout) * time.Second)
		}
		qi.IsPending = true
		qi.TimeoutAfter = &timeout

		if err := storage.UpdateDeviceQueueItem(common.DB, &qi); err != nil {
			return errors.Wrap(err, "update device-queue item error")
		}
	}

	return nil
}

func getMACCommands(ctx *DataContext) error {
	allowEncryptedMACCommands := (ctx.FPort == 0)

	macBlocks, encryptMACCommands, pendingMACCommands, err := getAndFilterMACQueueItems(ctx.DeviceSession, allowEncryptedMACCommands, ctx.RemainingPayloadSize)
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
		if err = maccommand.SetPending(common.RedisPool, ctx.DeviceSession.DevEUI, block); err != nil {
			return errors.Wrap(err, "set mac-command block as pending error")
		}

		if err = maccommand.DeleteQueueItem(common.RedisPool, ctx.DeviceSession.DevEUI, block); err != nil {
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
			DevAddr: ctx.DeviceSession.DevAddr,
			FCtrl: lorawan.FCtrl{
				ADR:      true,
				ACK:      ctx.ACK,
				FPending: ctx.MoreData,
			},
			FCnt: ctx.DeviceSession.FCntDown,
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
			if err := phy.EncryptFRMPayload(ctx.DeviceSession.NwkSKey); err != nil {
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

	if err := phy.SetMIC(ctx.DeviceSession.NwkSKey); err != nil {
		return errors.Wrap(err, "set MIC error")
	}

	logDownlink(common.DB, ctx.DeviceSession.DevEUI, phy, ctx.TXInfo)

	// send the packet to the gateway
	if err := common.Gateway.SendTXPacket(gw.TXPacket{
		TXInfo:     ctx.TXInfo,
		PHYPayload: phy,
	}); err != nil {
		return errors.Wrap(err, "send tx packet to gateway error")
	}

	// increment downlink framecounter
	ctx.DeviceSession.FCntDown++

	return nil
}

func saveDeviceSession(ctx *DataContext) error {
	if err := storage.SaveDeviceSession(common.RedisPool, ctx.DeviceSession); err != nil {
		return errors.Wrap(err, "save device-session error")
	}
	return nil
}

func getDeviceProfile(ctx *DataContext) error {
	var err error
	ctx.DeviceProfile, err = storage.GetAndCacheDeviceProfile(common.DB, common.RedisPool, ctx.DeviceSession.DeviceProfileID)
	if err != nil {
		return errors.Wrap(err, "get device-profile error")
	}
	return nil
}

func getServiceProfile(ctx *DataContext) error {
	var err error
	ctx.ServiceProfile, err = storage.GetAndCacheServiceProfile(common.DB, common.RedisPool, ctx.DeviceSession.ServiceProfileID)
	if err != nil {
		return errors.Wrap(err, "get service-profile error")
	}
	return nil
}

func getDataDownTXInfoAndDR(ds storage.DeviceSession, rxInfo gw.RXInfo) (gw.TXInfo, int, error) {
	var dr int
	txInfo := gw.TXInfo{
		MAC:      rxInfo.MAC,
		CodeRate: rxInfo.CodeRate,
		Power:    common.Band.DefaultTXPower,
	}

	if ds.RXWindow == storage.RX1 {
		uplinkDR, err := common.Band.GetDataRate(rxInfo.DataRate)
		if err != nil {
			return txInfo, dr, err
		}

		// get rx1 dr
		dr, err = common.Band.GetRX1DataRate(uplinkDR, int(ds.RX1DROffset))
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
		if ds.RXDelay > 0 {
			txInfo.Timestamp = rxInfo.Timestamp + uint32(time.Duration(ds.RXDelay)*time.Second/time.Microsecond)
		}
	} else if ds.RXWindow == storage.RX2 {
		// rx2 dr
		dr = int(ds.RX2DR)
		if dr > len(common.Band.DataRates)-1 {
			return txInfo, 0, fmt.Errorf("invalid rx2 dr: %d (max dr: %d)", dr, len(common.Band.DataRates)-1)
		}
		txInfo.DataRate = common.Band.DataRates[dr]

		// rx2 frequency
		txInfo.Frequency = common.Band.RX2Frequency

		// rx2 timestamp (rx1 + 1 sec)
		txInfo.Timestamp = rxInfo.Timestamp + uint32(common.Band.ReceiveDelay1/time.Microsecond)
		if ds.RXDelay > 0 {
			txInfo.Timestamp = rxInfo.Timestamp + uint32(time.Duration(ds.RXDelay)*time.Second/time.Microsecond)
		}
		txInfo.Timestamp = txInfo.Timestamp + uint32(time.Second/time.Microsecond)
	} else {
		return txInfo, dr, fmt.Errorf("unknown RXWindow option %d", ds.RXWindow)
	}

	return txInfo, dr, nil
}

// getAndFilterMACQueueItems returns the mac-commands to send, based on the constraints:
// - allowEncrypted: when set to true, the FRMPayload may be used for
//   (encrypted) mac-commands, else only FOpt mac-commands will be returned
// - remainingPayloadSize: the number of bytes that are left for mac-commands
// It returns:
// - a slice of mac-command queue items
// - if the mac-commands must be put into FRMPayload
// - if there are remaining mac-commands in the queue
func getAndFilterMACQueueItems(ds storage.DeviceSession, allowEncrypted bool, remainingPayloadSize int) ([]maccommand.Block, bool, bool, error) {
	var encrypted bool
	var blocks []maccommand.Block

	// read the mac payload queue
	allBlocks, err := maccommand.ReadQueueItems(common.RedisPool, ds.DevEUI)
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
