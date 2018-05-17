package data

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/adr"
	"github.com/brocaar/loraserver/internal/channels"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/framelog"
	"github.com/brocaar/loraserver/internal/maccommand"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
)

type deviceClass int

const (
	classB deviceClass = iota
	classC
)

const defaultCodeRate = "4/5"

type incompatibleCIDMapping struct {
	CID              lorawan.CID
	IncompatibleCIDs []lorawan.CID
}

var incompatibleMACCommands = []incompatibleCIDMapping{
	{CID: lorawan.NewChannelReq, IncompatibleCIDs: []lorawan.CID{lorawan.LinkADRReq}},
	{CID: lorawan.LinkADRReq, IncompatibleCIDs: []lorawan.CID{lorawan.NewChannelReq}},
}

var setMACCommandsSet = setMACCommands(
	requestCustomChannelReconfiguration,
	requestChannelMaskReconfiguration,
	requestADRChange,
	requestDevStatus,
	setPingSlotParameters,
	setRXParameters,
	getMACCommandsFromQueue,
)

var responseTasks = []func(*dataContext) error{
	setToken,
	getDeviceProfile,
	getDataTXInfo,
	setRemainingPayloadSize,
	getNextDeviceQueueItem,
	setMACCommandsSet,
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
	forClass(classC,
		getDataTXInfoForRX2,
	),
	forClass(classB,
		checkBeaconLocked,
		setTXInfoForClassB,
	),
	setRemainingPayloadSize,
	getNextDeviceQueueItem,
	setMACCommandsSet,
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
	MACCommands []storage.MACCommandBlock

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

	return nil
}

func forClass(class deviceClass, tasks ...func(*dataContext) error) func(*dataContext) error {
	return func(ctx *dataContext) error {
		if class == classC && !ctx.DeviceProfile.SupportsClassC {
			return nil
		}

		if class == classB && !ctx.DeviceProfile.SupportsClassB {
			return nil
		}

		for _, f := range tasks {
			if err := f(ctx); err != nil {
				return err
			}
		}

		return nil
	}
}

// HandleResponse handles a downlink response.
func HandleResponse(rxPacket models.RXPacket, sp storage.ServiceProfile, ds storage.DeviceSession, adr, mustSend, ack bool, macCommands []storage.MACCommandBlock) error {
	ctx := dataContext{
		ServiceProfile: sp,
		DeviceSession:  ds,
		ACK:            ack,
		MustSend:       mustSend,
		RXPacket:       &rxPacket,
		MACCommands:    macCommands,
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
		ctx.MACCommands = append(ctx.MACCommands, maccommand.RequestDevStatus(&ctx.DeviceSession))
	}

	return nil
}

func setPingSlotParameters(ctx *dataContext) error {
	if !ctx.DeviceProfile.SupportsClassB {
		return nil
	}

	dr := config.C.NetworkServer.NetworkSettings.ClassB.PingSlotDR
	freq := config.C.NetworkServer.NetworkSettings.ClassB.PingSlotFrequency

	if dr != ctx.DeviceSession.PingSlotDR || freq != ctx.DeviceSession.PingSlotFrequency {
		block := maccommand.RequestPingSlotChannel(ctx.DeviceSession.DevEUI, dr, freq)
		ctx.MACCommands = append(ctx.MACCommands, block)
	}

	return nil
}

