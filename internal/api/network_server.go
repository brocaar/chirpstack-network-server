package api

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/brocaar/loraserver/api/ns"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/downlink"
	"github.com/brocaar/loraserver/internal/maccommand"
	"github.com/brocaar/loraserver/internal/session"
	"github.com/brocaar/lorawan"
)

// defaultCodeRate defines the default code rate
const defaultCodeRate = "4/5"

type NetworkServerAPI struct {
	ctx common.Context
}

// NewNetworkServerAPI returns a new NetworkServerAPI.
func NewNetworkServerAPI(ctx common.Context) *NetworkServerAPI {
	return &NetworkServerAPI{
		ctx: ctx,
	}
}

// CreateNodeSession create a node-session.
func (n *NetworkServerAPI) CreateNodeSession(ctx context.Context, req *ns.CreateNodeSessionRequest) (*ns.CreateNodeSessionResponse, error) {
	sess := session.NodeSession{
		FCntUp:             req.FCntUp,
		FCntDown:           req.FCntDown,
		RXDelay:            uint8(req.RxDelay),
		RX1DROffset:        uint8(req.Rx1DROffset),
		RXWindow:           session.RXWindow(req.RxWindow),
		RX2DR:              uint8(req.Rx2DR),
		RelaxFCnt:          req.RelaxFCnt,
		ADRInterval:        req.AdrInterval,
		InstallationMargin: req.InstallationMargin,
	}

	if len(req.CFList) > 0 {
		var cFList lorawan.CFList
		if len(req.CFList) > len(cFList) {
			return nil, grpc.Errorf(codes.InvalidArgument, "max length for CFList is %d", len(cFList))
		}

		for i, f := range req.CFList {
			cFList[i] = f
		}

		sess.CFList = &cFList
	}

	copy(sess.DevAddr[:], req.DevAddr)
	copy(sess.AppEUI[:], req.AppEUI)
	copy(sess.DevEUI[:], req.DevEUI)
	copy(sess.NwkSKey[:], req.NwkSKey)

	exists, err := session.NodeSessionExists(n.ctx.RedisPool, sess.DevEUI)
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, err.Error())
	}
	if exists {
		return nil, grpc.Errorf(codes.AlreadyExists, err.Error())
	}

	if err := session.SaveNodeSession(n.ctx.RedisPool, sess); err != nil {
		return nil, grpc.Errorf(codes.Unknown, err.Error())
	}

	return &ns.CreateNodeSessionResponse{}, nil
}

// GetNodeSession returns a node-session.
func (n *NetworkServerAPI) GetNodeSession(ctx context.Context, req *ns.GetNodeSessionRequest) (*ns.GetNodeSessionResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEUI)

	sess, err := session.GetNodeSessionByDevEUI(n.ctx.RedisPool, devEUI)
	if err != nil {
		return nil, grpc.Errorf(codes.Unknown, err.Error())
	}

	resp := &ns.GetNodeSessionResponse{
		DevAddr:            sess.DevAddr[:],
		AppEUI:             sess.AppEUI[:],
		DevEUI:             sess.DevEUI[:],
		NwkSKey:            sess.NwkSKey[:],
		FCntUp:             sess.FCntUp,
		FCntDown:           sess.FCntDown,
		RxDelay:            uint32(sess.RXDelay),
		Rx1DROffset:        uint32(sess.RX1DROffset),
		RxWindow:           ns.RXWindow(sess.RXWindow),
		Rx2DR:              uint32(sess.RX2DR),
		RelaxFCnt:          sess.RelaxFCnt,
		AdrInterval:        sess.ADRInterval,
		InstallationMargin: sess.InstallationMargin,
		NbTrans:            uint32(sess.NbTrans),
		TxPower:            uint32(sess.TXPower),
	}

	if sess.CFList != nil {
		resp.CFList = sess.CFList[:]
	}

	return resp, nil
}

