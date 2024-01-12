package info

import (
	"context"
	infoapi "github.com/joostvdg/gitstafette/api/info"
	api "github.com/joostvdg/gitstafette/api/v1"
	"github.com/joostvdg/gitstafette/internal/otel_util"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/trace"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

type InfoServer struct {
	infoapi.UnimplementedInfoServer
	RelayConfig  *api.RelayConfig
	ServerConfig *api.ServerConfig
	Tracer       trace.Tracer
	Type         infoapi.InstanceType
}

func (srv *InfoServer) GetInfo(ctx context.Context, req *infoapi.GetInfoRequest) (*infoapi.GetInfoResponse, error) {

	otelEnabled := otel_util.IsOTelEnabled()
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	sublogger := log.With().Logger()
	var span trace.Span
	if otelEnabled {
		traceContext, _, tmpSpan := otel_util.StartServerSpanFromClientContext(ctx, srv.Tracer, "GetInfo", trace.SpanKindServer)
		span = tmpSpan
		defer span.End()

		sublogger = log.With().
			Str("span_id", span.SpanContext().SpanID().String()).
			Str("trace_id", span.SpanContext().TraceID().String()).
			Str("incoming_trace_id", traceContext.TraceID().String()).
			Logger()
	}
	sublogger.Info().Msgf("received GetInfo request from %v @%v", req.ClientId, req.ClientEndpoint)

	// get hostname from env, otherwise use localhost
	hostname := "localhost"
	if os.Getenv("HOSTNAME") != "" {
		hostname = os.Getenv("HOSTNAME")
	}

	repos := strings.Join(srv.ServerConfig.Repositories, ",")
	server := &infoapi.ServerInfo{
		Hostname:     hostname,
		Port:         srv.ServerConfig.Port,
		Protocol:     "grpc",
		Repositories: &repos,
	}

	response := &infoapi.GetInfoResponse{
		Alive:        true,
		Version:      "0.0.1",
		Name:         srv.ServerConfig.Name,
		InstanceType: srv.Type,
		Server:       server,
	}

	if srv.RelayConfig.Enabled {
		relay := &infoapi.ServerInfo{
			Hostname:     srv.RelayConfig.Host,
			Port:         srv.RelayConfig.Port,
			Protocol:     srv.RelayConfig.Protocol,
			Repositories: &repos,
		}
		response.Relay = relay
	}

	return response, nil
}
