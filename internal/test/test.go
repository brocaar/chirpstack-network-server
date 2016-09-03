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
	PublishDataUpErr       error
	JoinRequestErr         error
	GetDataDownErr         error
	JoinRequestChan        chan as.JoinRequestRequest
	PublishDataUpChan      chan as.PublishDataUpRequest
	PublishDataDownACKChan chan as.PublishDataDownACKRequest
	PublishErrorChan       chan as.PublishErrorRequest
	GetDataDownChan        chan as.GetDataDownRequest

	JoinRequestResponse        as.JoinRequestResponse
	PublishDataUpResponse      as.PublishDataUpResponse
	PublishDataDownACKResponse as.PublishDataDownACKResponse
	PublishErrorResponse       as.PublishErrorResponse
	GetDataDownResponse        as.GetDataDownResponse
}

func NewTestApplicationClient() *TestApplicationClient {
	return &TestApplicationClient{
		JoinRequestChan:        make(chan as.JoinRequestRequest, 100),
		PublishDataUpChan:      make(chan as.PublishDataUpRequest, 100),
		PublishDataDownACKChan: make(chan as.PublishDataDownACKRequest, 100),
		PublishErrorChan:       make(chan as.PublishErrorRequest, 100),
		GetDataDownChan:        make(chan as.GetDataDownRequest, 100),
	}
}

func (t *TestApplicationClient) JoinRequest(ctx context.Context, in *as.JoinRequestRequest, opts ...grpc.CallOption) (*as.JoinRequestResponse, error) {
	if t.JoinRequestErr != nil {
		return nil, t.JoinRequestErr
	}
	t.JoinRequestChan <- *in
	return &t.JoinRequestResponse, nil
}

func (t *TestApplicationClient) PublishDataUp(ctx context.Context, in *as.PublishDataUpRequest, opts ...grpc.CallOption) (*as.PublishDataUpResponse, error) {
	if t.PublishDataUpErr != nil {
		return nil, t.PublishDataUpErr
	}
	t.PublishDataUpChan <- *in
	return &t.PublishDataUpResponse, nil
}

func (t *TestApplicationClient) GetDataDown(ctx context.Context, in *as.GetDataDownRequest, opts ...grpc.CallOption) (*as.GetDataDownResponse, error) {
	if t.GetDataDownErr != nil {
		return nil, t.GetDataDownErr
	}
	t.GetDataDownChan <- *in
	return &t.GetDataDownResponse, nil
}

func (t *TestApplicationClient) PublishDataDownACK(ctx context.Context, in *as.PublishDataDownACKRequest, opts ...grpc.CallOption) (*as.PublishDataDownACKResponse, error) {
	t.PublishDataDownACKChan <- *in
	return &t.PublishDataDownACKResponse, nil
}

func (t *TestApplicationClient) PublishError(ctx context.Context, in *as.PublishErrorRequest, opts ...grpc.CallOption) (*as.PublishErrorResponse, error) {
	t.PublishErrorChan <- *in
	return &t.PublishErrorResponse, nil
}

type TestNetworkControllerClient struct {
	PublishRXInfoChan           chan nc.PublishRXInfoRequest
	PublishDataUpMACCommandChan chan nc.PublishDataUpMACCommandRequest
	PublishErrorChan            chan nc.PublishErrorRequest

	PublishRXInfoResponse           nc.PublishRXInfoResponse
	PublishDataUpMACCommandResponse nc.PublishDataUpMACCommandResponse
	PublishErrorResponse            nc.PublishErrorResponse
}

func NewTestNetworkControllerClient() *TestNetworkControllerClient {
	return &TestNetworkControllerClient{
		PublishRXInfoChan:           make(chan nc.PublishRXInfoRequest, 100),
		PublishDataUpMACCommandChan: make(chan nc.PublishDataUpMACCommandRequest, 100),
		PublishErrorChan:            make(chan nc.PublishErrorRequest, 100),
	}
}

func (t *TestNetworkControllerClient) PublishRXInfo(ctx context.Context, in *nc.PublishRXInfoRequest, opts ...grpc.CallOption) (*nc.PublishRXInfoResponse, error) {
	t.PublishRXInfoChan <- *in
	return &t.PublishRXInfoResponse, nil
}

func (t *TestNetworkControllerClient) PublishDataUpMACCommand(ctx context.Context, in *nc.PublishDataUpMACCommandRequest, opts ...grpc.CallOption) (*nc.PublishDataUpMACCommandResponse, error) {
	t.PublishDataUpMACCommandChan <- *in
	return &t.PublishDataUpMACCommandResponse, nil
}

func (t *TestNetworkControllerClient) PublishError(ctx context.Context, in *nc.PublishErrorRequest, opts ...grpc.CallOption) (*nc.PublishErrorResponse, error) {
	t.PublishErrorChan <- *in
	return &t.PublishErrorResponse, nil
}
