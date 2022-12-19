package gitstafette_v1

import "log"

type GRPCServerConfig struct {
	Host     string
	Port     string
	Insecure bool
}

func CreateConfig(host string, port string, insecure bool) *GRPCServerConfig {
	config := &GRPCServerConfig{
		Host:     host,
		Port:     port,
		Insecure: insecure,
	}

	log.Printf("Constructed GRPC Server configuration: %v", *config)
	return config
}
