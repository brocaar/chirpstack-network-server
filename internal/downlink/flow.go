package downlink

import (
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
)

// Flow holds all the different downlink flows.
var Flow = newFlow().JoinResponse(
	getJoinAcceptTXInfo,
	logJoinAcceptFrame,
	sendJoinAcceptResponse,
).UplinkResponse(
	getDeviceProfile,
	requestDevStatus,
	getDataTXInfo,
	setRemainingPayloadSize,
	getNextDeviceQueueItem,
	getMACCommands,
	stopOnNothingToSend,
	sendDataDown,
	saveDeviceSession,
).ProprietaryDown(
	sendProprietaryDown,
).ScheduleNextDeviceQueueItem(
	getDeviceProfile,
	getServiceProfile,
	requestDevStatus,
	getDataTXInfoForRX2,
	setRemainingPayloadSize,
	getNextDeviceQueueItem,
	getMACCommands,
	stopOnNothingToSend,
	sendDataDown,
	saveDeviceSession,
)

// DataContext holds the context of a downlink transmission.
type DataContext struct {
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
}

// Validate validates the DataContext data.
func (ctx DataContext) Validate() error {
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

// JoinContext holds the context of a join response.
type JoinContext struct {
	DeviceSession storage.DeviceSession
	TXInfo        gw.TXInfo
	PHYPayload    lorawan.PHYPayload
}

// ProprietaryDownContext holds the context of a proprietary down context.
type ProprietaryDownContext struct {
	MACPayload  []byte
	MIC         lorawan.MIC
	GatewayMACs []lorawan.EUI64
	IPol        bool
	Frequency   int
	DR          int
}

// UplinkResponseTask is the signature of an uplink response task.
type UplinkResponseTask func(*DataContext) error

// JoinResponseTask is the signature of a join response task.
type JoinResponseTask func(*JoinContext) error

// ProprietaryDownTask is the signature of a proprietary down task.
type ProprietaryDownTask func(*ProprietaryDownContext) error

// ScheduleNextDeviceQueueItemTask is the signature of a schedule next
// device-queue item task.
type ScheduleNextDeviceQueueItemTask func(*DataContext) error

// Flow contains one or multiple tasks to execute.
type flow struct {
	uplinkResponseTasks              []UplinkResponseTask
	joinResponseTasks                []JoinResponseTask
	proprietaryDownTasks             []ProprietaryDownTask
	scheduleNextDeviceQueueItemTasks []ScheduleNextDeviceQueueItemTask
}

func newFlow() *flow {
	return &flow{}
}

// UplinkResponse adds uplink response tasks to the flow.
func (f *flow) UplinkResponse(tasks ...UplinkResponseTask) *flow {
	f.uplinkResponseTasks = tasks
	return f
}

// JoinResponse adds join response tasks to the flow.
func (f *flow) JoinResponse(tasks ...JoinResponseTask) *flow {
	f.joinResponseTasks = tasks
	return f
}

// ProprietaryDown adds proprietary down tasks to the flow.
func (f *flow) ProprietaryDown(tasks ...ProprietaryDownTask) *flow {
	f.proprietaryDownTasks = tasks
	return f
}

// ScheduleNextDeviceQueueItem adds tasks to the schedule next
// device-queue item flow.
func (f *flow) ScheduleNextDeviceQueueItem(tasks ...ScheduleNextDeviceQueueItemTask) *flow {
	f.scheduleNextDeviceQueueItemTasks = tasks
	return f
}

// RunUplinkResponse runs the uplink response flow.
func (f *flow) RunUplinkResponse(sp storage.ServiceProfile, ds storage.DeviceSession, adr, mustSend, ack bool) error {
	ctx := DataContext{
		ServiceProfile: sp,
		DeviceSession:  ds,
		ACK:            ack,
		MustSend:       mustSend,
	}

	for _, t := range f.uplinkResponseTasks {
		if err := t(&ctx); err != nil {
			if err == ErrAbort {
				return nil
			}

			return err
		}
	}

	return nil
}

// RunJoinResponse runs the join response flow.
func (f *flow) RunJoinResponse(ds storage.DeviceSession, phy lorawan.PHYPayload) error {
	ctx := JoinContext{
		DeviceSession: ds,
		PHYPayload:    phy,
	}

	for _, t := range f.joinResponseTasks {
		if err := t(&ctx); err != nil {
			if err == ErrAbort {
				return nil
			}

			return err
		}
	}

	return nil
}

// RunProprietaryDown runs the proprietary down flow.
func (f *flow) RunProprietaryDown(macPayload []byte, mic lorawan.MIC, gwMACs []lorawan.EUI64, iPol bool, frequency, dr int) error {
	ctx := ProprietaryDownContext{
		MACPayload:  macPayload,
		MIC:         mic,
		GatewayMACs: gwMACs,
		IPol:        iPol,
		Frequency:   frequency,
		DR:          dr,
	}

	for _, t := range f.proprietaryDownTasks {
		if err := t(&ctx); err != nil {
			if err == ErrAbort {
				return nil
			}

			return err
		}
	}

	return nil
}

// ScheduleNexDeviceQueueItem.
func (f *flow) RunScheduleNextDeviceQueueItem(ds storage.DeviceSession) error {
	ctx := DataContext{
		DeviceSession: ds,
	}

	for _, t := range f.scheduleNextDeviceQueueItemTasks {
		if err := t(&ctx); err != nil {
			if err == ErrAbort {
				return nil
			}
			return err
		}
	}

	return nil
}
