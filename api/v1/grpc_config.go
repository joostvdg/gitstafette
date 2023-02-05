package gitstafette_v1

import (
	"crypto/tls"
	"log"
)

type GRPCServerConfig struct {
	Host         string
	Port         string
	StreamWindow int
	Insecure     bool
	OAuthToken   string
	TLSConfig    *tls.Config
}

func CreateServerConfig(host string, port string, streamWindow int, insecure bool, oauthToken string, tlsConfig *tls.Config) *GRPCServerConfig {
	config := &GRPCServerConfig{
		Host:         host,
		Port:         port,
		StreamWindow: streamWindow,
		Insecure:     insecure,
		OAuthToken:   oauthToken,
		TLSConfig:    tlsConfig,
	}

	log.Printf("Constructed GRPC Server configuration: %v", *config)
	return config
}

type GRPCClientConfig struct {
	ClientID     string
	RepositoryId string
	StreamWindow int
	WebhookHMAC  string
}

func CreateClientConfig(clientId string, repositoryId string, streamWindow int, webhookHMAC string) *GRPCClientConfig {
	config := &GRPCClientConfig{
		ClientID:     clientId,
		RepositoryId: repositoryId,
		StreamWindow: streamWindow,
		WebhookHMAC:  webhookHMAC,
	}
	log.Printf("Constructed GRPC Client configuration: %v", *config)
	return config
}
