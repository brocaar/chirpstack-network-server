package data

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/gofrs/uuid"
	"github.com/golang/protobuf/ptypes"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-api/go/v3/as"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	adrr "github.com/brocaar/chirpstack-network-server/v3/adr"
	"github.com/brocaar/chirpstack-network-server/v3/internal/adr"
	"github.com/brocaar/chirpstack-network-server/v3/internal/backend/applicationserver"
	"github.com/brocaar/chirpstack-network-server/v3/internal/backend/gateway"
	"github.com/brocaar/chirpstack-network-server/v3/internal/band"
	"github.com/brocaar/chirpstack-network-server/v3/internal/channels"
	"github.com/brocaar/chirpstack-network-server/v3/internal/config"
	"github.com/brocaar/chirpstack-network-server/v3/internal/downlink/ack"
	dwngateway "github.com/brocaar/chirpstack-network-server/v3/internal/downlink/gateway"
	"github.com/brocaar/chirpstack-network-server/v3/internal/gps"
	"github.com/brocaar/chirpstack-network-server/v3/internal/helpers"
	"github.com/brocaar/chirpstack-network-server/v3/internal/logging"
	"github.com/brocaar/chirpstack-network-server/v3/internal/maccommand"
	"github.com/brocaar/chirpstack-network-server/v3/internal/models"
	"github.com/brocaar/chirpstack-network-server/v3/internal/roaming"
	"github.com/brocaar/chirpstack-network-server/v3/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
	loraband "github.com/brocaar/lorawan/band"
	"github.com/brocaar/lorawan/sensitivity"
)

const (
	defaultCodeRate        = "4/5"
	deviceDownlinkLockKey  = "lora:ns:device:%s:down:lock"
	gatewayDownlinkLockKey = "lora:ns:gw:%s:down:lock"
)

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
	classBPingSlotFrequency uint32

	// RX window
	rxWindow              int
	rx2PreferOnRX1DRLt    int
	rx2PreferOnLinkBudget bool

	// RX2 params
	rx2Frequency uint32
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
	classCDeviceDownlinkLockDuration  time.Duration
	classCGatewayDownlinkLockDuration time.Duration

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
	saveDownlinkFrame,
	saveDeviceSession,
	isRoaming(false,
		sendDownlinkFrame,
	),
	isRoaming(true,
		sendDownlinkFramePassiveRoaming,
	),
	isRoaming(true,
		handleRoamingTxAck,
	),
}

