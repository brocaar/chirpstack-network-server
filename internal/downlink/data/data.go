package data

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
	"github.com/brocaar/loraserver/internal/maccommand"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
)

var responseTasks = []func(*dataContext) error{
	setToken,
	getDeviceProfile,
	requestDevStatus,
	getDataTXInfo,
	setRemainingPayloadSize,
	getNextDeviceQueueItem,
	getMACCommands,
	stopOnNothingToSend,
	sendDataDown,
	saveDeviceSession,
	logDownlinkFrame,
}

var scheduleNextQueueItemTasks = []func(*dataContext) error{
	setToken,
	getDeviceProfile,
	getServiceProfile,
	checkLastDownlinkTimestamp,
	requestDevStatus,
	getDataTXInfoForRX2,
	setRemainingPayloadSize,
	getNextDeviceQueueItem,
	getMACCommands,
	stopOnNothingToSend,
	sendDataDown,
	saveDeviceSession,
	logDownlinkFrame,
}

type dataContext struct {
	// Token defines a random token.
	Token uint16

	// ServiceProfile of the device.
	ServiceProfile storage.ServiceProfile

	// DeviceProfile of the device.
	DeviceProfile storage.DeviceProfile

	// DeviceSession holds the device-session of the device for which to send
	// the downlink data.
	DeviceSession storage.DeviceSession

	// TXInfo holds the data needed for transmission.
	TXInfo gw.TXInfo

	// DataRate holds the data-rate for transmission.
	DataRate int

	// MustSend defines if a frame must be send. In some cases (e.g. ADRACKReq)
	// the network-server must respond, even when there are no mac-commands or
	// FRMPayload.
	MustSend bool

	// The remaining payload size which can be used for mac-commands and / or
	// FRMPayload.
	RemainingPayloadSize int

	// ACK defines if ACK must be set to true (e.g. the frame acknowledges
	// an uplink frame).
	ACK bool

	// FPort to use for transmission. This must be set to a value != 0 in case
	// Data is not empty.
	FPort uint8

	// MACCommands contains the mac-commands to send (if any). Make sure the
	// total size fits within the FRMPayload or FPort (depending on if
	// EncryptMACCommands is set to true).
	MACCommands []lorawan.MACCommand

	// EncryptMACCommands defines if the mac-commands (if any) must be
	// encrypted and put in the FRMPayload. Note that it is not possible to
	// send encrypted mac-commands when FPort is set to a value != 0.
	EncryptMACCommands bool

	// Confirmed defines if the frame must be send as confirmed-data.
	Confirmed bool

	// MoreData defines if there is more data pending.
	MoreData bool

	// Data contains the bytes to send. Note that this requires FPort to be a
	// value other than 0.
	Data []byte

	// PHYPayload holds the LoRaWAN PHYPayload.
	PHYPayload lorawan.PHYPayload

	// RXPacket holds the received uplink packet (in case of Class-A downlink).
	RXPacket *models.RXPacket
}

func (ctx dataContext) Validate() error {
	if ctx.FPort == 0 && len(ctx.Data) > 0 {
		return ErrFPortMustNotBeZero
	}

	if ctx.FPort > 224 {
		return ErrInvalidAppFPort
	}

	if ctx.FPort > 0 && ctx.EncryptMACCommands {
		return ErrFPortMustBeZero
	}

	return nil
}

// HandleResponse handles a downlink response.
func HandleResponse(rxPacket models.RXPacket, sp storage.ServiceProfile, ds storage.DeviceSession, adr, mustSend, ack bool) error {
	ctx := dataContext{
		ServiceProfile: sp,
		DeviceSession:  ds,
		ACK:            ack,
		MustSend:       mustSend,
		RXPacket:       &rxPacket,
	}

	for _, t := range responseTasks {
		if err := t(&ctx); err != nil {
			if err == ErrAbort {
				return nil
			}

			return err
		}
	}

	return nil
}

// HandleScheduleNextQueueItem handles scheduling the next device-queue item.
func HandleScheduleNextQueueItem(ds storage.DeviceSession) error {
	ctx := dataContext{
		DeviceSession: ds,
	}

	for _, t := range scheduleNextQueueItemTasks {
		if err := t(&ctx); err != nil {
			if err == ErrAbort {
				return nil
			}
			return err
		}
	}

	return nil
}

func setToken(ctx *dataContext) error {
	b := make([]byte, 2)
	_, err := rand.Read(b)
	if err != nil {
		return errors.Wrap(err, "read random error")
	}
	ctx.Token = binary.BigEndian.Uint16(b)
	return nil
}

