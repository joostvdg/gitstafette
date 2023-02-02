package gitstafette_v1

import (
	"fmt"
	"log"
	"net/url"
)

type RelayConfig struct {
	Enabled        bool
	Host           string
	Path           string
	Port           string
	Protocol       string
	Endpoint       *url.URL
	HealthEndpoint *url.URL
	Insecure       bool
}

func CreateRelayConfig(relayEnabled bool, relayHost string, relayPath string, relayHealthCheckPath string, relayPort string, relayProtocol string, insecure bool) (*RelayConfig, error) {
	relayEndpoint := fmt.Sprintf("%s://%s:%s%s", relayProtocol, relayHost, relayPort, relayPath)
	relayEndpointURL, err := url.Parse(relayEndpoint)
	if err != nil {
		log.Fatal("Malformed URL: ", err.Error())
		return nil, err
	}

	heatlhCheckEndpoint := fmt.Sprintf("%s://%s:%s%s", relayProtocol, relayHost, relayPort, relayHealthCheckPath)
	heatlhCheckEndpointURL, err := url.Parse(heatlhCheckEndpoint)
	if err != nil {
		log.Fatal("Malformed URL: ", err.Error())
		return nil, err
	}

	// TODO remove debug statement
	log.Printf("Configured relay endpoint URL: %v\n", relayEndpointURL.String())
	log.Printf("Configured relay healthcheck endpoint URL: %v\n", heatlhCheckEndpointURL.String())

	return &RelayConfig{
		Enabled:        relayEnabled,
		Host:           relayHost,
		Path:           relayPath,
		Port:           relayPort,
		Protocol:       relayProtocol,
		Endpoint:       relayEndpointURL,
		HealthEndpoint: heatlhCheckEndpointURL,
		Insecure:       insecure,
	}, nil
}
