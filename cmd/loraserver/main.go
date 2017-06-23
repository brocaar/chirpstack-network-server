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

	log "github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	migrate "github.com/rubenv/sql-migrate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"

	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/api/nc"
	"github.com/brocaar/loraserver/api/ns"
	"github.com/brocaar/loraserver/internal/api"
	"github.com/brocaar/loraserver/internal/backend/controller"
	"github.com/brocaar/loraserver/internal/backend/gateway"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/migrations"
	// TODO: merge backend/gateway into internal/gateway?
	gw "github.com/brocaar/loraserver/internal/gateway"
	"github.com/brocaar/loraserver/internal/migration"
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
	string(band.KR_920_923),
	string(band.RU_864_869),
	string(band.US_902_928),
}

func run(c *cli.Context) error {
	// parse the NetID
	var netID lorawan.NetID
	if err := netID.UnmarshalText([]byte(c.String("net-id"))); err != nil {
		log.Fatalf("NetID parse error: %s", err)
	}

	// get the band config
	if c.String("band") == "" {
		log.Fatalf("--band is undefined, valid options are: %s", strings.Join(bands, ", "))
	}
	dwellTime := lorawan.DwellTimeNoLimit
	if c.Bool("band-dwell-time-400ms") {
		dwellTime = lorawan.DwellTime400ms
	}
	bandConfig, err := band.GetConfig(band.Name(c.String("band")), c.Bool("band-repeater-compatible"), dwellTime)
	if err != nil {
		log.Fatal(err)
	}
	for _, f := range c.IntSlice("extra-frequencies") {
		if err := bandConfig.AddChannel(f); err != nil {
			log.Fatalf("add channel error: %s", err)
		}
	}

	// get the gw stats aggregation intervals
	gw.MustSetStatsAggregationIntervals(strings.Split(c.String("gw-stats-aggregation-intervals"), ","))

	// get the timezone
	if c.String("timezone") != "" {
		l, err := time.LoadLocation(c.String("timezone"))
		if err != nil {
			log.Fatalf("load timezone location error: %s", err)
		}
		common.TimeLocation = l
	}

	common.Band = bandConfig
	common.BandName = band.Name(c.String("band"))
	common.DeduplicationDelay = c.Duration("deduplication-delay")
	common.GetDownlinkDataDelay = c.Duration("get-downlink-data-delay")
	common.CreateGatewayOnStats = c.Bool("gw-create-on-stats")
	common.NodeSessionTTL = c.Duration("node-session-ttl")
	common.LogNodeFrames = c.Bool("log-node-frames")

	log.WithFields(log.Fields{
		"version": version,
		"net_id":  netID.String(),
		"band":    c.String("band"),
		"docs":    "https://docs.loraserver.io/",
	}).Info("starting LoRa Server")

	mustEnableUplinkChannels(c)

	lsCtx := mustGetContext(netID, c)

	// migrate old node-session keys to new layout
	if err = migration.MigrateNodeSessionDevAddrDevEUI(lsCtx.RedisPool); err != nil {
		log.Fatalf("node-session migration error: %s", err)
	}

	// migrate the database
	if c.Bool("db-automigrate") {
		log.Info("applying database migrations")
		m := &migrate.AssetMigrationSource{
			Asset:    migrations.Asset,
			AssetDir: migrations.AssetDir,
			Dir:      "",
		}
		n, err := migrate.Exec(lsCtx.DB.DB, "postgres", m, migrate.Up)
		if err != nil {
			log.Fatalf("applying migrations failed: %s", err)
		}
		log.WithField("count", n).Info("migrations applied")
	}

	// start the api server
	log.WithFields(log.Fields{
		"bind":     c.String("bind"),
		"ca-cert":  c.String("ca-cert"),
		"tls-cert": c.String("tls-cert"),
		"tls-key":  c.String("tls-key"),
	}).Info("starting api server")
	apiServer := mustGetAPIServer(lsCtx, c)
	ln, err := net.Listen("tcp", c.String("bind"))
	if err != nil {
		log.Fatalf("start api listener error: %s", err)
	}
	go apiServer.Serve(ln)

	// start the loraserver
	server := uplink.NewServer(lsCtx)
	if err := server.Start(); err != nil {
		log.Fatal(err)
	}
	// start the stats server
	gwStats := gw.NewStatsHandler(lsCtx)
	if err := gwStats.Start(); err != nil {
		log.Fatal(err)
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

func mustGetContext(netID lorawan.NetID, c *cli.Context) common.Context {
	// setup redis pool
	log.WithField("url", c.String("redis-url")).Info("setup redis connection pool")
	rp := common.NewRedisPool(c.String("redis-url"))

	// setup PostgreSQL connection
	log.Info("connecting to postgresql")
	db, err := common.OpenDatabase(c.String("postgres-dsn"))
	if err != nil {
		log.Fatalf("database connection error: %s", err)
	}

	// setup gateway backend
	gw, err := gateway.NewBackend(rp, c.String("gw-mqtt-server"), c.String("gw-mqtt-username"), c.String("gw-mqtt-password"), c.String("gw-mqtt-ca-file"))
	if err != nil {
		log.Fatalf("gateway-backend setup failed: %s", err)
	}

	// setup application client
	log.WithFields(log.Fields{
		"server":   c.String("as-server"),
		"ca-cert":  c.String("as-ca-cert"),
		"tls-cert": c.String("as-tls-cert"),
		"tls-key":  c.String("as-tls-key"),
	}).Info("connecting to application-server")
	var asDialOptions []grpc.DialOption
	if c.String("as-tls-cert") != "" && c.String("as-tls-key") != "" {
		asDialOptions = append(asDialOptions, grpc.WithTransportCredentials(
			mustGetTransportCredentials(c.String("as-tls-cert"), c.String("as-tls-key"), c.String("as-ca-cert"), false),
		))
	} else {
		asDialOptions = append(asDialOptions, grpc.WithInsecure())
	}
	asConn, err := grpc.Dial(c.String("as-server"), asDialOptions...)
	if err != nil {
		log.Fatalf("application-server dial error: %s", err)
	}
	asClient := as.NewApplicationServerClient(asConn)

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
			log.Fatalf("network-controller dial error: %s", err)
		}
		ncClient = nc.NewNetworkControllerClient(ncConn)
	} else {
		log.Info("no network-controller configured")
		ncClient = &controller.NopNetworkControllerClient{}
	}

	return common.Context{
		RedisPool:   rp,
		DB:          db,
		Gateway:     gw,
		Application: asClient,
		Controller:  ncClient,
		NetID:       netID,
	}
}

