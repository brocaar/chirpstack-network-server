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

	"github.com/brocaar/loraserver/internal/adr"
	"github.com/brocaar/loraserver/internal/backend/gateway/azureiothub"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_logrus "github.com/grpc-ecosystem/go-grpc-middleware/logging/logrus"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/brocaar/loraserver/api/geo"
	"github.com/brocaar/loraserver/api/nc"
	"github.com/brocaar/loraserver/api/ns"
	"github.com/brocaar/loraserver/internal/api"
	"github.com/brocaar/loraserver/internal/backend/applicationserver"
	"github.com/brocaar/loraserver/internal/backend/controller"
	gwbackend "github.com/brocaar/loraserver/internal/backend/gateway"
	"github.com/brocaar/loraserver/internal/backend/gateway/gcppubsub"
	"github.com/brocaar/loraserver/internal/backend/gateway/mqtt"
	"github.com/brocaar/loraserver/internal/backend/geolocationserver"
	"github.com/brocaar/loraserver/internal/backend/joinserver"
	"github.com/brocaar/loraserver/internal/band"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/downlink"
	"github.com/brocaar/loraserver/internal/gateway"
	"github.com/brocaar/loraserver/internal/migrations/code"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/uplink"
)

func run(cmd *cobra.Command, args []string) error {
	var server = new(uplink.Server)
	var gwStats = new(gateway.StatsHandler)

	tasks := []func() error{
		setLogLevel,
		setupBand,
		setRXParameters,
		setupMetrics,
		printStartMessage,
		enableUplinkChannels,
		setupStorage,
		setGatewayBackend,
		setupApplicationServer,
		setupADR,
		setupGeolocationServer,
		setupJoinServer,
		setupNetworkController,
		setupUplink,
		setupDownlink,
		fixV2RedisCache,
		migrateGatewayStats,
		startAPIServer,
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

func setupBand() error {
	if err := band.Setup(config.C); err != nil {
		return errors.Wrap(err, "setup band error")
	}

	return nil
}

func setRXParameters() error {
	defaults := band.Band().GetDefaults()

	if config.C.NetworkServer.NetworkSettings.RX2DR == -1 {
		config.C.NetworkServer.NetworkSettings.RX2DR = defaults.RX2DataRate
	}

	if config.C.NetworkServer.NetworkSettings.RX2Frequency == -1 {
		config.C.NetworkServer.NetworkSettings.RX2Frequency = defaults.RX2Frequency
	}

	return nil
}

func setupMetrics() error {
	// setup aggregation intervals
	var intervals []storage.AggregationInterval
	for _, agg := range config.C.Metrics.Redis.AggregationIntervals {
		intervals = append(intervals, storage.AggregationInterval(strings.ToUpper(agg)))
	}
	if err := storage.SetAggregationIntervals(intervals); err != nil {
		return errors.Wrap(err, "set aggregation intervals error")
	}

	// setup timezone
	var err error
	if config.C.Metrics.Timezone == "" {
		err = storage.SetTimeLocation(config.C.NetworkServer.Gateway.Stats.Timezone)
	} else {
		err = storage.SetTimeLocation(config.C.Metrics.Timezone)
	}
	if err != nil {
		return errors.Wrap(err, "set time location error")
	}

	// setup storage TTL
	storage.SetMetricsTTL(
		config.C.Metrics.Redis.MinuteAggregationTTL,
		config.C.Metrics.Redis.HourAggregationTTL,
		config.C.Metrics.Redis.DayAggregationTTL,
		config.C.Metrics.Redis.MonthAggregationTTL,
	)

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
		"docs":    "https:/www.loraserver.io/",
	}).Info("starting LoRa Server")
	return nil
}

func enableUplinkChannels() error {
	if len(config.C.NetworkServer.NetworkSettings.EnabledUplinkChannels) == 0 {
		return nil
	}

	log.Info("disabling all channels")
	for _, c := range band.Band().GetEnabledUplinkChannelIndices() {
		if err := band.Band().DisableUplinkChannelIndex(c); err != nil {
			return errors.Wrap(err, "disable uplink channel error")
		}
	}

	log.WithField("channels", config.C.NetworkServer.NetworkSettings.EnabledUplinkChannels).Info("enabling channels")
	for _, c := range config.C.NetworkServer.NetworkSettings.EnabledUplinkChannels {
		if err := band.Band().EnableUplinkChannelIndex(c); err != nil {
			errors.Wrap(err, "enable uplink channel error")
		}
	}

	return nil
}

