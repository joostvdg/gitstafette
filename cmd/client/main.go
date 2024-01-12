package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	infoapi "github.com/joostvdg/gitstafette/api/info"
	api "github.com/joostvdg/gitstafette/api/v1"
	v1 "github.com/joostvdg/gitstafette/internal/api/v1"
	"github.com/joostvdg/gitstafette/internal/cache"
	"github.com/joostvdg/gitstafette/internal/config"
	gcontext "github.com/joostvdg/gitstafette/internal/context"
	grpc_internal "github.com/joostvdg/gitstafette/internal/grpc"
	"github.com/joostvdg/gitstafette/internal/info"
	"github.com/joostvdg/gitstafette/internal/otel_util"
	"github.com/joostvdg/gitstafette/internal/relay"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/credentials/oauth"
	"google.golang.org/grpc/health/grpc_health_v1"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// TODO do not close if we have not relayed our events yet!

var requestInterval = time.Second * 5

var tracer trace.Tracer

const envOauthToken = "OAUTH_TOKEN"

var (
	resource          *sdkresource.Resource
	initResourcesOnce sync.Once
	otelEnabled       = false
)

type ServerState struct {
	HasError     bool
	ErrorMessage string
}

func main() {
	// TODO retrieve only events for specific repository
	// TODO so we need a repositories flag, like with the config
	name := flag.String("name", "GSF-Relay", "Name of the GitstafetteServer")
	grpcServerPort := flag.String("port", "50051", "Port used for connecting to the GRPC Server")
	grpcServerHost := flag.String("server", "127.0.0.1", "Server host to connect to")
	grpcServerInsecure := flag.Bool("insecure", false, "If the grpc streaming config should be handled insecurely, must provide either `secure` or `insecure` flag")
	grpcServerSecure := flag.Bool("secure", false, "If the grpc streaming config should be handled securely, must provide either `secure` or `insecure` flag")
	repositoryId := flag.String("repo", "", "GitHub Repository ID to receive webhook events for")
	grpcInfoPort := flag.String("infoPort", "50052", "Port used for connecting to the GRPC Info Server")
	relayEnabled := flag.Bool("relayEnabled", false, "If the config should relay received events, rather than caching them for clients")
	relayHost := flag.String("relayHost", "127.0.0.1", "Host address to relay events to")
	relayPath := flag.String("relayPath", "/", "Path on the host address to relay events to")
	relayHealthCheckPath := flag.String("relayHealthCheckPath", "/", "Path on the host address to do health check on, for relay target")
	relayPort := flag.String("relayPort", "50051", "The port of the relay address")
	relayProtocol := flag.String("relayProtocol", "grpc", "The protocol for the relay address (grpc, or http)")
	relayInsecure := flag.Bool("relayInsecure", false, "If the relay config should be handled insecurely")
	caFileLocation := flag.String("caFileLocation", "", "The root CA file for trusting clients using TLS connection")
	certFileLocation := flag.String("certFileLocation", "", "The certificate file for trusting clients using TLS connection")
	certKeyFileLocation := flag.String("certKeyFileLocation", "", "The certificate key file for trusting clients using TLS connection")
	clientId := flag.String("clientId", "gitstafette-client", "The id of the client to identify connections")
	streamWindow := flag.Int("streamWindow", 180, "The time we spend streaming with the server, in seconds")
	healthCheckPort := flag.String("healthCheckPort", "8080", "Port used for a http health check server, used for running in container environments")
	webhookHMAC := flag.String("webhookHMAC", "", "The hmac token used to verify the webhook events")
	flag.Parse()

	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	sublogger := log.With().Str("component", "init").Logger()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if otel_util.IsOTelEnabled() {
		sublogger.Info().Msg("OTEL is enabled")
		otelShutdown, err, _, tp := otel_util.SetupOTelSDK(ctx, "gsf-client", "0.0.1")
		tracer = tp.Tracer("gsf-client")
		if err != nil {
			log.Fatal().Err(err).Msg("Could not configure OTEL URL")
		}
		// Handle shutdown properly so nothing leaks.
		defer func() {
			err = errors.Join(err, otelShutdown(context.Background()))
		}()
	}

	relayConfig, err := api.CreateRelayConfig(*relayEnabled, *relayHost, *relayPath, *relayHealthCheckPath, *relayPort, *relayProtocol, *relayInsecure)
	if err != nil {
		sublogger.Fatal().Err(err).Msg("Malformed Relay URL")
	}

	repoIds := []string{*repositoryId}
	serverConfig := &api.ServerConfig{
		Name:         *name,
		Host:         "localhost",
		Port:         *healthCheckPort,
		GrpcPort:     *grpcInfoPort,
		Repositories: repoIds,
	}
	runInfoServer(ctx, serverConfig, relayConfig, *grpcInfoPort)

	serviceContext := &gcontext.ServiceContext{
		Context: ctx,
		Relay:   relayConfig,
	}
	relay.InitiateRelay(serviceContext, *repositoryId)
	cache.InitCache(*repositoryId, nil)
	go initHealthCheckServer(ctx, *healthCheckPort)

	insecure := *grpcServerInsecure
	if *grpcServerSecure {
		insecure = false
	}

	oauthToken, oauthOk := os.LookupEnv(envOauthToken)
	if !oauthOk {
		oauthToken = ""
	}

	tlsConfig, err := config.NewTLSConfig(*caFileLocation, *certFileLocation, *certKeyFileLocation, false)
	if err != nil {
		sublogger.Fatal().Err(err).Msg("Invalid certificate configuration")
	}

	grpcServerConfig := api.CreateServerConfig(*grpcServerHost, *grpcServerPort, *streamWindow, insecure, oauthToken, tlsConfig)
	grpcClientConfig := api.CreateClientConfig(*clientId, *repositoryId, *streamWindow, *webhookHMAC)

	for {
		err := handleWebhookEventStream(grpcServerConfig, grpcClientConfig, ctx)
		if err != nil {
			sublogger.Fatal().Err(err).Msg("Error streaming from server")
		}
		if ctx.Err() != nil {
			log.Info().Msgf("[Main-in] Closing FetchWebhookEvents (context error: %v)", ctx.Err())
			break
		}
		if ctx.Done() != nil && ctx.Err() == nil {
			log.Info().Msg("[Main] Restarting FetchWebhookEvents as context expired but no error occurred")
			ctx, stop = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		}
		// TODO: add exponential backoff in case we fail to connect to the server
		sleepTime := time.Second * 3
		sublogger.Info().Msgf("Sleeping for %v", sleepTime)
		time.Sleep(sleepTime)
	}
	sublogger.Info().Msg("Closing client")
}