func mustGetAPIServer(ctx common.Context, c *cli.Context) *grpc.Server {
	var opts []grpc.ServerOption
	if c.String("tls-cert") != "" && c.String("tls-key") != "" {
		creds := mustGetTransportCredentials(c.String("tls-cert"), c.String("tls-key"), c.String("ca-cert"), false)
		opts = append(opts, grpc.Creds(creds))
	}
	gs := grpc.NewServer(opts...)
	nsAPI := api.NewNetworkServerAPI(ctx)
	ns.RegisterNetworkServerServer(gs, nsAPI)

	return gs
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

func mustEnableUplinkChannels(c *cli.Context) {
	if c.String("enable-uplink-channels") == "" {
		return
	}

	log.Info("disabling all channels")
	for _, c := range common.Band.GetEnabledUplinkChannels() {
		if err := common.Band.DisableUplinkChannel(c); err != nil {
			log.Fatalf("disable uplink channel error: %s", err)
		}
	}

	blocks := strings.Split(c.String("enable-uplink-channels"), ",")
	for _, block := range blocks {
		block = strings.Trim(block, " ")
		var start, end int
		if _, err := fmt.Sscanf(block, "%d-%d", &start, &end); err != nil {
			if _, err := fmt.Sscanf(block, "%d", &start); err != nil {
				log.Fatalf("parse channel range error: %s", err)
			}
			end = start
		}

		log.WithFields(log.Fields{
			"first_channel": start,
			"last_channel":  end,
		}).Info("enabling channel block")

		for ; start <= end; start++ {
			if err := common.Band.EnableUplinkChannel(start); err != nil {
				log.Fatalf("enable uplink channel error: %s", err)
			}
		}
	}
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
		cli.StringFlag{
			Name:   "ca-cert",
			Usage:  "ca certificate used by the api server (optional)",
			EnvVar: "CA_CERT",
		},
		cli.StringFlag{
			Name:   "tls-cert",
			Usage:  "tls certificate used by the api server (optional)",
			EnvVar: "TLS_CERT",
		},
		cli.StringFlag{
			Name:   "tls-key",
			Usage:  "tls key used by the api server (optional)",
			EnvVar: "TLS_KEY",
		},
		cli.StringFlag{
			Name:   "bind",
			Usage:  "ip:port to bind the api server",
			Value:  "0.0.0.0:8000",
			EnvVar: "BIND",
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
			Name:   "gw-mqtt-ca-file",
			Usage:  "mqtt CA certificate file used by the gateway backend (optional)",
			EnvVar: "GW_MQTT_CA_CERT",
		},
		cli.StringFlag{
			Name:   "as-server",
			Usage:  "hostname:port of the application-server api server (optional)",
			Value:  "127.0.0.1:8001",
			EnvVar: "AS_SERVER",
		},
		cli.StringFlag{
			Name:   "as-ca-cert",
			Usage:  "ca certificate used by the application-server client (optional)",
			EnvVar: "AS_CA_CERT",
		},
		cli.StringFlag{
			Name:   "as-tls-cert",
			Usage:  "tls certificate used by the application-server client (optional)",
			EnvVar: "AS_TLS_CERT",
		},
		cli.StringFlag{
			Name:   "as-tls-key",
			Usage:  "tls key used by the application-server client (optional)",
			EnvVar: "AS_TLS_KEY",
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
	}
	app.Run(os.Args)
}