var scheduleNextQueueItemTasks = []func(*dataContext) error{
	getDeviceProfile,
	getServiceProfile,
	forClass(storage.DeviceModeC,
		getDownlinkDeviceLock,
	),
	setDeviceGatewayRXInfo,
	selectDownlinkGateway,
	forClass(storage.DeviceModeC,
		getDownlinkGatewayLock,
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
	stopOnNothingToSend,
	setPHYPayloads,
	saveDeviceSession,
	saveDownlinkFrame,
	sendDownlinkFrame,
	setDeviceQueueItemRetryAfter,
}

// Setup configures the package.
func Setup(conf config.Config) error {
	nsConf := conf.NetworkServer.NetworkSettings
	rejoinRequestEnabled = nsConf.RejoinRequest.Enabled
	rejoinRequestMaxCountN = nsConf.RejoinRequest.MaxCountN
	rejoinRequestMaxTimeN = nsConf.RejoinRequest.MaxTimeN

	classBPingSlotDR = nsConf.ClassB.PingSlotDR
	classBPingSlotFrequency = nsConf.ClassB.PingSlotFrequency

	rx2Frequency = uint32(nsConf.RX2Frequency)
	rx2DR = nsConf.RX2DR
	rx1DROffset = nsConf.RX1DROffset
	rx1Delay = nsConf.RX1Delay
	rxWindow = nsConf.RXWindow

	rx2PreferOnRX1DRLt = nsConf.RX2PreferOnRX1DRLt
	rx2PreferOnLinkBudget = nsConf.RX2PreferOnLinkBudget

	downlinkTXPower = nsConf.DownlinkTXPower

	disableMACCommands = nsConf.DisableMACCommands
	disableADR = nsConf.DisableADR

	classCDeviceDownlinkLockDuration = conf.NetworkServer.Scheduler.ClassC.DeviceDownlinkLockDuration
	classCGatewayDownlinkLockDuration = conf.NetworkServer.Scheduler.ClassC.GatewayDownlinkLockDuration

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

	// Database connection or transaction.
	DB              sqlx.Ext
	DBInTransaction bool

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

	// RXPacket holds the received uplink packet (in case of Class-A downlink).
	RXPacket *models.RXPacket

	// Immediately indicates that the response must be sent immediately.
	Immediately bool

	// MACCommands contains the mac-commands to send (if any). Make sure the
	// total size fits within the FRMPayload or FOpts.
	MACCommands []storage.MACCommandBlock

	// DeviceQueueItem contains the possible device-queue item to send to the
	// device.
	DeviceQueueItem *storage.DeviceQueueItem

	// MoreDeviceQueueItems defines if there are more items in the queue besides
	// DeviceQueueItem.
	MoreDeviceQueueItems bool

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
		ctx:             ctx,
		DB:              storage.DB(),
		DBInTransaction: false,
		ServiceProfile:  sp,
		DeviceSession:   ds,
		ACK:             ack,
		MustSend:        mustSend,
		RXPacket:        &rxPacket,
		MACCommands:     macCommands,
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
func HandleScheduleNextQueueItem(ctx context.Context, db sqlx.Ext, ds storage.DeviceSession, mode storage.DeviceMode) error {
	nqctx := dataContext{
		ctx:             ctx,
		DB:              db,
		DBInTransaction: true, // the scheduler loop is in transaction as it needs to block the device rows
		DeviceMode:      mode,
		DeviceSession:   ds,
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
	rx1Freq, err := band.Band().GetRX1FrequencyForUplinkFrequency(ctx.RXPacket.TXInfo.GetFrequency())
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
	freq, err := band.Band().GetRX1FrequencyForUplinkFrequency(ctx.RXPacket.TXInfo.Frequency)
	if err != nil {
		return errors.Wrap(err, "get rx1 frequency error")
	}
	txInfo.Frequency = freq

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
		txInfo.Power = int32(band.Band().GetDownlinkTXPower(txInfo.Frequency))
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
		txInfo.Power = int32(band.Band().GetDownlinkTXPower(txInfo.Frequency))
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
		txInfo.Power = int32(band.Band().GetDownlinkTXPower(txInfo.Frequency))
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
	if len(ctx.DownlinkFrameItems) == 0 {
		return errors.New("DownlinkFrameItems is 0")
	}

	// In case of the scheduler loop there is already a transaction and we don't
	// want to start a new transaction as it could create a deadlock.
	var transactionWrapper func(func(sqlx.Ext) error) error
	if ctx.DBInTransaction {
		transactionWrapper = func(f func(sqlx.Ext) error) error {
			return f(ctx.DB)
		}
	} else {
		transactionWrapper = storage.Transaction
	}

	// The current time as time since GPS epoch.
	timeSinceGPSEpochNow := gps.Time(time.Now()).TimeSinceGPSEpoch()

	// We use the first downlink opportunity to determine the max-payload size
	// for the downlink.
	maxPayloadSize := ctx.DownlinkFrameItems[0].RemainingPayloadSize

	// Set the expected downlink frame-counter
	var fCnt uint32
	if ctx.DeviceSession.GetMACVersion() == lorawan.LoRaWAN1_0 {
		fCnt = ctx.DeviceSession.NFCntDown
	} else {
		fCnt = ctx.DeviceSession.AFCntDown
	}

	// It might require a couple of iterations to get the device-queue item.
	for {
		qi, more, err := storage.GetNextDeviceQueueItemForDevEUI(ctx.ctx, ctx.DB, ctx.DeviceSession.DevEUI)
		if err != nil {
			if errors.Cause(err) == storage.ErrDoesNotExist {
				// no downlink in queue
				return nil
			} else {
				return errors.Wrap(err, "get next device-queue item error")
			}
		}

		// * The payload FCnt must match the expected FCnt
		// * The downlink should not have timed out.
		// * The payload size must not exceed the max. payload size
		// * The payload emit_at_time_since_gps_epoch must be in the future
		if qi.FCnt == fCnt && (qi.TimeoutAfter == nil || !qi.TimeoutAfter.Before(time.Now())) && len(qi.FRMPayload) <= maxPayloadSize && (qi.EmitAtTimeSinceGPSEpoch == nil || *qi.EmitAtTimeSinceGPSEpoch > timeSinceGPSEpochNow) {
			ctx.DeviceQueueItem = &qi
			ctx.MoreDeviceQueueItems = more

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

			return nil
		}

		////
		// If this point is reached, the downlink queue-item can not be used
		// because of one of the reasons below.
		////

		rp, err := storage.GetRoutingProfile(ctx.ctx, ctx.DB, ctx.DeviceSession.RoutingProfileID)
		if err != nil {
			return errors.Wrap(err, "get routing-profile error")
		}
		asClient, err := applicationserver.Pool().Get(rp.ASID, []byte(rp.CACert), []byte(rp.TLSCert), []byte(rp.TLSKey))
		if err != nil {
			return errors.Wrap(err, "get application-server client error")
		}

		// Handle timeout.
		// We drop the item from the queue and notify the AS.
		if qi.TimeoutAfter != nil && qi.TimeoutAfter.Before(time.Now()) {
			if err := storage.DeleteDeviceQueueItem(ctx.ctx, ctx.DB, qi.ID); err != nil {
				return errors.Wrap(err, "delete device-queue item error")
			}

			_, err = asClient.HandleDownlinkACK(ctx.ctx, &as.HandleDownlinkACKRequest{
				DevEui:       ctx.DeviceSession.DevEUI[:],
				FCnt:         qi.FCnt,
				Acknowledged: false,
			})
			if err != nil {
				return errors.Wrap(err, "application-server client error")
			}

			log.WithFields(log.Fields{
				"dev_eui":                ctx.DeviceSession.DevEUI,
				"device_queue_item_fcnt": qi.FCnt,
			}).Warning("downlink/data: device-queue item discarded because of timeout")

			// Re-run the loop again, to fetch the next queue item.
			continue
		}

		// Handle payload size.
		// In this case, we will drop the device-queue item and report an error
		// to the AS.
		if len(qi.FRMPayload) > maxPayloadSize {
			if err := storage.DeleteDeviceQueueItem(ctx.ctx, ctx.DB, qi.ID); err != nil {
				return errors.Wrap(err, "delete device-queue item error")
			}

			log.WithFields(log.Fields{
				"fcnt":              qi.FCnt,
				"dev_eui":           ctx.DeviceSession.DevEUI,
				"max_payload_size":  maxPayloadSize,
				"item_payload_size": len(qi.FRMPayload),
				"ctx_id":            ctx.ctx.Value(logging.ContextIDKey),
			}).Warning("downlink/data: device-queue item discarded as it exceeds the max payload size")

			_, err := asClient.HandleError(ctx.ctx, &as.HandleErrorRequest{
				DevEui: ctx.DeviceSession.DevEUI[:],
				Type:   as.ErrorType_DEVICE_QUEUE_ITEM_SIZE,
				FCnt:   qi.FCnt,
				Error:  "payload exceeds max payload size",
			})
			if err != nil {
				return errors.Wrap(err, "application-server client error")
			}

			// Re-run the loop again, to fetch the next queue item.
			continue
		}

		// Handle frame-counter sync error.
		// In this case, the queue-item does not have the expected frame-counter.
		// We will ask the AS to re-encrypt the device-queue to re-sync with the
		// expected frame-counters.
		// Note: we can't do this ourself, as the FCnt is used in the encryption
		// scheme, and the encryption is within the domain of the AS.
		if qi.FCnt != fCnt {
			err := transactionWrapper(func(tx sqlx.Ext) error {
				// Read the queue.
				items, err := storage.GetDeviceQueueItemsForDevEUI(ctx.ctx, tx, ctx.DeviceSession.DevEUI)
				if err != nil {
					return errors.Wrap(err, "get device-queue items error")
				}

				// Create the re-encrypt request.
				req := as.ReEncryptDeviceQueueItemsRequest{
					DevEui:    ctx.DeviceSession.DevEUI[:],
					DevAddr:   ctx.DeviceSession.DevAddr[:],
					FCntStart: fCnt,
				}
				for i := range items {
					req.Items = append(req.Items, &as.ReEncryptDeviceQueueItem{
						FrmPayload: items[i].FRMPayload,
						FCnt:       items[i].FCnt,
						FPort:      uint32(items[i].FPort),
						Confirmed:  items[i].Confirmed,
					})

				}

				// Request the AS to re-encrypt the queue items.
				resp, err := asClient.ReEncryptDeviceQueueItems(ctx.ctx, &req)
				if err != nil {
					return errors.Wrap(err, "application-server client error")
				}

				// Flush the device-queue.
				err = storage.FlushDeviceQueueForDevEUI(ctx.ctx, tx, ctx.DeviceSession.DevEUI)
				if err != nil {
					return errors.Wrap(err, "flush device-queue error")
				}

				// Enqueue re-encrypted payloads
				for i := range resp.Items {
					qi := storage.DeviceQueueItem{
						DevAddr:    ctx.DeviceSession.DevAddr,
						DevEUI:     ctx.DeviceSession.DevEUI,
						FRMPayload: resp.Items[i].FrmPayload,
						FCnt:       resp.Items[i].FCnt,
						FPort:      uint8(resp.Items[i].FPort),
						Confirmed:  resp.Items[i].Confirmed,
					}

					if err := storage.CreateDeviceQueueItem(ctx.ctx, tx, &qi, ctx.DeviceProfile, ctx.DeviceSession); err != nil {
						return errors.Wrap(err, "create device-queue item error")
					}
				}

				return nil
			})
			if err != nil {
				return err
			}

			// Re-run the loop again, to fetch the next queue item.
			continue
		}

		// Handle emit at GPS epoch in the past.
		// In this case we are trying to send a Class-B downlink at a GPS epoch time
		// which has already occured. In this case we need to re-enqueue all downlink
		// items in order to shift the downlink timing.
		if qi.EmitAtTimeSinceGPSEpoch != nil && *qi.EmitAtTimeSinceGPSEpoch <= timeSinceGPSEpochNow {
			err := transactionWrapper(func(tx sqlx.Ext) error {
				// Read the queue.
				items, err := storage.GetDeviceQueueItemsForDevEUI(ctx.ctx, tx, ctx.DeviceSession.DevEUI)
				if err != nil {
					return errors.Wrap(err, "get device-queue items error")
				}

				// Flush the device-queue.
				err = storage.FlushDeviceQueueForDevEUI(ctx.ctx, tx, ctx.DeviceSession.DevEUI)
				if err != nil {
					return errors.Wrap(err, "flush device-queue error")
				}

				for _, qi := range items {
					qi.ID = 0
					qi.IsPending = false
					qi.EmitAtTimeSinceGPSEpoch = nil
					qi.TimeoutAfter = nil
					qi.RetryAfter = nil

					// The CreateDeviceQueueItem will take care of adding the Class-B scheduling
					// timing.
					if err := storage.CreateDeviceQueueItem(ctx.ctx, tx, &qi, ctx.DeviceProfile, ctx.DeviceSession); err != nil {
						return errors.Wrap(err, "create device-queue item error")
					}
				}

				return nil
			})
			if err != nil {
				return err
			}

			// Re-run the loop again.
			continue
		}
	}
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
	if conf.NetworkServer.NetworkSettings.DisableADR {
		return nil
	}

	var maxTxPowerIndex int
	var requiredSNRforDR float32
	var uplinkHistory []adrr.UplinkMetaData

	// maxTxPowerIndex
	if ctx.DeviceSession.MaxSupportedTXPowerIndex != 0 {
		maxTxPowerIndex = ctx.DeviceSession.MaxSupportedTXPowerIndex
	} else {
		for i := 0; ; i++ {
			offset, err := band.Band().GetTXPowerOffset(i)
			if err != nil {
				break
			}
			if offset != 0 {
				maxTxPowerIndex = i
			}
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
		Region:             band.Band().Name(),
		DevEUI:             ctx.DeviceSession.DevEUI,
		MACVersion:         ctx.DeviceSession.MACVersion,
		RegParamsRevision:  ctx.DeviceProfile.RegParamsRevision,
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
	// No device-queue item to send, no mac-commands to send, no ACK to send
	// in reply to a confirmed-uplink and no requirement to send an empty downlink
	// (e.g. in case of ADRACKReq).
	if ctx.DeviceQueueItem == nil && len(ctx.MACCommands) == 0 && !ctx.ACK && !ctx.MustSend {
		if ctx.DeviceMode == storage.DeviceModeC {
			// If we made it this far for a class C device we most certainly own
			// the device and gateway locks so make sure to release them before returning.
			key := storage.GetRedisKey(deviceDownlinkLockKey, ctx.DeviceSession.DevEUI)
			_ = storage.RedisClient().Del(ctx.ctx, key).Err()

			if classCGatewayDownlinkLockDuration != 0 {
				var id lorawan.EUI64
				copy(id[:], ctx.DownlinkFrame.GatewayId)
				key := storage.GetRedisKey(gatewayDownlinkLockKey, id)
				_ = storage.RedisClient().Del(ctx.ctx, key).Err()
			}
		}

		// ErrAbort will not be handled as a real error
		return ErrAbort
	}
	return nil
}

// setPHYPayloads sets the PHYPayload for each downlink opportunity. Please note
// that mac-commands have priority over application payloads as per LoRaWAN 1.0.4
// specs. In case a downlink frame-counter will be used for mac-commands only and
// a device-queue item exists for the same frame-counter, then the getNextDeviceQueueItem
// function will request the AS to re-encrypt the device-queue item(s) using
// the new frame-counters.
// Note that it is possible that the payload is different in case there are
// multiple receive windows, as each receive window has its own max payload
// constraint. E.g. the RX1 payload might contain mac-commands + app PL and
// the RX2 receive window might only contain mac-commands.
// Therefore the frame-counter will be incremented when the gateway has acknowledged
// which downlink item will be transmitted (only one will be transmitted),
// as only then we know which of the frame-counters to increment
// (AFCntDown vs NFCntDown).
func setPHYPayloads(ctx *dataContext) error {
	for i := range ctx.DownlinkFrameItems {
		var macCommandSize int
		var macCommands []lorawan.Payload

		// collect all mac-commands up to RemainingPayloadSize bytes.
		for j := range ctx.MACCommands {
			// get size of mac-command block
			s, err := ctx.MACCommands[j].Size()
			if err != nil {
				return errors.Wrap(err, "get mac-command block size error")
			}

			// break if it does not fit within the RemainingPayloadSize
			if ctx.DownlinkFrameItems[i].RemainingPayloadSize-s < 0 {
				break
			}

			ctx.DownlinkFrameItems[i].RemainingPayloadSize = ctx.DownlinkFrameItems[i].RemainingPayloadSize - s
			macCommandSize += s

			for k := range ctx.MACCommands[j].MACCommands {
				macCommands = append(macCommands, &ctx.MACCommands[j].MACCommands[k])
			}

		}

		// LoRaWAN MHDR
		mhdr := lorawan.MHDR{
			MType: lorawan.UnconfirmedDataDown,
			Major: lorawan.LoRaWANR1,
		}

		// LoRaWAN MAC payload
		macPL := lorawan.MACPayload{
			FHDR: lorawan.FHDR{
				DevAddr: ctx.DeviceSession.DevAddr,
				FCnt:    ctx.DeviceSession.NFCntDown,
				FCtrl: lorawan.FCtrl{
					ADR:      !disableADR,
					ACK:      ctx.ACK,
					FPending: ctx.MoreDeviceQueueItems,
				},
			},
		}

		// In this case mac-commands are sent as FRMPayload. We will not be able to
		// send a device-queue item in this case.
		if macCommandSize > 15 {
			// Set the FPending to true if we were planning to send a downlink
			// device-queue item.
			macPL.FHDR.FCtrl.FPending = (ctx.DeviceQueueItem != nil)

			// Set the mac-commands as FRMPayload.
			macPL.FRMPayload = macCommands

			// MAC-layer FPort.
			fPort := uint8(0)
			macPL.FPort = &fPort

			// Network-server FCnt.
			macPL.FHDR.FCnt = ctx.DeviceSession.NFCntDown
		}

		// In this case mac-commands are sent using the FOpts field. In case there
		// is a device-queue item, we will validate if it still fits within the
		// RemainingPayloadSize.
		if macCommandSize <= 15 {
			// Set the mac-commands as FOpts.
			macPL.FHDR.FOpts = macCommands

			// Test if we still can send a device-queue item.
			if ctx.DeviceQueueItem != nil && len(ctx.DeviceQueueItem.FRMPayload) <= ctx.DownlinkFrameItems[i].RemainingPayloadSize {
				// Set the device-queue item.
				macPL.FPort = &ctx.DeviceQueueItem.FPort
				macPL.FHDR.FCnt = ctx.DeviceQueueItem.FCnt
				macPL.FRMPayload = []lorawan.Payload{
					&lorawan.DataPayload{Bytes: ctx.DeviceQueueItem.FRMPayload},
				}

				if ctx.DeviceQueueItem.Confirmed {
					mhdr.MType = lorawan.ConfirmedDataDown
				}

				ctx.DownlinkFrameItems[i].RemainingPayloadSize = ctx.DownlinkFrameItems[i].RemainingPayloadSize - len(ctx.DeviceQueueItem.FRMPayload)
			} else if ctx.DeviceQueueItem != nil {
				macPL.FHDR.FCtrl.FPending = true
			}
		}

		// Construct LoRaWAN PHYPayload.
		phy := lorawan.PHYPayload{
			MHDR:       mhdr,
			MACPayload: &macPL,
		}

		// Encrypt FRMPayload mac-commands.
		if macPL.FPort != nil && *macPL.FPort == 0 {
			if err := phy.EncryptFRMPayload(ctx.DeviceSession.NwkSEncKey); err != nil {
				return errors.Wrap(err, "encrypt frmpayload error")
			}
		}

		// Encrypt FOpts mac-commands (LoRaWAN 1.1).
		if ctx.DeviceSession.GetMACVersion() != lorawan.LoRaWAN1_0 {
			if err := phy.EncryptFOpts(ctx.DeviceSession.NwkSEncKey); err != nil {
				return errors.Wrap(err, "encrypt FOpts error")
			}
		}

		// Set MIC.
		// If this is an ACK, then FCntUp has already been incremented by one. If
		// this is not an ACK, then DownlinkDataMIC will zero out ConfFCnt.
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
	dlFreq1, err := band.Band().GetRX1FrequencyForUplinkFrequency(ctx.RXPacket.TXInfo.Frequency)
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

	logFields := log.Fields{
		"ctx_id":  ctx.ctx.Value(logging.ContextIDKey),
		"net_id":  netID,
		"dev_eui": ctx.DeviceSession.DevEUI,
	}
	resp, err := client.XmitDataReq(ctx.ctx, req)
	if err != nil {
		log.WithFields(logFields).WithError(err).Error("downlink/data: XmitDataReq failed")
		return ErrAbort
	}
	if resp.Result.ResultCode != backend.Success {
		log.WithFields(logFields).Errorf("expected: %s, got: %s (%s)", backend.Success, resp.Result.ResultCode, resp.Result.Description)
		return ErrAbort
	}

	log.WithFields(logFields).Info("downlink/data: forwarded downlink using passive-roaming")

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
	ctx.DeviceProfile, err = storage.GetAndCacheDeviceProfile(ctx.ctx, ctx.DB, ctx.DeviceSession.DeviceProfileID)
	if err != nil {
		return errors.Wrap(err, "get device-profile error")
	}
	return nil
}

func getServiceProfile(ctx *dataContext) error {
	var err error
	ctx.ServiceProfile, err = storage.GetAndCacheServiceProfile(ctx.ctx, ctx.DB, ctx.DeviceSession.ServiceProfileID)
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

// getDownlinkDeviceLock acquires a downlink device lock. This is used for Class-C
// scheduling where the queue items are sent immediately / as soon as possible.
// This avoids race-conditions when running multiple NS instances.
func getDownlinkDeviceLock(ctx *dataContext) error {
	key := storage.GetRedisKey(deviceDownlinkLockKey, ctx.DeviceSession.DevEUI)
	set, err := storage.RedisClient().SetNX(ctx.ctx, key, "lock", classCDeviceDownlinkLockDuration).Result()
	if err != nil {
		return errors.Wrap(err, "acquire downlink device lock error")
	}

	if !set {
		// the device is already locked
		return ErrAbort
	}

	return nil
}

// getDownlinkGatewayLock acquires a downlink gateway lock. This can be useful
// to make sure that half-duplex gateways don't end up sending downlinks continuously
// and missing uplink responses to transmitted Class-C downlinks.
func getDownlinkGatewayLock(ctx *dataContext) error {
	// nothing to do when it is not configured
	if classCGatewayDownlinkLockDuration == 0 {
		return nil
	}

	var id lorawan.EUI64
	copy(id[:], ctx.DownlinkFrame.GatewayId)
	key := storage.GetRedisKey(gatewayDownlinkLockKey, id)
	set, err := storage.RedisClient().SetNX(ctx.ctx, key, "lock", classCGatewayDownlinkLockDuration).Result()
	if err != nil {
		return errors.Wrap(err, "acquire downlink gateway lock error")
	}

	if !set {
		// the gateway is already locked, remove the device lock (as nothing was sent
		// for this device) and abort.
		key := storage.GetRedisKey(deviceDownlinkLockKey, ctx.DeviceSession.DevEUI)
		_ = storage.RedisClient().Del(ctx.ctx, key).Err()

		return ErrAbort
	}

	return nil
}

func saveDownlinkFrame(ctx *dataContext) error {
	df := storage.DownlinkFrame{
		DevEui:           ctx.DeviceSession.DevEUI[:],
		Token:            ctx.DownlinkFrame.Token,
		RoutingProfileId: ctx.DeviceSession.RoutingProfileID.Bytes(),
		EncryptedFopts:   ctx.DeviceSession.GetMACVersion() != lorawan.LoRaWAN1_0,
		NwkSEncKey:       ctx.DeviceSession.NwkSEncKey[:],
		DownlinkFrame:    &ctx.DownlinkFrame,
	}

	if ctx.DeviceQueueItem != nil {
		df.DeviceQueueItemId = ctx.DeviceQueueItem.ID
	}

	if err := storage.SaveDownlinkFrame(ctx.ctx, &df); err != nil {
		return errors.Wrap(err, "save downlink-frame error")
	}

	return nil
}

func handleRoamingTxAck(ctx *dataContext) error {
	if err := ack.HandleRoamingTxAck(ctx.ctx, gw.DownlinkTXAck{
		Token:      ctx.DownlinkFrame.Token,
		DownlinkId: ctx.DownlinkFrame.DownlinkId,
	}); err != nil {
		return errors.Wrap(err, "Handle roaming tx ack")
	}

	return nil
}

// setDeviceQueueItemRetryAfter sets the retry_after field of the device-queue
// item. Note that there is no need to call this for Class-A devices, as the
// downlink is triggered only by an uplink event. The purpose of the retry_after
// is to avoid that multiple Class-B and C scheduler loops schedule the
// downlink while waiting for the gateway tx acknowledgement.
func setDeviceQueueItemRetryAfter(ctx *dataContext) error {
	if ctx.DeviceQueueItem == nil {
		return nil
	}

	conf := config.Get()
	retryAfter := time.Now().Add(conf.NetworkServer.Gateway.DownlinkTimeout)

	ctx.DeviceQueueItem.RetryAfter = &retryAfter

	if err := storage.UpdateDeviceQueueItem(ctx.ctx, ctx.DB, ctx.DeviceQueueItem); err != nil {
		return errors.Wrap(err, "update device-queue item error")
	}

	return nil
}

// This should only happen when the cached device-session is not in sync
// with the actual device-session.
func returnInvalidDeviceClassError(ctx *dataContext) error {
	return errors.New("the device is in an invalid device-class for this action")
}
