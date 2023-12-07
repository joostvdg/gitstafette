package otel_util

import (
	"context"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	codes2 "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/metadata"
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
	enabled := false

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

	enabled = IsOTelEnabled()

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

func StartServerSpanFromClientContext(ctx context.Context, tracer trace.Tracer, serviceNames string, spanKind trace.SpanKind) (trace.SpanContext, context.Context, trace.Span) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		md = metadata.MD{}
	}
	_, traceContext := otelgrpc.Extract(ctx, &md)
	otelgrpc.Inject(ctx, &md)
	name, attr, _ := TelemetryAttributes(serviceNames, PeerFromCtx(ctx))
	startOpts := append([]trace.SpanStartOption{
		trace.WithSpanKind(spanKind),
		trace.WithAttributes(attr...),
	})

	spanParentContext := trace.ContextWithRemoteSpanContext(ctx, traceContext)
	spanContext, span := tracer.Start(spanParentContext, name, startOpts...)
	return traceContext, spanContext, span
}

func StartClientSpan(ctx context.Context, tracer trace.Tracer, serviceName string, connectionTarget string) (context.Context, trace.Span) {
	name, attr, _ := TelemetryAttributes(serviceName, connectionTarget)
	startOpts := append([]trace.SpanStartOption{
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attr...),
	})

	spanParentContext := trace.ContextWithRemoteSpanContext(ctx, trace.SpanContextFromContext(ctx))
	spanContext, span := tracer.Start(spanParentContext, name, startOpts...)
	md, ok := metadata.FromOutgoingContext(spanContext)
	if !ok {
		md = metadata.MD{}
	}

	otelgrpc.Inject(spanContext, &md)
	spanContext = metadata.NewOutgoingContext(spanContext, md)
	return spanContext, span
}

func AddSpanEvent(span trace.Span, event string) {
	if span != nil {
		span.AddEvent(event)
	}
}

func AddSpanEventWithOption(span trace.Span, event string, options trace.SpanStartEventOption) {
	if span != nil {
		span.AddEvent(event, options)
	}
}

func IsOTelEnabled() bool {
	if osEnabled := os.Getenv(OTEL_ENABLED); osEnabled != "" && osEnabled == "true" {
		return true
	}
	return false
}

func SetSpanStatus(span trace.Span, code codes2.Code, statusMessage string) {
	if span != nil {
		span.SetStatus(code, statusMessage)
	}
}