// UpdateNodeSession updates a node-session.
func (n *NetworkServerAPI) UpdateNodeSession(ctx context.Context, req *ns.UpdateNodeSessionRequest) (*ns.UpdateNodeSessionResponse, error) {
	var devAddr lorawan.DevAddr
	copy(devAddr[:], req.DevAddr)

	var devEUI, appEUI lorawan.EUI64
	copy(devEUI[:], req.DevEUI)
	copy(appEUI[:], req.AppEUI)

	sess, err := session.GetNodeSession(n.ctx.RedisPool, devAddr)
	if err != nil {
		return nil, grpc.Errorf(codes.Unknown, err.Error())
	}

	if sess.DevEUI != devEUI {
		return nil, grpc.Errorf(codes.InvalidArgument, "node-session belongs to a different DevEUI")
	}
	if sess.AppEUI != appEUI {
		return nil, grpc.Errorf(codes.InvalidArgument, "node-session belongs to a different AppEUI")
	}

	newSess := session.NodeSession{
		FCntUp:             req.FCntUp,
		FCntDown:           req.FCntDown,
		RXDelay:            uint8(req.RxDelay),
		RX1DROffset:        uint8(req.Rx1DROffset),
		RXWindow:           session.RXWindow(req.RxWindow),
		RX2DR:              uint8(req.Rx2DR),
		RelaxFCnt:          req.RelaxFCnt,
		ADRInterval:        req.AdrInterval,
		InstallationMargin: req.InstallationMargin,

		// these values can't be overwritten
		NbTrans:       sess.NbTrans,
		TXPower:       sess.TXPower,
		UplinkHistory: sess.UplinkHistory,
	}

	if len(req.CFList) > 0 {
		var cFList lorawan.CFList
		if len(req.CFList) > len(cFList) {
			return nil, grpc.Errorf(codes.InvalidArgument, "max length for CFList is %d", len(cFList))
		}

		for i, f := range req.CFList {
			cFList[i] = f
		}

		newSess.CFList = &cFList
	}

	copy(newSess.DevAddr[:], req.DevAddr)
	copy(newSess.AppEUI[:], req.AppEUI)
	copy(newSess.DevEUI[:], req.DevEUI)
	copy(newSess.NwkSKey[:], req.NwkSKey)

	if err := session.SaveNodeSession(n.ctx.RedisPool, newSess); err != nil {
		return nil, grpc.Errorf(codes.Unknown, err.Error())
	}

	return &ns.UpdateNodeSessionResponse{}, nil
}

// DeleteNodeSession deletes a node-session.
func (n *NetworkServerAPI) DeleteNodeSession(ctx context.Context, req *ns.DeleteNodeSessionRequest) (*ns.DeleteNodeSessionResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEUI)

	sess, err := session.GetNodeSessionByDevEUI(n.ctx.RedisPool, devEUI)
	if err != nil {
		return nil, grpc.Errorf(codes.Unknown, err.Error())
	}

	if err := session.DeleteNodeSession(n.ctx.RedisPool, sess.DevAddr); err != nil {
		return nil, grpc.Errorf(codes.Unknown, err.Error())
	}

	return &ns.DeleteNodeSessionResponse{}, nil
}

// GetRandomDevAddr returns a random DevAddr.
func (n *NetworkServerAPI) GetRandomDevAddr(ctx context.Context, req *ns.GetRandomDevAddrRequest) (*ns.GetRandomDevAddrResponse, error) {
	devAddr, err := session.GetRandomDevAddr(n.ctx.RedisPool, n.ctx.NetID)
	if err != nil {
		return nil, grpc.Errorf(codes.Unknown, err.Error())
	}

	return &ns.GetRandomDevAddrResponse{
		DevAddr: devAddr[:],
	}, nil
}

// EnqueueDataDownMACCommand adds a data down MAC command to the queue.
func (n *NetworkServerAPI) EnqueueDataDownMACCommand(ctx context.Context, req *ns.EnqueueDataDownMACCommandRequest) (*ns.EnqueueDataDownMACCommandResponse, error) {
	macPL := maccommand.QueueItem{
		FRMPayload: req.FrmPayload,
		Data:       req.Data,
	}
	copy(macPL.DevEUI[:], req.DevEUI)
	if err := maccommand.AddToQueue(n.ctx.RedisPool, macPL); err != nil {
		return nil, grpc.Errorf(codes.Unknown, err.Error())
	}
	return &ns.EnqueueDataDownMACCommandResponse{}, nil
}

// PushDataDown pushes the given downlink payload to the node (only works for Class-C nodes).
func (n *NetworkServerAPI) PushDataDown(ctx context.Context, req *ns.PushDataDownRequest) (*ns.PushDataDownResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEUI)

	sess, err := session.GetNodeSessionByDevEUI(n.ctx.RedisPool, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	if req.FCnt != sess.FCntDown {
		return nil, grpc.Errorf(codes.InvalidArgument, "invalid FCnt (expected: %d)", sess.FCntDown)
	}

	err = downlink.HandlePushDataDown(n.ctx, sess, req.Confirmed, uint8(req.FPort), req.Data)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.PushDataDownResponse{}, nil
}
