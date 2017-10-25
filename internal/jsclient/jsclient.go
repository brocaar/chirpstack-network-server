package jsclient

import (
	"bytes"
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
	JoinReq(pl backend.JoinReqPayload) (backend.JoinAnsPayload, error)
}

type client struct {
	server     string
	httpClient *http.Client
}

// JoinReq issues a join-request.
func (c *client) JoinReq(pl backend.JoinReqPayload) (backend.JoinAnsPayload, error) {
	var ans backend.JoinAnsPayload

	b, err := json.Marshal(pl)
	if err != nil {
		return ans, errors.Wrap(err, "marshal request error")
	}

	resp, err := c.httpClient.Post(c.server, "application/json", bytes.NewReader(b))
	if err != nil {
		return ans, errors.Wrap(err, "http post error")
	}

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
func NewClient(server, caCert, tlsCert, tlsKey string) (Client, error) {
	log.WithFields(log.Fields{
		"server":   server,
		"ca_cert":  caCert,
		"tls_cert": tlsCert,
		"tls_key":  tlsKey,
	}).Info("configuring join-server")

	if caCert == "" && tlsCert == "" && tlsKey == "" {
		return &client{
			server:     server,
			httpClient: http.DefaultClient,
		}, nil
	}

	cert, err := tls.LoadX509KeyPair(tlsCert, tlsKey)
	if err != nil {
		return nil, errors.Wrap(err, "load x509 keypair error")
	}

	rawCACert, err := ioutil.ReadFile(caCert)
	if err != nil {
		return nil, errors.Wrap(err, "load ca cert error")
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(rawCACert) {
		return nil, errors.New("append ca cert to pool error")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
	}
	// tlsConfig.BuildNameToCertificate(uildNameToCertificate()

	return &client{
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
		},
		server: server,
	}, nil
}
