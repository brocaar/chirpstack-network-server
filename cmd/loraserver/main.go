//go:generate go-bindata -prefix ../../migrations/ -pkg migrations -o ../../internal/migrations/migrations_gen.go ../../migrations/

package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/codegangsta/cli"
	"github.com/pkg/errors"
	migrate "github.com/rubenv/sql-migrate"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/api/nc"
	"github.com/brocaar/loraserver/api/ns"
	"github.com/brocaar/loraserver/internal/api"
	"github.com/brocaar/loraserver/internal/api/auth"
	"github.com/brocaar/loraserver/internal/api/client/asclient"
	"github.com/brocaar/loraserver/internal/api/client/jsclient"
	"github.com/brocaar/loraserver/internal/backend/controller"
	gwBackend "github.com/brocaar/loraserver/internal/backend/gateway"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/downlink"
	"github.com/brocaar/loraserver/internal/migrations"
	// TODO: merge backend/gateway into internal/gateway?
	"github.com/brocaar/loraserver/internal/gateway"
	"github.com/brocaar/loraserver/internal/uplink"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
)

func init() {
	grpclog.SetLogger(log.StandardLogger())
}

var version string // set by the compiler
var bands = []string{
	string(band.AS_923),
	string(band.AU_915_928),
	string(band.CN_470_510),
	string(band.CN_779_787),
	string(band.EU_433),
	string(band.EU_863_870),
	string(band.IN_865_867),
	string(band.KR_920_923),
	string(band.US_902_928),
}

func run(c *cli.Context) error {
	var server = new(uplink.Server)
	var gwStats = new(gateway.StatsHandler)

	tasks := []func(*cli.Context) error{
		setLogLevel,
		setNetID,
		setBandConfig,
		setRXParameters,
		setDeduplicationDelay,
		setGetDownlinkDataDelay,
		setCreateGatewayOnStats,
		setNodeSessionTTL,
		setLogNodeFrames,
		setGatewayServerJWTSecret,
		setStatsAggregationIntervals,
		setTimezone,
		printStartMessage,
		enableUplinkChannels,
		setInstallationMargin,
		setRedisPool,
		setPostgreSQLConnection,
		setGatewayBackend,
		setApplicationServer,
		setJoinServer,
		setNetworkController,
		runDatabaseMigrations,
		startAPIServer,
		startGatewayAPIServer,
		startLoRaServer(server),
		startStatsServer(gwStats),
		startQueueScheduler,
	}

	for _, t := range tasks {
		if err := t(c); err != nil {
			log.Fatal(err)
		}
	}

	sigChan := make(chan os.Signal)
	exitChan := make(chan struct{})
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	log.WithField("signal", <-sigChan).Info("signal received")
	go func() {
		log.Warning("stopping loraserver")
		if err := server.Stop(); err != nil {
			log.Fatal(err)
		}
		if err := gwStats.Stop(); err != nil {
			log.Fatal(err)
		}
		exitChan <- struct{}{}
	}()
	select {
	case <-exitChan:
	case s := <-sigChan:
		log.WithField("signal", s).Info("signal received, stopping immediately")
	}

	return nil
}

func setNetID(c *cli.Context) error {
	var netID lorawan.NetID
	if err := netID.UnmarshalText([]byte(c.String("net-id"))); err != nil {
		return errors.Wrap(err, "NetID parse error")
	}
	common.NetID = netID
	return nil
}

func setBandConfig(c *cli.Context) error {
	if c.String("band") == "" {
		return fmt.Errorf("--band is undefined, valid options are: %s", strings.Join(bands, ", "))
	}
	dwellTime := lorawan.DwellTimeNoLimit
	if c.Bool("band-dwell-time-400ms") {
		dwellTime = lorawan.DwellTime400ms
	}
	bandConfig, err := band.GetConfig(band.Name(c.String("band")), c.Bool("band-repeater-compatible"), dwellTime)
	if err != nil {
		return errors.Wrap(err, "get band config error")
	}
	for _, f := range c.IntSlice("extra-frequencies") {
		if err := bandConfig.AddChannel(f); err != nil {
			return errors.Wrap(err, "add channel error")
		}
	}

	common.Band = bandConfig
	common.BandName = band.Name(c.String("band"))

	return nil
}

