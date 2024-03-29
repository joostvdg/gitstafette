package grpc

import (
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"os"
	"strings"
	"time"
)

const (
	envOauthToken = "OAUTH_TOKEN"
)

type WrappedStream struct {
	grpc.ServerStream
}

func (w *WrappedStream) RecvMsg(m interface{}) error {
	log.Printf("====== [Server Stream Interceptor Wrapper] Receive a message (Type: %T) at %s", m, time.Now().Format(time.RFC3339))
	return w.ServerStream.RecvMsg(m)
}

func (w *WrappedStream) SendMsg(m interface{}) error {
	log.Printf("====== [Server Stream Interceptor Wrapper] Send a message (Type: %T) at %s", m, time.Now().Format(time.RFC3339))
	return w.ServerStream.SendMsg(m)
}

func NewWrappedStream(s grpc.ServerStream) grpc.ServerStream {
	return &WrappedStream{s}
}

func EventsServerStreamInterceptor(srv interface{}, serverStream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	ctx := serverStream.Context()
	_, span := otel.Tracer("Server").Start(ctx, info.FullMethod, trace.WithSpanKind(trace.SpanKindServer))
	defer span.End()
	log.Printf("====== [EventsServerStreamInterceptor] Send a message (Type: %T) at %s", srv, time.Now().Format(time.RFC3339))
	span.SetAttributes(attribute.String("grpc.service", info.FullMethod))
	span.SetAttributes(attribute.String("grpc.stream.type", "server"))
	//name, attr, _ := otel_util.TelemetryAttributes(info.FullMethod, otel_util.PeerFromCtx(ctx))

	return handler(srv, serverStream)
}

func ValidateToken(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	log.Info().Msg("Validating token for GRPC Stream Request")
	//ctx := ss.Context()
	//newCtx, span := otel.Tracer("Server").Start(ctx, info.FullMethod, trace.WithSpanKind(trace.SpanKindServer))
	//defer span.End()
	//span.SetAttributes(attribute.String("grpc.service", info.FullMethod))
	//span.SetAttributes(attribute.String("grpc.stream.type", "server"))
	//span.AddEvent("Validating token for GRPC Stream Request")

	oauthToken, oauthOk := os.LookupEnv(envOauthToken)
	if oauthOk {
		log.Printf("Validating token for GRPC Stream Request -> TOKEN FOUND")
		md, ok := metadata.FromIncomingContext(ss.Context())
		if !ok {
			errorMessage := "missing metadata when validating OAuth Token"
			log.Warn().Msg(errorMessage)
			return status.Error(codes.InvalidArgument, errorMessage)
		}

		if !valid(md["authorization"], oauthToken) {
			errorMessage := "OAuth Token Missing Or Not Valid"
			log.Warn().Msg(errorMessage)
			return status.Error(codes.Unauthenticated, errorMessage)
		} else {
			log.Printf("Validating token for GRPC Stream Request -> TOKEN VALID")
		}
	} else {
		log.Warn().Msg("Validating token for GRPC Stream Request -> TOKEN MISSING")
	}

	return handler(srv, ss)
}

func valid(authorization []string, expectedToken string) bool {
	if len(authorization) < 1 {
		return false
	}
	receivedToken := strings.TrimPrefix(authorization[0], "Bearer ")
	// If you have more than one client then you will have to update this line.
	return receivedToken == expectedToken
}
