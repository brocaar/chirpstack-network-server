package joinserver

import (
	"github.com/pkg/errors"

	"github.com/brocaar/loraserver/internal/config"
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

	var certificates []certificate
	for _, cert := range conf.Certificates {
		var eui lorawan.EUI64
		if err := eui.UnmarshalText([]byte(cert.JoinEUI)); err != nil {
			return errors.Wrap(err, "joinserver: unmarshal JoinEUI error")
		}

		certificates = append(certificates, certificate{
			joinEUI: eui,
			caCert:  cert.CACert,
			tlsCert: cert.TLSCert,
			tlsKey:  cert.TLSKey,
		})
	}

	p = &pool{
		defaultClient:       defaultClient,
		resolveJoinEUI:      conf.ResolveJoinEUI,
		resolveDomainSuffix: conf.ResolveDomainSuffix,
		clients:             make(map[lorawan.EUI64]poolClient),
		certificates:        certificates,
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
