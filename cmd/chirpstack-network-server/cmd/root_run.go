package cmd

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/resolver"

	"github.com/brocaar/chirpstack-api/go/v3/nc"
	"github.com/brocaar/chirpstack-network-server/v3/internal/adr"
	"github.com/brocaar/chirpstack-network-server/v3/internal/api/ns"
	roamingapi "github.com/brocaar/chirpstack-network-server/v3/internal/api/roaming"
	"github.com/brocaar/chirpstack-network-server/v3/internal/backend/applicationserver"
	"github.com/brocaar/chirpstack-network-server/v3/internal/backend/controller"
	gwbackend "github.com/brocaar/chirpstack-network-server/v3/internal/backend/gateway"
	"github.com/brocaar/chirpstack-network-server/v3/internal/backend/gateway/amqp"
	"github.com/brocaar/chirpstack-network-server/v3/internal/backend/gateway/azureiothub"
	"github.com/brocaar/chirpstack-network-server/v3/internal/backend/gateway/gcppubsub"
	"github.com/brocaar/chirpstack-network-server/v3/internal/backend/gateway/mqtt"
	"github.com/brocaar/chirpstack-network-server/v3/internal/backend/joinserver"
	"github.com/brocaar/chirpstack-network-server/v3/internal/band"
	"github.com/brocaar/chirpstack-network-server/v3/internal/config"
	"github.com/brocaar/chirpstack-network-server/v3/internal/downlink"
	"github.com/brocaar/chirpstack-network-server/v3/internal/gateway"
	"github.com/brocaar/chirpstack-network-server/v3/internal/monitoring"
	"github.com/brocaar/chirpstack-network-server/v3/internal/roaming"
	"github.com/brocaar/chirpstack-network-server/v3/internal/storage"
	"github.com/brocaar/chirpstack-network-server/v3/internal/uplink"
)

func run(cmd *cobra.Command, args []string) error {
	var server = new(uplink.Server)

	if cpuprofile != "" {
		f, err := os.Create(cpuprofile)
		if err != nil {
			return errors.Wrap(err, "could not create cpu profile file")
		}
		defer f.Close()

		if err := pprof.StartCPUProfile(f); err != nil {
			return errors.Wrap(err, "could not start cpu profile")
		}
		defer pprof.StopCPUProfile()
	}

	tasks := []func() error{
		setLogLevel,
		setSyslog,
		setGRPCResolver,
		setupBand,
		setRXParameters,
		printStartMessage,
		setupMonitoring,
		enableUplinkChannels,
		setupStorage,
		setGatewayBackend,
		setupApplicationServer,
		setupADR,
		setupJoinServer,
		setupNetworkController,
		setupUplink,
		setupDownlink,
		setupNetworkServerAPI,
		setupRoaming,
		setupGateways,
		startLoRaServer(server),
		startQueueScheduler,
	}

	for _, t := range tasks {
		if err := t(); err != nil {
			log.Fatal(err)
		}
	}

	sigChan := make(chan os.Signal, 1)
	exitChan := make(chan struct{})
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	log.WithField("signal", <-sigChan).Info("signal received")
	go func() {
		log.Warning("stopping chirpstack-network-server")
		if err := server.Stop(); err != nil {
			log.Fatal(err)
		}
		if err := gateway.Stop(); err != nil {
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

func setGRPCResolver() error {
	resolver.SetDefaultScheme(config.C.General.GRPCDefaultResolverScheme)
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
		config.C.NetworkServer.NetworkSettings.RX2Frequency = int64(defaults.RX2Frequency)
	}

	return nil
}

// TODO: cleanup and put in Setup functions.
func setupMonitoring() error {
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

	if err := monitoring.Setup(config.C); err != nil {
		return errors.Wrap(err, "setup metrics error")
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
		"docs":    "https://www.chirpstack.io/",
	}).Info("starting ChirpStack Network Server")
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
			return errors.Wrap(err, "enable uplink channel error")
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
		return errors.Wrap(err, "setup adr error")
	}
	return nil
}

func setGatewayBackend() error {
	var err error
	var gw gwbackend.Gateway

	switch config.C.NetworkServer.Gateway.Backend.Type {
	case "mqtt":
		gw, err = mqtt.NewBackend(
			config.C,
		)
	case "amqp":
		gw, err = amqp.NewBackend(config.C)
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
		ncDialOptions := []grpc.DialOption{
			grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
		}
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

func setupNetworkServerAPI() error {
	if err := ns.Setup(config.C); err != nil {
		return errors.Wrap(err, "setup network-server api error")
	}

	return nil
}

func setupRoaming() error {
	if err := roamingapi.Setup(config.C); err != nil {
		return errors.Wrap(err, "setup roaming api error")
	}

	if err := roaming.Setup(config.C); err != nil {
		return errors.Wrap(err, "setup roaming error")
	}

	return nil
}

func startLoRaServer(server *uplink.Server) func() error {
	return func() error {
		*server = *uplink.NewServer()
		return server.Start()
	}
}

func setupGateways() error {
	if err := gateway.Setup(config.C); err != nil {
		return errors.Wrap(err, "setup gateway error")
	}
	return nil
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
