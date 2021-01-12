package data

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/gofrs/uuid"
	"github.com/golang/protobuf/ptypes"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-api/go/v3/gw"
	adrr "github.com/brocaar/chirpstack-network-server/adr"
	"github.com/brocaar/chirpstack-network-server/internal/adr"
	"github.com/brocaar/chirpstack-network-server/internal/backend/gateway"
	"github.com/brocaar/chirpstack-network-server/internal/band"
	"github.com/brocaar/chirpstack-network-server/internal/channels"
	"github.com/brocaar/chirpstack-network-server/internal/config"
	dwngateway "github.com/brocaar/chirpstack-network-server/internal/downlink/gateway"
	"github.com/brocaar/chirpstack-network-server/internal/helpers"
	"github.com/brocaar/chirpstack-network-server/internal/logging"
	"github.com/brocaar/chirpstack-network-server/internal/maccommand"
	"github.com/brocaar/chirpstack-network-server/internal/models"
	"github.com/brocaar/chirpstack-network-server/internal/roaming"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
	loraband "github.com/brocaar/lorawan/band"
	"github.com/brocaar/lorawan/sensitivity"
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
	rxWindow              int
	rx2PreferOnRX1DRLt    int
	rx2PreferOnLinkBudget bool

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

	// Dwell time.
	uplinkDwellTime400ms   bool
	downlinkDwellTime400ms bool
	uplinkMaxEIRPIndex     uint8

	// Max mac-command error count.
	maxMACCommandErrorCount int

	// Prefer gateways with min uplink SNR margin
	gatewayPreferMinMargin float64
)

var setMACCommandsSet = setMACCommands(
	requestCustomChannelReconfiguration,
	requestChannelMaskReconfiguration,
	requestADRChange,
	requestDevStatus,
	requestRejoinParamSetup,
	setPingSlotParameters,
	setRXParameters,
	setTXParameters,
	getMACCommandsFromQueue,
)

var responseTasks = []func(*dataContext) error{
	getDeviceProfile,
	getServiceProfile,
	setDeviceGatewayRXInfo,
	selectDownlinkGateway,
	setDataTXInfo,
	setToken,
	getNextDeviceQueueItem,
	setMACCommandsSet,
	stopOnNothingToSend,
	setPHYPayloads,
	isRoaming(false,
		sendDownlinkFrame,
	),
	isRoaming(true,
		sendDownlinkFramePassiveRoaming,
	),
	saveDeviceSession,
	saveDownlinkFrame,
}

var scheduleNextQueueItemTasks = []func(*dataContext) error{
	getDeviceProfile,
	getServiceProfile,
	checkLastDownlinkTimestamp,
	setDeviceGatewayRXInfo,
	selectDownlinkGateway,
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
	saveDownlinkFrame,
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

	rx2PreferOnRX1DRLt = nsConf.RX2PreferOnRX1DRLt
	rx2PreferOnLinkBudget = nsConf.RX2PreferOnLinkBudget

	downlinkTXPower = nsConf.DownlinkTXPower

	disableMACCommands = nsConf.DisableMACCommands
	disableADR = nsConf.DisableADR

	classCDownlinkLockDuration = conf.NetworkServer.Scheduler.ClassC.DownlinkLockDuration

	uplinkDwellTime400ms = conf.NetworkServer.Band.UplinkDwellTime400ms
	downlinkDwellTime400ms = conf.NetworkServer.Band.DownlinkDwellTime400ms

	maxEIRP := conf.NetworkServer.Band.UplinkMaxEIRP
	if maxEIRP == -1 {
		maxEIRP = band.Band().GetDefaultMaxUplinkEIRP()
	}
	uplinkMaxEIRPIndex = lorawan.GetTXParamSetupEIRPIndex(maxEIRP)

	maxMACCommandErrorCount = conf.NetworkServer.NetworkSettings.MaxMACCommandErrorCount
	gatewayPreferMinMargin = conf.NetworkServer.NetworkSettings.GatewayPreferMinMargin

	return nil
}

