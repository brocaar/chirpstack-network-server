package controller

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/Frankz/loraserver/api/nc"
)

// NopNetworkControllerClient is a dummy network-controller client which is
// used when no network-controller is present / configured.
type NopNetworkControllerClient struct{}

// HandleRXInfo publishes rx related meta-data.
func (n *NopNetworkControllerClient) HandleRXInfo(ctx context.Context, in *nc.HandleRXInfoRequest, opts ...grpc.CallOption) (*nc.HandleRXInfoResponse, error) {
	return &nc.HandleRXInfoResponse{}, nil
}

// HandleDataUpMACCommand publishes a mac-command received by an end-device.
// This method will only be called in case the mac-command request was
// enqueued throught the API or when the CID is >= 0x80 (proprietary
// mac-command range).
func (n *NopNetworkControllerClient) HandleDataUpMACCommand(ctx context.Context, in *nc.HandleDataUpMACCommandRequest, opts ...grpc.CallOption) (*nc.HandleDataUpMACCommandResponse, error) {
	return &nc.HandleDataUpMACCommandResponse{}, nil
}
