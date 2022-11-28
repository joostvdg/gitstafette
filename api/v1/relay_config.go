package gitstafette_v1

import (
	"fmt"
	"log"
	"net/url"
)

type RelayConfig struct {
	Enabled  bool
	Host     string
	Port     string
	Protocol string
	Endpoint *url.URL
}

func CreateRelayConfig(relayEnabled bool, relayHost string, relayPort string, relayProtocol string) (*RelayConfig, error) {
	relayEndpoint := fmt.Sprintf("%s://%s:%s", relayProtocol, relayHost, relayPort)
	relayEndpointURL, err := url.Parse(relayEndpoint)
	if err != nil {
		log.Fatal("Malformed URL: ", err.Error())
		return nil, err
	}
	// TODO remove debug statement
	log.Printf("Configured relay endpoint URL. (URL: %v, Port: %v)\n",
		relayEndpointURL, relayEndpointURL.Port())
	return &RelayConfig{
		Enabled:  relayEnabled,
		Host:     relayHost,
		Port:     relayPort,
		Protocol: relayProtocol,
		Endpoint: relayEndpointURL,
	}, nil
}
