package test

import (
	"os"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/gomodule/redigo/redis"
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
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/migrations"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
	"github.com/brocaar/lorawan/band"
)

func init() {
	log.SetLevel(log.ErrorLevel)

	config.C.NetworkServer.DeviceSessionTTL = time.Hour
	config.C.NetworkServer.Band.Name = band.EU_863_870
	config.C.NetworkServer.Band.Band, _ = band.GetConfig(config.C.NetworkServer.Band.Name, false, lorawan.DwellTimeNoLimit)

	config.C.NetworkServer.DeduplicationDelay = 5 * time.Millisecond
	config.C.NetworkServer.GetDownlinkDataDelay = 5 * time.Millisecond
	config.C.NetworkServer.NetworkSettings.DownlinkTXPower = -1

	config.C.NetworkServer.Gateway.Stats.Timezone = "Europe/Amsterdam"
	loc, err := time.LoadLocation(config.C.NetworkServer.Gateway.Stats.Timezone)
	if err != nil {
		panic(err)
	}
	config.C.NetworkServer.Gateway.Stats.TimezoneLocation = loc
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

	config.C.NetworkServer.Band.Band, err = band.GetConfig(band.EU_863_870, false, lorawan.DwellTimeNoLimit)
	if err != nil {
		panic(err)
	}

	config.C.NetworkServer.NetworkSettings.RX2Frequency = config.C.NetworkServer.Band.Band.GetDefaults().RX2Frequency
	config.C.NetworkServer.NetworkSettings.RX2DR = config.C.NetworkServer.Band.Band.GetDefaults().RX2DataRate
	config.C.NetworkServer.NetworkSettings.RX1Delay = 0

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
func MustResetDB(db *common.DBLogger) {
	m := &migrate.AssetMigrationSource{
		Asset:    migrations.Asset,
		AssetDir: migrations.AssetDir,
		Dir:      "",
	}
	if _, err := migrate.Exec(db.DB.DB, "postgres", m, migrate.Down); err != nil {
		log.Fatal(err)
	}
	if _, err := migrate.Exec(db.DB.DB, "postgres", m, migrate.Up); err != nil {
		log.Fatal(err)
	}
}

// GatewayBackend is a test gateway backend.
type GatewayBackend struct {
	rxPacketChan            chan gw.UplinkFrame
	TXPacketChan            chan gw.DownlinkFrame
	GatewayConfigPacketChan chan gw.GatewayConfiguration
	statsPacketChan         chan gw.GatewayStats
}

// NewGatewayBackend returns a new GatewayBackend.
func NewGatewayBackend() *GatewayBackend {
	return &GatewayBackend{
		rxPacketChan:            make(chan gw.UplinkFrame, 100),
		TXPacketChan:            make(chan gw.DownlinkFrame, 100),
		GatewayConfigPacketChan: make(chan gw.GatewayConfiguration, 100),
	}
}

// SendTXPacket method.
func (b *GatewayBackend) SendTXPacket(txPacket gw.DownlinkFrame) error {
	b.TXPacketChan <- txPacket
	return nil
}

// SendGatewayConfigPacket method.
func (b *GatewayBackend) SendGatewayConfigPacket(config gw.GatewayConfiguration) error {
	b.GatewayConfigPacketChan <- config
	return nil
}

// RXPacketChan method.
func (b *GatewayBackend) RXPacketChan() chan gw.UplinkFrame {
	return b.rxPacketChan
}

// StatsPacketChan method.
func (b *GatewayBackend) StatsPacketChan() chan gw.GatewayStats {
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
	JoinReqPayloadChan   chan backend.JoinReqPayload
	RejoinReqPayloadChan chan backend.RejoinReqPayload
	JoinReqError         error
	RejoinReqError       error
	JoinAnsPayload       backend.JoinAnsPayload
	RejoinAnsPayload     backend.RejoinAnsPayload
}

// NewJoinServerClient creates a new join-server client.
func NewJoinServerClient() *JoinServerClient {
	return &JoinServerClient{
		JoinReqPayloadChan:   make(chan backend.JoinReqPayload, 100),
		RejoinReqPayloadChan: make(chan backend.RejoinReqPayload, 100),
	}
}

// JoinReq method.
func (c *JoinServerClient) JoinReq(pl backend.JoinReqPayload) (backend.JoinAnsPayload, error) {
	c.JoinReqPayloadChan <- pl
	return c.JoinAnsPayload, c.JoinReqError
}

// RejoinReq method.
func (c *JoinServerClient) RejoinReq(pl backend.RejoinReqPayload) (backend.RejoinAnsPayload, error) {
	c.RejoinReqPayloadChan <- pl
	return c.RejoinAnsPayload, c.RejoinReqError
}

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
	HandleDataUpErr        error
	HandleProprietaryUpErr error
	HandleDownlinkACKErr   error
	SetDeviceStatusError   error

	HandleDataUpChan        chan as.HandleUplinkDataRequest
	HandleProprietaryUpChan chan as.HandleProprietaryUplinkRequest
	HandleErrorChan         chan as.HandleErrorRequest
	HandleDownlinkACKChan   chan as.HandleDownlinkACKRequest
	SetDeviceStatusChan     chan as.SetDeviceStatusRequest

	HandleDataUpResponse        empty.Empty
	HandleProprietaryUpResponse empty.Empty
	HandleErrorResponse         empty.Empty
	HandleDownlinkACKResponse   empty.Empty
	SetDeviceStatusResponse     empty.Empty
}

