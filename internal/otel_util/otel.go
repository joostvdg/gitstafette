package otel_util

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/rs/zerolog/log"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"google.golang.org/grpc/peer"
)

// setupOTelSDK bootstraps the OpenTelemetry pipeline.
// If it does not return an error, make sure to call shutdown for proper cleanup.
func SetupOTelSDK(ctx context.Context, serviceName, serviceVersion string) (shutdown func(context.Context) error, meterProvider *metric.MeterProvider, tracerProvider *trace.TracerProvider, err error) {
	var shutdownFuncs []func(context.Context) error
	otelConfig := NewOTELConfig()
	if otelConfig.serviceName == "" {
		otelConfig.serviceName = serviceName
	}

	// shutdown calls cleanup functions registered via shutdownFuncs.
	// The errors from the calls are joined.
	// Each registered cleanup will be invoked once.
	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// handleErr calls shutdown for cleanup and makes sure that all errors are returned.
	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	// Set up resource.
	res, err := newResource(otelConfig.serviceName, serviceVersion)
	if err != nil {
		log.Warn().Err(err).Msg("failed to create resource")
		handleErr(err)
		return
	}

	// Set up propagator.
	prop := newPropagator()
	otel.SetTextMapPropagator(prop)

	// Set up trace provider.
	tracerProvider, err = newTraceProvider(res, otelConfig)
	if err != nil {
		log.Warn().Err(err).Msg("failed to create trace provider")
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	// Set up meter provider.
	meterProvider, err = newMeterProvider(res)
	if err != nil {
		log.Warn().Err(err).Msg("failed to create meter provider")
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
	otel.SetMeterProvider(meterProvider)

	return
}

func newResource(serviceName, serviceVersion string) (*resource.Resource, error) {
	return resource.Merge(resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		))
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func newTraceProvider(res *resource.Resource, config *OTELConfig) (*trace.TracerProvider, error) {
	otelConfig := NewOTELConfig()
	// TODO: how do we provide the correct context to this?
	conn, err := grpc.NewClient(otelConfig.GetOTELEndpoint(), grpc.WithTransportCredentials(insecure.NewCredentials()))

	if err != nil {
		if otelConfig.enabled {
			return nil, fmt.Errorf("failed to create gRPC connection to collector: %w", err)
		} else {
			log.Warn().Msg("failed to create gRPC connection to collector, but OTEL is disabled")
		}
	}

	traceExporter, err := otlptracegrpc.New(context.Background(), otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		if otelConfig.enabled {
			return nil, fmt.Errorf("failed to create trace exporter: %w", err)
		} else {
			log.Warn().Msg("failed to create trace exporter, but OTEL is disabled")
		}
	}

	// Register the trace exporter with a TracerProvider, using a batch
	// span processor to aggregate spans before export.
	bsp := trace.NewBatchSpanProcessor(traceExporter)

	sublogger := log.With().Str("component", "otel_util").Logger()
	sublogger.Info().Msgf("OTEL Config: %v", otelConfig)
	sampler := trace.ParentBased(trace.TraceIDRatioBased(otelConfig.samplingRate))
	tracerProvider := trace.NewTracerProvider(
		trace.WithSampler(sampler),
		trace.WithResource(res),
		trace.WithSpanProcessor(bsp),
	)
	return tracerProvider, nil
}

func newMeterProvider(res *resource.Resource) (*metric.MeterProvider, error) {
	otelConfig := NewOTELConfig()
	ctx := context.Background()
	if !otelConfig.enabled {
		return nil, nil
	}

	exporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithInsecure(), otlpmetricgrpc.WithEndpoint(otelConfig.GetOTELEndpoint()))
	if err != nil {
		log.Fatal().Err(err).Msg("new otlp metric grpc exporter failed")
	}

	meterProvider := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(exporter,
			// Default is 1m. Set to 3s for demonstrative purposes.
			metric.WithInterval(3*time.Second))),
	)
	return meterProvider, nil
}

// telemetryAttributes returns a span name and span and metric attributes from
// the gRPC method and peer address.
func TelemetryAttributes(fullMethod, peerAddress string) (string, []attribute.KeyValue, []attribute.KeyValue) {
	name, methodAttrs := ParseFullMethod(fullMethod)
	peerAttrs := PeerAttr(peerAddress)

	attrs := make([]attribute.KeyValue, 0, 1+len(methodAttrs)+len(peerAttrs))
	attrs = append(attrs, semconv.RPCSystemGRPC)
	attrs = append(attrs, methodAttrs...)
	metricAttrs := attrs[:1+len(methodAttrs)]
	attrs = append(attrs, peerAttrs...)
	return name, attrs, metricAttrs
}

// peerAttr returns attributes about the peer address.
func PeerAttr(addr string) []attribute.KeyValue {
	host, p, err := net.SplitHostPort(addr)
	if err != nil {
		return nil
	}

	if host == "" {
		host = "127.0.0.1"
	}
	port, err := strconv.Atoi(p)
	if err != nil {
		return nil
	}

	var attr []attribute.KeyValue
	if ip := net.ParseIP(host); ip != nil {
		attr = []attribute.KeyValue{
			semconv.NetSockPeerAddr(host),
			semconv.NetSockPeerPort(port),
		}
	} else {
		attr = []attribute.KeyValue{
			semconv.NetPeerName(host),
			semconv.NetPeerPort(port),
		}
	}

	return attr
}

// ParseFullMethod returns a span name following the OpenTelemetry semantic
// conventions as well as all applicable span attribute.KeyValue attributes based
// on a gRPC's FullMethod.
//
// Parsing is consistent with grpc-go implementation:
// https://github.com/grpc/grpc-go/blob/v1.57.0/internal/grpcutil/method.go#L26-L39
func ParseFullMethod(fullMethod string) (string, []attribute.KeyValue) {
	if !strings.HasPrefix(fullMethod, "/") {
		// Invalid format, does not follow `/package.service/method`.
		return fullMethod, nil
	}
	name := fullMethod[1:]
	pos := strings.LastIndex(name, "/")
	if pos < 0 {
		// Invalid format, does not follow `/package.service/method`.
		return name, nil
	}
	service, method := name[:pos], name[pos+1:]

	var attrs []attribute.KeyValue
	if service != "" {
		attrs = append(attrs, semconv.RPCService(service))
	}
	if method != "" {
		attrs = append(attrs, semconv.RPCMethod(method))
	}
	return name, attrs
}

// peerFromCtx returns a peer address from a context, if one exists.
func PeerFromCtx(ctx context.Context) string {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return ""
	}
	return p.Addr.String()
}
