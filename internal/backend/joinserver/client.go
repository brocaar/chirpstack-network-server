package joinserver

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/lorawan/backend"
)

// Client defines the join-server client interface.
type Client interface {
	JoinReq(ctx context.Context, pl backend.JoinReqPayload) (backend.JoinAnsPayload, error)
	RejoinReq(ctx context.Context, pl backend.RejoinReqPayload) (backend.RejoinAnsPayload, error)
}

type client struct {
	server     string
	httpClient *http.Client
}

// JoinReq issues a join-request.
func (c *client) JoinReq(ctx context.Context, pl backend.JoinReqPayload) (backend.JoinAnsPayload, error) {
	var ans backend.JoinAnsPayload

	b, err := json.Marshal(pl)
	if err != nil {
		return ans, errors.Wrap(err, "marshal request error")
	}

	resp, err := c.httpClient.Post(c.server, "application/json", bytes.NewReader(b))
	if err != nil {
		return ans, errors.Wrap(err, "http post error")
	}
	defer resp.Body.Close()

	err = json.NewDecoder(resp.Body).Decode(&ans)
	if err != nil {
		return ans, errors.Wrap(err, "unmarshal response error")
	}

	if ans.Result.ResultCode != backend.Success {
		return ans, fmt.Errorf("response error, code: %s, description: %s", ans.Result.ResultCode, ans.Result.Description)
	}

	return ans, nil
}

// RejoinReq issues a rejoin-request.
func (c *client) RejoinReq(ctx context.Context, pl backend.RejoinReqPayload) (backend.RejoinAnsPayload, error) {
	var ans backend.RejoinAnsPayload

	b, err := json.Marshal(pl)
	if err != nil {
		return ans, errors.Wrap(err, "marshal request error")
	}

	resp, err := c.httpClient.Post(c.server, "application/json", bytes.NewReader(b))
	if err != nil {
		return ans, errors.Wrap(err, "http post error")
	}
	defer resp.Body.Close()

	err = json.NewDecoder(resp.Body).Decode(&ans)
	if err != nil {
		return ans, errors.Wrap(err, "unmarshal response error")
	}

	if ans.Result.ResultCode != backend.Success {
		return ans, fmt.Errorf("response error, code: %s, description: %s", ans.Result.ResultCode, ans.Result.Description)
	}

	return ans, nil
}

// NewClient creates a new join-server client.
// If the caCert is set, it will configure the CA certificate to validate the
// join-server server certificate. When the tlsCert and tlsKey are set, then
// these will be configured as client-certificates for authentication.
func NewClient(server, caCert, tlsCert, tlsKey string) (Client, error) {
	log.WithFields(log.Fields{
		"server":   server,
		"ca_cert":  caCert,
		"tls_cert": tlsCert,
		"tls_key":  tlsKey,
	}).Info("configuring join-server client")

	if caCert == "" && tlsCert == "" && tlsKey == "" {
		return &client{
			server:     server,
			httpClient: http.DefaultClient,
		}, nil
	}

	tlsConfig := &tls.Config{}

	if caCert != "" {
		rawCACert, err := ioutil.ReadFile(caCert)
		if err != nil {
			return nil, errors.Wrap(err, "load ca cert error")
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(rawCACert) {
			return nil, errors.New("append ca cert to pool error")
		}

		tlsConfig.RootCAs = caCertPool
	}

	if tlsCert != "" || tlsKey != "" {
		cert, err := tls.LoadX509KeyPair(tlsCert, tlsKey)
		if err != nil {
			return nil, errors.Wrap(err, "load x509 keypair error")
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return &client{
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
		},
		server: server,
	}, nil
}