type dataContext struct {
	ctx context.Context

	// Device mode.
	DeviceMode storage.DeviceMode

	// ServiceProfile of the device.
	ServiceProfile storage.ServiceProfile

	// DeviceProfile of the device.
	DeviceProfile storage.DeviceProfile

	// DeviceSession holds the device-session of the device for which to send
	// the downlink data.
	DeviceSession storage.DeviceSession

	// DeviceGatewayRXInfo contains the RXInfo of one or multiple gateways
	// within reach of the device. These gateways can be used for transmitting
	// downlinks.
	DeviceGatewayRXInfo []storage.DeviceGatewayRXInfo

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

	// Downlink frame.
	DownlinkFrame gw.DownlinkFrame

	// Downlink frame items. This can be multiple items so that failing the
	// first one, there can be a retry with the next item.
	DownlinkFrameItems []downlinkFrameItem

	// Gateway to use for downlink.
	DownlinkGateway storage.DeviceGatewayRXInfo
}

type downlinkFrameItem struct {
	// Downlink frame item
	DownlinkFrameItem gw.DownlinkFrameItem

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

func isRoaming(r bool, tasks ...func(*dataContext) error) func(*dataContext) error {
	return func(ctx *dataContext) error {
		if r == (ctx.RXPacket.RoamingMetaData != nil) {
			for _, f := range tasks {
				if err := f(ctx); err != nil {
					return err
				}
			}
		}

		return nil
	}
}

// HandleResponse handles a downlink response.
func HandleResponse(ctx context.Context, rxPacket models.RXPacket, sp storage.ServiceProfile, ds storage.DeviceSession, adr, mustSend, ack bool, macCommands []storage.MACCommandBlock) error {
	rctx := dataContext{
		ctx:            ctx,
		ServiceProfile: sp,
		DeviceSession:  ds,
		ACK:            ack,
		MustSend:       mustSend,
		RXPacket:       &rxPacket,
		MACCommands:    macCommands,
	}

	for _, t := range responseTasks {
		if err := t(&rctx); err != nil {
			if err == ErrAbort {
				return nil
			}

			return err
		}
	}

	return nil
}

// HandleScheduleNextQueueItem handles scheduling the next device-queue item.
func HandleScheduleNextQueueItem(ctx context.Context, ds storage.DeviceSession, mode storage.DeviceMode) error {
	nqctx := dataContext{
		ctx:           ctx,
		DeviceMode:    mode,
		DeviceSession: ds,
	}

	for _, t := range scheduleNextQueueItemTasks {
		if err := t(&nqctx); err != nil {
			if err == ErrAbort {
				return nil
			}
			return err
		}
	}

	return nil
}

func setToken(ctx *dataContext) error {
	var downID uuid.UUID
	if ctxID := ctx.ctx.Value(logging.ContextIDKey); ctxID != nil {
		if id, ok := ctxID.(uuid.UUID); ok {
			downID = id
		}
	}

	// We use the downID bytes for the token so that with either the Token
	// as the DownlinkID value from the ACK, we can retrieve the pending items.
	ctx.DownlinkFrame.Token = uint32(binary.BigEndian.Uint16(downID[0:2]))
	ctx.DownlinkFrame.DownlinkId = downID[:]

	return nil
}

func requestDevStatus(ctx *dataContext) error {
	if ctx.ServiceProfile.DevStatusReqFreq == 0 {
		return nil
	}

	reqInterval := 24 * time.Hour / time.Duration(ctx.ServiceProfile.DevStatusReqFreq)
	curInterval := time.Now().Sub(ctx.DeviceSession.LastDevStatusRequested)

	if curInterval >= reqInterval {
		ctx.MACCommands = append(ctx.MACCommands, maccommand.RequestDevStatus(ctx.ctx, &ctx.DeviceSession))
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

func setTXParameters(ctx *dataContext) error {
	if !band.Band().ImplementsTXParamSetup(ctx.DeviceSession.MACVersion) {
		// band doesn't implement the TXParamSetup mac-command
		return nil
	}

	// Calculate the device max. EIRP.
	// We take the smallest value (chirpstack-network-server.toml vs device-profile) to avoid
	// that the device-profile sets a higher EIRP than that is allowed on the
	// network.
	deviceMaxEIRPIndex := uplinkMaxEIRPIndex
	if i := lorawan.GetTXParamSetupEIRPIndex(float32(ctx.DeviceProfile.MaxEIRP)); i < deviceMaxEIRPIndex {
		deviceMaxEIRPIndex = i
	}

	if ctx.DeviceSession.UplinkDwellTime400ms != uplinkDwellTime400ms ||
		ctx.DeviceSession.DownlinkDwellTime400ms != downlinkDwellTime400ms ||
		ctx.DeviceSession.UplinkMaxEIRPIndex != deviceMaxEIRPIndex {

		block := maccommand.RequestTXParamSetup(uplinkDwellTime400ms, downlinkDwellTime400ms, deviceMaxEIRPIndex)
		ctx.MACCommands = append(ctx.MACCommands, block)
	}

	return nil
}

func preferRX2DR(ctx *dataContext) (bool, error) {
	// The device has not yet been updated to the network-server RX2 parameters
	// (using mac-commands). Do not prefer RX2 over RX1 in this case.
	if ctx.DeviceSession.RX2Frequency != rx2Frequency || ctx.DeviceSession.RX2DR != uint8(rx2DR) ||
		ctx.DeviceSession.RX1DROffset != uint8(rx1DROffset) || ctx.DeviceSession.RXDelay != uint8(rx1Delay) {
		return false, nil
	}

	// get rx1 data-rate
	drRX1Index, err := band.Band().GetRX1DataRateIndex(ctx.DeviceSession.DR, int(ctx.DeviceSession.RX1DROffset))
	if err != nil {
		return false, errors.Wrap(err, "get rx1 data-rate index error")
	}

	if drRX1Index < rx2PreferOnRX1DRLt {
		return true, nil
	}

	return false, nil
}

func preferRX2LinkBudget(ctx *dataContext) (b bool, err error) {
	// The device has not yet been updated to the network-server RX2 parameters
	// (using mac-commands). Do not prefer RX2 over RX1 in this case.
	if ctx.DeviceSession.RX2Frequency != rx2Frequency || ctx.DeviceSession.RX2DR != uint8(rx2DR) ||
		ctx.DeviceSession.RX1DROffset != uint8(rx1DROffset) || ctx.DeviceSession.RXDelay != uint8(rx1Delay) {
		return false, nil
	}

	// get rx1 data-rate
	drRX1Index, err := band.Band().GetRX1DataRateIndex(ctx.DeviceSession.DR, int(ctx.DeviceSession.RX1DROffset))
	if err != nil {
		return false, errors.Wrap(err, "get rx1 data-rate index error")
	}

	// get rx1 data-rate
	drRX1, err := band.Band().GetDataRate(drRX1Index)
	if err != nil {
		return false, errors.Wrap(err, "get data-rate error")
	}

	// get rx2 data-rate
	drRX2, err := band.Band().GetDataRate(int(ctx.DeviceSession.RX2DR))
	if err != nil {
		return false, errors.Wrap(err, "get data-rate error")
	}

	// the calculation below only applies for LORA modulation
	if drRX1.Modulation != loraband.LoRaModulation || drRX2.Modulation != loraband.LoRaModulation {
		return false, nil
	}

	// get RX1 and RX2 freq
	rx1Freq, err := band.Band().GetRX1FrequencyForUplinkFrequency(int(ctx.RXPacket.TXInfo.GetFrequency()))
	if err != nil {
		return false, errors.Wrap(err, "get rx1 frequency for uplink frequency error")
	}
	rx2Freq := rx2Frequency

	// get RX1 and RX2 TX Power
	var txPowerRX1, txPowerRX2 int
	if downlinkTXPower != -1 {
		txPowerRX1 = downlinkTXPower
		txPowerRX2 = downlinkTXPower
	} else {
		txPowerRX1 = band.Band().GetDownlinkTXPower(rx1Freq)
		txPowerRX2 = band.Band().GetDownlinkTXPower(rx2Freq)
	}

	linkBudgetRX1 := sensitivity.CalculateLinkBudget(drRX1.Bandwidth*1000, 6, float32(config.SpreadFactorToRequiredSNRTable[drRX1.SpreadFactor]), float32(txPowerRX1))
	linkBudgetRX2 := sensitivity.CalculateLinkBudget(drRX2.Bandwidth*1000, 6, float32(config.SpreadFactorToRequiredSNRTable[drRX2.SpreadFactor]), float32(txPowerRX2))

	return linkBudgetRX2 > linkBudgetRX1, nil
}

func selectDownlinkGateway(ctx *dataContext) error {
	var err error
	ctx.DownlinkGateway, err = dwngateway.SelectDownlinkGateway(gatewayPreferMinMargin, ctx.DeviceSession.DR, ctx.DeviceGatewayRXInfo)
	if err != nil {
		return err
	}

	ctx.DownlinkFrame.GatewayId = ctx.DownlinkGateway.GatewayID[:]

	return nil
}

func setDataTXInfo(ctx *dataContext) error {
	preferRX2overRX1, err := preferRX2DR(ctx)
	if err != nil {
		return err
	}

	if rx2PreferOnLinkBudget {
		prefer, err := preferRX2LinkBudget(ctx)
		if err != nil {
			return err
		}
		preferRX2overRX1 = prefer || preferRX2overRX1
	}

	// RX2 is prefered and the RX window is set to automatic.
	if preferRX2overRX1 && rxWindow == 0 {
		// RX2
		if err := setTXInfoForRX2(ctx); err != nil {
			return err
		}

		// RX1
		if err := setTXInfoForRX1(ctx); err != nil {
			return err
		}
	} else {
		// RX1
		if rxWindow == 0 || rxWindow == 1 {
			if err := setTXInfoForRX1(ctx); err != nil {
				return err
			}
		}

		// RX2
		if rxWindow == 0 || rxWindow == 2 {
			if err := setTXInfoForRX2(ctx); err != nil {
				return err
			}
		}
	}

	return nil
}

func setTXInfoForRX1(ctx *dataContext) error {
	txInfo := gw.DownlinkTXInfo{
		Board:   ctx.DownlinkGateway.Board,
		Antenna: ctx.DownlinkGateway.Antenna,
		Context: ctx.DownlinkGateway.Context,
	}

	rx1DR, err := band.Band().GetRX1DataRateIndex(ctx.DeviceSession.DR, int(ctx.DeviceSession.RX1DROffset))
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
	delay := band.Band().GetDefaults().ReceiveDelay1
	if ctx.DeviceSession.RXDelay > 0 {
		delay = time.Duration(ctx.DeviceSession.RXDelay) * time.Second
	}
	txInfo.Timing = gw.DownlinkTiming_DELAY
	txInfo.TimingInfo = &gw.DownlinkTXInfo_DelayTimingInfo{
		DelayTimingInfo: &gw.DelayTimingInfo{
			Delay: ptypes.DurationProto(delay),
		},
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

	ctx.DownlinkFrameItems = append(ctx.DownlinkFrameItems, downlinkFrameItem{
		DownlinkFrameItem: gw.DownlinkFrameItem{
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
	txInfo := gw.DownlinkTXInfo{
		Board:     ctx.DownlinkGateway.Board,
		Antenna:   ctx.DownlinkGateway.Antenna,
		Frequency: uint32(ctx.DeviceSession.RX2Frequency),
		Context:   ctx.DownlinkGateway.Context,
	}

	// get data-rate
	err := helpers.SetDownlinkTXInfoDataRate(&txInfo, int(ctx.DeviceSession.RX2DR), band.Band())
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
		delay := band.Band().GetDefaults().ReceiveDelay2
		if ctx.DeviceSession.RXDelay > 0 {
			delay = (time.Duration(ctx.DeviceSession.RXDelay) * time.Second) + time.Second
		}
		txInfo.Timing = gw.DownlinkTiming_DELAY
		txInfo.TimingInfo = &gw.DownlinkTXInfo_DelayTimingInfo{
			DelayTimingInfo: &gw.DelayTimingInfo{
				Delay: ptypes.DurationProto(delay),
			},
		}
	}

	if ctx.Immediately {
		txInfo.Timing = gw.DownlinkTiming_IMMEDIATELY
		txInfo.TimingInfo = &gw.DownlinkTXInfo_ImmediatelyTimingInfo{
			ImmediatelyTimingInfo: &gw.ImmediatelyTimingInfo{},
		}
	}

	// get remaining payload size
	plSize, err := band.Band().GetMaxPayloadSizeForDataRateIndex(ctx.DeviceProfile.MACVersion, ctx.DeviceProfile.RegParamsRevision, int(ctx.DeviceSession.RX2DR))
	if err != nil {
		return errors.Wrap(err, "get max-payload size error")
	}

	ctx.DownlinkFrameItems = append(ctx.DownlinkFrameItems, downlinkFrameItem{
		DownlinkFrameItem: gw.DownlinkFrameItem{
			TxInfo: &txInfo,
		},
		RemainingPayloadSize: plSize.N,
	})

	return nil
}

func setTXInfoForClassB(ctx *dataContext) error {
	txInfo := gw.DownlinkTXInfo{
		Board:     ctx.DownlinkGateway.Board,
		Antenna:   ctx.DownlinkGateway.Antenna,
		Frequency: uint32(ctx.DeviceSession.PingSlotFrequency),
		Context:   ctx.DownlinkGateway.Context,
	}

	// get data-rate
	err := helpers.SetDownlinkTXInfoDataRate(&txInfo, ctx.DeviceSession.PingSlotDR, band.Band())
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

	ctx.DownlinkFrameItems = append(ctx.DownlinkFrameItems, downlinkFrameItem{
		DownlinkFrameItem: gw.DownlinkFrameItem{
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
	if len(ctx.DownlinkFrameItems) > 0 {
		remainingPayloadSize = ctx.DownlinkFrameItems[0].RemainingPayloadSize
	}

	qi, err := storage.GetNextDeviceQueueItemForDevEUIMaxPayloadSizeAndFCnt(ctx.ctx, storage.DB(), ctx.DeviceSession.DevEUI, remainingPayloadSize, fCnt, ctx.DeviceSession.RoutingProfileID)
	if err != nil {
		if errors.Cause(err) == storage.ErrDoesNotExist {
			return nil
		}
		return errors.Wrap(err, "get next device-queue item for max payload error")
	}

	ctx.Confirmed = qi.Confirmed
	ctx.Data = qi.FRMPayload
	ctx.FPort = qi.FPort

	for i := range ctx.DownlinkFrameItems {
		ctx.DownlinkFrameItems[i].RemainingPayloadSize = ctx.DownlinkFrameItems[i].RemainingPayloadSize - len(ctx.Data)
	}

	items, err := storage.GetDeviceQueueItemsForDevEUI(ctx.ctx, storage.DB(), ctx.DeviceSession.DevEUI)
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
	if ctx.RXPacket == nil && qi.EmitAtTimeSinceGPSEpoch != nil && len(ctx.DownlinkFrameItems) == 1 {
		ctx.DownlinkFrameItems[0].DownlinkFrameItem.TxInfo.Timing = gw.DownlinkTiming_GPS_EPOCH
		ctx.DownlinkFrameItems[0].DownlinkFrameItem.TxInfo.TimingInfo = &gw.DownlinkTXInfo_GpsEpochTimingInfo{
			GpsEpochTimingInfo: &gw.GPSEpochTimingInfo{
				TimeSinceGpsEpoch: ptypes.DurationProto(*qi.EmitAtTimeSinceGPSEpoch),
			},
		}

		if ctx.DeviceSession.PingSlotFrequency == 0 {
			beaconTime := *qi.EmitAtTimeSinceGPSEpoch - (*qi.EmitAtTimeSinceGPSEpoch % (128 * time.Second))
			freq, err := band.Band().GetPingSlotFrequency(ctx.DeviceSession.DevAddr, beaconTime)
			if err != nil {
				return errors.Wrap(err, "get ping-slot frequency error")
			}
			ctx.DownlinkFrameItems[0].DownlinkFrameItem.TxInfo.Frequency = uint32(freq)
		}
	}

	if !qi.Confirmed {
		// delete when not confirmed
		if err := storage.DeleteDeviceQueueItem(ctx.ctx, storage.DB(), qi.ID); err != nil {
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

		if err := storage.UpdateDeviceQueueItem(ctx.ctx, storage.DB(), &qi); err != nil {
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

		// In case mac-commands are disabled in the ChirpStack Network Server configuration,
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

		// Filter out mac-commands that exceed the max. error count.
		var filteredMACCommands []storage.MACCommandBlock
		for i := range ctx.MACCommands {
			if ctx.DeviceSession.MACCommandErrorCount[ctx.MACCommands[i].CID] <= maxMACCommandErrorCount {
				filteredMACCommands = append(filteredMACCommands, ctx.MACCommands[i])
			}
		}
		ctx.MACCommands = filteredMACCommands

		ctx.MACCommands = filterIncompatibleMACCommands(ctx.MACCommands)

		var remainingPayloadSize, remainingMACCommandSize int
		if len(ctx.DownlinkFrameItems) > 0 {
			remainingPayloadSize = ctx.DownlinkFrameItems[0].RemainingPayloadSize
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
			if err := storage.SetPendingMACCommand(ctx.ctx, ctx.DeviceSession.DevEUI, block); err != nil {
				return errors.Wrap(err, "set mac-command pending error")
			}

			// delete from queue, if external
			if block.External {
				if err := storage.DeleteMACCommandQueueItem(ctx.ctx, ctx.DeviceSession.DevEUI, block); err != nil {
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
			"ctx_id":  ctx.ctx.Value(logging.ContextIDKey),
		}).Warningf("handle channel reconfigure error: %s", err)
	} else {
		ctx.MACCommands = append(ctx.MACCommands, blocks...)
	}

	return nil
}

func requestADRChange(ctx *dataContext) error {
	conf := config.Get()

	var maxTxPowerIndex int
	var requiredSNRforDR float32
	var uplinkHistory []adrr.UplinkMetaData

	// maxTxPowerIndex
	for i := 0; ; i++ {
		offset, err := band.Band().GetTXPowerOffset(i)
		if err != nil {
			break
		}
		if offset != 0 {
			maxTxPowerIndex = i
		}
	}

	// requiredSNRforDR
	dr, err := band.Band().GetDataRate(ctx.DeviceSession.DR)
	if err != nil {
		return errors.Wrap(err, "get data-rate error")
	}
	requiredSNRforDR = float32(config.SpreadFactorToRequiredSNRTable[dr.SpreadFactor])

	// uplink history
	for _, uh := range ctx.DeviceSession.UplinkHistory {
		uplinkHistory = append(uplinkHistory, adrr.UplinkMetaData{
			FCnt:         uh.FCnt,
			MaxSNR:       float32(uh.MaxSNR),
			TXPowerIndex: uh.TXPowerIndex,
			GatewayCount: uh.GatewayCount,
		})
	}

	handleReq := adrr.HandleRequest{
		DevEUI:             ctx.DeviceSession.DevEUI,
		ADR:                ctx.DeviceSession.ADR,
		DR:                 ctx.DeviceSession.DR,
		TxPowerIndex:       ctx.DeviceSession.TXPowerIndex,
		NbTrans:            int(ctx.DeviceSession.NbTrans),
		MaxTxPowerIndex:    maxTxPowerIndex,
		RequiredSNRForDR:   requiredSNRforDR,
		InstallationMargin: float32(conf.NetworkServer.NetworkSettings.InstallationMargin),
		MinDR:              ctx.ServiceProfile.DRMin,
		MaxDR:              ctx.ServiceProfile.DRMax,
		UplinkHistory:      uplinkHistory,
	}

	handler := adr.GetHandler(ctx.DeviceProfile.ADRAlgorithmID)
	handleResp, err := handler.Handle(handleReq)
	if err != nil {
		return errors.Wrap(err, "handle adr error")
	}

	// The response values are different than the request values, thus we must
	// send a LinkADRReq to the device.
	if handleResp.DR != handleReq.DR || handleResp.TxPowerIndex != handleReq.TxPowerIndex || handleResp.NbTrans != handleReq.NbTrans {
		var linkADRReq *storage.MACCommandBlock
		for i := range ctx.MACCommands {
			if ctx.MACCommands[i].CID == lorawan.LinkADRReq {
				linkADRReq = &ctx.MACCommands[i]
			}
		}

		if linkADRReq != nil {
			// There is already a pending LinkADRReq. Note that the LinkADRReq is also
			// used for setting the channel-mask. Set ADR parameters to the last
			// mac-command in the block.

			lastMAC := linkADRReq.MACCommands[len(linkADRReq.MACCommands)-1]
			lastMACPl, ok := lastMAC.Payload.(*lorawan.LinkADRReqPayload)
			if !ok {
				return fmt.Errorf("expected *lorawan.LinkADRReqPayload, got: %T", lastMAC.Payload)
			}

			lastMACPl.DataRate = uint8(handleResp.DR)
			lastMACPl.TXPower = uint8(handleResp.TxPowerIndex)
			lastMACPl.Redundancy.NbRep = uint8(handleResp.NbTrans)
		} else {
			var chMask lorawan.ChMask
			chMaskCntl := -1
			for _, c := range ctx.DeviceSession.EnabledUplinkChannels {
				if chMaskCntl != c/16 {
					if chMaskCntl == -1 {
						// set the chMaskCntl
						chMaskCntl = c / 16
					} else {
						// break the loop as we only need to send one block of channels
						break
					}
				}
				chMask[c%16] = true
			}

			ctx.MACCommands = append(ctx.MACCommands, storage.MACCommandBlock{
				CID: lorawan.LinkADRReq,
				MACCommands: []lorawan.MACCommand{
					{
						CID: lorawan.LinkADRReq,
						Payload: &lorawan.LinkADRReqPayload{
							DataRate: uint8(handleResp.DR),
							TXPower:  uint8(handleResp.TxPowerIndex),
							ChMask:   chMask,
							Redundancy: lorawan.Redundancy{
								ChMaskCntl: uint8(chMaskCntl),
								NbRep:      uint8(handleResp.NbTrans),
							},
						},
					},
				},
			})
		}
	}

	return nil
}

func getMACCommandsFromQueue(ctx *dataContext) error {
	blocks, err := storage.GetMACCommandQueueItems(ctx.ctx, ctx.DeviceSession.DevEUI)
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

	for i := range ctx.DownlinkFrameItems {
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

			if ctx.DownlinkFrameItems[i].RemainingPayloadSize-s < 0 {
				break
			}

			ctx.DownlinkFrameItems[i].RemainingPayloadSize = ctx.DownlinkFrameItems[i].RemainingPayloadSize - s
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
				"ctx_id":  ctx.ctx.Value(logging.ContextIDKey),
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

		ctx.DownlinkFrameItems[i].DownlinkFrameItem.PhyPayload = b
		ctx.DownlinkFrame.Items = append(ctx.DownlinkFrame.Items, &ctx.DownlinkFrameItems[i].DownlinkFrameItem)
	}

	return nil
}

func sendDownlinkFrame(ctx *dataContext) error {
	if len(ctx.DownlinkFrameItems) == 0 {
		return nil
	}

	// send the packet to the gateway
	if err := gateway.Backend().SendTXPacket(ctx.DownlinkFrame); err != nil {
		return errors.Wrap(err, "send downlink-frame to gateway error")
	}

	// set last downlink tx timestamp
	ctx.DeviceSession.LastDownlinkTX = time.Now()

	return nil
}

func sendDownlinkFramePassiveRoaming(ctx *dataContext) error {
	var netID lorawan.NetID
	if err := netID.UnmarshalText([]byte(ctx.RXPacket.RoamingMetaData.BasePayload.SenderID)); err != nil {
		return errors.Wrap(err, "decode senderid error")
	}

	client, err := roaming.GetClientForNetID(netID)
	if err != nil {
		return errors.Wrap(err, "get roaming client error")
	}

	classMode := "A"

	req := backend.XmitDataReqPayload{
		PHYPayload: backend.HEXBytes(ctx.DownlinkFrame.Items[0].PhyPayload),
		DLMetaData: &backend.DLMetaData{
			ClassMode:  &classMode,
			DevEUI:     &ctx.DeviceSession.DevEUI,
			FNSULToken: ctx.RXPacket.RoamingMetaData.ULMetaData.FNSULToken,
		},
	}

	// DLFreq1
	dlFreq1, err := band.Band().GetRX1FrequencyForUplinkFrequency(int(ctx.RXPacket.TXInfo.Frequency))
	if err != nil {
		return errors.Wrap(err, "get rx1 frequency error")
	}
	dlFreq1Mhz := float64(dlFreq1) / 1000000
	req.DLMetaData.DLFreq1 = &dlFreq1Mhz

	// DLFreq2
	dlFreq2Mhz := float64(ctx.DeviceSession.RX2Frequency) / 1000000
	req.DLMetaData.DLFreq2 = &dlFreq2Mhz

	// DataRate1
	rx1DR, err := band.Band().GetRX1DataRateIndex(ctx.DeviceSession.DR, int(ctx.DeviceSession.RX1DROffset))
	if err != nil {
		return errors.Wrap(err, "get rx1 data-rate index error")
	}
	req.DLMetaData.DataRate1 = &rx1DR

	// DataRate2
	rx2DR := int(ctx.DeviceSession.RX2DR)
	req.DLMetaData.DataRate2 = &rx2DR

	// RXDelay1
	rxDelay1 := int(ctx.DeviceSession.RXDelay)
	req.DLMetaData.RXDelay1 = &rxDelay1

	for i := range ctx.RXPacket.RoamingMetaData.ULMetaData.GWInfo {
		gwInfo := ctx.RXPacket.RoamingMetaData.ULMetaData.GWInfo[i]
		if !gwInfo.DLAllowed {
			continue
		}

		req.DLMetaData.GWInfo = append(req.DLMetaData.GWInfo, backend.GWInfoElement{
			ULToken: gwInfo.ULToken,
		})
	}

	go func() {
		logFields := log.Fields{
			"ctx_id":  ctx.ctx.Value(logging.ContextIDKey),
			"net_id":  netID,
			"dev_eui": ctx.DeviceSession.DevEUI,
		}
		resp, err := client.XmitDataReq(ctx.ctx, req)
		if err != nil {
			log.WithFields(logFields).WithError(err).Error("downlink/data: XmitDataReq failed")
			return
		}
		if resp.Result.ResultCode != backend.Success {
			log.WithFields(logFields).Errorf("expected: %s, got: %s (%s)", backend.Success, resp.Result.ResultCode, resp.Result.Description)
			return
		}

		log.WithFields(logFields).Info("downlink/data: forwarded downlink using passive-roaming")
	}()

	return nil
}

func saveDeviceSession(ctx *dataContext) error {
	if err := storage.SaveDeviceSession(ctx.ctx, ctx.DeviceSession); err != nil {
		return errors.Wrap(err, "save device-session error")
	}
	return nil
}

func getDeviceProfile(ctx *dataContext) error {
	var err error
	ctx.DeviceProfile, err = storage.GetAndCacheDeviceProfile(ctx.ctx, storage.DB(), ctx.DeviceSession.DeviceProfileID)
	if err != nil {
		return errors.Wrap(err, "get device-profile error")
	}
	return nil
}

func getServiceProfile(ctx *dataContext) error {
	var err error
	ctx.ServiceProfile, err = storage.GetAndCacheServiceProfile(ctx.ctx, storage.DB(), ctx.DeviceSession.ServiceProfileID)
	if err != nil {
		return errors.Wrap(err, "get service-profile error")
	}
	return nil
}

func setDeviceGatewayRXInfo(ctx *dataContext) error {
	if ctx.RXPacket != nil {
		// Class-A response.
		for i := range ctx.RXPacket.RXInfoSet {
			// In case of roaming, check if this gateway can be used for sending
			// downlinks.
			if ctx.RXPacket.RoamingMetaData != nil {
				var downlink bool
				for _, gwInfo := range ctx.RXPacket.RoamingMetaData.ULMetaData.GWInfo {
					if bytes.Equal(gwInfo.ID[:], ctx.RXPacket.RXInfoSet[i].GatewayId) && gwInfo.DLAllowed {
						downlink = true
					}
				}

				if !downlink {
					continue
				}
			}

			ctx.DeviceGatewayRXInfo = append(ctx.DeviceGatewayRXInfo, storage.DeviceGatewayRXInfo{
				GatewayID: helpers.GetGatewayID(ctx.RXPacket.RXInfoSet[i]),
				RSSI:      int(ctx.RXPacket.RXInfoSet[i].Rssi),
				LoRaSNR:   ctx.RXPacket.RXInfoSet[i].LoraSnr,
				Board:     ctx.RXPacket.RXInfoSet[i].Board,
				Antenna:   ctx.RXPacket.RXInfoSet[i].Antenna,
				Context:   ctx.RXPacket.RXInfoSet[i].Context,
			})
		}

	} else {
		// Class-B or Class-C.
		rxInfo, err := storage.GetDeviceGatewayRXInfoSet(ctx.ctx, ctx.DeviceSession.DevEUI)
		if err != nil {
			return errors.Wrap(err, "get device gateway RXInfoSet error")
		}

		ctx.DeviceGatewayRXInfo = rxInfo.Items
	}

	if len(ctx.DeviceGatewayRXInfo) == 0 {
		return errors.New("DeviceGatewayRXInfo, the device needs to send an uplink first")
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
			"ctx_id":                         ctx.ctx.Value(logging.ContextIDKey),
		}).Debug("skip next downlink queue scheduling dueue to class-c downlink lock")
		return ErrAbort
	}

	return nil
}

func saveDownlinkFrame(ctx *dataContext) error {
	var fCnt uint32
	if ctx.DeviceSession.GetMACVersion() == lorawan.LoRaWAN1_0 || ctx.FPort == 0 {
		fCnt = ctx.DeviceSession.NFCntDown
		ctx.DeviceSession.NFCntDown++
	} else {
		fCnt = ctx.DeviceSession.AFCntDown
		ctx.DeviceSession.AFCntDown++
	}

	df := storage.DownlinkFrame{
		DevEui:           ctx.DeviceSession.DevEUI[:],
		Token:            ctx.DownlinkFrame.Token,
		RoutingProfileId: ctx.DeviceSession.RoutingProfileID.Bytes(),
		FCnt:             fCnt,
		EncryptedFopts:   ctx.DeviceSession.GetMACVersion() != lorawan.LoRaWAN1_0,
		NwkSEncKey:       ctx.DeviceSession.NwkSEncKey[:],
		DownlinkFrame:    &ctx.DownlinkFrame,
	}

	if err := storage.SaveDownlinkFrame(ctx.ctx, df); err != nil {
		return errors.Wrap(err, "save downlink-frame error")
	}

	return nil
}

// This should only happen when the cached device-session is not in sync
// with the actual device-session.
func returnInvalidDeviceClassError(ctx *dataContext) error {
	return errors.New("the device is in an invalid device-class for this action")
}
