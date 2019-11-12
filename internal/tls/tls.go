package tls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"

	"github.com/pkg/errors"
	"google.golang.org/grpc/credentials"
)

// GetTransportCredentials returns the gRPC transport credentials.
func GetTransportCredentials(caCert, tlsCert, tlsKey string, verifyClientCert bool) (credentials.TransportCredentials, error) {
	cert, err := tls.LoadX509KeyPair(tlsCert, tlsKey)
	if err != nil {
		return nil, errors.Wrap(err, "load tls key-pair error")
	}

	var caCertPool *x509.CertPool
	if caCert != "" {
		rawCaCert, err := ioutil.ReadFile(caCert)
		if err != nil {
			return nil, errors.Wrap(err, "load ca certificate error")
		}

		caCertPool = x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(rawCaCert) {
			return nil, fmt.Errorf("append ca certificate error: %s", caCert)
		}
	}

	if verifyClientCert {
		return credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{cert},
			ClientCAs:    caCertPool,
			ClientAuth:   tls.RequireAndVerifyClientCert,
		}), nil
	}

	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
	}), nil
}