func setRXParameters(c *cli.Context) error {
	common.RX1Delay = c.Int("rx1-delay")
	common.RX1DROffset = c.Int("rx1-dr-offset")
	if rx2DR := c.Int("rx2-dr"); rx2DR != -1 {
		common.RX2DR = rx2DR
	} else {
		common.RX2DR = common.Band.RX2DataRate
	}
	return nil
}

func setDeduplicationDelay(c *cli.Context) error {
	common.DeduplicationDelay = c.Duration("deduplication-delay")
	return nil
}

func setGetDownlinkDataDelay(c *cli.Context) error {
	common.GetDownlinkDataDelay = c.Duration("get-downlink-data-delay")
	return nil
}

func setCreateGatewayOnStats(c *cli.Context) error {
	common.CreateGatewayOnStats = c.Bool("gw-create-on-stats")
	return nil
}

func setNodeSessionTTL(c *cli.Context) error {
	common.NodeSessionTTL = c.Duration("node-session-ttl")
	return nil
}

func setLogNodeFrames(c *cli.Context) error {
	common.LogNodeFrames = c.Bool("log-node-frames")
	return nil
}

func setGatewayServerJWTSecret(c *cli.Context) error {
	common.GatewayServerJWTSecret = c.String("gw-server-jwt-secret")
	return nil
}

func setStatsAggregationIntervals(c *cli.Context) error {
	// get the gw stats aggregation intervals
	gateway.MustSetStatsAggregationIntervals(strings.Split(c.String("gw-stats-aggregation-intervals"), ","))
	return nil
}

func setTimezone(c *cli.Context) error {
	// get the timezone
	if c.String("timezone") != "" {
		l, err := time.LoadLocation(c.String("timezone"))
		if err != nil {
			return errors.Wrap(err, "load timezone location error")
		}
		common.TimeLocation = l
	}
	return nil
}

func setLogLevel(c *cli.Context) error {
	log.SetLevel(log.Level(uint8(c.Int("log-level"))))
	return nil
}

func printStartMessage(c *cli.Context) error {
	log.WithFields(log.Fields{
		"version": version,
		"net_id":  common.NetID.String(),
		"band":    c.String("band"),
		"docs":    "https://docs.loraserver.io/",
	}).Info("starting LoRa Server")
	return nil
}

func setInstallationMargin(c *cli.Context) error {
	common.InstallationMargin = c.Float64("installation-margin")
	return nil
}

func enableUplinkChannels(c *cli.Context) error {
	if c.String("enable-uplink-channels") == "" {
		return nil
	}

	log.Info("disabling all channels")
	for _, c := range common.Band.GetEnabledUplinkChannels() {
		if err := common.Band.DisableUplinkChannel(c); err != nil {
			return errors.Wrap(err, "disable uplink channel error")
		}
	}

	blocks := strings.Split(c.String("enable-uplink-channels"), ",")
	for _, block := range blocks {
		block = strings.Trim(block, " ")
		var start, end int
		if _, err := fmt.Sscanf(block, "%d-%d", &start, &end); err != nil {
			if _, err := fmt.Sscanf(block, "%d", &start); err != nil {
				return errors.Wrap(err, "parse channel range error")
			}
			end = start
		}

		log.WithFields(log.Fields{
			"first_channel": start,
			"last_channel":  end,
		}).Info("enabling channel block")

		for ; start <= end; start++ {
			if err := common.Band.EnableUplinkChannel(start); err != nil {
				errors.Wrap(err, "enable uplink channel error")
			}
		}
	}
	return nil
}

func setRedisPool(c *cli.Context) error {
	log.WithField("url", c.String("redis-url")).Info("setup redis connection pool")
	common.RedisPool = common.NewRedisPool(c.String("redis-url"))
	return nil
}

func setPostgreSQLConnection(c *cli.Context) error {
	log.Info("connecting to postgresql")
	db, err := common.OpenDatabase(c.String("postgres-dsn"))
	if err != nil {
		return errors.Wrap(err, "database connection error")
	}
	common.DB = db
	return nil
}

