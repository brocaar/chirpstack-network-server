package cmd

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

	"github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/logrus"
	"github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"github.com/pkg/errors"
	migrate "github.com/rubenv/sql-migrate"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

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
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/downlink"
	"github.com/brocaar/loraserver/internal/gateway"
	"github.com/brocaar/loraserver/internal/migrations"
	"github.com/brocaar/loraserver/internal/uplink"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
)

func run(cmd *cobra.Command, args []string) error {
	var server = new(uplink.Server)
	var gwStats = new(gateway.StatsHandler)

	tasks := []func() error{
		setLogLevel,
		setBandConfig,
		setRXParameters,
		setStatsAggregationIntervals,
		setTimezone,
		printStartMessage,
		enableUplinkChannels,
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
		if err := t(); err != nil {
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

func setBandConfig() error {
	if config.C.NetworkServer.Band.Name == "" {
		return fmt.Errorf("band is undefined, valid options are: %s", strings.Join(bands, ", "))
	}
	dwellTime := lorawan.DwellTimeNoLimit
	if config.C.NetworkServer.Band.DwellTime400ms {
		dwellTime = lorawan.DwellTime400ms
	}
	bandConfig, err := band.GetConfig(config.C.NetworkServer.Band.Name, config.C.NetworkServer.Band.RepeaterCompatible, dwellTime)
	if err != nil {
		return errors.Wrap(err, "get band config error")
	}
	for _, c := range config.C.NetworkServer.NetworkSettings.ExtraChannels {
		if err := bandConfig.AddChannel(c.Frequency, c.MinDR, c.MaxDR); err != nil {
			return errors.Wrap(err, "add channel error")
		}
	}

	config.C.NetworkServer.Band.Band = bandConfig

	return nil
}

func setRXParameters() error {
	defaults := config.C.NetworkServer.Band.Band.GetDefaults()

	if config.C.NetworkServer.NetworkSettings.RX2DR == -1 {
		config.C.NetworkServer.NetworkSettings.RX2DR = defaults.RX2DataRate
	}

	if config.C.NetworkServer.NetworkSettings.RX2Frequency == -1 {
		config.C.NetworkServer.NetworkSettings.RX2Frequency = defaults.RX2Frequency
	}

	return nil
}

func setStatsAggregationIntervals() error {
	// get the gw stats aggregation intervals
	gateway.MustSetStatsAggregationIntervals(config.C.NetworkServer.Gateway.Stats.AggregationIntervals)
	return nil
}

func setTimezone() error {
	// get the timezone
	if config.C.NetworkServer.Gateway.Stats.Timezone != "" {
		l, err := time.LoadLocation(config.C.NetworkServer.Gateway.Stats.Timezone)
		if err != nil {
			return errors.Wrap(err, "load timezone location error")
		}
		config.C.NetworkServer.Gateway.Stats.TimezoneLocation = l
	}
	return nil
}

func setLogLevel() error {
	log.SetLevel(log.Level(uint8(config.C.General.LogLevel)))
	return nil
}

func printStartMessage() error {
	log.WithFields(log.Fields{
		"version": version,
		"net_id":  config.C.NetworkServer.NetID.String(),
		"band":    config.C.NetworkServer.Band.Name,
		"docs":    "https://docs.loraserver.io/",
	}).Info("starting LoRa Server")
	return nil
}

func enableUplinkChannels() error {
	if len(config.C.NetworkServer.NetworkSettings.EnabledUplinkChannels) == 0 {
		return nil
	}

	log.Info("disabling all channels")
	for _, c := range config.C.NetworkServer.Band.Band.GetEnabledUplinkChannelIndices() {
		if err := config.C.NetworkServer.Band.Band.DisableUplinkChannelIndex(c); err != nil {
			return errors.Wrap(err, "disable uplink channel error")
		}
	}

	log.WithField("channels", config.C.NetworkServer.NetworkSettings.EnabledUplinkChannels).Info("enabling channels")
	for _, c := range config.C.NetworkServer.NetworkSettings.EnabledUplinkChannels {
		if err := config.C.NetworkServer.Band.Band.EnableUplinkChannelIndex(c); err != nil {
			errors.Wrap(err, "enable uplink channel error")
		}
	}

	return nil
}

func setRedisPool() error {
	log.WithField("url", config.C.Redis.URL).Info("setup redis connection pool")
	config.C.Redis.Pool = common.NewRedisPool(config.C.Redis.URL)
	return nil
}

func setPostgreSQLConnection() error {
	log.Info("connecting to postgresql")
	db, err := common.OpenDatabase(config.C.PostgreSQL.DSN)
	if err != nil {
		return errors.Wrap(err, "database connection error")
	}
	config.C.PostgreSQL.DB = db
	return nil
}

func setGatewayBackend() error {
	gw, err := gwBackend.NewMQTTBackend(
		config.C.Redis.Pool,
		config.C.NetworkServer.Gateway.Backend.MQTT,
	)
	if err != nil {
		return errors.Wrap(err, "gateway-backend setup failed")
	}
	config.C.NetworkServer.Gateway.Backend.Backend = gw
	return nil
}

func setApplicationServer() error {
	config.C.ApplicationServer.Pool = asclient.NewPool()
	return nil
}

func setJoinServer() error {
	jsClient, err := jsclient.NewClient(
		config.C.JoinServer.Default.Server,
		config.C.JoinServer.Default.CACert,
		config.C.JoinServer.Default.TLSCert,
		config.C.JoinServer.Default.TLSKey,
	)
	if err != nil {
		return errors.Wrap(err, "create new join-server client error")
	}
	config.C.JoinServer.Pool = jsclient.NewPool(jsClient)

	return nil
}

func setNetworkController() error {
	var ncClient nc.NetworkControllerClient
	if config.C.NetworkController.Server != "" {
		// setup network-controller client
		log.WithFields(log.Fields{
			"server":   config.C.NetworkController.Server,
			"ca-cert":  config.C.NetworkController.CACert,
			"tls-cert": config.C.NetworkController.TLSCert,
			"tls-key":  config.C.NetworkController.TLSKey,
		}).Info("connecting to network-controller")
		var ncDialOptions []grpc.DialOption
		if config.C.NetworkController.TLSCert != "" && config.C.NetworkController.TLSKey != "" {
			ncDialOptions = append(ncDialOptions, grpc.WithTransportCredentials(
				mustGetTransportCredentials(config.C.NetworkController.TLSCert, config.C.NetworkController.TLSKey, config.C.NetworkController.CACert, false),
			))
		} else {
			ncDialOptions = append(ncDialOptions, grpc.WithInsecure())
		}
		ncConn, err := grpc.Dial(config.C.NetworkController.Server, ncDialOptions...)
		if err != nil {
			return errors.Wrap(err, "network-controller dial error")
		}
		ncClient = nc.NewNetworkControllerClient(ncConn)
	} else {
		log.Info("no network-controller configured")
		ncClient = &controller.NopNetworkControllerClient{}
	}
	config.C.NetworkController.Client = ncClient
	return nil
}

func runDatabaseMigrations() error {
	if config.C.PostgreSQL.Automigrate {
		log.Info("applying database migrations")
		m := &migrate.AssetMigrationSource{
			Asset:    migrations.Asset,
			AssetDir: migrations.AssetDir,
			Dir:      "",
		}
		n, err := migrate.Exec(config.C.PostgreSQL.DB.DB.DB, "postgres", m, migrate.Up)
		if err != nil {
			return errors.Wrap(err, "applying migrations failed")
		}
		log.WithField("count", n).Info("migrations applied")
	}
	return nil
}

func gRPCLoggingServerOptions() []grpc.ServerOption {
	logrusEntry := log.NewEntry(log.StandardLogger())
	logrusOpts := []grpc_logrus.Option{
		grpc_logrus.WithLevels(grpc_logrus.DefaultCodeToLevel),
	}

	return []grpc.ServerOption{
		grpc_middleware.WithUnaryServerChain(
			grpc_ctxtags.UnaryServerInterceptor(grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor)),
			grpc_logrus.UnaryServerInterceptor(logrusEntry, logrusOpts...),
		),
		grpc_middleware.WithStreamServerChain(
			grpc_ctxtags.StreamServerInterceptor(grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor)),
			grpc_logrus.StreamServerInterceptor(logrusEntry, logrusOpts...),
		),
	}
}

func startAPIServer() error {
	log.WithFields(log.Fields{
		"bind":     config.C.NetworkServer.API.Bind,
		"ca-cert":  config.C.NetworkServer.API.CACert,
		"tls-cert": config.C.NetworkServer.API.TLSCert,
		"tls-key":  config.C.NetworkServer.API.TLSKey,
	}).Info("starting api server")

	opts := gRPCLoggingServerOptions()
	if config.C.NetworkServer.API.CACert != "" && config.C.NetworkServer.API.TLSCert != "" && config.C.NetworkServer.API.TLSKey != "" {
		creds := mustGetTransportCredentials(config.C.NetworkServer.API.TLSCert, config.C.NetworkServer.API.TLSKey, config.C.NetworkServer.API.CACert, true)
		opts = append(opts, grpc.Creds(creds))
	}
	gs := grpc.NewServer(opts...)
	nsAPI := api.NewNetworkServerAPI()
	ns.RegisterNetworkServerServer(gs, nsAPI)

	ln, err := net.Listen("tcp", config.C.NetworkServer.API.Bind)
	if err != nil {
		return errors.Wrap(err, "start api listener error")
	}
	go gs.Serve(ln)
	return nil
}

func startGatewayAPIServer() error {
	log.WithFields(log.Fields{
		"bind":     config.C.NetworkServer.Gateway.API.Bind,
		"ca-cert":  config.C.NetworkServer.Gateway.API.CACert,
		"tls-cert": config.C.NetworkServer.Gateway.API.TLSCert,
		"tls-key":  config.C.NetworkServer.Gateway.API.TLSKey,
	}).Info("starting gateway api server")

	var validator auth.Validator
	if config.C.NetworkServer.Gateway.API.JWTSecret != "" {
		validator = auth.NewJWTValidator("HS256", config.C.NetworkServer.Gateway.API.JWTSecret)
	} else {
		return errors.New("gateway jwt_secret must be set")
	}

	opts := gRPCLoggingServerOptions()
	if config.C.NetworkServer.Gateway.API.TLSCert != "" && config.C.NetworkServer.Gateway.API.TLSKey != "" {
		creds := mustGetTransportCredentials(config.C.NetworkServer.Gateway.API.TLSCert, config.C.NetworkServer.Gateway.API.TLSKey, config.C.NetworkServer.Gateway.API.CACert, false)
		opts = append(opts, grpc.Creds(creds))
	}
	gs := grpc.NewServer(opts...)
	gwAPI := api.NewGatewayAPI(validator)
	gw.RegisterGatewayServer(gs, gwAPI)

	gwServerLn, err := net.Listen("tcp", config.C.NetworkServer.Gateway.API.Bind)
	if err != nil {
		return errors.Wrap(err, "start gateway api server listener error")
	}
	go gs.Serve(gwServerLn)
	return nil
}

func startLoRaServer(server *uplink.Server) func() error {
	return func() error {
		*server = *uplink.NewServer()
		return server.Start()
	}
}

func startStatsServer(gwStats *gateway.StatsHandler) func() error {
	return func() error {
		*gwStats = *gateway.NewStatsHandler()
		if err := gwStats.Start(); err != nil {
			log.Fatal(err)
		}
		return nil
	}
}

func startQueueScheduler() error {
	log.Info("starting downlink device-queue scheduler")
	go downlink.SchedulerLoop()
	return nil
}

func mustGetTransportCredentials(tlsCert, tlsKey, caCert string, verifyClientCert bool) credentials.TransportCredentials {
	cert, err := tls.LoadX509KeyPair(tlsCert, tlsKey)
	if err != nil {
		log.WithFields(log.Fields{
			"cert": tlsCert,
			"key":  tlsKey,
		}).Fatalf("load key-pair error: %s", err)
	}

	rawCaCert, err := ioutil.ReadFile(caCert)
	if err != nil {
		log.WithField("ca", caCert).Fatalf("load ca cert error: %s", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(rawCaCert) {
		log.WithField("ca_cert", caCert).Fatal("append ca certificate error")
	}

	if verifyClientCert {
		return credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{cert},
			ClientCAs:    caCertPool,
			ClientAuth:   tls.RequireAndVerifyClientCert,
		})
	}

	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
	})
}
