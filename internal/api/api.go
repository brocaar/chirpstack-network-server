package api

import (
	"net"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_logrus "github.com/grpc-ecosystem/go-grpc-middleware/logging/logrus"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	"github.com/brocaar/chirpstack-network-server/api/ns"
	"github.com/brocaar/chirpstack-network-server/internal/config"
	"github.com/brocaar/chirpstack-network-server/internal/logging"
	"github.com/brocaar/chirpstack-network-server/internal/tls"
)

func Setup(c config.Config) error {
	apiConfig := c.NetworkServer.API

	log.WithFields(log.Fields{
		"bind":     apiConfig.Bind,
		"ca-cert":  apiConfig.CACert,
		"tls-cert": apiConfig.TLSCert,
		"tls-key":  apiConfig.TLSKey,
	}).Info("api: starting network-server api server")

	opts := serverOptions()

	if apiConfig.CACert != "" || apiConfig.TLSCert != "" || apiConfig.TLSKey != "" {
		creds, err := tls.GetTransportCredentials(apiConfig.CACert, apiConfig.TLSCert, apiConfig.TLSKey, true)
		if err != nil {
			return errors.Wrap(err, "get transport credentials error")
		}

		opts = append(opts, grpc.Creds(creds))
	}

	gs := grpc.NewServer(opts...)
	nsAPI := NewNetworkServerAPI()
	ns.RegisterNetworkServerServiceServer(gs, nsAPI)

	ln, err := net.Listen("tcp", apiConfig.Bind)
	if err != nil {
		return errors.Wrap(err, "start api listener error")
	}
	go gs.Serve(ln)

	return nil
}

func serverOptions() []grpc.ServerOption {
	logrusEntry := log.NewEntry(log.StandardLogger())
	logrusOpts := []grpc_logrus.Option{
		grpc_logrus.WithLevels(grpc_logrus.DefaultCodeToLevel),
	}

	return []grpc.ServerOption{
		grpc_middleware.WithUnaryServerChain(
			grpc_ctxtags.UnaryServerInterceptor(grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor)),
			grpc_logrus.UnaryServerInterceptor(logrusEntry, logrusOpts...),
			logging.UnaryServerCtxIDInterceptor,
			grpc_prometheus.UnaryServerInterceptor,
		),
		grpc_middleware.WithStreamServerChain(
			grpc_ctxtags.StreamServerInterceptor(grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor)),
			grpc_logrus.StreamServerInterceptor(logrusEntry, logrusOpts...),
			grpc_prometheus.StreamServerInterceptor,
		),
	}
}