func requestDevStatus(ctx *dataContext) error {
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

func getDataTXInfo(ctx *dataContext) error {
	if len(ctx.DeviceSession.LastRXInfoSet) == 0 {
		return ErrNoLastRXInfoSet
	}
	rxInfo := ctx.DeviceSession.LastRXInfoSet[0]
	var err error
	ctx.TXInfo, ctx.DataRate, err = getDataDownTXInfoAndDR(ctx.DeviceSession, ctx.RXPacket.TXInfo, rxInfo)
	if err != nil {
		return errors.Wrap(err, "get data down tx-info error")
	}

	return nil
}

func getDataTXInfoForRX2(ctx *dataContext) error {
	if len(ctx.DeviceSession.LastRXInfoSet) == 0 {
		return ErrNoLastRXInfoSet
	}
	rxInfo := ctx.DeviceSession.LastRXInfoSet[0]

	if int(ctx.DeviceSession.RX2DR) > len(config.C.NetworkServer.Band.Band.DataRates)-1 {
		return errors.Wrapf(ErrInvalidDataRate, "dr: %d (max dr: %d)", ctx.DeviceSession.RX2DR, len(config.C.NetworkServer.Band.Band.DataRates)-1)
	}

	ctx.TXInfo = gw.TXInfo{
		MAC:         rxInfo.MAC,
		Immediately: true,
		Frequency:   int(config.C.NetworkServer.Band.Band.RX2Frequency),
		Power:       config.C.NetworkServer.Band.Band.DefaultTXPower,
		DataRate:    config.C.NetworkServer.Band.Band.DataRates[int(ctx.DeviceSession.RX2DR)],
		CodeRate:    "4/5",
	}
	ctx.DataRate = int(ctx.DeviceSession.RX2DR)

	return nil
}

func setRemainingPayloadSize(ctx *dataContext) error {
	ctx.RemainingPayloadSize = config.C.NetworkServer.Band.Band.MaxPayloadSize[ctx.DataRate].N - len(ctx.Data)

	if ctx.RemainingPayloadSize < 0 {
		return ErrMaxPayloadSizeExceeded
	}

	return nil
}

func getNextDeviceQueueItem(ctx *dataContext) error {
	qi, err := storage.GetNextDeviceQueueItemForDevEUIMaxPayloadSizeAndFCnt(config.C.PostgreSQL.DB, ctx.DeviceSession.DevEUI, ctx.RemainingPayloadSize, ctx.DeviceSession.FCntDown, ctx.DeviceSession.RoutingProfileID)
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

	items, err := storage.GetDeviceQueueItemsForDevEUI(config.C.PostgreSQL.DB, ctx.DeviceSession.DevEUI)
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
		if err := storage.DeleteDeviceQueueItem(config.C.PostgreSQL.DB, qi.ID); err != nil {
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

		if err := storage.UpdateDeviceQueueItem(config.C.PostgreSQL.DB, &qi); err != nil {
			return errors.Wrap(err, "update device-queue item error")
		}
	}

	return nil
}

func getMACCommands(ctx *dataContext) error {
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
		if err = maccommand.SetPending(config.C.Redis.Pool, ctx.DeviceSession.DevEUI, block); err != nil {
			return errors.Wrap(err, "set mac-command block as pending error")
		}

		if err = maccommand.DeleteQueueItem(config.C.Redis.Pool, ctx.DeviceSession.DevEUI, block); err != nil {
			return errors.Wrap(err, "delete mac-command block from queue error")
		}
	}

	return nil
}

func stopOnNothingToSend(ctx *dataContext) error {
	if ctx.FPort == 0 && len(ctx.MACCommands) == 0 && !ctx.ACK && !ctx.MustSend {
		// ErrAbort will not be handled as a real error
		return ErrAbort
	}

	return nil
}

func sendDataDown(ctx *dataContext) error {
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

	ctx.PHYPayload = phy

	// send the packet to the gateway
	if err := config.C.NetworkServer.Gateway.Backend.Backend.SendTXPacket(gw.TXPacket{
		Token:      ctx.Token,
		TXInfo:     ctx.TXInfo,
		PHYPayload: phy,
	}); err != nil {
		return errors.Wrap(err, "send tx packet to gateway error")
	}

	// increment downlink framecounter
	ctx.DeviceSession.FCntDown++

	// set last downlink tx timestamp
	ctx.DeviceSession.LastDownlinkTX = time.Now()

	return nil
}

func saveDeviceSession(ctx *dataContext) error {
	if err := storage.SaveDeviceSession(config.C.Redis.Pool, ctx.DeviceSession); err != nil {
		return errors.Wrap(err, "save device-session error")
	}
	return nil
}

