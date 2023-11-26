package otel_util

// import (
//
//	"context"
//	"github.com/rs/zerolog/log"
//	"go.opentelemetry.io/otel"
//	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
//	"go.opentelemetry.io/otel/propagation"
//	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
//	sdkresource "go.opentelemetry.io/otel/sdk/resource"
//	sdktrace "go.opentelemetry.io/otel/sdk/trace"
//	"os"
//	"sync"
//
// )
import (
	"os"
)

const (
	OTEL_HOSTNAME = "OTEL_HOSTNAME"
	OTEL_PROTOCOL = "OTEL_PROTOCOL"
	OTEL_PORT     = "OTEL_PORT"
	OTEL_ENABLED  = "OTEL_ENABLED"
)

type OTELConfig struct {
	hostName     string
	protocol     string
	port         string
	grpcExporter bool
	enabled      bool
}

func NewOTELConfig() *OTELConfig {
	hostName := "localhost"
	protocol := "http"
	port := "4317"
	grcpExporter := true
	enabled := true

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

//
//func InitTracerProvider(ctx context.Context) *sdktrace.TracerProvider {
//	otelConfig := NewOTELConfig()
//	if !otelConfig.enabled {
//		return nil
//	}
//
//	exporter, err := otlptracehttp.New(ctx)
//	//exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure(), otlptracegrpc.WithEndpoint(otelConfig.GetOTELEndpoint()))
//	if err != nil {
//		log.Fatal().Err(err).Msg("OTLP Trace http Creation failed")
//	}
//
//	tp := sdktrace.NewTracerProvider(
//		sdktrace.WithBatcher(exporter),
//		sdktrace.WithResource(initResource()),
//	)
//	otel.SetTracerProvider(tp)
//	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
//	return tp
//}
//
//func InitMeterProvider(ctx context.Context) *sdkmetric.MeterProvider {
//	otelConfig := NewOTELConfig()
//	if !otelConfig.enabled {
//		return nil
//	}
//	exporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithInsecure(), otlpmetricgrpc.WithEndpoint(otelConfig.GetOTELEndpoint()))
//	if err != nil {
//		log.Fatal().Err(err).Msg("new otlp metric grpc exporter failed")
//	}
//
//	mp := sdkmetric.NewMeterProvider(
//		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
//		sdkmetric.WithResource(initResource()),
//	)
//	global.SetMeterProvider(mp)
//	return mp
//}
