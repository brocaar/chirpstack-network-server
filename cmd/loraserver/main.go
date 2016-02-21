package main

import (
	"os"

	"github.com/DavidHuie/gomigrate"
	log "github.com/Sirupsen/logrus"
	"github.com/brocaar/loraserver"
	"github.com/codegangsta/cli"
	_ "github.com/lib/pq"
)

var version string // set by the compiler

func run(c *cli.Context) {
	// connect to the database
	db, err := loraserver.OpenDatabase(c.String("postgres-dsn"))
	if err != nil {
		log.Fatalf("could not connect to the database: %s", err)
	}

	// auto-migrate the database
	if c.Bool("db-automigrate") {
		migrator, err := gomigrate.NewMigrator(db.DB, gomigrate.Postgres{}, c.String("db-migrations-path"))
		if err != nil {
			log.Fatalf("could not create the migrator: %s", err)
		}
		if err = migrator.Migrate(); err != nil {
			log.Fatalf("could run the migrations: %s", err)
		}
	}
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
	}
	app.Run(os.Args)
}