func setGatewayBackend(c *cli.Context) error {
	gw, err := gwBackend.NewBackend(c.String("gw-mqtt-server"), c.String("gw-mqtt-username"), c.String("gw-mqtt-password"), c.String("gw-mqtt-ca-cert"))
	if err != nil {
		return errors.Wrap(err, "gateway-backend setup failed")
	}
	common.Gateway = gw
	return nil
}

func setApplicationServer(c *cli.Context) error {
	common.ApplicationServerPool = asclient.NewPool()
	return nil
}

func setJoinServer(c *cli.Context) error {
	jsClient, err := jsclient.NewClient(
		c.String("js-server"),
		c.String("js-ca-cert"),
		c.String("js-tls-cert"),
		c.String("js-tls-key"),
	)
	if err != nil {
		return errors.Wrap(err, "create new join-server client error")
	}
	common.JoinServerPool = jsclient.NewPool(jsClient)

	return nil
}

func setNetworkController(c *cli.Context) error {
	var ncClient nc.NetworkControllerClient
	if c.String("nc-server") != "" {
		// setup network-controller client
		log.WithFields(log.Fields{
			"server":   c.String("nc-server"),
			"ca-cert":  c.String("nc-ca-cert"),
			"tls-cert": c.String("nc-tls-cert"),
			"tls-key":  c.String("nc-tls-key"),
		}).Info("connecting to network-controller")
		var ncDialOptions []grpc.DialOption
		if c.String("nc-tls-cert") != "" && c.String("nc-tls-key") != "" {
			ncDialOptions = append(ncDialOptions, grpc.WithTransportCredentials(
				mustGetTransportCredentials(c.String("nc-tls-cert"), c.String("nc-tls-key"), c.String("nc-ca-cert"), false),
			))
		} else {
			ncDialOptions = append(ncDialOptions, grpc.WithInsecure())
		}
		ncConn, err := grpc.Dial(c.String("nc-server"), ncDialOptions...)
		if err != nil {
			return errors.Wrap(err, "network-controller dial error")
		}
		ncClient = nc.NewNetworkControllerClient(ncConn)
	} else {
		log.Info("no network-controller configured")
		ncClient = &controller.NopNetworkControllerClient{}
	}
	common.Controller = ncClient
	return nil
}

func runDatabaseMigrations(c *cli.Context) error {
	if c.Bool("db-automigrate") {
		log.Info("applying database migrations")
		m := &migrate.AssetMigrationSource{
			Asset:    migrations.Asset,
			AssetDir: migrations.AssetDir,
			Dir:      "",
		}
		n, err := migrate.Exec(common.DB.DB, "postgres", m, migrate.Up)
		if err != nil {
			return errors.Wrap(err, "applying migrations failed")
		}
		log.WithField("count", n).Info("migrations applied")
	}
	return nil
}

func startAPIServer(c *cli.Context) error {
	log.WithFields(log.Fields{
		"bind":     c.String("bind"),
		"ca-cert":  c.String("ca-cert"),
		"tls-cert": c.String("tls-cert"),
		"tls-key":  c.String("tls-key"),
	}).Info("starting api server")

	var opts []grpc.ServerOption
	if c.String("tls-cert") != "" && c.String("tls-key") != "" {
		creds := mustGetTransportCredentials(c.String("tls-cert"), c.String("tls-key"), c.String("ca-cert"), false)
		opts = append(opts, grpc.Creds(creds))
	}
	gs := grpc.NewServer(opts...)
	nsAPI := api.NewNetworkServerAPI()
	ns.RegisterNetworkServerServer(gs, nsAPI)

	ln, err := net.Listen("tcp", c.String("bind"))
	if err != nil {
		return errors.Wrap(err, "start api listener error")
	}
	go gs.Serve(ln)
	return nil
}

