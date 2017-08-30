package downlink

import "github.com/brocaar/loraserver/internal/session"
import "github.com/brocaar/loraserver/api/gw"
import "github.com/brocaar/lorawan"

// Flow holds all the different downlink flows.
var Flow = newFlow().JoinResponse(
	getJoinAcceptTXInfo,
	logJoinAcceptFrame,
	sendJoinAcceptResponse,
).UplinkResponse(
	getDataTXInfo,
	setRemainingPayloadSize,
	getDataDownFromApplicationServer,
	getMACCommands,
	stopOnNothingToSend,
	sendDataDown,
	saveNodeSession,
).PushDataDown(
	getDataTXInfoForRX2,
	setRemainingPayloadSize,
	getMACCommands,
	sendDataDown,
	saveNodeSession,
)

// DataContext holds the context of a downlink transmission.
type DataContext struct {
	// NodeSession holds the node-session of the node for which to send
	// the downlink data.
	NodeSession session.NodeSession

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
	NodeSession session.NodeSession
	TXInfo      gw.TXInfo
	PHYPayload  lorawan.PHYPayload
}

// UplinkResponseTask is the signature of an uplink response task.
type UplinkResponseTask func(*DataContext) error

// PushDataDownTask is the signature of a downlink push task.
type PushDataDownTask func(*DataContext) error

// JoinResponseTask is the signature of a join response task.
type JoinResponseTask func(*JoinContext) error

// Flow contains one or multiple tasks to execute.
type flow struct {
	uplinkResponseTasks []UplinkResponseTask
	pushDataDownTasks   []PushDataDownTask
	joinResponseTasks   []JoinResponseTask
}

func newFlow() *flow {
	return &flow{}
}

// UplinkResponse adds uplink response tasks to the flow.
func (f *flow) UplinkResponse(tasks ...UplinkResponseTask) *flow {
	f.uplinkResponseTasks = tasks
	return f
}

// PushDataDown adds push data-down tasks to the flow.
func (f *flow) PushDataDown(tasks ...PushDataDownTask) *flow {
	f.pushDataDownTasks = tasks
	return f
}

// JoinResponse adds join response tasks to the flow.
func (f *flow) JoinResponse(tasks ...JoinResponseTask) *flow {
	f.joinResponseTasks = tasks
	return f
}

// RunUplinkResponse runs the uplink response flow.
func (f *flow) RunUplinkResponse(ns session.NodeSession, adr, mustSend, ack bool) error {
	ctx := DataContext{
		NodeSession: ns,
		ACK:         ack,
		MustSend:    mustSend,
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

// RunPushDataDown runs the push data-down flow.
func (f *flow) RunPushDataDown(ns session.NodeSession, confirmed bool, fPort uint8, data []byte) error {
	ctx := DataContext{
		NodeSession: ns,
		Confirmed:   confirmed,
		FPort:       fPort,
		Data:        data,
	}

	for _, t := range f.pushDataDownTasks {
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
func (f *flow) RunJoinResponse(ns session.NodeSession, phy lorawan.PHYPayload) error {
	ctx := JoinContext{
		NodeSession: ns,
		PHYPayload:  phy,
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
