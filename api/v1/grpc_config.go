package gitstafette_v1

import (
	"crypto/tls"
	"log"
)

type GRPCServerConfig struct {
	Host      string
	Port      string
	Insecure  bool
	TLSConfig *tls.Config
}

func CreateConfig(host string, port string, insecure bool, tlsConfig *tls.Config) *GRPCServerConfig {
	config := &GRPCServerConfig{
		Host:      host,
		Port:      port,
		Insecure:  insecure,
		TLSConfig: tlsConfig,
	}

	log.Printf("Constructed GRPC Server configuration: %v", *config)
	return config
}
