package test

import (
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
	context "golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/api/nc"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/lorawan/band"
)

func init() {
	var err error
	log.SetLevel(log.ErrorLevel)

	common.Band, err = band.GetConfig(band.EU_863_870)
	if err != nil {
		panic(err)
	}
}

// TestConfig contains the test configuration.
type TestConfig struct {
	RedisURL string
}

// GetTestConfig returns the test configuration.
func GetTestConfig() *TestConfig {
	var err error
	log.SetLevel(log.ErrorLevel)

	common.Band, err = band.GetConfig(band.EU_863_870)
	if err != nil {
		panic(err)
	}

	c := &TestConfig{
		RedisURL: "redis://localhost:6379",
	}

	if v := os.Getenv("TEST_REDIS_URL"); v != "" {
		c.RedisURL = v
	}

	return c
}

// MustFlushRedis flushes the Redis storage.
func MustFlushRedis(p *redis.Pool) {
	c := p.Get()
	defer c.Close()
	if _, err := c.Do("FLUSHALL"); err != nil {
		log.Fatal(err)
	}
}

type TestGatewayBackend struct {
	rxPacketChan chan gw.RXPacket
	TXPacketChan chan gw.TXPacket
}

func NewTestGatewayBackend() *TestGatewayBackend {
	return &TestGatewayBackend{
		rxPacketChan: make(chan gw.RXPacket, 100),
		TXPacketChan: make(chan gw.TXPacket, 100),
	}
}

func (b *TestGatewayBackend) SendTXPacket(txPacket gw.TXPacket) error {
	b.TXPacketChan <- txPacket
	return nil
}

func (b *TestGatewayBackend) RXPacketChan() chan gw.RXPacket {
	return b.rxPacketChan
}

func (b *TestGatewayBackend) Close() error {
	if b.rxPacketChan != nil {
		close(b.rxPacketChan)
	}
	return nil
}

type TestApplicationClient struct {
	HandleDataUpErr       error
	JoinRequestErr        error
	GetDataDownErr        error
	JoinRequestChan       chan as.JoinRequestRequest
	HandleDataUpChan      chan as.HandleDataUpRequest
	HandleDataDownACKChan chan as.HandleDataDownACKRequest
	HandleErrorChan       chan as.HandleErrorRequest
	GetDataDownChan       chan as.GetDataDownRequest

	JoinRequestResponse       as.JoinRequestResponse
	HandleDataUpResponse      as.HandleDataUpResponse
	HandleDataDownACKResponse as.HandleDataDownACKResponse
	HandleErrorResponse       as.HandleErrorResponse
	GetDataDownResponse       as.GetDataDownResponse
}

func NewTestApplicationClient() *TestApplicationClient {
	return &TestApplicationClient{
		JoinRequestChan:       make(chan as.JoinRequestRequest, 100),
		HandleDataUpChan:      make(chan as.HandleDataUpRequest, 100),
		HandleDataDownACKChan: make(chan as.HandleDataDownACKRequest, 100),
		HandleErrorChan:       make(chan as.HandleErrorRequest, 100),
		GetDataDownChan:       make(chan as.GetDataDownRequest, 100),
	}
}

func (t *TestApplicationClient) JoinRequest(ctx context.Context, in *as.JoinRequestRequest, opts ...grpc.CallOption) (*as.JoinRequestResponse, error) {
	if t.JoinRequestErr != nil {
		return nil, t.JoinRequestErr
	}
	t.JoinRequestChan <- *in
	return &t.JoinRequestResponse, nil
}

func (t *TestApplicationClient) HandleDataUp(ctx context.Context, in *as.HandleDataUpRequest, opts ...grpc.CallOption) (*as.HandleDataUpResponse, error) {
	if t.HandleDataUpErr != nil {
		return nil, t.HandleDataUpErr
	}
	t.HandleDataUpChan <- *in
	return &t.HandleDataUpResponse, nil
}

func (t *TestApplicationClient) GetDataDown(ctx context.Context, in *as.GetDataDownRequest, opts ...grpc.CallOption) (*as.GetDataDownResponse, error) {
	if t.GetDataDownErr != nil {
		return nil, t.GetDataDownErr
	}
	t.GetDataDownChan <- *in
	return &t.GetDataDownResponse, nil
}

func (t *TestApplicationClient) HandleDataDownACK(ctx context.Context, in *as.HandleDataDownACKRequest, opts ...grpc.CallOption) (*as.HandleDataDownACKResponse, error) {
	t.HandleDataDownACKChan <- *in
	return &t.HandleDataDownACKResponse, nil
}

func (t *TestApplicationClient) HandleError(ctx context.Context, in *as.HandleErrorRequest, opts ...grpc.CallOption) (*as.HandleErrorResponse, error) {
	t.HandleErrorChan <- *in
	return &t.HandleErrorResponse, nil
}

type TestNetworkControllerClient struct {
	HandleRXInfoChan           chan nc.HandleRXInfoRequest
	HandleDataUpMACCommandChan chan nc.HandleDataUpMACCommandRequest
	HandleErrorChan            chan nc.HandleErrorRequest

	HandleRXInfoResponse           nc.HandleRXInfoResponse
	HandleDataUpMACCommandResponse nc.HandleDataUpMACCommandResponse
	HandleErrorResponse            nc.HandleErrorResponse
}

func NewTestNetworkControllerClient() *TestNetworkControllerClient {
	return &TestNetworkControllerClient{
		HandleRXInfoChan:           make(chan nc.HandleRXInfoRequest, 100),
		HandleDataUpMACCommandChan: make(chan nc.HandleDataUpMACCommandRequest, 100),
		HandleErrorChan:            make(chan nc.HandleErrorRequest, 100),
	}
}

func (t *TestNetworkControllerClient) HandleRXInfo(ctx context.Context, in *nc.HandleRXInfoRequest, opts ...grpc.CallOption) (*nc.HandleRXInfoResponse, error) {
	t.HandleRXInfoChan <- *in
	return &t.HandleRXInfoResponse, nil
}

func (t *TestNetworkControllerClient) HandleDataUpMACCommand(ctx context.Context, in *nc.HandleDataUpMACCommandRequest, opts ...grpc.CallOption) (*nc.HandleDataUpMACCommandResponse, error) {
	t.HandleDataUpMACCommandChan <- *in
	return &t.HandleDataUpMACCommandResponse, nil
}

func (t *TestNetworkControllerClient) HandleError(ctx context.Context, in *nc.HandleErrorRequest, opts ...grpc.CallOption) (*nc.HandleErrorResponse, error) {
	t.HandleErrorChan <- *in
	return &t.HandleErrorResponse, nil
}
