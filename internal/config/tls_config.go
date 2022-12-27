package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
)

func NewTLSConfig(caFileLocation string, certificateFileLocation string, certificateKeyFileLocation string, isServer bool) (*tls.Config, error) {
	var err error
	tlsConfig := &tls.Config{}

	if certificateFileLocation != "" && certificateKeyFileLocation != "" {
		tlsConfig.Certificates = make([]tls.Certificate, 1)
		tlsConfig.Certificates[0], err = tls.LoadX509KeyPair(
			certificateFileLocation,
			certificateKeyFileLocation,
		)
	}

	if caFileLocation != "" {
		b, err := ioutil.ReadFile(caFileLocation)
		if err != nil {
			return nil, err
		}

		ca := x509.NewCertPool()
		ok := ca.AppendCertsFromPEM([]byte(b))

		if !ok {
			return nil, fmt.Errorf(
				"failed to parse root certificate: %q",
				caFileLocation,
			)
		}

		if isServer {
			tlsConfig.ClientCAs = ca
			tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		} else {
			tlsConfig.RootCAs = ca
		}

		// TODO: should we configure the server name?
		tlsConfig.ServerName = "gitstafette-server"
	}

	return tlsConfig, err
}