func setupStorage() error {
	if err := storage.Setup(config.C); err != nil {
		return errors.Wrap(err, "setup storage error")
	}
	return nil
}

func setupADR() error {
	if err := adr.Setup(config.C); err != nil {
		errors.Wrap(err, "setup adr error")
	}
	return nil
}

func setGatewayBackend() error {
	var err error
	var gw gwbackend.Gateway

	switch config.C.NetworkServer.Gateway.Backend.Type {
	case "mqtt":
		gw, err = mqtt.NewBackend(
			storage.RedisPool(),
			config.C,
		)
	case "gcp_pub_sub":
		gw, err = gcppubsub.NewBackend(config.C)
	case "azure_iot_hub":
		gw, err = azureiothub.NewBackend(config.C)
	default:
		return fmt.Errorf("unexpected gateway backend type: %s", config.C.NetworkServer.Gateway.Backend.Type)
	}

	if err != nil {
		return errors.Wrap(err, "gateway-backend setup failed")
	}

	gwbackend.SetBackend(gw)
	return nil
}

func setupApplicationServer() error {
	if err := applicationserver.Setup(); err != nil {
		return errors.Wrap(err, "application-server setup error")
	}
	return nil
}

func setupGeolocationServer() error {
	// TODO: move setup to gelolocation.Setup
	if config.C.GeolocationServer.Server == "" {
		log.Info("no geolocation-server configured")
		return nil
	}

	log.WithFields(log.Fields{
		"server":   config.C.GeolocationServer.Server,
		"ca_cert":  config.C.GeolocationServer.CACert,
		"tls_cert": config.C.GeolocationServer.TLSCert,
		"tls_key":  config.C.GeolocationServer.TLSKey,
	}).Info("connecting to geolocation-server")

	var dialOptions []grpc.DialOption
	if config.C.GeolocationServer.TLSCert != "" && config.C.GeolocationServer.TLSKey != "" {
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(
			mustGetTransportCredentials(config.C.GeolocationServer.TLSCert, config.C.GeolocationServer.TLSKey, config.C.GeolocationServer.CACert, false),
		))
	} else {
		dialOptions = append(dialOptions, grpc.WithInsecure())
	}

	geoConn, err := grpc.Dial(config.C.GeolocationServer.Server, dialOptions...)
	if err != nil {
		return errors.Wrap(err, "geolocation-server dial error")
	}

	geolocationserver.SetClient(geo.NewGeolocationServerServiceClient(geoConn))

	return nil
}

func setupJoinServer() error {
	if err := joinserver.Setup(config.C); err != nil {
		return errors.Wrap(err, "setup join-server backend error")
	}
	return nil
}

func setupNetworkController() error {
	// TODO: move this logic to controller.Setup function
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
		ncClient := nc.NewNetworkControllerServiceClient(ncConn)
		controller.SetClient(ncClient)
	}

	return nil
}

func setupUplink() error {
	if err := uplink.Setup(config.C); err != nil {
		return errors.Wrap(err, "setup link error")
	}
	return nil
}

func setupDownlink() error {
	if err := downlink.Setup(config.C); err != nil {
		return errors.Wrap(err, "setup downlink error")
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
	ns.RegisterNetworkServerServiceServer(gs, nsAPI)

	ln, err := net.Listen("tcp", config.C.NetworkServer.API.Bind)
	if err != nil {
		return errors.Wrap(err, "start api listener error")
	}
	go gs.Serve(ln)
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
	go downlink.DeviceQueueSchedulerLoop()

	log.Info("starting multicast scheduler")
	go downlink.MulticastQueueSchedulerLoop()

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

	var caCertPool *x509.CertPool
	if caCert != "" {
		rawCaCert, err := ioutil.ReadFile(caCert)
		if err != nil {
			log.WithField("ca", caCert).Fatalf("load ca cert error: %s", err)
		}

		caCertPool = x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(rawCaCert) {
			log.WithField("ca_cert", caCert).Fatal("append ca certificate error")
		}
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

func fixV2RedisCache() error {
	return code.Migrate("v1_to_v2_flush_profiles_cache", func(db sqlx.Ext) error {
		return code.FlushProfilesCache(storage.RedisPool(), db)
	})
}

func migrateGatewayStats() error {
	return code.Migrate("migrate_gateway_stats_to_redis", func(db sqlx.Ext) error {
		return code.MigrateGatewayStats(storage.RedisPool(), db)
	})
}
