package test

import (
	"github.com/golang/protobuf/ptypes/empty"
	context "golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/brocaar/chirpstack-api/go/v3/as"
	"github.com/brocaar/chirpstack-network-server/v3/internal/api/client/asclient"
)

// ApplicationServerPool is an application-server pool for testing.
type ApplicationServerPool struct {
	Client      as.ApplicationServerServiceClient
	GetHostname string
}

// Get returns the Client.
func (p *ApplicationServerPool) Get(hostname string, caCert, tlsCert, tlsKey []byte) (as.ApplicationServerServiceClient, error) {
	p.GetHostname = hostname
	return p.Client, nil
}

// NewApplicationServerPool create an application-server client pool which
// always returns the given client on Get.
func NewApplicationServerPool(client *ApplicationClient) asclient.Pool {
	return &ApplicationServerPool{
		Client: client,
	}
}

// ApplicationClient is an application client for testing.
type ApplicationClient struct {
	HandleDataUpErr                error
	HandleProprietaryUpErr         error
	HandleDownlinkACKErr           error
	HandleTxAckError               error
	SetDeviceStatusError           error
	SetDeviceLocationErrror        error
	ReEncryptDeviceQueueItemsError error

	HandleDataUpChan              chan as.HandleUplinkDataRequest
	HandleProprietaryUpChan       chan as.HandleProprietaryUplinkRequest
	HandleErrorChan               chan as.HandleErrorRequest
	HandleDownlinkACKChan         chan as.HandleDownlinkACKRequest
	HandleTxAckChan               chan as.HandleTxAckRequest
	HandleGatewayStatsChan        chan as.HandleGatewayStatsRequest
	SetDeviceStatusChan           chan as.SetDeviceStatusRequest
	SetDeviceLocationChan         chan as.SetDeviceLocationRequest
	ReEncryptDeviceQueueItemsChan chan as.ReEncryptDeviceQueueItemsRequest

	HandleDataUpResponse              empty.Empty
	HandleProprietaryUpResponse       empty.Empty
	HandleErrorResponse               empty.Empty
	HandleDownlinkACKResponse         empty.Empty
	HandleTxAckResponse               empty.Empty
	HandleGatewayStatsResponse        empty.Empty
	SetDeviceStatusResponse           empty.Empty
	SetDeviceLocationResponse         empty.Empty
	ReEncryptDeviceQueueItemsResponse as.ReEncryptDeviceQueueItemsResponse
}

// NewApplicationClient returns a new ApplicationClient.
func NewApplicationClient() *ApplicationClient {
	return &ApplicationClient{
		HandleDataUpChan:              make(chan as.HandleUplinkDataRequest, 100),
		HandleProprietaryUpChan:       make(chan as.HandleProprietaryUplinkRequest, 100),
		HandleErrorChan:               make(chan as.HandleErrorRequest, 100),
		HandleDownlinkACKChan:         make(chan as.HandleDownlinkACKRequest, 100),
		HandleTxAckChan:               make(chan as.HandleTxAckRequest, 100),
		HandleGatewayStatsChan:        make(chan as.HandleGatewayStatsRequest, 100),
		SetDeviceStatusChan:           make(chan as.SetDeviceStatusRequest, 100),
		SetDeviceLocationChan:         make(chan as.SetDeviceLocationRequest, 100),
		ReEncryptDeviceQueueItemsChan: make(chan as.ReEncryptDeviceQueueItemsRequest, 100),
	}
}

// HandleUplinkData method.
func (t *ApplicationClient) HandleUplinkData(ctx context.Context, in *as.HandleUplinkDataRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	if t.HandleDataUpErr != nil {
		return nil, t.HandleDataUpErr
	}
	t.HandleDataUpChan <- *in
	return &t.HandleDataUpResponse, nil
}

// HandleProprietaryUplink method.
func (t *ApplicationClient) HandleProprietaryUplink(ctx context.Context, in *as.HandleProprietaryUplinkRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	if t.HandleProprietaryUpErr != nil {
		return nil, t.HandleProprietaryUpErr
	}
	t.HandleProprietaryUpChan <- *in
	return &t.HandleProprietaryUpResponse, nil
}

// HandleError method.
func (t *ApplicationClient) HandleError(ctx context.Context, in *as.HandleErrorRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	t.HandleErrorChan <- *in
	return &t.HandleErrorResponse, nil
}

// HandleDownlinkACK method.
func (t *ApplicationClient) HandleDownlinkACK(ctx context.Context, in *as.HandleDownlinkACKRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	t.HandleDownlinkACKChan <- *in
	return &t.HandleDownlinkACKResponse, nil
}

// HandleTxAck method.
func (t *ApplicationClient) HandleTxAck(ctx context.Context, in *as.HandleTxAckRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	t.HandleTxAckChan <- *in
	return &t.HandleTxAckResponse, nil
}

// HandleGatewayStats method.
func (t *ApplicationClient) HandleGatewayStats(ctx context.Context, in *as.HandleGatewayStatsRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	t.HandleGatewayStatsChan <- *in
	return &t.HandleGatewayStatsResponse, nil
}

// SetDeviceStatus method.
func (t *ApplicationClient) SetDeviceStatus(ctx context.Context, in *as.SetDeviceStatusRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	t.SetDeviceStatusChan <- *in
	return &t.SetDeviceStatusResponse, t.SetDeviceStatusError
}

// SetDeviceLocation method.
func (t *ApplicationClient) SetDeviceLocation(ctx context.Context, in *as.SetDeviceLocationRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	t.SetDeviceLocationChan <- *in
	return &t.SetDeviceLocationResponse, t.SetDeviceLocationErrror
}

// ReEncryptDeviceQueueItems method.
func (t *ApplicationClient) ReEncryptDeviceQueueItems(ctx context.Context, in *as.ReEncryptDeviceQueueItemsRequest, opts ...grpc.CallOption) (*as.ReEncryptDeviceQueueItemsResponse, error) {
	t.ReEncryptDeviceQueueItemsChan <- *in
	return &t.ReEncryptDeviceQueueItemsResponse, t.ReEncryptDeviceQueueItemsError
}