func setRXParameters(ctx *dataContext) error {
	if ctx.DeviceSession.RX2Frequency != config.C.NetworkServer.NetworkSettings.RX2Frequency || ctx.DeviceSession.RX2DR != uint8(config.C.NetworkServer.NetworkSettings.RX2DR) || ctx.DeviceSession.RX1DROffset != uint8(config.C.NetworkServer.NetworkSettings.RX1DROffset) {
		block := maccommand.RequestRXParamSetup(config.C.NetworkServer.NetworkSettings.RX1DROffset, config.C.NetworkServer.NetworkSettings.RX2Frequency, config.C.NetworkServer.NetworkSettings.RX2DR)
		ctx.MACCommands = append(ctx.MACCommands, block)
	}

	if ctx.DeviceSession.RXDelay != uint8(config.C.NetworkServer.NetworkSettings.RX1Delay) {
		block := maccommand.RequestRXTimingSetup(config.C.NetworkServer.NetworkSettings.RX1Delay)
		ctx.MACCommands = append(ctx.MACCommands, block)
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

	dr, err := config.C.NetworkServer.Band.Band.GetDataRate(int(ctx.DeviceSession.RX2DR))
	if err != nil {
		return errors.Wrap(err, "get data-rate error")
	}

	ctx.TXInfo = gw.TXInfo{
		MAC:         rxInfo.MAC,
		Immediately: true,
		Frequency:   ctx.DeviceSession.RX2Frequency,
		Power:       config.C.NetworkServer.Band.Band.GetDownlinkTXPower(ctx.DeviceSession.RX2Frequency),
		DataRate:    dr,
		CodeRate:    defaultCodeRate,
	}
	ctx.DataRate = int(ctx.DeviceSession.RX2DR)

	return nil
}

func checkBeaconLocked(ctx *dataContext) error {
	if !ctx.DeviceSession.BeaconLocked {
		return ErrAbort
	}
	return nil
}

func setTXInfoForClassB(ctx *dataContext) error {
	if len(ctx.DeviceSession.LastRXInfoSet) == 0 {
		return ErrNoLastRXInfoSet
	}
	rxInfo := ctx.DeviceSession.LastRXInfoSet[0]

	dr, err := config.C.NetworkServer.Band.Band.GetDataRate(ctx.DeviceSession.PingSlotDR)
	if err != nil {
		return errors.Wrap(err, "get data-rate error")
	}

	ctx.TXInfo = gw.TXInfo{
		MAC:       rxInfo.MAC,
		Frequency: ctx.DeviceSession.PingSlotFrequency,
		Power:     config.C.NetworkServer.Band.Band.GetDownlinkTXPower(ctx.DeviceSession.PingSlotFrequency),
		DataRate:  dr,
		CodeRate:  defaultCodeRate,
	}
	ctx.DataRate = ctx.DeviceSession.PingSlotDR

	return nil
}

func setRemainingPayloadSize(ctx *dataContext) error {
	plSize, err := config.C.NetworkServer.Band.Band.GetMaxPayloadSizeForDataRateIndex(ctx.DeviceProfile.DeviceProfile.MACVersion, ctx.DeviceProfile.DeviceProfile.RegParamsRevision, ctx.DataRate)
	if err != nil {
		return errors.Wrap(err, "get max-payload size error")
	}

	ctx.RemainingPayloadSize = plSize.N - len(ctx.Data)

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

	// Update TXInfo with Class-B scheduling info
	if ctx.RXPacket == nil && qi.EmitAtTimeSinceGPSEpoch != nil {
		gwDuration := gw.Duration(*qi.EmitAtTimeSinceGPSEpoch)
		ctx.TXInfo.TimeSinceGPSEpoch = &gwDuration

		if ctx.DeviceSession.PingSlotFrequency == 0 {
			beaconTime := *qi.EmitAtTimeSinceGPSEpoch - (*qi.EmitAtTimeSinceGPSEpoch % (128 * time.Second))
			ctx.TXInfo.Frequency, err = config.C.NetworkServer.Band.Band.GetPingSlotFrequency(ctx.DeviceSession.DevAddr, beaconTime)
			if err != nil {
				return errors.Wrap(err, "get ping-slot frequency error")
			}
		}
	}

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
		qi.IsPending = true

		// in case of class-b it is already set, we don't want to overwrite it
		if qi.TimeoutAfter == nil {
			qi.TimeoutAfter = &timeout
		}

		if err := storage.UpdateDeviceQueueItem(config.C.PostgreSQL.DB, &qi); err != nil {
			return errors.Wrap(err, "update device-queue item error")
		}
	}

	return nil
}

