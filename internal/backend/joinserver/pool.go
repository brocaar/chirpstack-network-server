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

// ErrServerNotFound indicates that no server is configured for the given JoinEUI.
var ErrServerNotFound = errors.New("server not found")

// Pool defines the join-server client pool.
type Pool interface {
	Get(joinEUI lorawan.EUI64) (Client, error)
}

type poolClient struct {
	client Client
}

type pool struct {
	sync.RWMutex
	defaultClient       Client
	resolveJoinEUI      bool
	clients             map[lorawan.EUI64]poolClient
	servers             []server
	resolveDomainSuffix string
}

type server struct {
	server  string
	joinEUI lorawan.EUI64
	caCert  string
	tlsCert string
	tlsKey  string
}

// Get returns the join-server client for the given joinEUI.
func (p *pool) Get(joinEUI lorawan.EUI64) (Client, error) {
	// return from cache
	p.RLock()
	pc, ok := p.clients[joinEUI]
	p.RUnlock()
	if ok {
		return pc.client, nil
	}

	client := p.getClient(joinEUI)
	if client != p.defaultClient {
		p.Lock()
		p.clients[joinEUI] = poolClient{client: client}
		p.Unlock()
	}

	return client, nil
}

func (p *pool) getClient(joinEUI lorawan.EUI64) Client {
	var err error
	s := p.getServer(joinEUI)

	// if the server endpoint is empty and resolve JoinEUI is enabled, try to get it from DNS
	if s.server == "" && p.resolveJoinEUI {
		s.server, err = p.resolveJoinEUIToJoinServerURL(joinEUI)
		if err != nil {
			log.WithFields(log.Fields{
				"join_eui": joinEUI,
			}).WithError(err).Warning("resolving JoinEUI failed, returning default join-server client")
			return p.defaultClient
		}
	}

	// if the server endpoint is set, return the client
	if s.server != "" {
		c, err := NewClient(s.server, s.caCert, s.tlsCert, s.tlsKey)
		if err != nil {
			log.WithFields(log.Fields{
				"join_eui": joinEUI,
				"server":   s.server,
			}).WithError(err).Error("creating join-server client failed, returning default join-server client")
			return p.defaultClient
		}

		return c
	}

	// in any other case, return the default client
	return p.defaultClient
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

	return strings.Join(nibbles, ".") + p.resolveDomainSuffix
}

func (p *pool) getServer(joinEUI lorawan.EUI64) server {
	for _, s := range p.servers {
		if s.joinEUI == joinEUI {
			return s
		}
	}

	return server{}
}
