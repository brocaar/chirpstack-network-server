//go:generate go-bindata -prefix ../../migrations/ -pkg migrations -o ../../internal/migrations/migrations_gen.go ../../migrations/
//go:generate go-bindata -prefix ../../static/ -pkg static -o ../../internal/static/static_gen.go ../../static/...

package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"

	log "github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	assetfs "github.com/elazarl/go-bindata-assetfs"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"github.com/rubenv/sql-migrate"
	"golang.org/x/net/context"

	"github.com/brocaar/loraserver/internal/api"
	"github.com/brocaar/loraserver/internal/api/auth"
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

func init() {
	// this is basically disabling the gRPC logging. the reason is that the
	// grpc-gateway handler is started at the same time as the gRPC handler
	// as they are served on the same port (when tls is enabled).
	// since the grpc-gateway tries to connect right away, this generates
	// some initial connection errors (which are recovered) most times.
	// TODO: is there a good way to handle this?
	grpcLogger := log.New()
	grpcLogger.Level = log.ErrorLevel
	grpclog.SetLogger(grpcLogger)
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

	lsCtx := mustGetContext(netID, c)

	// auto-migrate the database
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

	// start the loraserver
	server := loraserver.NewServer(lsCtx)
	if err := server.Start(); err != nil {
		log.Fatal(err)
	}

	// setup the grpc api
	grpcHandler := mustGetGRPCHandler(ctx, lsCtx, c)

	// setup the http server
	httpHandler := mustGetHTTPHandler(ctx, lsCtx, c)

	if c.String("http-tls-cert") != "" && c.String("http-tls-key") != "" {
		// when tls is enabled, we combine the gRPC handler and http handler
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ProtoMajor == 2 && strings.Contains(r.Header.Get("Content-Type"), "application/grpc") {
				grpcHandler.ServeHTTP(w, r)
			} else {
				httpHandler.ServeHTTP(w, r)
			}
		})
		go func() {
			log.WithField("bind", c.String("http-bind")).Info("starting REST + gRPC api server (with tls)")
			log.Fatal(http.ListenAndServeTLS(c.String("http-bind"), c.String("http-tls-cert"), c.String("http-tls-key"), handler))
		}()
	} else {
		// since the gRPC server uses http/2, which is only enabled by the http
		// package when tls is enabled, we need to setup a separate server
		log.Warning("tls is disabled, the gRPC server will run on a separate port (as specified by grpc-insecure-bind)")

		list, err := net.Listen("tcp", c.String("grpc-insecure-bind"))
		if err != nil {
			log.Fatalf("create gRPC listener error: %s", err)
		}
		go func() {
			log.WithField("bind", c.String("grpc-insecure-bind")).Info("starting gRPC api server (without tls)")
			log.Fatal(grpcHandler.Serve(list))
		}()

		go func() {
			log.WithField("bind", c.String("http-bind")).Info("starting REST api server (without tls)")
			log.Fatal(http.ListenAndServe(c.String("http-bind"), httpHandler))
		}()

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
		exitChan <- struct{}{}
	}()
	select {
	case <-exitChan:
	case s := <-sigChan:
		log.WithField("signal", s).Info("signal received, stopping immediately")
	}

	return nil
}

func mustGetContext(netID lorawan.NetID, c *cli.Context) loraserver.Context {
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

	return loraserver.Context{
		DB:          db,
		RedisPool:   rp,
		Gateway:     gw,
		Application: app,
		Controller:  ctrl,
		NetID:       netID,
	}
}

func mustGetGRPCHandler(ctx context.Context, lsCtx loraserver.Context, c *cli.Context) *grpc.Server {
	var validator auth.Validator

	if c.String("jwt-secret") != "" && c.String("jwt-algorithm") != "" {
		validator = auth.NewJWTValidator(c.String("jwt-algorithm"), c.String("jwt-secret"))
	} else {
		log.Warning("api authentication and authorization is disabled (no jwt-algorithm or jwt-token are set)")
		validator = auth.NopValidator{}
	}

	return api.GetGRPCServer(ctx, lsCtx, validator, []grpc.ServerOption{})
}

func mustGetHTTPHandler(ctx context.Context, lsCtx loraserver.Context, c *cli.Context) http.Handler {
	// gRPC dial options for the grpc-gateway
	var bind string
	var grpcOpts []grpc.DialOption
	if c.String("http-tls-key") != "" && c.String("http-tls-cert") != "" {
		bind = c.String("http-bind")
		b, err := ioutil.ReadFile(c.String("http-tls-cert"))
		if err != nil {
			log.Fatalf("could not load tls cert: %s", err)
		}
		cp := x509.NewCertPool()
		if !cp.AppendCertsFromPEM(b) {
			log.Fatal("failed to append certificates")
		}

		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			// given the grpc-gateway is always connecting to localhost, does
			// InsecureSkipVerify=true cause any security issues?
			InsecureSkipVerify: true,
			RootCAs:            cp,
		})))
	} else {
		bind = c.String("grpc-insecure-bind")
		grpcOpts = append(grpcOpts, grpc.WithInsecure())
	}

	r := mux.NewRouter()

	// setup json api
	jsonHandler, err := api.GetJSONGateway(ctx, lsCtx, bind, grpcOpts)
	if err != nil {
		log.Fatalf("get json gateway error: %s", err)
	}
	log.WithField("path", "/api/v1").Info("registering REST api handler and documentation endpoint")
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

	return r
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
			Usage:  "ip:port to bind the http server to (web-interface, RESTful api and when TLS is setup the gRPC api)",
			Value:  "0.0.0.0:8000",
			EnvVar: "HTTP_BIND",
		},
		cli.StringFlag{
			Name:   "http-tls-cert",
			Usage:  "http server TLS certificate",
			EnvVar: "HTTP_TLS_CERT",
		},
		cli.StringFlag{
			Name:   "http-tls-key",
			Usage:  "http server TLS key",
			EnvVar: "HTTP_TLS_KEY",
		},
		cli.StringFlag{
			Name:   "grpc-insecure-bind",
			Usage:  "ip:port to bind the insecure gRPC api server to (only used when no http-tls-cert and http-tls-key are setup)",
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
		cli.StringFlag{
			Name:   "jwt-secret",
			Usage:  "JWT secret used for api authentication / authorization (disabled when left blank)",
			EnvVar: "JWT_SECRET",
		},
		cli.StringFlag{
			Name:   "jwt-algorithm",
			Usage:  "JWT algorithm used for to generate the signature",
			Value:  "HS256",
			EnvVar: "JWT_ALGORITHM",
		},
	}
	app.Run(os.Args)
}
