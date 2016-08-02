package api

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	pb "github.com/brocaar/loraserver/api"
	"github.com/brocaar/loraserver/internal/api/auth"
	"github.com/brocaar/loraserver/internal/loraserver"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/models"
)

// ChannelListAPI exports the channel-list related functions.
type ChannelListAPI struct {
	ctx       loraserver.Context
	validator auth.Validator
}

// NewChannelListAPI creates a new ChannelListAPI.
func NewChannelListAPI(ctx loraserver.Context, validator auth.Validator) *ChannelListAPI {
	return &ChannelListAPI{
		ctx:       ctx,
		validator: validator,
	}
}

// Create creates the given channel-list.
func (a *ChannelListAPI) Create(ctx context.Context, req *pb.CreateChannelListRequest) (*pb.CreateChannelListResponse, error) {
	if err := a.validator.Validate(ctx, auth.ValidateAPIMethod("ChannelList.Create")); err != nil {
		return nil, grpc.Errorf(codes.Unauthenticated, "authentication failed: %s", err)
	}

	cl := models.ChannelList{
		Name: req.Name,
	}
	err := storage.CreateChannelList(a.ctx.DB, &cl)
	if err != nil {
		return nil, grpc.Errorf(codes.Unknown, err.Error())
	}

	return &pb.CreateChannelListResponse{Id: cl.ID}, nil
}

// Update updates the given channel-list.
func (a *ChannelListAPI) Update(ctx context.Context, req *pb.UpdateChannelListRequest) (*pb.UpdateChannelListResponse, error) {
	if err := a.validator.Validate(ctx, auth.ValidateAPIMethod("ChannelList.Update")); err != nil {
		return nil, grpc.Errorf(codes.Unauthenticated, "authentication failed: %s", err)
	}

	cl := models.ChannelList{
		ID:   req.Id,
		Name: req.Name,
	}
	err := storage.UpdateChannelList(a.ctx.DB, cl)
	if err != nil {
		return nil, grpc.Errorf(codes.Unknown, err.Error())
	}
	return &pb.UpdateChannelListResponse{}, nil
}

// Get returns the channel-list matching the given id.
func (a *ChannelListAPI) Get(ctx context.Context, req *pb.GetChannelListRequest) (*pb.GetChannelListResponse, error) {
	if err := a.validator.Validate(ctx, auth.ValidateAPIMethod("ChannelList.Get")); err != nil {
		return nil, grpc.Errorf(codes.Unauthenticated, "authentication failed: %s", err)
	}

	cl, err := storage.GetChannelList(a.ctx.DB, req.Id)
	if err != nil {
		return nil, grpc.Errorf(codes.Unknown, err.Error())
	}
	return &pb.GetChannelListResponse{
		Id:   cl.ID,
		Name: cl.Name,
	}, nil
}

// List lists the channel-lists.
func (a *ChannelListAPI) List(ctx context.Context, req *pb.ListChannelListRequest) (*pb.ListChannelListResponse, error) {
	if err := a.validator.Validate(ctx, auth.ValidateAPIMethod("ChannelList.List")); err != nil {
		return nil, grpc.Errorf(codes.Unauthenticated, "authentication failed: %s", err)
	}

	lists, err := storage.GetChannelLists(a.ctx.DB, int(req.Limit), int(req.Offset))
	if err != nil {
		return nil, grpc.Errorf(codes.Unknown, err.Error())
	}
	count, err := storage.GetChannelListsCount(a.ctx.DB)
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, err.Error())
	}

	resp := pb.ListChannelListResponse{
		TotalCount: int64(count),
	}
	for _, l := range lists {
		resp.Result = append(resp.Result, &pb.GetChannelListResponse{
			Id:   l.ID,
			Name: l.Name,
		})
	}
	return &resp, nil
}

// Delete deletes the channel-list matching the given id.
func (a *ChannelListAPI) Delete(ctx context.Context, req *pb.DeleteChannelListRequest) (*pb.DeleteChannelListResponse, error) {
	if err := a.validator.Validate(ctx, auth.ValidateAPIMethod("ChannelList.Delete")); err != nil {
		return nil, grpc.Errorf(codes.Unauthenticated, "authentication failed: %s", err)
	}

	err := storage.DeleteChannelList(a.ctx.DB, req.Id)
	if err != nil {
		return nil, grpc.Errorf(codes.Unknown, err.Error())
	}
	return &pb.DeleteChannelListResponse{}, nil
}

