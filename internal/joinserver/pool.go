package joinserver

import (
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/lorawan"
)

// Pool defines the join-server client pool.
type Pool interface {
	Get(joinEUI lorawan.EUI64) (Client, error)
}

type poolClient struct {
	client Client
}

type pool struct {
	sync.RWMutex
	config        Config
	defaultClient Client
	clients       map[lorawan.EUI64]poolClient
}

// NewPool creates a new Pool.
func NewPool(config Config) (Pool, error) {
	defaultClient, err := NewClient(
		config.Default.Server,
		config.Default.CACert,
		config.Default.TLSCert,
		config.Default.TLSKey,
	)
	if err != nil {
		return nil, errors.Wrap(err, "create default join-server client error")
	}
	return &pool{
		defaultClient: defaultClient,
		config:        config,
		clients:       make(map[lorawan.EUI64]poolClient),
	}, nil
}

// Get returns the join-server client for the given joinEUI.
func (p *pool) Get(joinEUI lorawan.EUI64) (Client, error) {
	if !p.config.ResolveJoinEUI {
		return p.defaultClient, nil
	}

	p.RLock()
	pc, ok := p.clients[joinEUI]
	p.RUnlock()
	if ok {
		return pc.client, nil
	}

	client, err := p.resolveJoinServer(joinEUI)
	if err != nil {
		log.WithField("join_eui", joinEUI).WithError(err).Warning("resolving JoinEUI failed, using default join-server")
		return p.defaultClient, nil
	}

	p.Lock()
	p.clients[joinEUI] = poolClient{client: client}
	p.Unlock()

	return client, nil
}

func (p *pool) resolveJoinServer(joinEUI lorawan.EUI64) (Client, error) {
	// resolve the join-server EUI to an url (using DNS)
	server, err := p.resolveJoinEUIToJoinServerURL(joinEUI)
	if err != nil {
		return nil, errors.Wrap(err, "resolve joineui to join-server url error")
	}

	log.WithFields(log.Fields{
		"join_eui": joinEUI,
		"server":   server,
	}).Debug("resolved joineui to join-server")

	var caCert, tlsCert, tlsKey string
	for _, cert := range p.config.Certificates {
		var certJoinEUI lorawan.EUI64
		if err := certJoinEUI.UnmarshalText([]byte(cert.JoinEUI)); err != nil {
			return nil, errors.Wrapf(err, "unmarshal joineui '%s' error", cert.JoinEUI)
		}

		if certJoinEUI == joinEUI {
			caCert = cert.CaCert
			tlsCert = cert.TLSCert
			tlsKey = cert.TLSKey
		}
	}

	return NewClient(server, caCert, tlsCert, tlsKey)
}

func (p *pool) resolveJoinEUIToJoinServerURL(joinEUI lorawan.EUI64) (string, error) {
	server := p.joinEUIToServer(joinEUI)

	return p.aToURL(server, true, 443)
}

func (p *pool) aToURL(server string, secure bool, port int) (string, error) {
	_, err := net.LookupIP(server)
	if err != nil {
		return "", errors.Wrap(err, "lookup ip failed")
	}

	var protocol string
	if secure {
		protocol = "https://"
	} else {
		protocol = "http://"
	}

	return fmt.Sprintf("%s%s:%d/", protocol, server, port), nil
}

func (p *pool) joinEUIToServer(joinEUI lorawan.EUI64) string {
	nibbles := strings.Split(joinEUI.String(), "")

	for i, j := 0, len(nibbles)-1; i < j; i, j = i+1, j-1 {
		nibbles[i], nibbles[j] = nibbles[j], nibbles[i]
	}

	return strings.Join(nibbles, ".") + p.config.ResolveDomainSuffix
}
