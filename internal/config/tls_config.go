package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"log"
)

func NewTLSConfig(caFileLocation string, certificateFileLocation string, certificateKeyFileLocation string, isServer bool) (*tls.Config, error) {
	if certificateFileLocation == "" && certificateKeyFileLocation == "" && caFileLocation == "" {
		return nil, nil
	}
	var err error
	tlsConfig := &tls.Config{}

	if certificateFileLocation != "" && certificateKeyFileLocation != "" {
		tlsConfig.Certificates = make([]tls.Certificate, 1)
		tlsConfig.Certificates[0], err = tls.LoadX509KeyPair(
			certificateFileLocation,
			certificateKeyFileLocation,
		)
	} else {
		log.Printf("Did not find a certificate with a key, no TLS certs")
	}

	if caFileLocation != "" {
		b, err := ioutil.ReadFile(caFileLocation)
		if err != nil {
			return nil, err
		}

		ca, err := x509.SystemCertPool()
		if err != nil {
			log.Printf("cannot load root CA certs: %v", err)
		}
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
			// TODO: should we configure the config name?
			tlsConfig.ServerName = "gitstafette-server"
			log.Printf("Configuring TLS for Server")
		} else {
			tlsConfig.RootCAs = ca
			log.Printf("Configuring TLS for Client")
		}
	} else {
		log.Printf("Did not find a CA cert, no TLS RootCA set")
	}

	return tlsConfig, err
}
