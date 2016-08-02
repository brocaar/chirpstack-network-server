//go:generate go-bindata -prefix ../../migrations/ -pkg migrations -o ../../internal/migrations/migrations_gen.go ../../migrations/
//go:generate go-bindata -prefix ../../static/ -pkg static -o ../../internal/static/static_gen.go ../../static/...

package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	log "github.com/Sirupsen/logrus"
	assetfs "github.com/elazarl/go-bindata-assetfs"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"github.com/rubenv/sql-migrate"
	"github.com/urfave/cli"
	"golang.org/x/net/context"

	"github.com/brocaar/loraserver/internal/api"
	"github.com/brocaar/loraserver/internal/backend/application"
	"github.com/brocaar/loraserver/internal/backend/controller"
	"github.com/brocaar/loraserver/internal/backend/gateway"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/loraserver"
	"github.com/brocaar/loraserver/internal/migrations"
	"github.com/brocaar/loraserver/internal/static"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
)

var version string // set by the compiler
var bands = []string{
	string(band.AU_915_928),
	string(band.EU_863_870),
	string(band.US_902_928),
}

func run(c *cli.Context) error {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// parse the NetID
	var netID lorawan.NetID
	if err := netID.UnmarshalText([]byte(c.String("net-id"))); err != nil {
		log.Fatalf("NetID parse error: %s", err)
	}

	// get the band config
	if c.String("band") == "" {
		log.Fatalf("--band is undefined, valid options are: %s", strings.Join(bands, ", "))
	}
	bandConfig, err := band.GetConfig(band.Name(c.String("band")))
	if err != nil {
		log.Fatal(err)
	}
	common.Band = bandConfig

	log.WithFields(log.Fields{
		"version": version,
		"net_id":  netID.String(),
		"band":    c.String("band"),
		"docs":    "https://docs.loraserver.io/",
	}).Info("starting LoRa Server")

	// connect to the database
	log.Info("connecting to postgresql")
	db, err := storage.OpenDatabase(c.String("postgres-dsn"))
	if err != nil {
		log.Fatalf("database connection error: %s", err)
	}

	// setup redis pool
	log.Info("setup redis connection pool")
	rp := storage.NewRedisPool(c.String("redis-url"))

	// setup gateway backend
	gw, err := gateway.NewBackend(c.String("gw-mqtt-server"), c.String("gw-mqtt-username"), c.String("gw-mqtt-password"))
	if err != nil {
		log.Fatalf("gateway-backend setup failed: %s", err)
	}

	// setup application backend
	app, err := application.NewBackend(rp, c.String("app-mqtt-server"), c.String("app-mqtt-username"), c.String("app-mqtt-password"))
	if err != nil {
		log.Fatalf("application-backend setup failed: %s", err)
	}

	// setup controller backend
	ctrl, err := controller.NewBackend(rp, c.String("controller-mqtt-server"), c.String("controller-mqtt-username"), c.String("controller-mqtt-password"))
	if err != nil {
		log.Fatalf("controller-backend setup failed: %s", err)
	}

	// auto-migrate the database
	if c.Bool("db-automigrate") {
		log.Info("applying database migrations")
		m := &migrate.AssetMigrationSource{
			Asset:    migrations.Asset,
			AssetDir: migrations.AssetDir,
			Dir:      "",
		}
		n, err := migrate.Exec(db.DB, "postgres", m, migrate.Up)
		if err != nil {
			log.Fatalf("applying migrations failed: %s", err)
		}
		log.WithField("count", n).Info("migrations applied")
	}

	lsCtx := loraserver.Context{
		DB:          db,
		RedisPool:   rp,
		Gateway:     gw,
		Application: app,
		Controller:  ctrl,
		NetID:       netID,
	}

	// start the loraserver
	server := loraserver.NewServer(lsCtx)
	if err := server.Start(); err != nil {
		log.Fatal(err)
	}

	// setup the grpc api
	go func() {
		server := api.GetGRPCServer(ctx, lsCtx)
		list, err := net.Listen("tcp", c.String("grpc-bind"))
		if err != nil {
			log.Fatalf("error creating gRPC listener: %s", err)
		}
		log.WithField("bind", c.String("grpc-bind")).Info("starting gRPC server")
		log.Fatal(server.Serve(list))
	}()

	// setup the http server
	r := mux.NewRouter()

	// setup json api
	jsonHandler, err := api.GetJSONGateway(ctx, lsCtx, c.String("grpc-bind"))
	if err != nil {
		log.Fatalf("get json gateway error: %s", err)
	}
	log.WithField("path", "/api/v1").Info("registering api handler and documentation endpoint")
	r.HandleFunc("/api/v1", api.SwaggerHandlerFunc).Methods("get")
	r.PathPrefix("/api/v1/").Handler(jsonHandler)

	// setup static file server (for the gui)
	log.WithField("path", "/").Info("registering gui handler")
	r.PathPrefix("/").Handler(http.FileServer(&assetfs.AssetFS{
		Asset:     static.Asset,
		AssetDir:  static.AssetDir,
		AssetInfo: static.AssetInfo,
		Prefix:    "",
	}))

	// start the http server
	go func() {
		log.WithField("bind", c.String("http-bind")).Info("starting rest api / gui server")
		log.Fatal(http.ListenAndServe(c.String("http-bind"), r))
	}()

	sigChan := make(chan os.Signal)
	exitChan := make(chan struct{})
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	log.WithField("signal", <-sigChan).Info("signal received")
	go func() {
		log.Warning("stopping loraserver")
		if err := server.Stop(); err != nil {
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
			Usage:  fmt.Sprintf("ism band configuration to use. valid options are: %s", strings.Join(bands, ", ")),
			EnvVar: "BAND",
		},
		cli.StringFlag{
			Name:   "http-bind",
			Usage:  "ip:port to bind the http api server to",
			Value:  "0.0.0.0:8000",
			EnvVar: "HTTP_BIND",
		},
		cli.StringFlag{
			Name:   "grpc-bind",
			Usage:  "ip:port to bind the gRPC api server to",
			Value:  "0.0.0.0:9000",
			EnvVar: "GRPC_BIND",
		},
		cli.StringFlag{
			Name:   "postgres-dsn",
			Usage:  "postgresql dsn (e.g.: postgres://user:password@hostname/database?sslmode=disable)",
			Value:  "postgres://localhost/loraserver?sslmode=disable",
			EnvVar: "POSTGRES_DSN",
		},
		cli.StringFlag{
			Name:   "redis-url",
			Usage:  "redis url",
			Value:  "redis://localhost:6379",
			EnvVar: "REDIS_URL",
		},
		cli.BoolFlag{
			Name:   "db-automigrate",
			Usage:  "automatically apply database migrations",
			EnvVar: "DB_AUTOMIGRATE",
		},
		cli.StringFlag{
			Name:   "gw-mqtt-server",
			Usage:  "Gateway-backend MQTT server",
			Value:  "tcp://localhost:1883",
			EnvVar: "GW_MQTT_SERVER",
		},
		cli.StringFlag{
			Name:   "gw-mqtt-username",
			Usage:  "Gateway-backend MQTT username",
			EnvVar: "GW_MQTT_USERNAME",
		},
		cli.StringFlag{
			Name:   "gw-mqtt-password",
			Usage:  "Gateway-backend MQTT password",
			EnvVar: "GW_MQTT_PASSWORD",
		},
		cli.StringFlag{
			Name:   "app-mqtt-server",
			Usage:  "Application-backend MQTT server",
			Value:  "tcp://localhost:1883",
			EnvVar: "APP_MQTT_SERVER",
		},
		cli.StringFlag{
			Name:   "app-mqtt-username",
			Usage:  "Application-backend MQTT username",
			EnvVar: "APP_MQTT_USERNAME",
		},
		cli.StringFlag{
			Name:   "app-mqtt-password",
			Usage:  "Application-backend MQTT password",
			EnvVar: "APP_MQTT_PASSWORD",
		},
		cli.StringFlag{
			Name:   "controller-mqtt-server",
			Usage:  "Network-controller backend MQTT server",
			Value:  "tcp://localhost:1883",
			EnvVar: "CONTROLLER_MQTT_SERVER",
		},
		cli.StringFlag{
			Name:   "controller-mqtt-username",
			Usage:  "Network-controller backend MQTT username",
			EnvVar: "APP_MQTT_USERNAME",
		},
		cli.StringFlag{
			Name:   "controller-mqtt-password",
			Usage:  "Network-controller backend MQTT password",
			EnvVar: "APP_MQTT_PASSWORD",
		},
	}
	app.Run(os.Args)
}
