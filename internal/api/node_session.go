package api

import (
	"encoding/hex"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"golang.org/x/net/context"

	pb "github.com/brocaar/loraserver/api"
	"github.com/brocaar/loraserver/internal/api/auth"
	"github.com/brocaar/loraserver/internal/loraserver"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
)

// NodeSessionAPI exposes the node-session related functions.
type NodeSessionAPI struct {
	ctx       loraserver.Context
	validator auth.Validator
}

// NewNodeSessionAPI creates a new NodeSessionAPI.
func NewNodeSessionAPI(ctx loraserver.Context, validator auth.Validator) *NodeSessionAPI {
	return &NodeSessionAPI{
		ctx:       ctx,
		validator: validator,
	}
}

// Create creates the given node-session. The DevAddr must contain the same NwkID as the configured NetID. Node-sessions will expire automatically after the configured TTL.
func (a *NodeSessionAPI) Create(ctx context.Context, req *pb.CreateNodeSessionRequest) (*pb.CreateNodeSessionResponse, error) {
	var appEUI, devEUI lorawan.EUI64
	var appSKey, nwkSKey lorawan.AES128Key
	var devAddr lorawan.DevAddr

	if err := appEUI.UnmarshalText([]byte(req.AppEUI)); err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, err.Error())
	}
	if err := devEUI.UnmarshalText([]byte(req.DevEUI)); err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, err.Error())
	}
	if err := appSKey.UnmarshalText([]byte(req.AppSKey)); err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, err.Error())
	}
	if err := nwkSKey.UnmarshalText([]byte(req.NwkSKey)); err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, err.Error())
	}
	if err := devAddr.UnmarshalText([]byte(req.DevAddr)); err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, err.Error())
	}

	if err := a.validator.Validate(ctx,
		auth.ValidateAPIMethod("NodeSession.Create"),
		auth.ValidateApplication(appEUI),
		auth.ValidateNode(devEUI),
	); err != nil {
		return nil, grpc.Errorf(codes.Unauthenticated, "authentication failed: %s", err)
	}

	var cFList *lorawan.CFList
	if len(req.CFList) > len(cFList) {
		return nil, fmt.Errorf("max CFList channels is %d", len(cFList))
	}
	if len(req.CFList) > 0 {
		cFList = &lorawan.CFList{}
		for i, f := range req.CFList {
			cFList[i] = f
		}
	}

	ns := models.NodeSession{
		DevAddr:     devAddr,
		AppEUI:      appEUI,
		DevEUI:      devEUI,
		AppSKey:     appSKey,
		NwkSKey:     nwkSKey,
		FCntUp:      req.FCntUp,
		FCntDown:    req.FCntDown,
		RXDelay:     uint8(req.RxDelay),
		RX1DROffset: uint8(req.Rx1DROffset),
		CFList:      cFList,
	}

	if err := a.validateNodeSession(ns); err != nil {
		return nil, err
	}

	if err := storage.CreateNodeSession(a.ctx.RedisPool, ns); err != nil {
		return nil, grpc.Errorf(codes.Internal, err.Error())
	}

	return &pb.CreateNodeSessionResponse{}, nil
}

func (a *NodeSessionAPI) validateNodeSession(ns models.NodeSession) error {
	// validate the NwkID
	if ns.DevAddr.NwkID() != a.ctx.NetID.NwkID() {
		return grpc.Errorf(codes.InvalidArgument, "DevAddr must contain NwkID %s", hex.EncodeToString([]byte{a.ctx.NetID.NwkID()}))
	}

	// validate that the node exists
	var node models.Node
	var err error
	if node, err = storage.GetNode(a.ctx.DB, ns.DevEUI); err != nil {
		return grpc.Errorf(codes.Unknown, err.Error())
	}

	// validate that the node belongs to the given AppEUI.
	if ns.AppEUI != node.AppEUI {
		return grpc.Errorf(codes.InvalidArgument, "DevEUI belongs to different AppEUI")
	}

	return nil
}

// Get returns the node-session matching the given DevAddr.
func (a *NodeSessionAPI) Get(ctx context.Context, req *pb.GetNodeSessionRequest) (*pb.GetNodeSessionResponse, error) {
	var devAddr lorawan.DevAddr
	if err := devAddr.UnmarshalText([]byte(req.DevAddr)); err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, err.Error())
	}

	ns, err := storage.GetNodeSession(a.ctx.RedisPool, devAddr)
	if err != nil {
		return nil, grpc.Errorf(codes.Unknown, err.Error())
	}

	if err := a.validator.Validate(ctx,
		auth.ValidateAPIMethod("NodeSession.Get"),
		auth.ValidateApplication(ns.AppEUI),
		auth.ValidateNode(ns.DevEUI),
	); err != nil {
		return nil, grpc.Errorf(codes.Unauthenticated, "authentication failed: %s", err)
	}

	return a.nodeSessionToResponse(ns)
}

func (a *NodeSessionAPI) nodeSessionToResponse(ns models.NodeSession) (*pb.GetNodeSessionResponse, error) {
	resp := pb.GetNodeSessionResponse{
		DevAddr:     ns.DevAddr.String(),
		AppEUI:      ns.AppEUI.String(),
		DevEUI:      ns.DevEUI.String(),
		AppSKey:     ns.AppSKey.String(),
		NwkSKey:     ns.NwkSKey.String(),
		FCntUp:      ns.FCntUp,
		FCntDown:    ns.FCntDown,
		RxDelay:     uint32(ns.RXDelay),
		Rx1DROffset: uint32(ns.RX1DROffset),
	}

	if ns.CFList != nil {
		resp.CFList = ns.CFList[:]
	}

	return &resp, nil
}

