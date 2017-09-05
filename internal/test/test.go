package test

import (
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
	"github.com/jmoiron/sqlx"
	migrate "github.com/rubenv/sql-migrate"
	context "golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/api/nc"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/migrations"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
)

func init() {
	var err error
	log.SetLevel(log.ErrorLevel)

	common.Band, err = band.GetConfig(band.EU_863_870, false, lorawan.DwellTimeNoLimit)
	if err != nil {
		panic(err)
	}
	common.BandName = band.EU_863_870
	common.DeduplicationDelay = 5 * time.Millisecond
	common.GetDownlinkDataDelay = 5 * time.Millisecond

	loc, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		panic(err)
	}
	common.TimeLocation = loc
}

// Config contains the test configuration.
type Config struct {
	RedisURL    string
	PostgresDSN string
}

// GetConfig returns the test configuration.
func GetConfig() *Config {
	log.SetLevel(log.ErrorLevel)

	c := &Config{
		RedisURL:    "redis://localhost:6379",
		PostgresDSN: "postgres://localhost/loraserver_ns_test?sslmode=disable",
	}

	if v := os.Getenv("TEST_REDIS_URL"); v != "" {
		c.RedisURL = v
	}

	if v := os.Getenv("TEST_POSTGRES_DSN"); v != "" {
		c.PostgresDSN = v
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

// MustResetDB re-applies all database migrations.
func MustResetDB(db *sqlx.DB) {
	m := &migrate.AssetMigrationSource{
		Asset:    migrations.Asset,
		AssetDir: migrations.AssetDir,
		Dir:      "",
	}
	if _, err := migrate.Exec(db.DB, "postgres", m, migrate.Down); err != nil {
		log.Fatal(err)
	}
	if _, err := migrate.Exec(db.DB, "postgres", m, migrate.Up); err != nil {
		log.Fatal(err)
	}
}

// GatewayBackend is a test gateway backend.
type GatewayBackend struct {
	rxPacketChan    chan gw.RXPacket
	TXPacketChan    chan gw.TXPacket
	statsPacketChan chan gw.GatewayStatsPacket
}

// NewGatewayBackend returns a new GatewayBackend.
func NewGatewayBackend() *GatewayBackend {
	return &GatewayBackend{
		rxPacketChan: make(chan gw.RXPacket, 100),
		TXPacketChan: make(chan gw.TXPacket, 100),
	}
}

// SendTXPacket method.
func (b *GatewayBackend) SendTXPacket(txPacket gw.TXPacket) error {
	b.TXPacketChan <- txPacket
	return nil
}

// RXPacketChan method.
func (b *GatewayBackend) RXPacketChan() chan gw.RXPacket {
	return b.rxPacketChan
}

// StatsPacketChan method.
func (b *GatewayBackend) StatsPacketChan() chan gw.GatewayStatsPacket {
	return b.statsPacketChan
}

// Close method.
func (b *GatewayBackend) Close() error {
	if b.rxPacketChan != nil {
		close(b.rxPacketChan)
	}
	return nil
}

// ApplicationClient is an application client for testing.
type ApplicationClient struct {
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

// NewApplicationClient returns a new ApplicationClient.
func NewApplicationClient() *ApplicationClient {
	return &ApplicationClient{
		JoinRequestChan:       make(chan as.JoinRequestRequest, 100),
		HandleDataUpChan:      make(chan as.HandleDataUpRequest, 100),
		HandleDataDownACKChan: make(chan as.HandleDataDownACKRequest, 100),
		HandleErrorChan:       make(chan as.HandleErrorRequest, 100),
		GetDataDownChan:       make(chan as.GetDataDownRequest, 100),
	}
}

// JoinRequest method.
func (t *ApplicationClient) JoinRequest(ctx context.Context, in *as.JoinRequestRequest, opts ...grpc.CallOption) (*as.JoinRequestResponse, error) {
	if t.JoinRequestErr != nil {
		return nil, t.JoinRequestErr
	}
	t.JoinRequestChan <- *in
	return &t.JoinRequestResponse, nil
}

// HandleDataUp method.
func (t *ApplicationClient) HandleDataUp(ctx context.Context, in *as.HandleDataUpRequest, opts ...grpc.CallOption) (*as.HandleDataUpResponse, error) {
	if t.HandleDataUpErr != nil {
		return nil, t.HandleDataUpErr
	}
	t.HandleDataUpChan <- *in
	return &t.HandleDataUpResponse, nil
}

// GetDataDown method.
func (t *ApplicationClient) GetDataDown(ctx context.Context, in *as.GetDataDownRequest, opts ...grpc.CallOption) (*as.GetDataDownResponse, error) {
	if t.GetDataDownErr != nil {
		return nil, t.GetDataDownErr
	}
	t.GetDataDownChan <- *in
	return &t.GetDataDownResponse, nil
}

// HandleDataDownACK method.
func (t *ApplicationClient) HandleDataDownACK(ctx context.Context, in *as.HandleDataDownACKRequest, opts ...grpc.CallOption) (*as.HandleDataDownACKResponse, error) {
	t.HandleDataDownACKChan <- *in
	return &t.HandleDataDownACKResponse, nil
}

// HandleError method.
func (t *ApplicationClient) HandleError(ctx context.Context, in *as.HandleErrorRequest, opts ...grpc.CallOption) (*as.HandleErrorResponse, error) {
	t.HandleErrorChan <- *in
	return &t.HandleErrorResponse, nil
}

// NetworkControllerClient is a network-controller client for testing.
type NetworkControllerClient struct {
	HandleRXInfoChan           chan nc.HandleRXInfoRequest
	HandleDataUpMACCommandChan chan nc.HandleDataUpMACCommandRequest

	HandleRXInfoResponse           nc.HandleRXInfoResponse
	HandleDataUpMACCommandResponse nc.HandleDataUpMACCommandResponse
}

// NewNetworkControllerClient returns a new NetworkControllerClient.
func NewNetworkControllerClient() *NetworkControllerClient {
	return &NetworkControllerClient{
		HandleRXInfoChan:           make(chan nc.HandleRXInfoRequest, 100),
		HandleDataUpMACCommandChan: make(chan nc.HandleDataUpMACCommandRequest, 100),
	}
}

// HandleRXInfo method.
func (t *NetworkControllerClient) HandleRXInfo(ctx context.Context, in *nc.HandleRXInfoRequest, opts ...grpc.CallOption) (*nc.HandleRXInfoResponse, error) {
	t.HandleRXInfoChan <- *in
	return &t.HandleRXInfoResponse, nil
}

// HandleDataUpMACCommand method.
func (t *NetworkControllerClient) HandleDataUpMACCommand(ctx context.Context, in *nc.HandleDataUpMACCommandRequest, opts ...grpc.CallOption) (*nc.HandleDataUpMACCommandResponse, error) {
	t.HandleDataUpMACCommandChan <- *in
	return &t.HandleDataUpMACCommandResponse, nil
}
