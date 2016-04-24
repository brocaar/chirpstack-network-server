//go:generate go-bindata -prefix ../../migrations/ -pkg migrations -o ../../internal/loraserver/migrations/migrations_gen.go ../../migrations/
//go:generate go-bindata -prefix ../../static/ -pkg static -o ../../internal/loraserver/static/static_gen.go ../../static/...

package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/elazarl/go-bindata-assetfs"
	_ "github.com/lib/pq"

	log "github.com/Sirupsen/logrus"
	application "github.com/brocaar/loraserver/internal/loraserver/application/mqttpubsub"
	gateway "github.com/brocaar/loraserver/internal/loraserver/gateway/mqttpubsub"

	"github.com/brocaar/loraserver/internal/loraserver"
	"github.com/brocaar/loraserver/internal/loraserver/migrations"
	"github.com/brocaar/loraserver/internal/loraserver/static"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
	"github.com/codegangsta/cli"
	"github.com/rubenv/sql-migrate"
)

var version string // set by the compiler

func run(c *cli.Context) {
	// parse the NetID
	var netID lorawan.NetID
	if err := netID.UnmarshalText([]byte(c.String("net-id"))); err != nil {
		log.Fatalf("could not parse NetID: %s", err)
	}

	// connect to the database
	log.Info("connecting to postgresql")
	db, err := loraserver.OpenDatabase(c.String("postgres-dsn"))
	if err != nil {
		log.Fatalf("could not connect to the database: %s", err)
	}

	// setup redis pool
	log.Info("setup redis connection pool")
	rp := loraserver.NewRedisPool(c.String("redis-url"))

	// setup gateway backend
	gw, err := gateway.NewBackend(c.String("gw-mqtt-server"), c.String("gw-mqtt-username"), c.String("gw-mqtt-password"))
	if err != nil {
		log.Fatalf("could not setup gateway backend: %s", err)
	}

	// setup application backend
	app, err := application.NewBackend(rp, c.String("app-mqtt-server"), c.String("app-mqtt-username"), c.String("app-mqtt-password"))
	if err != nil {
		log.Fatalf("could not setup application backend: %s", err)
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
			log.Fatalf("migrations failed: %s", err)
		}
		log.WithField("count", n).Info("migrations applied")
	}

	nodeManager, err := loraserver.NewNodeManager(db)
	if err != nil {
		log.Fatalf("could not setup node manager: %v", err)
	}
	nodeApplicationsManager, err := loraserver.NewNodeApplicationsManager(db)
	if err != nil {
		log.Fatalf("could not setup node applications manager: %v", err)
	}

	ctx := loraserver.Context{
		NodeManager:    nodeManager,
		NodeAppManager: nodeApplicationsManager,
		RedisPool:      rp,
		Gateway:        gw,
		Application:    app,
		NetID:          netID,
	}

	// start the loraserver
	server := loraserver.NewServer(ctx)
	if err := server.Start(); err != nil {
		log.Fatal(err)
	}

	// setup json-rpc api handler
	apiHandler, err := loraserver.NewJSONRPCHandler(
		loraserver.NewApplicationAPI(ctx),
		loraserver.NewNodeAPI(ctx),
		loraserver.NewNodeSessionAPI(ctx),
	)
	if err != nil {
		log.Fatal(err)
	}
	log.WithField("path", "/rpc").Info("registering json-rpc handler")
	http.Handle("/rpc", apiHandler)

	// setup static file server (for the gui)
	log.WithField("path", "/").Info("registering gui handler")
	http.Handle("/", http.FileServer(&assetfs.AssetFS{
		Asset:     static.Asset,
		AssetDir:  static.AssetDir,
		AssetInfo: static.AssetInfo,
		Prefix:    "",
	}))

	// start the http server
	go func() {
		log.WithField("bind", c.String("http-bind")).Info("starting http server")
		log.Fatal(http.ListenAndServe(c.String("http-bind"), nil))
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
}

func main() {
	app := cli.NewApp()
	app.Name = "loraserver"
	app.Usage = "network-server for LoRaWAN networks"
	app.Version = fmt.Sprintf("%s (compiled for %s)", version, band.Name)
	app.Copyright = "See http://github.com/brocaar/loraserver for copyright information"
	app.Action = run
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:   "db-automigrate",
			Usage:  "automatically apply database migrations",
			EnvVar: "DB_AUTOMIGRATE",
		},
		cli.StringFlag{
			Name:   "net-id",
			Usage:  "network identifier (NetID, 3 bytes) encoded as HEX (e.g. 010203)",
			EnvVar: "NET_ID",
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
		cli.StringFlag{
			Name:   "http-bind",
			Usage:  "ip:port to bind the http api server to",
			Value:  "0.0.0.0:8000",
			EnvVar: "HTTP_BIND",
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
	}
	app.Run(os.Args)
}