func runInfoServer(ctx context.Context, serverConfig *api.ServerConfig, relayConfig *api.RelayConfig, grpcPort string) {
	grpcServer := grpc.NewServer()
	go func(s *grpc.Server) {
		grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
		if err != nil {
			log.Fatal().Err(err).Msg("failed to listen")
		}

		infoapi.RegisterInfoServer(s, &info.InfoServer{
			RelayConfig:  relayConfig,
			ServerConfig: serverConfig,
			Tracer:       tracer,
			Type:         infoapi.InstanceType_RELAY,
		})
		grpc_health_v1.RegisterHealthServer(s, &grpc_internal.HealthCheckService{})

		if err := s.Serve(grpcListener); err != nil {
			log.Fatal().Err(err).Msg("failed to serve")
		}
		log.Info().Msg("Shutdown GRPC gitstafette GitstafetteServer")
	}(grpcServer)

}

func initHealthCheckServer(ctx context.Context, port string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthCheck)
	muxServer := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
		BaseContext: func(l net.Listener) context.Context {
			return ctx
		},
	}

	go func() {
		err := muxServer.ListenAndServe()

		if err != nil {
			log.Fatal().Err(err).Msg("Could not start health check service")
		}
	}()
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "OK\n")
}

func handleWebhookEventStream(serverConfig *api.GRPCServerConfig, clientConfig *api.GRPCClientConfig, mainCtx context.Context) error {
	grpcOpts := createGrpcOptions(serverConfig)
	address := serverConfig.Host + ":" + serverConfig.Port
	conn, err := grpc.Dial(address, grpcOpts...)
	if err != nil {
		log.Fatal().Err(err).Msg("did not connect")
	}
	defer conn.Close()

	sublogger := log.With().Str("component", "handleWebhookEventStream").Logger()
	connectionCtx := mainCtx
	var span trace.Span
	otelEnabled := otel_util.IsOTelEnabled()
	if otelEnabled {
		spanContext, tmpSpan := otel_util.StartClientSpan(mainCtx, tracer, "handleWebhookEventStream", conn.Target())
		span = tmpSpan
		defer span.End()
		sublogger = log.With().
			Str("span_id", span.SpanContext().SpanID().String()).
			Str("trace_id", span.SpanContext().TraceID().String()).
			Logger()
		connectionCtx = spanContext
	}

	sublogger.Info().Msg("[handleWebhookEventStream] Starting FetchWebhookEvents")
	client := api.NewGitstafetteClient(conn)
	request := &api.WebhookEventsRequest{
		ClientId:            clientConfig.ClientID,
		RepositoryId:        clientConfig.RepositoryId,
		LastReceivedEventId: 0,
		DurationSecs:        uint32(serverConfig.StreamWindow),
	}

	stream, err := client.FetchWebhookEvents(connectionCtx, request)
	if err != nil {
		log.Fatal().Err(err).Msg("could not fetch webhook events")
	}

	finish := time.Now().Add(time.Second * time.Duration(clientConfig.StreamWindow))
	contextClosed := false

	for time.Now().Before(finish) {
		select {
		case <-time.After(requestInterval):
			if contextClosed {
				sublogger.Info().Msg("Context is already closed")
				break
			}

			otel_util.AddSpanEvent(span, "requesting stream")
			response, err := stream.Recv()
			if err == io.EOF {
				sublogger.Info().Msg("Server send end of stream, closing")
				contextClosed = true
				break
			}
			if err != nil {
				sublogger.Warn().Msgf("Error receiving stream: %v\n", err) // is this recoverable or not?
				contextClosed = true
				return err
			}

			sublogger.Info().Msgf("Received %d WebhookEvents", len(response.WebhookEvents))
			otel_util.AddSpanEventWithOption(span, "EventsReceived", trace.WithAttributes(attribute.Int("events", len(response.WebhookEvents))))

			if len(response.WebhookEvents) > 0 {

				if otelEnabled {
					_, span := otel.Tracer("Client").Start(connectionCtx, "EventsReceived", trace.WithSpanKind(trace.SpanKindClient))
					defer span.End()
					span.AddEvent("EventsReceived", trace.WithAttributes(attribute.Int("events", len(response.WebhookEvents))))
				}

				for _, event := range response.WebhookEvents {

					sublogger.Printf("[handleWebhookEventStream] InternalEvent: %d, body size: %d, number of headers:  %d\n", event.EventId, len(event.Body), len(event.Headers))
					eventIsValid := v1.ValidateEvent(clientConfig.WebhookHMAC, event)
					messageAddition := ""
					if clientConfig.WebhookHMAC != "" {
						messageAddition = " against hmac token on digest header"
					}
					sublogger.Printf("[handleWebhookEventStream] Event %v is validated"+messageAddition+", valid: %v",
						event.EventId, eventIsValid)
					cache.Event(clientConfig.RepositoryId, event)
				}

			}
		case <-stream.Context().Done():
			otel_util.AddSpanEvent(span, "stream context done")
			sublogger.Info().Msg("[handleWebhookEventStream] Closing FetchWebhookEvents (stream context done)")
			contextClosed = true
			break
		case <-mainCtx.Done(): // Activated when ctx.Done() closes
			otel_util.AddSpanEvent(span, "mainCtx done")
			sublogger.Info().Msg("[handleWebhookEventStream] Closing FetchWebhookEvents (mainCtx done)")
			contextClosed = true
			break
		}
		if contextClosed {
			sublogger.Info().Msg("[handleWebhookEventStream] Closing FetchWebhookEvents (context checkpoint)")
			break
		}
	}

	if stream.Context().Err() != nil {
		sublogger.Info().Msg("[handleWebhookEventStream] Closing FetchWebhookEvents (stream context error)")
		if otelEnabled {
			span.SetStatus(codes.Error, "Stream Context Error")
			span.AddEvent("finish")
		}
		return stream.Context().Err()
	}
	sublogger.Info().Msg("[handleWebhookEventStream] Closing FetchWebhookEvents")
	if otelEnabled {
		span.SetStatus(codes.Ok, "OK")
		span.AddEvent("finish")
	}
	return nil
}

