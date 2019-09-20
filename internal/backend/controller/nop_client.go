package controller

import (
	"context"

	"github.com/brocaar/loraserver/api/nc"
	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
)

// NopNetworkControllerClient is a dummy network-controller client which is
// used when no network-controller is present / configured.
type NopNetworkControllerClient struct{}

// HandleUplinkMetaData handles uplink meta-rata.
func (n *NopNetworkControllerClient) HandleUplinkMetaData(ctx context.Context, in *nc.HandleUplinkMetaDataRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	return &empty.Empty{}, nil
}

// HandleUplinkMACCommand handles an uplink mac-command.
// This method will only be called in case the mac-command request was
// enqueued throught the API or when the CID is >= 0x80 (proprietary
// mac-command range).
func (n *NopNetworkControllerClient) HandleDownlinkMetaData(ctx context.Context, in *nc.HandleDownlinkMetaDataRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	return &empty.Empty{}, nil
}

// HandleUplinkMACCommand handles an uplink mac-command.
// This method will only be called in case the mac-command request was
// enqueued throught the API or when the CID is >= 0x80 (proprietary
// mac-command range).
func (n *NopNetworkControllerClient) HandleUplinkMACCommand(ctx context.Context, in *nc.HandleUplinkMACCommandRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	return &empty.Empty{}, nil
}
