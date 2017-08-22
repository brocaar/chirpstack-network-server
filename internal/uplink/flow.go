package uplink

import (
	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/session"
	"github.com/brocaar/lorawan"
)

// JoinRequestContext holds the context of a join-request.
type JoinRequestContext struct {
	RXPacket            models.RXPacket
	JoinRequestPayload  *lorawan.JoinRequestPayload
	DevAddr             lorawan.DevAddr
	CFList              []uint32
	JoinRequestResponse *as.JoinRequestResponse
	NodeSession         session.NodeSession
}

// DataUpContext holds the context of an uplink data.
type DataUpContext struct {
	RXPacket models.RXPacket
}

// JoinRequestTask is the signature of a join-request task.
type JoinRequestTask func(*JoinRequestContext) error

// DataUpTask is the signature of an uplink data task.
type DataUpTask func(*DataUpContext) error

// Flow contains one or multiple tasks to execute.
type Flow struct {
	joinRequestTasks []JoinRequestTask
	dataUpTasks      []DataUpTask
}

// NewFlow creates a new Flow.
func NewFlow() *Flow {
	return &Flow{}
}

// Run runs the flow for the given frame collection.
func (f *Flow) Run(rxPacket models.RXPacket) error {
	switch rxPacket.PHYPayload.MHDR.MType {
	case lorawan.JoinRequest:
		return f.runJoinRequestTasks(rxPacket)
	case lorawan.UnconfirmedDataUp, lorawan.ConfirmedDataUp:
		return f.runDataUpTasks(rxPacket)
	default:
		return nil
	}
}

// JoinRequest add JoinRequestTasks to the flow.
func (f *Flow) JoinRequest(tasks ...JoinRequestTask) *Flow {
	f.joinRequestTasks = tasks
	return f
}

// DataUp adds DataUpTasks to the flow.
func (f *Flow) DataUp(tasks ...DataUpTask) *Flow {
	f.dataUpTasks = tasks
	return f
}

func (f *Flow) runJoinRequestTasks(rxPacket models.RXPacket) error {
	ctx := JoinRequestContext{
		RXPacket: rxPacket,
	}

	for _, t := range f.joinRequestTasks {
		if err := t(&ctx); err != nil {
			return err
		}
	}

	return nil
}

func (f *Flow) runDataUpTasks(rxPacket models.RXPacket) error {
	ctx := DataUpContext{
		RXPacket: rxPacket,
	}

	for _, t := range f.dataUpTasks {
		if err := t(&ctx); err != nil {
			return err
		}
	}

	return nil
}
