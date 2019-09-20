package test

import (
	"os"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/gomodule/redigo/redis"
	migrate "github.com/rubenv/sql-migrate"
	log "github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/api/geo"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/api/nc"
	"github.com/brocaar/loraserver/internal/api/client/asclient"
	"github.com/brocaar/loraserver/internal/band"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/migrations"
	"github.com/brocaar/lorawan"
	loraband "github.com/brocaar/lorawan/band"
)

func init() {
	log.SetLevel(log.ErrorLevel)

}

// Config contains the test configuration.
type Config struct {
	RedisURL     string
	PostgresDSN  string
	MQTTServer   string
	MQTTUsername string
	MQTTPassword string
}

// GetConfig returns the test configuration.
func GetConfig() config.Config {
	log.SetLevel(log.FatalLevel)

	var c config.Config
	c.NetworkServer.Band.Name = loraband.EU_863_870

	if err := band.Setup(c); err != nil {
		panic(err)
	}

	c.Redis.URL = "redis://localhost:6379/1"
	c.PostgreSQL.DSN = "postgres://localhost/loraserver_ns_test?sslmode=disable"

	c.NetworkServer.NetID = lorawan.NetID{3, 2, 1}
	c.NetworkServer.DeviceSessionTTL = time.Hour
	c.NetworkServer.DeduplicationDelay = 5 * time.Millisecond
	c.NetworkServer.GetDownlinkDataDelay = 5 * time.Millisecond

	c.NetworkServer.NetworkSettings.RX2Frequency = band.Band().GetDefaults().RX2Frequency
	c.NetworkServer.NetworkSettings.RX2DR = band.Band().GetDefaults().RX2DataRate
	c.NetworkServer.NetworkSettings.RX1Delay = 0
	c.NetworkServer.NetworkSettings.DownlinkTXPower = -1

	c.NetworkServer.Scheduler.SchedulerInterval = time.Second

	c.NetworkServer.Gateway.Backend.MQTT.Server = "tcp://127.0.0.1:1883"
	c.NetworkServer.Gateway.Backend.MQTT.CleanSession = true
	c.NetworkServer.Gateway.Backend.MQTT.EventTopic = "gateway/+/event/+"
	c.NetworkServer.Gateway.Backend.MQTT.CommandTopicTemplate = "gateway/{{ .GatewayID }}/command/{{ .CommandType }}"

	if v := os.Getenv("TEST_REDIS_URL"); v != "" {
		c.Redis.URL = v
	}
	if v := os.Getenv("TEST_POSTGRES_DSN"); v != "" {
		c.PostgreSQL.DSN = v
	}
	if v := os.Getenv("TEST_MQTT_SERVER"); v != "" {
		c.NetworkServer.Gateway.Backend.MQTT.Server = v
	}
	if v := os.Getenv("TEST_MQTT_USERNAME"); v != "" {
		c.NetworkServer.Gateway.Backend.MQTT.Username = v
	}
	if v := os.Getenv("TEST_MQTT_PASSWORD"); v != "" {
		c.NetworkServer.Gateway.Backend.MQTT.Password = v
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
	rxPacketChan            chan gw.UplinkFrame
	TXPacketChan            chan gw.DownlinkFrame
	GatewayConfigPacketChan chan gw.GatewayConfiguration
	statsPacketChan         chan gw.GatewayStats
	downlinkTXAckChan       chan gw.DownlinkTXAck
}

// NewGatewayBackend returns a new GatewayBackend.
func NewGatewayBackend() *GatewayBackend {
	return &GatewayBackend{
		rxPacketChan:            make(chan gw.UplinkFrame, 100),
		TXPacketChan:            make(chan gw.DownlinkFrame, 100),
		GatewayConfigPacketChan: make(chan gw.GatewayConfiguration, 100),
		downlinkTXAckChan:       make(chan gw.DownlinkTXAck, 100),
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

// DownlinkTXAckChan method.
func (b *GatewayBackend) DownlinkTXAckChan() chan gw.DownlinkTXAck {
	return b.downlinkTXAckChan
}

// Close method.
func (b *GatewayBackend) Close() error {
	if b.rxPacketChan != nil {
		close(b.rxPacketChan)
	}
	return nil
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
	HandleDataUpErr         error
	HandleProprietaryUpErr  error
	HandleDownlinkACKErr    error
	SetDeviceStatusError    error
	SetDeviceLocationErrror error

	HandleDataUpChan        chan as.HandleUplinkDataRequest
	HandleProprietaryUpChan chan as.HandleProprietaryUplinkRequest
	HandleErrorChan         chan as.HandleErrorRequest
	HandleDownlinkACKChan   chan as.HandleDownlinkACKRequest
	HandleGatewayStatsChan  chan as.HandleGatewayStatsRequest
	SetDeviceStatusChan     chan as.SetDeviceStatusRequest
	SetDeviceLocationChan   chan as.SetDeviceLocationRequest

	HandleDataUpResponse        empty.Empty
	HandleProprietaryUpResponse empty.Empty
	HandleErrorResponse         empty.Empty
	HandleDownlinkACKResponse   empty.Empty
	HandleGatewayStatsResponse  empty.Empty
	SetDeviceStatusResponse     empty.Empty
	SetDeviceLocationResponse   empty.Empty
}

// NewApplicationClient returns a new ApplicationClient.
func NewApplicationClient() *ApplicationClient {
	return &ApplicationClient{
		HandleDataUpChan:        make(chan as.HandleUplinkDataRequest, 100),
		HandleProprietaryUpChan: make(chan as.HandleProprietaryUplinkRequest, 100),
		HandleErrorChan:         make(chan as.HandleErrorRequest, 100),
		HandleDownlinkACKChan:   make(chan as.HandleDownlinkACKRequest, 100),
		HandleGatewayStatsChan:  make(chan as.HandleGatewayStatsRequest, 100),
		SetDeviceStatusChan:     make(chan as.SetDeviceStatusRequest, 100),
		SetDeviceLocationChan:   make(chan as.SetDeviceLocationRequest, 100),
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

// NetworkControllerClient is a network-controller client for testing.
type NetworkControllerClient struct {
	HandleUplinkMetaDataChan   chan nc.HandleUplinkMetaDataRequest
	HandleDownlinkMetaDataChan chan nc.HandleDownlinkMetaDataRequest
	HandleDataUpMACCommandChan chan nc.HandleUplinkMACCommandRequest

	HandleRXInfoResponse           empty.Empty
	HandleDownlinkMetaDataResponse empty.Empty
	HandleDataUpMACCommandResponse empty.Empty
}

// NewNetworkControllerClient returns a new NetworkControllerClient.
func NewNetworkControllerClient() *NetworkControllerClient {
	return &NetworkControllerClient{
		HandleUplinkMetaDataChan:   make(chan nc.HandleUplinkMetaDataRequest, 100),
		HandleDownlinkMetaDataChan: make(chan nc.HandleDownlinkMetaDataRequest, 100),
		HandleDataUpMACCommandChan: make(chan nc.HandleUplinkMACCommandRequest, 100),
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

// GeolocationClient is a geolocation client for testing.
type GeolocationClient struct {
	ResolveTDOAChan               chan geo.ResolveTDOARequest
	ResolveMultiFrameTDOAChan     chan geo.ResolveMultiFrameTDOARequest
	ResolveTDOAResponse           geo.ResolveTDOAResponse
	ResolveMultiFrameTDOAResponse geo.ResolveMultiFrameTDOAResponse
}

// NewGeolocationClient creates a new GeolocationClient.
func NewGeolocationClient() *GeolocationClient {
	return &GeolocationClient{
		ResolveTDOAChan:           make(chan geo.ResolveTDOARequest, 100),
		ResolveMultiFrameTDOAChan: make(chan geo.ResolveMultiFrameTDOARequest, 100),
	}
}

// ResolveTDOA method.
func (g *GeolocationClient) ResolveTDOA(ctx context.Context, in *geo.ResolveTDOARequest, opts ...grpc.CallOption) (*geo.ResolveTDOAResponse, error) {
	g.ResolveTDOAChan <- *in
	return &g.ResolveTDOAResponse, nil
}

// ResolveMultiFrameTDOA method.
func (g *GeolocationClient) ResolveMultiFrameTDOA(ctx context.Context, in *geo.ResolveMultiFrameTDOARequest, opts ...grpc.CallOption) (*geo.ResolveMultiFrameTDOAResponse, error) {
	g.ResolveMultiFrameTDOAChan <- *in
	return &g.ResolveMultiFrameTDOAResponse, nil
}
