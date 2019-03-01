package data

import (
	"crypto/rand"
	"encoding/binary"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/adr"
	"github.com/brocaar/loraserver/internal/backend/gateway"
	"github.com/brocaar/loraserver/internal/band"
	"github.com/brocaar/loraserver/internal/channels"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/framelog"
	"github.com/brocaar/loraserver/internal/helpers"
	"github.com/brocaar/loraserver/internal/maccommand"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
	loraband "github.com/brocaar/lorawan/band"
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

var (
	// rejoin-request variabled
	rejoinRequestEnabled   bool
	rejoinRequestMaxCountN int
	rejoinRequestMaxTimeN  int

	// Class-B
	classBPingSlotDR        int
	classBPingSlotFrequency int

	// RX window
	rxWindow int

	// RX2 params
	rx2Frequency int
	rx2DR        int

	// RX1 params
	rx1DROffset int
	rx1Delay    int

	// Downlink TX power
	downlinkTXPower int

	// MAC Commands
	disableMACCommands bool

	// ADR
	disableADR bool

	// ClassC
	classCDownlinkLockDuration time.Duration
)

var setMACCommandsSet = setMACCommands(
	requestCustomChannelReconfiguration,
	requestChannelMaskReconfiguration,
	requestADRChange,
	requestDevStatus,
	requestRejoinParamSetup,
	setPingSlotParameters,
	setRXParameters,
	getMACCommandsFromQueue,
)

var responseTasks = []func(*dataContext) error{
	getDeviceProfile,
	getServiceProfile,
	setDataTXInfo,
	setToken,
	getNextDeviceQueueItem,
	setMACCommandsSet,
	stopOnNothingToSend,
	setPHYPayloads,
	sendDownlinkFrame,
	saveDeviceSession,
	saveRemainingFrames,
}

var scheduleNextQueueItemTasks = []func(*dataContext) error{
	getDeviceProfile,
	getServiceProfile,
	checkLastDownlinkTimestamp,
	forClass(storage.DeviceModeC,
		setImmediately,
		setTXInfoForRX2,
	),
	forClass(storage.DeviceModeB,
		setTXInfoForClassB,
	),
	forClass(storage.DeviceModeA,
		returnInvalidDeviceClassError,
	),
	setToken,
	getNextDeviceQueueItem,
	setMACCommandsSet,
	stopOnNothingToSend,
	setPHYPayloads,
	sendDownlinkFrame,
	saveDeviceSession,
}

// Setup configures the package.
func Setup(conf config.Config) error {
	nsConf := conf.NetworkServer.NetworkSettings
	rejoinRequestEnabled = nsConf.RejoinRequest.Enabled
	rejoinRequestMaxCountN = nsConf.RejoinRequest.MaxCountN
	rejoinRequestMaxTimeN = nsConf.RejoinRequest.MaxTimeN

	classBPingSlotDR = nsConf.ClassB.PingSlotDR
	classBPingSlotFrequency = nsConf.ClassB.PingSlotFrequency

	rx2Frequency = nsConf.RX2Frequency
	rx2DR = nsConf.RX2DR
	rx1DROffset = nsConf.RX1DROffset
	rx1Delay = nsConf.RX1Delay
	rxWindow = nsConf.RXWindow

	downlinkTXPower = nsConf.DownlinkTXPower

	disableMACCommands = nsConf.DisableMACCommands
	disableADR = nsConf.DisableADR

	classCDownlinkLockDuration = conf.NetworkServer.Scheduler.ClassC.DownlinkLockDuration

	return nil
}

type dataContext struct {
	// Device mode.
	DeviceMode storage.DeviceMode

	// ServiceProfile of the device.
	ServiceProfile storage.ServiceProfile

	// DeviceProfile of the device.
	DeviceProfile storage.DeviceProfile

	// DeviceSession holds the device-session of the device for which to send
	// the downlink data.
	DeviceSession storage.DeviceSession

	// MustSend defines if a frame must be send. In some cases (e.g. ADRACKReq)
	// the network-server must respond, even when there are no mac-commands or
	// FRMPayload.
	MustSend bool

	// ACK defines if ACK must be set to true (e.g. the frame acknowledges
	// an uplink frame).
	ACK bool

	// FPort to use for transmission. This must be set to a value != 0 in case
	// Data is not empty.
	FPort uint8

	// Confirmed defines if the frame must be send as confirmed-data.
	Confirmed bool

	// MoreData defines if there is more data pending.
	MoreData bool

	// Data contains the bytes to send. Note that this requires FPort to be a
	// value other than 0.
	Data []byte

	// RXPacket holds the received uplink packet (in case of Class-A downlink).
	RXPacket *models.RXPacket

	// Immediately indicates that the response must be sent immediately.
	Immediately bool

	// MACCommands contains the mac-commands to send (if any). Make sure the
	// total size fits within the FRMPayload or FOpts.
	MACCommands []storage.MACCommandBlock

	// Downlink frames to be emitted (this can be a slice e.g. to first try
	// using RX1 parameters, failing that RX2 parameters).
	// Only the first item will be emitted, the other(s) will be enqueued
	// and emitted on a scheduling error.
	DownlinkFrames []downlinkFrame
}

type downlinkFrame struct {
	// Downlink frame
	DownlinkFrame gw.DownlinkFrame

	// The remaining payload size which can be used for mac-commands and / or
	// FRMPayload.
	RemainingPayloadSize int
}

func (ctx dataContext) Validate() error {
	if ctx.FPort == 0 && len(ctx.Data) > 0 {
		return ErrFPortMustNotBeZero
	}

	return nil
}

func forClass(mode storage.DeviceMode, tasks ...func(*dataContext) error) func(*dataContext) error {
	return func(ctx *dataContext) error {
		if mode != ctx.DeviceMode {
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
func HandleScheduleNextQueueItem(ds storage.DeviceSession, mode storage.DeviceMode) error {
	ctx := dataContext{
		DeviceMode:    mode,
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

	for i := range ctx.DownlinkFrames {
		ctx.DownlinkFrames[i].DownlinkFrame.Token = uint32(binary.BigEndian.Uint16(b))
	}
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

func requestRejoinParamSetup(ctx *dataContext) error {
	if !rejoinRequestEnabled || ctx.DeviceSession.GetMACVersion() == lorawan.LoRaWAN1_0 {
		return nil
	}

	if !ctx.DeviceSession.RejoinRequestEnabled ||
		ctx.DeviceSession.RejoinRequestMaxCountN != rejoinRequestMaxCountN ||
		ctx.DeviceSession.RejoinRequestMaxTimeN != rejoinRequestMaxTimeN {
		ctx.MACCommands = append(ctx.MACCommands, maccommand.RequestRejoinParamSetup(
			rejoinRequestMaxTimeN,
			rejoinRequestMaxCountN,
		))
	}

	return nil
}

func setPingSlotParameters(ctx *dataContext) error {
	if !ctx.DeviceProfile.SupportsClassB {
		return nil
	}

	if classBPingSlotDR != ctx.DeviceSession.PingSlotDR || classBPingSlotFrequency != ctx.DeviceSession.PingSlotFrequency {
		block := maccommand.RequestPingSlotChannel(ctx.DeviceSession.DevEUI, classBPingSlotDR, classBPingSlotFrequency)
		ctx.MACCommands = append(ctx.MACCommands, block)
	}

	return nil
}

func setRXParameters(ctx *dataContext) error {
	if ctx.DeviceSession.RX2Frequency != rx2Frequency || ctx.DeviceSession.RX2DR != uint8(rx2DR) || ctx.DeviceSession.RX1DROffset != uint8(rx1DROffset) {
		block := maccommand.RequestRXParamSetup(rx1DROffset, rx2Frequency, rx2DR)
		ctx.MACCommands = append(ctx.MACCommands, block)
	}

	if ctx.DeviceSession.RXDelay != uint8(rx1Delay) {
		block := maccommand.RequestRXTimingSetup(rx1Delay)
		ctx.MACCommands = append(ctx.MACCommands, block)
	}

	return nil
}

func setDataTXInfo(ctx *dataContext) error {
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

func setTXInfoForRX1(ctx *dataContext) error {
	if len(ctx.RXPacket.RXInfoSet) == 0 {
		return ErrNoLastRXInfoSet
	}
	rxInfo := ctx.RXPacket.RXInfoSet[0]

	txInfo := gw.DownlinkTXInfo{
		GatewayId: rxInfo.GatewayId,
		Board:     rxInfo.Board,
		Antenna:   rxInfo.Antenna,
	}

	// get rx1 data-rate
	uplinkDR, err := helpers.GetDataRateIndex(true, ctx.RXPacket.TXInfo, band.Band())
	if err != nil {
		return errors.Wrap(err, "get data-rate index error")
	}

	rx1DR, err := band.Band().GetRX1DataRateIndex(uplinkDR, int(ctx.DeviceSession.RX1DROffset))
	if err != nil {
		return errors.Wrap(err, "get rx1 data-rate index error")
	}

	err = helpers.SetDownlinkTXInfoDataRate(&txInfo, rx1DR, band.Band())
	if err != nil {
		return errors.Wrap(err, "set downlink tx-info data-rate error")
	}

	// get rx1 frequency
	freq, err := band.Band().GetRX1FrequencyForUplinkFrequency(int(ctx.RXPacket.TXInfo.Frequency))
	if err != nil {
		return errors.Wrap(err, "get rx1 frequency error")
	}
	txInfo.Frequency = uint32(freq)

	// get timestamp
	txInfo.Timestamp = rxInfo.Timestamp + uint32(band.Band().GetDefaults().ReceiveDelay1/time.Microsecond)
	if ctx.DeviceSession.RXDelay > 0 {
		txInfo.Timestamp = rxInfo.Timestamp + uint32(time.Duration(ctx.DeviceSession.RXDelay)*time.Second/time.Microsecond)
	}

	// get tx power
	if downlinkTXPower != -1 {
		txInfo.Power = int32(downlinkTXPower)
	} else {
		txInfo.Power = int32(band.Band().GetDownlinkTXPower(int(txInfo.Frequency)))
	}

	// get remaining payload size
	plSize, err := band.Band().GetMaxPayloadSizeForDataRateIndex(ctx.DeviceProfile.MACVersion, ctx.DeviceProfile.RegParamsRevision, rx1DR)
	if err != nil {
		return errors.Wrap(err, "get max-payload size error")
	}

	ctx.DownlinkFrames = append(ctx.DownlinkFrames, downlinkFrame{
		DownlinkFrame: gw.DownlinkFrame{
			TxInfo: &txInfo,
		},
		RemainingPayloadSize: plSize.N,
	})

	return nil
}

func setImmediately(ctx *dataContext) error {
	ctx.Immediately = true
	return nil
}

func setTXInfoForRX2(ctx *dataContext) error {
	gatewayID, err := ctx.DeviceSession.GetDownlinkGatewayMAC()
	if err != nil {
		return err
	}

	var board, antenna, timestamp uint32
	if ctx.RXPacket != nil && len(ctx.RXPacket.RXInfoSet) != 0 {
		board = ctx.RXPacket.RXInfoSet[0].Board
		antenna = ctx.RXPacket.RXInfoSet[0].Antenna
		timestamp = ctx.RXPacket.RXInfoSet[0].Timestamp
	}

	txInfo := gw.DownlinkTXInfo{
		GatewayId:   gatewayID[:],
		Board:       board,
		Antenna:     antenna,
		Frequency:   uint32(ctx.DeviceSession.RX2Frequency),
		Immediately: ctx.Immediately,
	}

	// get data-rate
	err = helpers.SetDownlinkTXInfoDataRate(&txInfo, int(ctx.DeviceSession.RX2DR), band.Band())
	if err != nil {
		return errors.Wrap(err, "set downlink tx-info data-rate error")
	}

	// get tx power
	if downlinkTXPower != -1 {
		txInfo.Power = int32(downlinkTXPower)
	} else {
		txInfo.Power = int32(band.Band().GetDownlinkTXPower(int(txInfo.Frequency)))
	}

	// get timestamp (when not tx immediately)
	if !ctx.Immediately {
		txInfo.Timestamp = timestamp + uint32(band.Band().GetDefaults().ReceiveDelay2/time.Microsecond)
		if ctx.DeviceSession.RXDelay > 0 {
			txInfo.Timestamp = timestamp + uint32(time.Second*time.Duration(ctx.DeviceSession.RXDelay+1)/time.Microsecond)
		}
	}

	// get remaining payload size
	plSize, err := band.Band().GetMaxPayloadSizeForDataRateIndex(ctx.DeviceProfile.MACVersion, ctx.DeviceProfile.RegParamsRevision, int(ctx.DeviceSession.RX2DR))
	if err != nil {
		return errors.Wrap(err, "get max-payload size error")
	}

	ctx.DownlinkFrames = append(ctx.DownlinkFrames, downlinkFrame{
		DownlinkFrame: gw.DownlinkFrame{
			TxInfo: &txInfo,
		},
		RemainingPayloadSize: plSize.N,
	})

	return nil
}

func setTXInfoForClassB(ctx *dataContext) error {
	gatewayID, err := ctx.DeviceSession.GetDownlinkGatewayMAC()
	if err != nil {
		return err
	}

	var board, antenna uint32
	if ctx.RXPacket != nil && len(ctx.RXPacket.RXInfoSet) != 0 {
		board = ctx.RXPacket.RXInfoSet[0].Board
		antenna = ctx.RXPacket.RXInfoSet[0].Antenna
	}

	txInfo := gw.DownlinkTXInfo{
		GatewayId: gatewayID[:],
		Board:     board,
		Antenna:   antenna,
		Frequency: uint32(ctx.DeviceSession.PingSlotFrequency),
	}

	// get data-rate
	err = helpers.SetDownlinkTXInfoDataRate(&txInfo, ctx.DeviceSession.PingSlotDR, band.Band())
	if err != nil {
		return errors.Wrap(err, "set downlink tx-info data-rate error")
	}

	// get tx power
	if downlinkTXPower != -1 {
		txInfo.Power = int32(downlinkTXPower)
	} else {
		txInfo.Power = int32(band.Band().GetDownlinkTXPower(int(txInfo.Frequency)))
	}

	// get remaining payload size
	plSize, err := band.Band().GetMaxPayloadSizeForDataRateIndex(ctx.DeviceProfile.MACVersion, ctx.DeviceProfile.RegParamsRevision, int(ctx.DeviceSession.PingSlotDR))
	if err != nil {
		return errors.Wrap(err, "get max-payload size error")
	}

	ctx.DownlinkFrames = append(ctx.DownlinkFrames, downlinkFrame{
		DownlinkFrame: gw.DownlinkFrame{
			TxInfo: &txInfo,
		},
		RemainingPayloadSize: plSize.N,
	})

	return nil
}

func getNextDeviceQueueItem(ctx *dataContext) error {
	var fCnt uint32
	if ctx.DeviceSession.GetMACVersion() == lorawan.LoRaWAN1_0 {
		fCnt = ctx.DeviceSession.NFCntDown
	} else {
		fCnt = ctx.DeviceSession.AFCntDown
	}

	// the first downlink opportunity will be used to decide the
	// max payload size
	var remainingPayloadSize int
	if len(ctx.DownlinkFrames) > 0 {
		remainingPayloadSize = ctx.DownlinkFrames[0].RemainingPayloadSize
	}

	qi, err := storage.GetNextDeviceQueueItemForDevEUIMaxPayloadSizeAndFCnt(storage.DB(), ctx.DeviceSession.DevEUI, remainingPayloadSize, fCnt, ctx.DeviceSession.RoutingProfileID)
	if err != nil {
		if errors.Cause(err) == storage.ErrDoesNotExist {
			return nil
		}
		return errors.Wrap(err, "get next device-queue item for max payload error")
	}

	ctx.Confirmed = qi.Confirmed
	ctx.Data = qi.FRMPayload
	ctx.FPort = qi.FPort

	for i := range ctx.DownlinkFrames {
		ctx.DownlinkFrames[i].RemainingPayloadSize = ctx.DownlinkFrames[i].RemainingPayloadSize - len(ctx.Data)
	}

	items, err := storage.GetDeviceQueueItemsForDevEUI(storage.DB(), ctx.DeviceSession.DevEUI)
	if err != nil {
		return errors.Wrap(err, "get device-queue items error")
	}
	ctx.MoreData = len(items) > 1 // more than only the current frame

	// Set the device-session fCnt (down). We might have discarded one or
	// multiple frames (payload size) or the application-server might have
	// incremented the counter incorrectly. This is important since it is
	// used for decrypting the payload by the device!!
	if ctx.DeviceSession.GetMACVersion() == lorawan.LoRaWAN1_0 {
		ctx.DeviceSession.NFCntDown = qi.FCnt
	} else {
		ctx.DeviceSession.AFCntDown = qi.FCnt
	}

	// Update TXInfo with Class-B scheduling info
	if ctx.RXPacket == nil && qi.EmitAtTimeSinceGPSEpoch != nil && len(ctx.DownlinkFrames) == 1 {
		ctx.DownlinkFrames[0].DownlinkFrame.TxInfo.TimeSinceGpsEpoch = ptypes.DurationProto(*qi.EmitAtTimeSinceGPSEpoch)

		if ctx.DeviceSession.PingSlotFrequency == 0 {
			beaconTime := *qi.EmitAtTimeSinceGPSEpoch - (*qi.EmitAtTimeSinceGPSEpoch % (128 * time.Second))
			freq, err := band.Band().GetPingSlotFrequency(ctx.DeviceSession.DevAddr, beaconTime)
			if err != nil {
				return errors.Wrap(err, "get ping-slot frequency error")
			}
			ctx.DownlinkFrames[0].DownlinkFrame.TxInfo.Frequency = uint32(freq)
		}
	}

	if !qi.Confirmed {
		// delete when not confirmed
		if err := storage.DeleteDeviceQueueItem(storage.DB(), qi.ID); err != nil {
			return errors.Wrap(err, "delete device-queue item error")
		}
	} else {
		// Set the ConfFCnt to the FCnt of the queue-item.
		// When we receive an ACK, we need this to validate the MIC.
		ctx.DeviceSession.ConfFCnt = qi.FCnt

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

		if err := storage.UpdateDeviceQueueItem(storage.DB(), &qi); err != nil {
			return errors.Wrap(err, "update device-queue item error")
		}
	}

	return nil
}

func filterIncompatibleMACCommands(macCommands []storage.MACCommandBlock) []storage.MACCommandBlock {
	for _, mapping := range incompatibleMACCommands {
		var seen bool
		for _, mac := range macCommands {
			if mapping.CID == mac.CID {
				seen = true
			}
		}

		if seen {
			for _, conflictingCID := range mapping.IncompatibleCIDs {
				for i := len(macCommands) - 1; i >= 0; i-- {
					if macCommands[i].CID == conflictingCID {
						macCommands = append(macCommands[:i], macCommands[i+1:]...)
					}
				}
			}
		}
	}

	return macCommands
}

func setMACCommands(funcs ...func(*dataContext) error) func(*dataContext) error {
	return func(ctx *dataContext) error {
		// this will set the mac-commands to MACCommands, potentially exceeding the max size
		for _, f := range funcs {
			if err := f(ctx); err != nil {
				return err
			}
		}

		// In case mac-commands are disabled in the LoRa Server configuration,
		// only allow external mac-commands (e.g. scheduled by an external
		// controller).
		if disableMACCommands {
			var externalMACCommands []storage.MACCommandBlock

			for i := range ctx.MACCommands {
				if ctx.MACCommands[i].External {
					externalMACCommands = append(externalMACCommands, ctx.MACCommands[i])
				}
			}
			ctx.MACCommands = externalMACCommands
		}

		ctx.MACCommands = filterIncompatibleMACCommands(ctx.MACCommands)

		var remainingPayloadSize, remainingMACCommandSize int
		if len(ctx.DownlinkFrames) > 0 {
			remainingPayloadSize = ctx.DownlinkFrames[0].RemainingPayloadSize
		}

		if ctx.FPort > 0 {
			if remainingPayloadSize < 15 {
				remainingMACCommandSize = remainingPayloadSize
			} else {
				remainingMACCommandSize = 15
			}
		} else {
			remainingMACCommandSize = remainingPayloadSize
		}

		for i, block := range ctx.MACCommands {
			macSize, err := block.Size()
			if err != nil {
				return errors.Wrap(err, "get mac-command block size error")
			}

			remainingMACCommandSize = remainingMACCommandSize - macSize

			// truncate mac-commands when we exceed the max-size
			if remainingMACCommandSize < 0 {
				ctx.MACCommands = ctx.MACCommands[0:i]
				ctx.MoreData = true
				break
			}
		}

		for _, block := range ctx.MACCommands {
			// set mac-command pending
			if err := storage.SetPendingMACCommand(storage.RedisPool(), ctx.DeviceSession.DevEUI, block); err != nil {
				return errors.Wrap(err, "set mac-command pending error")
			}

			// delete from queue, if external
			if block.External {
				if err := storage.DeleteMACCommandQueueItem(storage.RedisPool(), ctx.DeviceSession.DevEUI, block); err != nil {
					return errors.Wrap(err, "delete mac-command block from queue error")
				}
			}
		}

		return nil
	}
}

func requestCustomChannelReconfiguration(ctx *dataContext) error {
	wantedChannels := make(map[int]loraband.Channel)
	for _, i := range band.Band().GetCustomUplinkChannelIndices() {
		c, err := band.Band().GetUplinkChannel(i)
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

	blocks, err := adr.HandleADR(ctx.ServiceProfile, ctx.DeviceSession, linkADRReq)
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
	blocks, err := storage.GetMACCommandQueueItems(storage.RedisPool(), ctx.DeviceSession.DevEUI)
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

func setPHYPayloads(ctx *dataContext) error {
	if err := ctx.Validate(); err != nil {
		return errors.Wrap(err, "validation error")
	}

	var fCnt uint32
	if ctx.DeviceSession.GetMACVersion() == lorawan.LoRaWAN1_0 || ctx.FPort == 0 {
		fCnt = ctx.DeviceSession.NFCntDown
		ctx.DeviceSession.NFCntDown++
	} else {
		fCnt = ctx.DeviceSession.AFCntDown
		ctx.DeviceSession.AFCntDown++
	}

	for i := range ctx.DownlinkFrames {
		// LoRaWAN MAC payload
		macPL := &lorawan.MACPayload{
			FHDR: lorawan.FHDR{
				DevAddr: ctx.DeviceSession.DevAddr,
				FCtrl: lorawan.FCtrl{
					ADR:      !disableADR,
					ACK:      ctx.ACK,
					FPending: ctx.MoreData,
				},
				FCnt: fCnt,
			},
		}

		// set application payload
		if ctx.FPort > 0 {
			macPL.FPort = &ctx.FPort
			macPL.FRMPayload = []lorawan.Payload{
				&lorawan.DataPayload{Bytes: ctx.Data},
			}
		}

		// add mac-commands
		var macCommandSize int
		var maccommands []lorawan.Payload

		for j := range ctx.MACCommands {
			s, err := ctx.MACCommands[j].Size()
			if err != nil {
				return errors.Wrap(err, "get mac-command block size")
			}

			if ctx.DownlinkFrames[i].RemainingPayloadSize-s < 0 {
				break
			}

			ctx.DownlinkFrames[i].RemainingPayloadSize = ctx.DownlinkFrames[i].RemainingPayloadSize - s
			macCommandSize += s

			for k := range ctx.MACCommands[j].MACCommands {
				maccommands = append(maccommands, &ctx.MACCommands[j].MACCommands[k])
			}
		}

		if macCommandSize > 15 && ctx.FPort == 0 {
			macPL.FPort = &ctx.FPort
			macPL.FRMPayload = maccommands
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

		// encrypt FRMPayload mac-commands
		if ctx.FPort == 0 {
			if err := phy.EncryptFRMPayload(ctx.DeviceSession.NwkSEncKey); err != nil {
				return errors.Wrap(err, "encrypt frmpayload error")
			}
		}

		// encrypt FOpts mac-commands (LoRaWAN 1.1)
		if ctx.DeviceSession.GetMACVersion() != lorawan.LoRaWAN1_0 {
			if err := phy.EncryptFOpts(ctx.DeviceSession.NwkSEncKey); err != nil {
				return errors.Wrap(err, "encrypt FOpts error")
			}
		}

		// set MIC
		if err := phy.SetDownlinkDataMIC(ctx.DeviceSession.GetMACVersion(), ctx.DeviceSession.FCntUp-1, ctx.DeviceSession.SNwkSIntKey); err != nil {
			return errors.Wrap(err, "set MIC error")
		}

		b, err := phy.MarshalBinary()
		if err != nil {
			return errors.Wrap(err, "marshal binary error")
		}

		ctx.DownlinkFrames[i].DownlinkFrame.PhyPayload = b
	}

	return nil
}

func sendDownlinkFrame(ctx *dataContext) error {
	if len(ctx.DownlinkFrames) == 0 {
		return nil
	}

	// send the packet to the gateway
	if err := gateway.Backend().SendTXPacket(ctx.DownlinkFrames[0].DownlinkFrame); err != nil {
		return errors.Wrap(err, "send downlink-frame to gateway error")
	}

	// set last downlink tx timestamp
	ctx.DeviceSession.LastDownlinkTX = time.Now()

	// log for gateway (with encrypted mac-commands)
	if err := framelog.LogDownlinkFrameForGateway(storage.RedisPool(), ctx.DownlinkFrames[0].DownlinkFrame); err != nil {
		log.WithError(err).Error("log downlink frame for gateway error")
	}

	// log for device (with decrypted mac-commands)
	if err := func() error {
		var phy lorawan.PHYPayload
		if err := phy.UnmarshalBinary(ctx.DownlinkFrames[0].DownlinkFrame.PhyPayload); err != nil {
			return err
		}

		// decrypt FRMPayload mac-commands
		if ctx.FPort == 0 {
			if err := phy.DecryptFRMPayload(ctx.DeviceSession.NwkSEncKey); err != nil {
				return errors.Wrap(err, "decrypt frmpayload error")
			}
		}

		// decrypt FOpts mac-commands (LoRaWAN 1.1)
		if ctx.DeviceSession.GetMACVersion() != lorawan.LoRaWAN1_0 {
			if err := phy.DecryptFOpts(ctx.DeviceSession.NwkSEncKey); err != nil {
				return errors.Wrap(err, "encrypt FOpts error")
			}
		}

		phyB, err := phy.MarshalBinary()
		if err != nil {
			return err
		}

		// log frame
		if err := framelog.LogDownlinkFrameForDevEUI(storage.RedisPool(), ctx.DeviceSession.DevEUI, gw.DownlinkFrame{
			Token:      uint32(ctx.DownlinkFrames[0].DownlinkFrame.Token),
			TxInfo:     ctx.DownlinkFrames[0].DownlinkFrame.TxInfo,
			PhyPayload: phyB,
		}); err != nil {
			return err
		}

		return nil
	}(); err != nil {
		log.WithError(err).Error("log downlink frame for device error")
	}

	return nil
}

func saveDeviceSession(ctx *dataContext) error {
	if err := storage.SaveDeviceSession(storage.RedisPool(), ctx.DeviceSession); err != nil {
		return errors.Wrap(err, "save device-session error")
	}
	return nil
}

func getDeviceProfile(ctx *dataContext) error {
	var err error
	ctx.DeviceProfile, err = storage.GetAndCacheDeviceProfile(storage.DB(), storage.RedisPool(), ctx.DeviceSession.DeviceProfileID)
	if err != nil {
		return errors.Wrap(err, "get device-profile error")
	}
	return nil
}

func getServiceProfile(ctx *dataContext) error {
	var err error
	ctx.ServiceProfile, err = storage.GetAndCacheServiceProfile(storage.DB(), storage.RedisPool(), ctx.DeviceSession.ServiceProfileID)
	if err != nil {
		return errors.Wrap(err, "get service-profile error")
	}
	return nil
}

func checkLastDownlinkTimestamp(ctx *dataContext) error {
	// in case of Class-C validate that between now and the last downlink
	// tx timestamp is at least the class-c lock duration
	if ctx.DeviceProfile.SupportsClassC && time.Now().Sub(ctx.DeviceSession.LastDownlinkTX) < classCDownlinkLockDuration {
		log.WithFields(log.Fields{
			"time":                           time.Now(),
			"last_downlink_tx_time":          ctx.DeviceSession.LastDownlinkTX,
			"class_c_downlink_lock_duration": classCDownlinkLockDuration,
		}).Debug("skip next downlink queue scheduling dueue to class-c downlink lock")
		return ErrAbort
	}

	return nil
}

func saveRemainingFrames(ctx *dataContext) error {
	if len(ctx.DownlinkFrames) < 2 {
		return nil
	}

	var downlinkFrames []gw.DownlinkFrame
	for i := range ctx.DownlinkFrames {
		if i == 0 || ctx.DownlinkFrames[i].RemainingPayloadSize < 0 {
			continue
		}
		downlinkFrames = append(downlinkFrames, ctx.DownlinkFrames[i].DownlinkFrame)
	}

	if err := storage.SaveDownlinkFrames(storage.RedisPool(), ctx.DeviceSession.DevEUI, downlinkFrames); err != nil {
		return errors.Wrap(err, "save downlink-frames error")
	}

	return nil
}

// This should only happen when the cached device-session is not in sync
// with the actual device-session.
func returnInvalidDeviceClassError(ctx *dataContext) error {
	return errors.New("the device is in an invalid device-class for this action")
}
