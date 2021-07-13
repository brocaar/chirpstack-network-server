package gateway

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io/ioutil"
	"log"
	"math/big"
	"time"

	"github.com/pkg/errors"

	"github.com/brocaar/lorawan"
)

// GenerateClientCertificate returns a client-certificate for the given gateway ID.
func GenerateClientCertificate(gatewayID lorawan.EUI64) (time.Time, []byte, []byte, []byte, error) {
	if caCert == "" || caKey == "" {
		return time.Time{}, nil, nil, nil, errors.New("no ca certificate or ca key configured")
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		log.Fatalf("Failed to generate serial number: %v", err)
	}

	caCertB, err := ioutil.ReadFile(caCert)
	if err != nil {
		return time.Time{}, nil, nil, nil, errors.Wrap(err, "read ca cert file error")
	}

	caKeyPair, err := tls.LoadX509KeyPair(caCert, caKey)
	if err != nil {
		return time.Time{}, nil, nil, nil, errors.Wrap(err, "load ca key-pair error")
	}

	caCert, err := x509.ParseCertificate(caKeyPair.Certificate[0])
	if err != nil {
		return time.Time{}, nil, nil, nil, errors.Wrap(err, "parse certificate error")
	}

	expiresAt := time.Now().Add(tlsLifetime)

	cert := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: gatewayID.String(),
		},
		NotBefore:   time.Now(),
		NotAfter:    expiresAt,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
	}

	certPrivKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return time.Time{}, nil, nil, nil, errors.Wrap(err, "generate key error")

	}

	certBytes, err := x509.CreateCertificate(rand.Reader, cert, caCert, &certPrivKey.PublicKey, caKeyPair.PrivateKey)
	if err != nil {
		return time.Time{}, nil, nil, nil, errors.Wrap(err, "create certificate error")

	}

	caPEM := new(bytes.Buffer)
	pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caKeyPair.Certificate[0],
	})

	certPEM := new(bytes.Buffer)
	pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	b, err := x509.MarshalECPrivateKey(certPrivKey)
	if err != nil {
		return time.Time{}, nil, nil, nil, errors.Wrap(err, "create certificate error")
	}

	certPrivKeyPEM := new(bytes.Buffer)
	pem.Encode(certPrivKeyPEM, &pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: b,
	})

	return expiresAt, caCertB, certPEM.Bytes(), certPrivKeyPEM.Bytes(), nil
}
