package otel_util

import (
	"os"
)

const (
	OTEL_HOSTNAME     = "OTEL_HOSTNAME"
	OTEL_PROTOCOL     = "OTEL_PROTOCOL"
	OTEL_PORT         = "OTEL_PORT"
	OTEL_ENABLED      = "OTEL_ENABLED"
	OTEL_SERVICE_NAME = "OTEL_SERVICE_NAME"
)

type OTELConfig struct {
	serviceName  string
	hostName     string
	protocol     string
	port         string
	grpcExporter bool
	enabled      bool
}

func NewOTELConfig() *OTELConfig {
	serviceName := "gitstafette"
	hostName := "localhost"
	protocol := "http"
	port := "4317"
	grcpExporter := true
	enabled := true

	if osServiceName := os.Getenv(OTEL_SERVICE_NAME); osServiceName != "" {
		serviceName = osServiceName
	}

	// retrieve hostname from environment variable
	if osHostName := os.Getenv(OTEL_HOSTNAME); osHostName != "" {
		hostName = osHostName
	}

	// retrieve protocol from environment variable
	if osProtocol := os.Getenv(OTEL_PROTOCOL); osProtocol != "" {
		protocol = osProtocol
	}

	// retrieve port from environment variable
	if osPort := os.Getenv(OTEL_PORT); osPort != "" {
		port = osPort
	}

	// retrieve enabled from environment variable
	if osEnabled := os.Getenv(OTEL_ENABLED); osEnabled != "" {
		enabled = osEnabled == "true"
	}

	return &OTELConfig{
		serviceName:  serviceName,
		hostName:     hostName,
		protocol:     protocol,
		port:         port,
		grpcExporter: grcpExporter,
		enabled:      enabled,
	}
}

func (o *OTELConfig) GetOTELEndpoint() string {
	// return empty string if otel is not enabled
	if !o.enabled {
		return ""
	}
	// else return the endpoint
	// return o.protocol + "://" + o.hostName + ":" + o.port
	return o.hostName + ":" + o.port
}
