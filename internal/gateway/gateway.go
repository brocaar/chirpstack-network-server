package gateway

import (
	"time"

	"github.com/pkg/errors"

	"github.com/brocaar/chirpstack-network-server/v3/internal/config"
)

var (
	statsHandler *StatsHandler
	caCert       string
	caKey        string
	tlsLifetime  time.Duration
)

// Setup configures the gateway package.
func Setup(c config.Config) error {
	conf := c.NetworkServer.Gateway

	statsHandler = &StatsHandler{}
	if err := statsHandler.Start(); err != nil {
		return errors.Wrap(err, "start stats handler error")
	}

	caCert = conf.CACert
	caKey = conf.CAKey
	tlsLifetime = conf.ClientCertLifetime

	return nil
}

// Stop stops the gateway stats handler.
func Stop() error {
	return statsHandler.Stop()
}
