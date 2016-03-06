package main

import (
	"io/ioutil"
	golog "log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/DavidHuie/gomigrate"
	log "github.com/Sirupsen/logrus"
	"github.com/brocaar/loraserver"
	application "github.com/brocaar/loraserver/application/mqttpubsub"
	gateway "github.com/brocaar/loraserver/gateway/mqttpubsub"
	"github.com/brocaar/lorawan"
	"github.com/codegangsta/cli"
	_ "github.com/lib/pq"
)

var version string // set by the compiler

func init() {
	golog.SetOutput(ioutil.Discard)
}

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
	app, err := application.NewBackend(c.String("app-mqtt-server"), c.String("app-mqtt-username"), c.String("app-mqtt-password"))
	if err != nil {
		log.Fatalf("could not setup application backend: %s", err)
	}

	// auto-migrate the database
	if c.Bool("db-automigrate") {
		log.Info("applying database migrations")
		migrator, err := gomigrate.NewMigrator(db.DB, gomigrate.Postgres{}, c.String("db-migrations-path"))
		if err != nil {
			log.Fatalf("could not create the migrator: %s", err)
		}
		if err = migrator.Migrate(); err != nil {
			log.Fatalf("could run the migrations: %s", err)
		}
	}

	ctx := loraserver.Context{
		DB:          db,
		RedisPool:   rp,
		Gateway:     gw,
		Application: app,
		NetID:       netID,
	}

	// start the loraserver
	go func() {
		if err := loraserver.Start(ctx); err != nil {
			log.Fatal(err)
		}
	}()

	// setup json-rpc api handler
	apiHandler, err := loraserver.NewJSONRPCHandler(loraserver.NewAPI(ctx))
	if err != nil {
		log.Fatal(err)
	}
	log.WithField("path", "/rpc").Info("registering json-rpc handler")
	http.Handle("/rpc", apiHandler)
	go func() {
		log.WithField("bind", c.String("http-bind")).Info("starting http server")
		log.Fatal(http.ListenAndServe(c.String("http-bind"), nil))
	}()

	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	log.WithField("signal", <-sigChan).Info("signal received")
	log.Warning("loraserver is shutting down")
}

func main() {
	app := cli.NewApp()
	app.Name = "loraserver"
	app.Usage = "network-server for LoRaWAN networks"
	app.Version = version
	app.Action = run
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:   "db-automigrate",
			Usage:  "automatically apply database migrations",
			EnvVar: "DB_AUTOMIGRATE",
		},
		cli.StringFlag{
			Name:   "db-migrations-path",
			Value:  "./migrations",
			Usage:  "path to the directory containing the database migrations",
			EnvVar: "DB_MIGRATIONS_PATH",
		}, cli.StringFlag{
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
