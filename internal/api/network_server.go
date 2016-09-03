package api

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/brocaar/loraserver/api/ns"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/queue"
	"github.com/brocaar/loraserver/internal/session"
	"github.com/brocaar/lorawan"
)

type NetworkServerAPI struct {
	ctx common.Context
}

// CreateNodeSession create a node-session.
func (n *NetworkServerAPI) CreateNodeSession(ctx context.Context, req *ns.CreateNodeSessionRequest) (*ns.CreateNodeSessionResponse, error) {
	sess := session.NodeSession{
		FCntUp:      req.FCntUp,
		FCntDown:    req.FCntDown,
		RXDelay:     uint8(req.RxDelay),
		RX1DROffset: uint8(req.Rx1DROffset),
		RXWindow:    session.RXWindow(req.RxWindow),
		RX2DR:       uint8(req.Rx2DR),
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

	if err := session.CreateNodeSession(n.ctx.RedisPool, sess); err != nil {
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
		DevAddr:     sess.DevAddr[:],
		AppEUI:      sess.AppEUI[:],
		DevEUI:      sess.DevEUI[:],
		NwkSKey:     sess.NwkSKey[:],
		FCntUp:      sess.FCntUp,
		FCntDown:    sess.FCntDown,
		RxDelay:     uint32(sess.RXDelay),
		Rx1DROffset: uint32(sess.RX1DROffset),
		RxWindow:    ns.RXWindow(sess.RXWindow),
		Rx2DR:       uint32(sess.RX2DR),
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
		FCntUp:      req.FCntUp,
		FCntDown:    req.FCntDown,
		RXDelay:     uint8(req.RxDelay),
		RX1DROffset: uint8(req.Rx1DROffset),
		RXWindow:    session.RXWindow(req.RxWindow),
		RX2DR:       uint8(req.Rx2DR),
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
	macPL := queue.MACPayload{
		FRMPayload: req.FrmPayload,
		Data:       req.Data,
	}
	copy(macPL.DevEUI[:], req.DevEUI)
	if err := queue.AddMACPayloadToTXQueue(n.ctx.RedisPool, macPL); err != nil {
		return nil, grpc.Errorf(codes.Unknown, err.Error())
	}
	return &ns.EnqueueDataDownMACCommandResponse{}, nil
}