// NewApplicationClient returns a new ApplicationClient.
func NewApplicationClient() *ApplicationClient {
	return &ApplicationClient{
		HandleDataUpChan:        make(chan as.HandleUplinkDataRequest, 100),
		HandleProprietaryUpChan: make(chan as.HandleProprietaryUplinkRequest, 100),
		HandleErrorChan:         make(chan as.HandleErrorRequest, 100),
		HandleDownlinkACKChan:   make(chan as.HandleDownlinkACKRequest, 100),
		SetDeviceStatusChan:     make(chan as.SetDeviceStatusRequest, 100),
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

// SetDeviceStatus method.
func (t *ApplicationClient) SetDeviceStatus(ctx context.Context, in *as.SetDeviceStatusRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	t.SetDeviceStatusChan <- *in
	return &t.SetDeviceStatusResponse, t.SetDeviceStatusError
}

// NetworkControllerClient is a network-controller client for testing.
type NetworkControllerClient struct {
	HandleRXInfoChan           chan nc.HandleUplinkMetaDataRequest
	HandleDataUpMACCommandChan chan nc.HandleUplinkMACCommandRequest

	HandleRXInfoResponse           empty.Empty
	HandleDataUpMACCommandResponse empty.Empty
}

// NewNetworkControllerClient returns a new NetworkControllerClient.
func NewNetworkControllerClient() *NetworkControllerClient {
	return &NetworkControllerClient{
		HandleRXInfoChan:           make(chan nc.HandleUplinkMetaDataRequest, 100),
		HandleDataUpMACCommandChan: make(chan nc.HandleUplinkMACCommandRequest, 100),
	}
}

// HandleUplinkMetaData method.
func (t *NetworkControllerClient) HandleUplinkMetaData(ctx context.Context, in *nc.HandleUplinkMetaDataRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	t.HandleRXInfoChan <- *in
	return &empty.Empty{}, nil
}

// HandleUplinkMACCommand method.
func (t *NetworkControllerClient) HandleUplinkMACCommand(ctx context.Context, in *nc.HandleUplinkMACCommandRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	t.HandleDataUpMACCommandChan <- *in
	return &empty.Empty{}, nil
}

// DatabaseTestSuiteBase provides the setup and teardown of the database
// for every test-run.
type DatabaseTestSuiteBase struct {
	db *common.DBLogger
	tx *common.TxLogger
	p  *redis.Pool
}

// SetupSuite is called once before starting the test-suite.
func (b *DatabaseTestSuiteBase) SetupSuite() {
	conf := GetConfig()
	db, err := common.OpenDatabase(conf.PostgresDSN)
	if err != nil {
		panic(err)
	}
	b.db = db
	MustResetDB(db)

	b.p = common.NewRedisPool(conf.RedisURL)

	config.C.PostgreSQL.DB = db
	config.C.Redis.Pool = b.p
}

// SetupTest is called before every test.
func (b *DatabaseTestSuiteBase) SetupTest() {
	tx, err := b.db.Beginx()
	if err != nil {
		panic(err)
	}
	b.tx = tx

	MustFlushRedis(b.p)
}

// TearDownTest is called after every test.
func (b *DatabaseTestSuiteBase) TearDownTest() {
	if err := b.tx.Rollback(); err != nil {
		panic(err)
	}
}

// Tx returns a database transaction (which is rolled back after every
// test).
func (b *DatabaseTestSuiteBase) Tx() sqlx.Ext {
	return b.tx
}

// DB returns the database object.
func (b *DatabaseTestSuiteBase) DB() *common.DBLogger {
	return b.db
}

// RedisPool returns the redis.Pool object.
func (b *DatabaseTestSuiteBase) RedisPool() *redis.Pool {
	return b.p
}
