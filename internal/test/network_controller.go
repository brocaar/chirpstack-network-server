package test

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"github.com/brocaar/chirpstack-api/go/v3/nc"
)

// NetworkControllerClient is a network-controller client for testing.
type NetworkControllerClient struct {
	HandleUplinkMetaDataChan         chan nc.HandleUplinkMetaDataRequest
	HandleDownlinkMetaDataChan       chan nc.HandleDownlinkMetaDataRequest
	HandleDataUpMACCommandChan       chan nc.HandleUplinkMACCommandRequest
	HandleRejectedUplinkFrameSetChan chan nc.HandleRejectedUplinkFrameSetRequest
}

// NewNetworkControllerClient returns a new NetworkControllerClient.
func NewNetworkControllerClient() *NetworkControllerClient {
	return &NetworkControllerClient{
		HandleUplinkMetaDataChan:         make(chan nc.HandleUplinkMetaDataRequest, 100),
		HandleDownlinkMetaDataChan:       make(chan nc.HandleDownlinkMetaDataRequest, 100),
		HandleDataUpMACCommandChan:       make(chan nc.HandleUplinkMACCommandRequest, 100),
		HandleRejectedUplinkFrameSetChan: make(chan nc.HandleRejectedUplinkFrameSetRequest, 100),
	}
}

// HandleUplinkMetaData method.
func (t *NetworkControllerClient) HandleUplinkMetaData(ctx context.Context, in *nc.HandleUplinkMetaDataRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	t.HandleUplinkMetaDataChan <- *in
	return &empty.Empty{}, nil
}

// HandleDownlinkMetaData method.
func (t *NetworkControllerClient) HandleDownlinkMetaData(ctx context.Context, in *nc.HandleDownlinkMetaDataRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	t.HandleDownlinkMetaDataChan <- *in
	return &empty.Empty{}, nil
}

// HandleUplinkMACCommand method.
func (t *NetworkControllerClient) HandleUplinkMACCommand(ctx context.Context, in *nc.HandleUplinkMACCommandRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	t.HandleDataUpMACCommandChan <- *in
	return &empty.Empty{}, nil
}

// HandleRejectedUplinkFrameSet method.
func (t *NetworkControllerClient) HandleRejectedUplinkFrameSet(ctx context.Context, in *nc.HandleRejectedUplinkFrameSetRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	t.HandleRejectedUplinkFrameSetChan <- *in
	return &empty.Empty{}, nil
}