func setMACCommands(funcs ...func(*dataContext) error) func(*dataContext) error {
	return func(ctx *dataContext) error {
		// this will set the mac-commands to MACCommands, potentially exceeding the max size
		for _, f := range funcs {
			if err := f(ctx); err != nil {
				return err
			}
		}

		for _, mapping := range incompatibleMACCommands {
			var seen bool
			for _, mac := range ctx.MACCommands {
				if mapping.CID == mac.CID {
					seen = true
				}
			}

			if seen {
				for _, conflictingCID := range mapping.IncompatibleCIDs {
					for i := len(ctx.MACCommands) - 1; i != 0; i-- {
						if ctx.MACCommands[i].CID == conflictingCID {
							ctx.MACCommands = append(ctx.MACCommands[:i], ctx.MACCommands[i+1:]...)
						}
					}
				}
			}
		}

		var remainingMACCommandSize int

		if ctx.FPort > 0 {
			if ctx.RemainingPayloadSize < 15 {
				remainingMACCommandSize = ctx.RemainingPayloadSize
			} else {
				remainingMACCommandSize = 15
			}
		} else {
			remainingMACCommandSize = ctx.RemainingPayloadSize
		}

		for i, block := range ctx.MACCommands {
			macSize, err := block.Size()
			if err != nil {
				return errors.Wrap(err, "get mac-command block size error")
			}

			// truncate mac-commands when we exceed the max-size
			if remainingMACCommandSize-macSize < 0 {
				ctx.MACCommands = ctx.MACCommands[0:i]
				ctx.MoreData = true
				break
			}
		}

		for _, block := range ctx.MACCommands {
			// set mac-command pending
			if err := storage.SetPendingMACCommand(config.C.Redis.Pool, ctx.DeviceSession.DevEUI, block); err != nil {
				return errors.Wrap(err, "set mac-command pending error")
			}

			// delete from queue, if external
			if block.External {
				if err := storage.DeleteMACCommandQueueItem(config.C.Redis.Pool, ctx.DeviceSession.DevEUI, block); err != nil {
					return errors.Wrap(err, "delete mac-command block from queue error")
				}
			}
		}

		return nil
	}
}

func requestCustomChannelReconfiguration(ctx *dataContext) error {
	wantedChannels := make(map[int]band.Channel)
	for _, i := range config.C.NetworkServer.Band.Band.GetCustomUplinkChannelIndices() {
		c, err := config.C.NetworkServer.Band.Band.GetUplinkChannel(i)
		if err != nil {
			return errors.Wrap(err, "get uplink channel error")
		}
		wantedChannels[i] = c
	}

	// cleanup channels that do not exist anydmore
	// these will be disabled by the LinkADRReq channel-mask reconfiguration
	for k := range ctx.DeviceSession.ExtraUplinkChannels {
		if _, ok := wantedChannels[k]; !ok {
			delete(ctx.DeviceSession.ExtraUplinkChannels, k)
		}
	}

	block := maccommand.RequestNewChannels(ctx.DeviceSession.DevEUI, 3, ctx.DeviceSession.ExtraUplinkChannels, wantedChannels)
	if block != nil {
		ctx.MACCommands = append(ctx.MACCommands, *block)
	}

	return nil
}

func requestChannelMaskReconfiguration(ctx *dataContext) error {
	// handle channel configuration
	// note that this must come before ADR!
	blocks, err := channels.HandleChannelReconfigure(ctx.DeviceSession)
	if err != nil {
		log.WithFields(log.Fields{
			"dev_eui": ctx.DeviceSession.DevEUI,
		}).Warningf("handle channel reconfigure error: %s", err)
	} else {
		ctx.MACCommands = append(ctx.MACCommands, blocks...)
	}

	return nil
}

func requestADRChange(ctx *dataContext) error {
	var linkADRReq *storage.MACCommandBlock
	for i := range ctx.MACCommands {
		if ctx.MACCommands[i].CID == lorawan.LinkADRReq {
			linkADRReq = &ctx.MACCommands[i]
		}
	}

	blocks, err := adr.HandleADR(ctx.DeviceSession, linkADRReq)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"dev_eui": ctx.DeviceSession.DevEUI,
		}).Warning("handle adr error")
		return nil
	}

	if linkADRReq == nil {
		ctx.MACCommands = append(ctx.MACCommands, blocks...)
	}
	return nil
}

