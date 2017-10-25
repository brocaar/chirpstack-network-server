package asclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/credentials"

	"github.com/brocaar/loraserver/api/as"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

// Pool defines the application-server client pool.
type Pool interface {
	Get(hostname string) (as.ApplicationServerClient, error)
}

type client struct {
	lastUsed time.Time
	client   as.ApplicationServerClient
}

type pool struct {
	sync.RWMutex
	caCert  string
	tlsCert string
	tlsKey  string
	clients map[string]client
}

// NewPool creates a new Pool.
func NewPool(caCert, tlsCert, tlsKey string) Pool {
	log.WithFields(log.Fields{
		"ca_cert":  caCert,
		"tls_cert": tlsCert,
		"tls_key":  tlsKey,
	}).Info("setup application-server client pool")

	return &pool{
		caCert:  caCert,
		tlsCert: tlsCert,
		tlsKey:  tlsKey,
		clients: make(map[string]client),
	}
}

// Get Returns an ApplicationServerClient for the given server (hostname:ip).
func (p *pool) Get(hostname string) (as.ApplicationServerClient, error) {
	defer p.Unlock()
	p.Lock()

	c, ok := p.clients[hostname]
	if !ok {
		asClient, err := p.createClient(hostname)
		if err != nil {
			return nil, errors.Wrap(err, "create application-server api client error")
		}
		c = client{
			lastUsed: time.Now(),
			client:   asClient,
		}
		p.clients[hostname] = c
	}

	return c.client, nil
}

func (p *pool) createClient(hostname string) (as.ApplicationServerClient, error) {
	asOpts := []grpc.DialOption{
		grpc.WithBlock(),
	}

	if p.tlsCert == "" && p.tlsKey == "" && p.caCert == "" {
		asOpts = append(asOpts, grpc.WithInsecure())
	} else {
		cert, err := tls.LoadX509KeyPair(p.tlsCert, p.tlsKey)
		if err != nil {
			return nil, errors.Wrap(err, "load x509 keypair error")
		}

		rawCACert, err := ioutil.ReadFile(p.caCert)
		if err != nil {
			return nil, errors.Wrap(err, "load ca cert error")
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(rawCACert) {
			return nil, errors.Wrap(err, "append ca cert to pool error")
		}

		asOpts = append(asOpts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{cert},
			RootCAs:      caCertPool,
		})))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	asClient, err := grpc.DialContext(ctx, hostname, asOpts...)
	if err != nil {
		return nil, errors.Wrap(err, "dial application-server api error")
	}

	return as.NewApplicationServerClient(asClient), nil
}