func startGatewayAPIServer(c *cli.Context) error {
	log.WithFields(log.Fields{
		"bind":     c.String("gw-server-bind"),
		"ca-cert":  c.String("gw-server-ca-cert"),
		"tls-cert": c.String("gw-server-tls-cert"),
		"tls-key":  c.String("gw-server-tls-key"),
	}).Info("starting gateway api server")

	var validator auth.Validator
	if c.String("gw-server-jwt-secret") != "" {
		validator = auth.NewJWTValidator("HS256", c.String("gw-server-jwt-secret"))
	} else {
		return errors.New("--gw-server-jwt-secret must be set")
	}

	var opts []grpc.ServerOption
	if c.String("gw-server-tls-cert") != "" && c.String("gw-server-tls-key") != "" {
		creds := mustGetTransportCredentials(c.String("gw-server-tls-cert"), c.String("gw-server-tls-key"), c.String("gw-server-ca-cert"), false)
		opts = append(opts, grpc.Creds(creds))
	}
	gs := grpc.NewServer(opts...)
	gwAPI := api.NewGatewayAPI(validator)
	gw.RegisterGatewayServer(gs, gwAPI)

	gwServerLn, err := net.Listen("tcp", c.String("gw-server-bind"))
	if err != nil {
		return errors.Wrap(err, "start gateway api server listener error")
	}
	go gs.Serve(gwServerLn)
	return nil
}

func startLoRaServer(server *uplink.Server) func(*cli.Context) error {
	return func(c *cli.Context) error {
		*server = *uplink.NewServer()
		return server.Start()
	}
}

func startStatsServer(gwStats *gateway.StatsHandler) func(*cli.Context) error {
	return func(c *cli.Context) error {
		*gwStats = *gateway.NewStatsHandler()
		if err := gwStats.Start(); err != nil {
			log.Fatal(err)
		}
		return nil
	}
}

func startQueueScheduler(c *cli.Context) error {
	log.Info("starting downlink device-queue scheduler")
	go downlink.ClassCSchedulerLoop()
	return nil
}

func mustGetTransportCredentials(tlsCert, tlsKey, caCert string, verifyClientCert bool) credentials.TransportCredentials {
	var caCertPool *x509.CertPool
	cert, err := tls.LoadX509KeyPair(tlsCert, tlsKey)
	if err != nil {
		log.WithFields(log.Fields{
			"cert": tlsCert,
			"key":  tlsKey,
		}).Fatalf("load key-pair error: %s", err)
	}

	if caCert != "" {
		rawCaCert, err := ioutil.ReadFile(caCert)
		if err != nil {
			log.WithField("ca", caCert).Fatalf("load ca cert error: %s", err)
		}

		caCertPool = x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(rawCaCert)
	}

	if verifyClientCert {
		return credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{cert},
			RootCAs:      caCertPool,
			ClientAuth:   tls.RequireAndVerifyClientCert,
		})
	}

	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
	})
}