func createGrpcOptions(serverConfig *api.GRPCServerConfig) []grpc.DialOption {
	sublogger := log.With().Str("component", "grpc-init").Logger()

	var opts []grpc.DialOption
	opts = append(opts, grpc.WithAuthority(serverConfig.Host))

	if serverConfig.OAuthToken != "" {
		rpcCreds := oauth.NewOauthAccess(&oauth2.Token{AccessToken: serverConfig.OAuthToken})
		opts = append(opts, grpc.WithPerRPCCredentials(rpcCreds))
	}

	if serverConfig.Insecure {
		sublogger.Info().Msg("Not using TLS for GRPC server connection (insecure set)")
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else if serverConfig.TLSConfig != nil && serverConfig.TLSConfig.RootCAs != nil { // TODO verify if this is al that is required
		sublogger.Info().Msg("Using provided TLS certificates for GRPC server connection (RootCA's set)")
		clientCreds := credentials.NewTLS(serverConfig.TLSConfig)
		opts = append(opts, grpc.WithTransportCredentials(clientCreds))
	} else {
		sublogger.Info().Msg("Using default system TLS certificates for GRPC server connection (secure, but no RootCA)")
		// https://www.googlecloudcommunity.com/gc/Serverless/Unable-to-connect-to-Cloud-Run-gRPC-server/m-p/422280/highlight/true#M345
		systemRoots, err := x509.SystemCertPool()
		if err != nil {
			sublogger.Warn().Err(err).Msg("cannot load root CA certs")
		}
		creds := credentials.NewTLS(&tls.Config{
			RootCAs: systemRoots,
		})
		opts = append(opts, grpc.WithTransportCredentials(creds))
	}

	return opts
}
