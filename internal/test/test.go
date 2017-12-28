package test

import (
	"os"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/jmoiron/sqlx"
	migrate "github.com/rubenv/sql-migrate"
	log "github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/api/nc"
	"github.com/brocaar/loraserver/internal/api/client/asclient"
	"github.com/brocaar/loraserver/internal/api/client/jsclient"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/migrations"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
	"github.com/brocaar/lorawan/band"
)

func init() {
	log.SetLevel(log.ErrorLevel)

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
	var err error
	log.SetLevel(log.ErrorLevel)

	common.Band, err = band.GetConfig(band.EU_863_870, false, lorawan.DwellTimeNoLimit)
	if err != nil {
		panic(err)
	}

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

// MustPrefillRedisPool pre-fills the pool with count connections.
func MustPrefillRedisPool(p *redis.Pool, count int) {
	conns := []redis.Conn{}

	for i := 0; i < count; i++ {
		conns = append(conns, p.Get())
	}

	for i := range conns {
		conns[i].Close()
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

// JoinServerPool is a join-server pool for testing.
type JoinServerPool struct {
	Client     jsclient.Client
	GetJoinEUI lorawan.EUI64
}

// NewJoinServerPool create a join-server pool for testing.
func NewJoinServerPool(client jsclient.Client) jsclient.Pool {
	return &JoinServerPool{
		Client: client,
	}
}

// Get method.
func (p *JoinServerPool) Get(joinEUI lorawan.EUI64) (jsclient.Client, error) {
	p.GetJoinEUI = joinEUI
	return p.Client, nil
}

// JoinServerClient is a join-server client for testing.
type JoinServerClient struct {
	JoinReqPayloadChan chan backend.JoinReqPayload
	JoinReqError       error
	JoinAnsPayload     backend.JoinAnsPayload
}

// NewJoinServerClient creates a new join-server client.
func NewJoinServerClient() *JoinServerClient {
	return &JoinServerClient{
		JoinReqPayloadChan: make(chan backend.JoinReqPayload, 100),
	}
}

// JoinReq method.
func (c *JoinServerClient) JoinReq(pl backend.JoinReqPayload) (backend.JoinAnsPayload, error) {
	c.JoinReqPayloadChan <- pl
	return c.JoinAnsPayload, c.JoinReqError
}

// ApplicationServerPool is an application-server pool for testing.
type ApplicationServerPool struct {
	Client      as.ApplicationServerClient
	GetHostname string
}

// Get returns the Client.
func (p *ApplicationServerPool) Get(hostname string) (as.ApplicationServerClient, error) {
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
	HandleDataUpErr        error
	HandleProprietaryUpErr error
	HandleDownlinkACKErr   error

	HandleDataUpChan        chan as.HandleUplinkDataRequest
	HandleProprietaryUpChan chan as.HandleProprietaryUplinkRequest
	HandleErrorChan         chan as.HandleErrorRequest
	HandleDownlinkACKChan   chan as.HandleDownlinkACKRequest

	HandleDataUpResponse        as.HandleUplinkDataResponse
	HandleProprietaryUpResponse as.HandleProprietaryUplinkResponse
	HandleErrorResponse         as.HandleErrorResponse
	HandleDownlinkACKResponse   as.HandleDownlinkACKResponse
}

// NewApplicationClient returns a new ApplicationClient.
func NewApplicationClient() *ApplicationClient {
	return &ApplicationClient{
		HandleDataUpChan:        make(chan as.HandleUplinkDataRequest, 100),
		HandleProprietaryUpChan: make(chan as.HandleProprietaryUplinkRequest, 100),
		HandleErrorChan:         make(chan as.HandleErrorRequest, 100),
		HandleDownlinkACKChan:   make(chan as.HandleDownlinkACKRequest, 100),
	}
}

// HandleUplinkData method.
func (t *ApplicationClient) HandleUplinkData(ctx context.Context, in *as.HandleUplinkDataRequest, opts ...grpc.CallOption) (*as.HandleUplinkDataResponse, error) {
	if t.HandleDataUpErr != nil {
		return nil, t.HandleDataUpErr
	}
	t.HandleDataUpChan <- *in
	return &t.HandleDataUpResponse, nil
}

// HandleProprietaryUplink method.
func (t *ApplicationClient) HandleProprietaryUplink(ctx context.Context, in *as.HandleProprietaryUplinkRequest, opts ...grpc.CallOption) (*as.HandleProprietaryUplinkResponse, error) {
	if t.HandleProprietaryUpErr != nil {
		return nil, t.HandleProprietaryUpErr
	}
	t.HandleProprietaryUpChan <- *in
	return &t.HandleProprietaryUpResponse, nil
}

// HandleError method.
func (t *ApplicationClient) HandleError(ctx context.Context, in *as.HandleErrorRequest, opts ...grpc.CallOption) (*as.HandleErrorResponse, error) {
	t.HandleErrorChan <- *in
	return &t.HandleErrorResponse, nil
}

// HandleDownlinkACK method.
func (t *ApplicationClient) HandleDownlinkACK(ctx context.Context, in *as.HandleDownlinkACKRequest, opts ...grpc.CallOption) (*as.HandleDownlinkACKResponse, error) {
	t.HandleDownlinkACKChan <- *in
	return &t.HandleDownlinkACKResponse, nil
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