func getDeviceProfile(ctx *dataContext) error {
	var err error
	ctx.DeviceProfile, err = storage.GetAndCacheDeviceProfile(config.C.PostgreSQL.DB, config.C.Redis.Pool, ctx.DeviceSession.DeviceProfileID)
	if err != nil {
		return errors.Wrap(err, "get device-profile error")
	}
	return nil
}

func getServiceProfile(ctx *dataContext) error {
	var err error
	ctx.ServiceProfile, err = storage.GetAndCacheServiceProfile(config.C.PostgreSQL.DB, config.C.Redis.Pool, ctx.DeviceSession.ServiceProfileID)
	if err != nil {
		return errors.Wrap(err, "get service-profile error")
	}
	return nil
}

func checkLastDownlinkTimestamp(ctx *dataContext) error {
	// in case of Class-C validate that between now and the last downlink
	// tx timestamp is at least the class-c lock duration
	if ctx.DeviceProfile.SupportsClassC && time.Now().Sub(ctx.DeviceSession.LastDownlinkTX) < config.ClassCDownlinkLockDuration {
		log.WithFields(log.Fields{
			"time":                           time.Now(),
			"last_downlink_tx_time":          ctx.DeviceSession.LastDownlinkTX,
			"class_c_downlink_lock_duration": config.ClassCDownlinkLockDuration,
		}).Debug("skip next downlink queue scheduling dueue to class-c downlink lock")
		return ErrAbort
	}

	return nil
}

func getDataDownTXInfoAndDR(ds storage.DeviceSession, lastTXInfo models.TXInfo, rxInfo models.RXInfo) (gw.TXInfo, int, error) {
	var dr int
	txInfo := gw.TXInfo{
		MAC:      rxInfo.MAC,
		CodeRate: lastTXInfo.CodeRate,
		Power:    config.C.NetworkServer.Band.Band.DefaultTXPower,
	}

	var timestamp uint32

	if ds.RXWindow == storage.RX1 {
		uplinkDR, err := config.C.NetworkServer.Band.Band.GetDataRate(lastTXInfo.DataRate)
		if err != nil {
			return txInfo, 0, errors.Wrap(err, "get data-rate error")
		}

		// get rx1 dr
		dr, err = config.C.NetworkServer.Band.Band.GetRX1DataRate(uplinkDR, int(ds.RX1DROffset))
		if err != nil {
			return txInfo, dr, err
		}
		txInfo.DataRate = config.C.NetworkServer.Band.Band.DataRates[dr]

		// get rx1 frequency
		txInfo.Frequency, err = config.C.NetworkServer.Band.Band.GetRX1Frequency(lastTXInfo.Frequency)
		if err != nil {
			return txInfo, dr, err
		}

		// get timestamp
		timestamp = rxInfo.Timestamp + uint32(config.C.NetworkServer.Band.Band.ReceiveDelay1/time.Microsecond)
		if ds.RXDelay > 0 {
			timestamp = rxInfo.Timestamp + uint32(time.Duration(ds.RXDelay)*time.Second/time.Microsecond)
		}
	} else if ds.RXWindow == storage.RX2 {
		// rx2 dr
		dr = int(ds.RX2DR)
		if dr > len(config.C.NetworkServer.Band.Band.DataRates)-1 {
			return txInfo, 0, fmt.Errorf("invalid rx2 dr: %d (max dr: %d)", dr, len(config.C.NetworkServer.Band.Band.DataRates)-1)
		}
		txInfo.DataRate = config.C.NetworkServer.Band.Band.DataRates[dr]

		// rx2 frequency
		txInfo.Frequency = config.C.NetworkServer.Band.Band.RX2Frequency

		// rx2 timestamp (rx1 + 1 sec)
		timestamp = rxInfo.Timestamp + uint32(config.C.NetworkServer.Band.Band.ReceiveDelay1/time.Microsecond)
		if ds.RXDelay > 0 {
			timestamp = rxInfo.Timestamp + uint32(time.Duration(ds.RXDelay)*time.Second/time.Microsecond)
		}
		timestamp = timestamp + uint32(time.Second/time.Microsecond)
	} else {
		return txInfo, dr, fmt.Errorf("unknown RXWindow option %d", ds.RXWindow)
	}

	txInfo.Timestamp = &timestamp

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
	allBlocks, err := maccommand.ReadQueueItems(config.C.Redis.Pool, ds.DevEUI)
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

func logDownlinkFrame(ctx *dataContext) error {
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