func getMACCommandsFromQueue(ctx *dataContext) error {
	blocks, err := storage.GetMACCommandQueueItems(config.C.Redis.Pool, ctx.DeviceSession.DevEUI)
	if err != nil {
		return errors.Wrap(err, "get mac-command queue items error")
	}

	for i := range blocks {
		ctx.MACCommands = append(ctx.MACCommands, blocks[i])
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

	if ctx.FPort > 0 {
		macPL.FPort = &ctx.FPort
		macPL.FRMPayload = []lorawan.Payload{
			&lorawan.DataPayload{Bytes: ctx.Data},
		}
	}

	var macCommandSize int
	var maccommands []lorawan.MACCommand

	for i := range ctx.MACCommands {
		s, err := ctx.MACCommands[i].Size()
		if err != nil {
			return errors.Wrap(err, "get mac-command block size")
		}
		macCommandSize += s

		maccommands = append(maccommands, ctx.MACCommands[i].MACCommands...)
	}

	if macCommandSize > 15 && ctx.FPort == 0 {
		var frmPayload []lorawan.Payload
		for i := range maccommands {
			frmPayload = append(frmPayload, &maccommands[i])
		}
		macPL.FPort = &ctx.FPort
		macPL.FRMPayload = frmPayload
	} else if macCommandSize <= 15 {
		macPL.FHDR.FOpts = maccommands
	} else {
		// this should not happen, but log it in case it would
		log.WithFields(log.Fields{
			"dev_eui": ctx.DeviceSession.DevEUI,
		}).Error("mac-commands exceeded size!")
	}

	phy := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: lorawan.UnconfirmedDataDown,
			Major: lorawan.LoRaWANR1,
		},
		MACPayload: macPL,
	}
	if ctx.Confirmed {
		phy.MHDR.MType = lorawan.ConfirmedDataDown
	}

	if macCommandSize > 15 && ctx.FPort == 0 {
		if err := phy.EncryptFRMPayload(ctx.DeviceSession.NwkSKey); err != nil {
			return errors.Wrap(err, "encrypt frmpayload error")
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
	}

	var timestamp uint32

	if ds.RXWindow == storage.RX1 {
		uplinkDR, err := config.C.NetworkServer.Band.Band.GetDataRateIndex(true, lastTXInfo.DataRate)
		if err != nil {
			return txInfo, 0, errors.Wrap(err, "get data-rate index error")
		}

		// get rx1 dr
		dr, err = config.C.NetworkServer.Band.Band.GetRX1DataRateIndex(uplinkDR, int(ds.RX1DROffset))
		if err != nil {
			return txInfo, dr, errors.Wrap(err, "get rx1 data-rate index error")
		}
		txInfo.DataRate, err = config.C.NetworkServer.Band.Band.GetDataRate(dr)
		if err != nil {
			return txInfo, dr, errors.Wrap(err, "get data-rate error")
		}

		// get rx1 frequency
		txInfo.Frequency, err = config.C.NetworkServer.Band.Band.GetRX1FrequencyForUplinkFrequency(lastTXInfo.Frequency)
		if err != nil {
			return txInfo, dr, errors.Wrap(err, "get rx1 frequency for uplink frequency error")
		}

		// get timestamp
		timestamp = rxInfo.Timestamp + uint32(config.C.NetworkServer.Band.Band.GetDefaults().ReceiveDelay1/time.Microsecond)
		if ds.RXDelay > 0 {
			timestamp = rxInfo.Timestamp + uint32(time.Duration(ds.RXDelay)*time.Second/time.Microsecond)
		}
	} else if ds.RXWindow == storage.RX2 {
		var err error
		txInfo.DataRate, err = config.C.NetworkServer.Band.Band.GetDataRate(int(ds.RX2DR))
		if err != nil {
			return txInfo, 0, fmt.Errorf("get data-rate error")
		}

		// rx2 frequency
		txInfo.Frequency = ds.RX2Frequency

		// rx2 timestamp (rx1 + 1 sec)
		timestamp = rxInfo.Timestamp + uint32(config.C.NetworkServer.Band.Band.GetDefaults().ReceiveDelay1/time.Microsecond)
		if ds.RXDelay > 0 {
			timestamp = rxInfo.Timestamp + uint32(time.Duration(ds.RXDelay)*time.Second/time.Microsecond)
		}
		timestamp = timestamp + uint32(time.Second/time.Microsecond)
	} else {
		return txInfo, dr, fmt.Errorf("unknown RXWindow option %d", ds.RXWindow)
	}

	txInfo.Timestamp = &timestamp
	txInfo.Power = config.C.NetworkServer.Band.Band.GetDownlinkTXPower(txInfo.Frequency)

	// when we received a FSK modulated uplink, and reply using LoRa modulation
	// we need to set the CodeRate as this is not set on the FSK uplink.
	if txInfo.DataRate.Modulation == band.LoRaModulation && txInfo.CodeRate == "" {
		txInfo.CodeRate = defaultCodeRate
	}

	return txInfo, dr, nil
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