func main() {
	app := cli.NewApp()
	app.Name = "loraserver"
	app.Usage = "network-server for LoRaWAN networks"
	app.Version = version
	app.Copyright = "See http://github.com/brocaar/loraserver for copyright information"
	app.Action = run
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "net-id",
			Usage:  "network identifier (NetID, 3 bytes) encoded as HEX (e.g. 010203)",
			EnvVar: "NET_ID",
		},
		cli.StringFlag{
			Name:   "band",
			Usage:  fmt.Sprintf("ism band configuration to use (options: %s)", strings.Join(bands, ", ")),
			EnvVar: "BAND",
		},
		cli.BoolFlag{
			Name:   "band-dwell-time-400ms",
			Usage:  "band configuration takes 400ms dwell-time into account",
			EnvVar: "BAND_DWELL_TIME_400ms",
		},
		cli.BoolFlag{
			Name:   "band-repeater-compatible",
			Usage:  "band configuration takes repeater encapsulation layer into account",
			EnvVar: "BAND_REPEATER_COMPATIBLE",
		},
		// TODO refactor to NS_SERVER_CA_CERT?
		cli.StringFlag{
			Name:   "ca-cert",
			Usage:  "ca certificate used by the api server (optional)",
			EnvVar: "CA_CERT",
		},
		// TODO refactor to NS_SERVER_TLS_CERT?
		cli.StringFlag{
			Name:   "tls-cert",
			Usage:  "tls certificate used by the api server (optional)",
			EnvVar: "TLS_CERT",
		},
		// TODO refactor to NS_SERVER_TLS_KEY?
		cli.StringFlag{
			Name:   "tls-key",
			Usage:  "tls key used by the api server (optional)",
			EnvVar: "TLS_KEY",
		},
		// TODO refactor to NS_SERVER_BIND?
		cli.StringFlag{
			Name:   "bind",
			Usage:  "ip:port to bind the api server",
			Value:  "0.0.0.0:8000",
			EnvVar: "BIND",
		},
		cli.StringFlag{
			Name:   "gw-server-ca-cert",
			Usage:  "ca certificate used by the gateway api server (optional)",
			EnvVar: "GW_SERVER_CA_CERT",
		},
		cli.StringFlag{
			Name:   "gw-server-tls-cert",
			Usage:  "tls certificate used by the gateway api server (optional)",
			EnvVar: "GW_SERVER_TLS_CERT",
		},
		cli.StringFlag{
			Name:   "gw-server-tls-key",
			Usage:  "tls key used by the gateway api server (optional)",
			EnvVar: "GW_SERVER_TLS_KEY",
		},
		cli.StringFlag{
			Name:   "gw-server-jwt-secret",
			Usage:  "JWT secret used by the gateway api server for gateway authentication / authorization",
			EnvVar: "GW_SERVER_JWT_SECRET",
		},
		cli.StringFlag{
			Name:   "gw-server-bind",
			Usage:  "ip:port to bind the gateway api server",
			Value:  "0.0.0.0:8002",
			EnvVar: "GW_SERVER_BIND",
		},
		cli.StringFlag{
			Name:   "redis-url",
			Usage:  "redis url (e.g. redis://user:password@hostname:port/0)",
			Value:  "redis://localhost:6379",
			EnvVar: "REDIS_URL",
		},
		cli.StringFlag{
			Name:   "postgres-dsn",
			Usage:  "postgresql dsn (e.g.: postgres://user:password@hostname/database?sslmode=disable)",
			Value:  "postgres://localhost/loraserver_ns?sslmode=disable",
			EnvVar: "POSTGRES_DSN",
		},
		cli.BoolFlag{
			Name:   "db-automigrate",
			Usage:  "automatically apply database migrations",
			EnvVar: "DB_AUTOMIGRATE",
		},
		cli.StringFlag{
			Name:   "gw-mqtt-server",
			Usage:  "mqtt broker server used by the gateway backend (e.g. scheme://host:port where scheme is tcp, ssl or ws)",
			Value:  "tcp://localhost:1883",
			EnvVar: "GW_MQTT_SERVER",
		},
		cli.StringFlag{
			Name:   "gw-mqtt-username",
			Usage:  "mqtt username used by the gateway backend (optional)",
			EnvVar: "GW_MQTT_USERNAME",
		},
		cli.StringFlag{
			Name:   "gw-mqtt-password",
			Usage:  "mqtt password used by the gateway backend (optional)",
			EnvVar: "GW_MQTT_PASSWORD",
		},
		cli.StringFlag{
			Name:   "gw-mqtt-ca-cert",
			Usage:  "mqtt CA certificate file used by the gateway backend (optional)",
			EnvVar: "GW_MQTT_CA_CERT",
		},
		cli.StringFlag{
			Name:   "nc-server",
			Usage:  "hostname:port of the network-controller api server (optional)",
			EnvVar: "NC_SERVER",
		},
		cli.StringFlag{
			Name:   "nc-ca-cert",
			Usage:  "ca certificate used by the network-controller client (optional)",
			EnvVar: "NC_CA_CERT",
		},
		cli.StringFlag{
			Name:   "nc-tls-cert",
			Usage:  "tls certificate used by the network-controller client (optional)",
			EnvVar: "NC_TLS_CERT",
		},
		cli.StringFlag{
			Name:   "nc-tls-key",
			Usage:  "tls key used by the network-controller client (optional)",
			EnvVar: "NC_TLS_KEY",
		},
		cli.DurationFlag{
			Name:   "deduplication-delay",
			Usage:  "time to wait for uplink de-duplication",
			EnvVar: "DEDUPLICATION_DELAY",
			Value:  200 * time.Millisecond,
		},
		cli.DurationFlag{
			Name:   "get-downlink-data-delay",
			Usage:  "delay between uplink delivery to the app server and getting the downlink data from the app server (if any)",
			EnvVar: "GET_DOWNLINK_DATA_DELAY",
			Value:  100 * time.Millisecond,
		},
		cli.StringFlag{
			Name:   "gw-stats-aggregation-intervals",
			Usage:  "aggregation intervals to use for aggregating the gateway stats (valid options: second, minute, hour, day, week, month, quarter, year)",
			EnvVar: "GW_STATS_AGGREGATION_INTERVALS",
			Value:  "minute,hour,day",
		},
		cli.StringFlag{
			Name:   "timezone",
			Usage:  "timezone to use when aggregating data (e.g. 'Europe/Amsterdam') (optional, by default the db timezone is used)",
			EnvVar: "TIMEZONE",
		},
		cli.BoolFlag{
			Name:   "gw-create-on-stats",
			Usage:  "create non-existing gateways on receiving of stats",
			EnvVar: "GW_CREATE_ON_STATS",
		},
		cli.IntSliceFlag{
			Name:   "extra-frequencies",
			Usage:  "extra frequencies to use for ISM bands that implement the CFList",
			EnvVar: "EXTRA_FREQUENCIES",
		},
		cli.StringFlag{
			Name:   "enable-uplink-channels",
			Usage:  "enable only a given sub-set of channels (e.g. '0-7,8-15')",
			EnvVar: "ENABLE_UPLINK_CHANNELS",
		},
		cli.DurationFlag{
			Name:   "node-session-ttl",
			Usage:  "the ttl after which a node-session expires after no activity",
			EnvVar: "NODE_SESSION_TTL",
			Value:  time.Hour * 24 * 31,
		},
		cli.BoolFlag{
			Name:   "log-node-frames",
			Usage:  "log uplink and downlink frames to the database",
			EnvVar: "LOG_NODE_FRAMES",
		},
		cli.IntFlag{
			Name:   "log-level",
			Value:  4,
			Usage:  "debug=5, info=4, warning=3, error=2, fatal=1, panic=0",
			EnvVar: "LOG_LEVEL",
		},
		cli.StringFlag{
			Name:   "js-server",
			Usage:  "hostname:port of the default join-server",
			EnvVar: "JS_SERVER",
			Value:  "http://localhost:8003",
		},
		cli.StringFlag{
			Name:   "js-ca-cert",
			Usage:  "ca certificate used by the default join-server client (optional)",
			EnvVar: "JS_CA_CERT",
		},
		cli.StringFlag{
			Name:   "js-tls-cert",
			Usage:  "tls certificate used by the default join-server client (optional)",
			EnvVar: "JS_TLS_CERT",
		},
		cli.StringFlag{
			Name:   "js-tls-key",
			Usage:  "tls key used by the default join-server client (optional)",
			EnvVar: "JS_TLS_KEY",
		},
		cli.Float64Flag{
			Name:   "installation-margin",
			Usage:  "installation margin (dB) used by the ADR engine",
			Value:  10,
			EnvVar: "INSTALLATION_MARGIN",
		},
		cli.IntFlag{
			Name:   "rx1-delay",
			Usage:  "class a rx1 delay",
			Value:  1,
			EnvVar: "RX1_DELAY",
		},
		cli.IntFlag{
			Name:   "rx1-dr-offset",
			Usage:  "rx1 data-rate offset (valid options documented in the LoRaWAN Regional Parameters specification)",
			Value:  0,
			EnvVar: "RX1_DR_OFFSET",
		},
		cli.IntFlag{
			Name:   "rx2-dr",
			Usage:  "rx2 data-rate (when set to -1, the default rx2 data-rate will be used)",
			Value:  -1,
			EnvVar: "RX2_DR",
		},
	}
	app.Run(os.Args)
}
