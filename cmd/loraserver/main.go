package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/DavidHuie/gomigrate"
	log "github.com/Sirupsen/logrus"
	"github.com/brocaar/loraserver"
	"github.com/brocaar/loraserver/gateway/mqttpubsub"
	"github.com/codegangsta/cli"
	_ "github.com/lib/pq"
)

var version string // set by the compiler

func run(c *cli.Context) {
	// connect to the database
	log.Info("connecting to database")
	db, err := loraserver.OpenDatabase(c.String("postgres-dsn"))
	if err != nil {
		log.Fatalf("could not connect to the database: %s", err)
	}

	// setup redis pool
	log.Info("setup redis connection pool")
	rp := loraserver.NewRedisPool(c.String("redis-url"))

	// setup gateway backend
	gw, err := mqttpubsub.NewBackend(c.String("gw-mqtt-server"), c.String("gw-mqtt-username"), c.String("gw-mqtt-password"))
	if err != nil {
		log.Fatalf("could not setup gateway backend: %s", err)
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

	// provision ABP node sessions
	if c.Bool("create-abp-node-sessions") {
		log.Info("creating node-sessions from ABP")
		if err := loraserver.NewNodeSessionsFromABP(db, rp); err != nil {
			log.Fatalf("could not create ABP node-sessions: %s", err)
		}
	}

	ctx := loraserver.Context{
		DB:        db,
		RedisPool: rp,
		Gateway:   gw,
	}

	go func() {
		if err := loraserver.Start(ctx); err != nil {
			log.Fatal(err)
		}
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
		},
		cli.StringFlag{
			Name:   "postgres-dsn",
			Usage:  "postgresql dsn (e.g.: postgres://pqtest:password@localhost/pqtest?sslmode=verify-full)",
			EnvVar: "POSTGRES_DSN",
		},
		cli.StringFlag{
			Name:   "redis-url",
			Usage:  "redis url",
			Value:  "redis://localhost:6379",
			EnvVar: "REDIS_URL",
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
		cli.BoolFlag{
			Name:   "create-abp-node-sessions",
			Usage:  "create ABP node sessions on startup of server",
			EnvVar: "CREATE_ABP_NODE_SESSIONS",
		},
	}
	app.Run(os.Args)
}
