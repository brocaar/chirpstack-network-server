package joinserver

import (
	"github.com/pkg/errors"

	"github.com/brocaar/chirpstack-network-server/internal/config"
	"github.com/brocaar/lorawan"
)

var p Pool

// Setup sets up the joinserver backend.
func Setup(c config.Config) error {
	conf := c.JoinServer

	defaultClient, err := NewClient(
		conf.Default.Server,
		conf.Default.CACert,
		conf.Default.TLSCert,
		conf.Default.TLSKey,
	)
	if err != nil {
		return errors.Wrap(err, "joinserver: create default client error")
	}

	var servers []server
	for _, s := range conf.Servers {
		var eui lorawan.EUI64
		if err := eui.UnmarshalText([]byte(s.JoinEUI)); err != nil {
			return errors.Wrap(err, "joinserver: unmarshal JoinEUI error")
		}

		servers = append(servers, server{
			server:  s.Server,
			joinEUI: eui,
			caCert:  s.CACert,
			tlsCert: s.TLSCert,
			tlsKey:  s.TLSKey,
		})
	}

	p = &pool{
		defaultClient:       defaultClient,
		resolveJoinEUI:      conf.ResolveJoinEUI,
		resolveDomainSuffix: conf.ResolveDomainSuffix,
		clients:             make(map[lorawan.EUI64]poolClient),
		servers:             servers,
	}

	return nil
}

// GetPool returns the joinserver pool.
func GetPool() Pool {
	return p
}

// SetPool sets the given join-server pool.
func SetPool(pp Pool) {
	p = pp
}
