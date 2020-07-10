package joinserver

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/brocaar/chirpstack-network-server/internal/config"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
)

var p Pool

type serverItem struct {
	joinEUI lorawan.EUI64
	client  backend.Client
}

var (
	servers []serverItem
	keks    map[string][]byte

	netID          lorawan.NetID
	defaultServer  string
	defaultCACert  string
	defaultTLSCert string
	defaultTLSKey  string
)

// Setup sets up the joinserver backend.
func Setup(c config.Config) error {
	conf := c.JoinServer
	keks = make(map[string][]byte)

	netID = c.NetworkServer.NetID
	defaultServer = c.JoinServer.Default.Server
	defaultCACert = c.JoinServer.Default.CACert
	defaultTLSCert = c.JoinServer.Default.TLSCert
	defaultTLSKey = c.JoinServer.Default.TLSKey

	for _, s := range conf.Servers {
		var joinEUI lorawan.EUI64
		if err := joinEUI.UnmarshalText([]byte(s.JoinEUI)); err != nil {
			return errors.Wrap(err, "decode joineui error")
		}

		if s.Server == "" {
			s.Server = joinEUIToServer(joinEUI, conf.ResolveDomainSuffix)
		}

		client, err := backend.NewClient(backend.ClientConfig{
			SenderID:   c.NetworkServer.NetID.String(),
			ReceiverID: joinEUI.String(),
			Server:     s.Server,
			CACert:     s.CACert,
			TLSCert:    s.TLSCert,
			TLSKey:     s.TLSKey,
		})
		if err != nil {
			return errors.Wrap(err, "new backend client error")
		}

		servers = append(servers, serverItem{
			joinEUI: joinEUI,
			client:  client,
		})
	}

	for _, k := range conf.KEK.Set {
		kek, err := hex.DecodeString(k.KEK)
		if err != nil {
			return errors.Wrap(err, "decode kek error")
		}

		keks[k.Label] = kek
	}

	/// TODO: cleanup the old pool and client in favor of the backend.Client.
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

// GetClientForJoinEUI returns the backend client for the given JoinEUI.
func GetClientForJoinEUI(joinEUI lorawan.EUI64) (backend.Client, error) {
	for _, s := range servers {
		if s.joinEUI == joinEUI {
			return s.client, nil
		}
	}

	defaultClient, err := backend.NewClient(backend.ClientConfig{
		SenderID:   netID.String(),
		ReceiverID: joinEUI.String(),
		Server:     defaultServer,
		CACert:     defaultCACert,
		TLSCert:    defaultTLSCert,
		TLSKey:     defaultTLSKey,
	})
	if err != nil {
		return nil, errors.Wrap(err, "joinserver: new default client error")
	}

	return defaultClient, nil
}

// GetKEKKey returns the KEK key for the given label.
func GetKEKKey(label string) ([]byte, error) {
	kek, ok := keks[label]
	if !ok {
		return nil, fmt.Errorf("kek label '%s' is not configured", label)
	}
	return kek, nil
}

func joinEUIToServer(joinEUI lorawan.EUI64, domain string) string {
	nibbles := strings.Split(joinEUI.String(), "")

	for i, j := 0, len(nibbles)-1; i < j; i, j = i+1, j-1 {
		nibbles[i], nibbles[j] = nibbles[j], nibbles[i]
	}

	return "https://" + strings.Join(nibbles, ".") + domain
}