// GetByDevEUI returns the node-session matching the given DevEUI.
func (a *NodeSessionAPI) GetByDevEUI(ctx context.Context, req *pb.GetNodeSessionByDevEUIRequest) (*pb.GetNodeSessionResponse, error) {
	var eui lorawan.EUI64
	if err := eui.UnmarshalText([]byte(req.DevEUI)); err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, err.Error())
	}

	ns, err := storage.GetNodeSessionByDevEUI(a.ctx.RedisPool, eui)
	if err != nil {
		return nil, grpc.Errorf(codes.Unknown, err.Error())
	}

	if err := a.validator.Validate(ctx,
		auth.ValidateAPIMethod("NodeSession.GetByDevEUI"),
		auth.ValidateApplication(ns.AppEUI),
		auth.ValidateNode(ns.DevEUI),
	); err != nil {
		return nil, grpc.Errorf(codes.Unauthenticated, "authentication failed: %s", err)
	}

	return a.nodeSessionToResponse(ns)
}

// Update updates the given node-session.
func (a *NodeSessionAPI) Update(ctx context.Context, req *pb.UpdateNodeSessionRequest) (*pb.UpdateNodeSessionResponse, error) {
	var appEUI, devEUI lorawan.EUI64
	var appSKey, nwkSKey lorawan.AES128Key
	var devAddr lorawan.DevAddr

	if err := appEUI.UnmarshalText([]byte(req.AppEUI)); err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, err.Error())
	}
	if err := devEUI.UnmarshalText([]byte(req.DevEUI)); err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, err.Error())
	}
	if err := appSKey.UnmarshalText([]byte(req.AppSKey)); err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, err.Error())
	}
	if err := nwkSKey.UnmarshalText([]byte(req.NwkSKey)); err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, err.Error())
	}
	if err := devAddr.UnmarshalText([]byte(req.DevAddr)); err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, err.Error())
	}

	if err := a.validator.Validate(ctx,
		auth.ValidateAPIMethod("NodeSession.Upate"),
		auth.ValidateApplication(appEUI),
		auth.ValidateNode(devEUI),
	); err != nil {
		return nil, grpc.Errorf(codes.Unauthenticated, "authentication failed: %s", err)
	}

	// get the existing node-session to validate if it belongs to the same
	// DevEUI and AppEUI
	ns, err := storage.GetNodeSession(a.ctx.RedisPool, devAddr)
	if err != nil {
		return nil, grpc.Errorf(codes.Unknown, err.Error())
	}
	if ns.DevEUI != devEUI {
		return nil, grpc.Errorf(codes.InvalidArgument, "node-session belongs to a different DevEUI")
	}
	if ns.AppEUI != appEUI {
		return nil, grpc.Errorf(codes.InvalidArgument, "node-session belongs to a different AppEUI")
	}

	var cFList *lorawan.CFList
	if len(req.CFList) > len(cFList) {
		return nil, fmt.Errorf("max CFList channels is %d", len(cFList))
	}
	if len(req.CFList) > 0 {
		cFList = &lorawan.CFList{}
		for i, f := range req.CFList {
			cFList[i] = f
		}
	}

	ns = models.NodeSession{
		DevAddr:     devAddr,
		AppEUI:      appEUI,
		DevEUI:      devEUI,
		AppSKey:     appSKey,
		NwkSKey:     nwkSKey,
		FCntUp:      req.FCntUp,
		FCntDown:    req.FCntDown,
		RXDelay:     uint8(req.RxDelay),
		RX1DROffset: uint8(req.Rx1DROffset),
		CFList:      cFList,
	}

	if err := a.validateNodeSession(ns); err != nil {
		return nil, err
	}

	if err := storage.SaveNodeSession(a.ctx.RedisPool, ns); err != nil {
		return nil, grpc.Errorf(codes.Unknown, err.Error())
	}

	return &pb.UpdateNodeSessionResponse{}, nil
}

// Delete deletes the node-session matching the given DevAddr.
func (a *NodeSessionAPI) Delete(ctx context.Context, req *pb.DeleteNodeSessionRequest) (*pb.DeleteNodeSessionResponse, error) {
	var devAddr lorawan.DevAddr
	if err := devAddr.UnmarshalText([]byte(req.DevAddr)); err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, err.Error())
	}

	ns, err := storage.GetNodeSession(a.ctx.RedisPool, devAddr)
	if err != nil {
		return nil, grpc.Errorf(codes.Unknown, err.Error())
	}

	if err := a.validator.Validate(ctx,
		auth.ValidateAPIMethod("NodeSession.Delete"),
		auth.ValidateApplication(ns.AppEUI),
		auth.ValidateNode(ns.DevEUI),
	); err != nil {
		return nil, grpc.Errorf(codes.Unauthenticated, "authentication failed: %s", err)
	}

	if err := storage.DeleteNodeSession(a.ctx.RedisPool, devAddr); err != nil {
		return nil, grpc.Errorf(codes.Unknown, err.Error())
	}

	return &pb.DeleteNodeSessionResponse{}, nil
}

// GetRandomDevAddr returns a random DevAddr taking the NwkID prefix into account.
func (a *NodeSessionAPI) GetRandomDevAddr(ctx context.Context, req *pb.GetRandomDevAddrRequest) (*pb.GetRandomDevAddrResponse, error) {
	if err := a.validator.Validate(ctx, auth.ValidateAPIMethod("NodeSession.GetRandomDevAddr")); err != nil {
		return nil, grpc.Errorf(codes.Unauthenticated, "authentication failed: %s", err)
	}

	devAddr, err := storage.GetRandomDevAddr(a.ctx.RedisPool, a.ctx.NetID)
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, err.Error())
	}

	return &pb.GetRandomDevAddrResponse{
		DevAddr: devAddr.String(),
	}, nil
}
