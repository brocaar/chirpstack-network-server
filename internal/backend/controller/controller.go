package controller

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/brocaar/loraserver/api/nc"
)

// NopNetworkControllerClient is a dummy network-controller client which is
// used when no network-controller is present / configured.
type NopNetworkControllerClient struct{}

func (n *NopNetworkControllerClient) HandleRXInfo(ctx context.Context, in *nc.HandleRXInfoRequest, opts ...grpc.CallOption) (*nc.HandleRXInfoResponse, error) {
	return &nc.HandleRXInfoResponse{}, nil
}

func (n *NopNetworkControllerClient) HandleDataUpMACCommand(ctx context.Context, in *nc.HandleDataUpMACCommandRequest, opts ...grpc.CallOption) (*nc.HandleDataUpMACCommandResponse, error) {
	return &nc.HandleDataUpMACCommandResponse{}, nil
}

func (n *NopNetworkControllerClient) HandleError(ctx context.Context, in *nc.HandleErrorRequest, opts ...grpc.CallOption) (*nc.HandleErrorResponse, error) {
	return &nc.HandleErrorResponse{}, nil
}