// ChannelAPI exports the channel related functions.
type ChannelAPI struct {
	ctx       loraserver.Context
	validator auth.Validator
}

// NewChannelAPI creates a new ChannelAPI.
func NewChannelAPI(ctx loraserver.Context, validator auth.Validator) *ChannelAPI {
	return &ChannelAPI{
		ctx:       ctx,
		validator: validator,
	}
}

// Create creates the given channel.
func (a *ChannelAPI) Create(ctx context.Context, req *pb.CreateChannelRequest) (*pb.CreateChannelResponse, error) {
	if err := a.validator.Validate(ctx, auth.ValidateAPIMethod("Channel.Create")); err != nil {
		return nil, grpc.Errorf(codes.Unauthenticated, "authentication failed: %s", err)
	}

	c := models.Channel{
		ChannelListID: req.ChannelListID,
		Channel:       int(req.Channel),
		Frequency:     int(req.Frequency),
	}
	err := storage.CreateChannel(a.ctx.DB, &c)
	if err != nil {
		return nil, grpc.Errorf(codes.Unknown, err.Error())
	}
	return &pb.CreateChannelResponse{Id: c.ID}, nil
}

// Get returns the channel matching the given id.
func (a *ChannelAPI) Get(ctx context.Context, req *pb.GetChannelRequest) (*pb.GetChannelResponse, error) {
	if err := a.validator.Validate(ctx, auth.ValidateAPIMethod("Channel.Get")); err != nil {
		return nil, grpc.Errorf(codes.Unauthenticated, "authentication failed: %s", err)
	}

	c, err := storage.GetChannel(a.ctx.DB, req.Id)
	if err != nil {
		return nil, grpc.Errorf(codes.Unknown, err.Error())
	}
	return &pb.GetChannelResponse{
		Id:            c.ID,
		ChannelListID: c.ChannelListID,
		Channel:       int64(c.Channel),
		Frequency:     int64(c.Frequency),
	}, nil
}

// Update updates the given channel.
func (a *ChannelAPI) Update(ctx context.Context, req *pb.UpdateChannelRequest) (*pb.UpdateChannelResponse, error) {
	if err := a.validator.Validate(ctx, auth.ValidateAPIMethod("Channel.Update")); err != nil {
		return nil, grpc.Errorf(codes.Unauthenticated, "authentication failed: %s", err)
	}

	c := models.Channel{
		ID:            req.Id,
		ChannelListID: req.ChannelListID,
		Channel:       int(req.Channel),
		Frequency:     int(req.Frequency),
	}
	err := storage.UpdateChannel(a.ctx.DB, c)
	if err != nil {
		return nil, grpc.Errorf(codes.Unknown, err.Error())
	}
	return &pb.UpdateChannelResponse{}, nil
}

// Delete deletest the channel matching the given id.
func (a *ChannelAPI) Delete(ctx context.Context, req *pb.DeleteChannelRequest) (*pb.DeleteChannelResponse, error) {
	if err := a.validator.Validate(ctx, auth.ValidateAPIMethod("Channel.Delete")); err != nil {
		return nil, grpc.Errorf(codes.Unauthenticated, "authentication failed: %s", err)
	}

	err := storage.DeleteChannel(a.ctx.DB, req.Id)
	if err != nil {
		return nil, grpc.Errorf(codes.Unknown, err.Error())
	}
	return &pb.DeleteChannelResponse{}, nil
}

// ListByChannelList lists the channels matching the given channel-list id.
func (a *ChannelAPI) ListByChannelList(ctx context.Context, req *pb.ListChannelsByChannelListRequest) (*pb.ListChannelsByChannelListResponse, error) {
	if err := a.validator.Validate(ctx, auth.ValidateAPIMethod("Channel.ListByChannelList")); err != nil {
		return nil, grpc.Errorf(codes.Unauthenticated, "authentication failed: %s", err)
	}

	channels, err := storage.GetChannelsForChannelList(a.ctx.DB, req.Id)
	if err != nil {
		return nil, grpc.Errorf(codes.Unknown, err.Error())
	}

	var resp pb.ListChannelsByChannelListResponse
	for _, c := range channels {
		resp.Result = append(resp.Result, &pb.GetChannelResponse{
			Id:            c.ID,
			ChannelListID: c.ChannelListID,
			Channel:       int64(c.Channel),
			Frequency:     int64(c.Frequency),
		})
	}
	return &resp, nil
}
