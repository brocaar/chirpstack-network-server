package asclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"sync"
	"time"

	grpc_logrus "github.com/grpc-ecosystem/go-grpc-middleware/logging/logrus"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/brocaar/chirpstack-api/go/v3/as"
	"github.com/brocaar/chirpstack-network-server/internal/logging"
)

// Pool defines the application-server client pool.
type Pool interface {
	Get(hostname string, caCert, tlsCert, tlsKey []byte) (as.ApplicationServerServiceClient, error)
}

type client struct {
	client     as.ApplicationServerServiceClient
	clientConn *grpc.ClientConn
	caCert     []byte
	tlsCert    []byte
	tlsKey     []byte
}

type pool struct {
	sync.RWMutex
	clients map[string]client
}

// NewPool creates a new Pool.
func NewPool() Pool {
	return &pool{
		clients: make(map[string]client),
	}
}

// Get Returns an ApplicationServerClient for the given server (hostname:ip).
func (p *pool) Get(hostname string, caCert, tlsCert, tlsKey []byte) (as.ApplicationServerServiceClient, error) {
	defer p.Unlock()
	p.Lock()

	var connect bool
	c, ok := p.clients[hostname]
	if !ok {
		connect = true
	}

	// if the connection exists in the map, but when the certificates changed
	// try to close the connection and re-connect
	if ok && (!bytes.Equal(c.caCert, caCert) || !bytes.Equal(c.tlsCert, tlsCert) || !bytes.Equal(c.tlsKey, tlsKey)) {
		c.clientConn.Close()
		delete(p.clients, hostname)
		connect = true
	}

	if connect {
		clientConn, asClient, err := p.createClient(hostname, caCert, tlsCert, tlsKey)
		if err != nil {
			return nil, errors.Wrap(err, "create application-server api client error")
		}
		c = client{
			client:     asClient,
			clientConn: clientConn,
			caCert:     caCert,
			tlsCert:    tlsCert,
			tlsKey:     tlsKey,
		}
		p.clients[hostname] = c
	}

	return c.client, nil
}

func (p *pool) createClient(hostname string, caCert, tlsCert, tlsKey []byte) (*grpc.ClientConn, as.ApplicationServerServiceClient, error) {
	logrusEntry := log.NewEntry(log.StandardLogger())
	logrusOpts := []grpc_logrus.Option{
		grpc_logrus.WithLevels(grpc_logrus.DefaultCodeToLevel),
	}

	asOpts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithUnaryInterceptor(
			logging.UnaryClientCtxIDInterceptor,
		),
		grpc.WithStreamInterceptor(
			grpc_logrus.StreamClientInterceptor(logrusEntry, logrusOpts...),
		),
	}

	if len(tlsCert) == 0 && len(tlsKey) == 0 && len(caCert) == 0 {
		asOpts = append(asOpts, grpc.WithInsecure())
		log.WithField("server", hostname).Warning("creating insecure application-server client")
	} else {
		log.WithField("server", hostname).Info("creating application-server client")
		cert, err := tls.X509KeyPair(tlsCert, tlsKey)
		if err != nil {
			return nil, nil, errors.Wrap(err, "load x509 keypair error")
		}

		var caCertPool *x509.CertPool
		if len(caCert) != 0 {
			caCertPool = x509.NewCertPool()
			if !caCertPool.AppendCertsFromPEM(caCert) {
				return nil, nil, errors.Wrap(err, "append ca cert to pool error")
			}
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
		return nil, nil, errors.Wrap(err, "dial application-server api error")
	}

	return asClient, as.NewApplicationServerServiceClient(asClient), nil
}
