package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"github.com/getsentry/sentry-go"
	sentryecho "github.com/getsentry/sentry-go/echo"
	internal_api "github.com/joostvdg/gitstafette/internal/api/v1"
	"github.com/joostvdg/gitstafette/internal/cache"
	"github.com/joostvdg/gitstafette/internal/config"
	gcontext "github.com/joostvdg/gitstafette/internal/context"
	grpc_internal "github.com/joostvdg/gitstafette/internal/grpc"
	"github.com/joostvdg/gitstafette/internal/otel_util"
	"github.com/joostvdg/gitstafette/internal/relay"
	"github.com/labstack/echo/v4/middleware"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	codes2 "go.opentelemetry.io/otel/codes"
	otelapi "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	//semconv "go.opentelemetry.io/otel_util/semconv/v1.18.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	api "github.com/joostvdg/gitstafette/api/v1"
	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
)

// TODO add flags for target for Relay

const (
	envSentry        = "SENTRY_DSN"
	responseInterval = time.Second * 5
)

var (
	mp *sdkmetric.MeterProvider
)

type server struct {
	api.UnimplementedGitstafetteServer
}

func main() {
	port := flag.String("port", "1323", "Port used for hosting the server")
	grpcPort := flag.String("grpcPort", "50051", "Port used for hosting the grpc streaming server")
	grpcHealthPort := flag.String("grpcHealthPort", "50052", "Port used for hosting the grpc health checks")
	repositoryIDs := flag.String("repositories", "", "Comma separated list of GitHub repository IDs to listen for")
	redisDatabase := flag.String("redisDatabase", "0", "Database used for redis")
	redisHost := flag.String("redisHost", "localhost", "Host of the Redis server")
	redisPort := flag.String("redisPort", "6379", "Port of the Redis server")
	redisPassword := flag.String("redisPassword", "", "Password of the Redis server (default is no password")
	relayEnabled := flag.Bool("relayEnabled", false, "If the server should relay received events, rather than caching them for clients")
	relayHost := flag.String("relayHost", "127.0.0.1", "Host address to relay events to")
	relayPath := flag.String("relayPath", "/", "Path on the host address to relay events to")
	relayHealthCheckPath := flag.String("relayHealthCheckPath", "/", "Path on the host address to do health check on, for relay target")
	relayPort := flag.String("relayPort", "50051", "The port of the relay address")
	relayProtocol := flag.String("relayProtocol", "grpc", "The protocol for the relay address (grpc, or http)")
	relayInsecure := flag.Bool("relayInsecure", false, "If the relay server should be handled insecurely")
	caFileLocation := flag.String("caFileLocation", "", "The root CA file for trusting clients using TLS connection")
	certFileLocation := flag.String("certFileLocation", "", "The certificate file for trusting clients using TLS connection")
	certKeyFileLocation := flag.String("certKeyFileLocation", "", "The certificate key file for trusting clients using TLS connection")
	webhookHMAC := flag.String("webhookHMAC", "", "The hmac token used to verify the webhook events")
	flag.Parse()

	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	tlsConfig, err := config.NewTLSConfig(*caFileLocation, *certFileLocation, *certKeyFileLocation, true)
	if err != nil {
		log.Fatal().Err(err).Msg("Invalid certificate configuration")
	}
	redisConfig := &cache.RedisConfig{
		Host:     *redisHost,
		Port:     *redisPort,
		Password: *redisPassword,
		Database: *redisDatabase,
	}
	repoIds := cache.InitCache(*repositoryIDs, redisConfig)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if otelEnabled := os.Getenv("OTEL_ENABLED"); otelEnabled != "" {
		var otelShutdown func(context.Context) error
		otelShutdown, err, mp = otel_util.SetupOTelSDK(ctx, "gsf-client", "0.0.1")
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
		log.Fatal().Err(err).Msg("Malformed URL")
	}

	initSentry() // has to happen before we init Echo
	var grpcHealthServer *grpc.Server
	if *grpcHealthPort != *grpcPort {
		grpcHealthServer = initializeGRPCHealthServer(*grpcHealthPort)
	}
	grpcServer := initializeGRPCServer(*grpcPort, tlsConfig, grpcHealthServer, ctx)
	echoServer := initializeEchoServer(relayConfig, *port, *webhookHMAC)
	log.Printf("Started http server on: %s, grpc server on: %s, and grpc health server on: %s\n", *port, *grpcPort, *grpcHealthPort)

	serviceContext := &gcontext.ServiceContext{
		Context: ctx,
		Relay:   relayConfig,
	}

	if relayConfig.Enabled {
		log.Printf("Relay mode enabled: %v", relayConfig)
		for _, repoId := range repoIds {
			// TODO confirm this works for 1 and multiple
			relay.InitiateRelay(serviceContext, repoId)
		}
	}
	go relay.CleanupRelayedEvents(serviceContext)

	// Wait for interrupt signal to gracefully shut down the config with a timeout of 10 seconds.
	// Use a buffered channel to avoid missing signals as recommended for signal.Notify
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	log.Info().Msg("Shutting down Echo server")
	if err := echoServer.Shutdown(ctx); err != nil {
		echoServer.Logger.Fatal(err)
	}
	log.Info().Msg("Shutting down GRPC gitstafette server")
	grpcServer.GracefulStop()
	log.Info().Msg("Shutting down GRPC health server")
	if *grpcHealthPort != *grpcPort {
		grpcHealthServer.GracefulStop()
	}
	cache.PrepareForShutdown()
	log.Info().Msg("Shutting down!\n")
}

func initSentry() {
	// To initialize Sentry's handler, you need to initialize Sentry itself beforehand
	sentryDsn, sentryOk := os.LookupEnv(envSentry)
	if sentryOk {
		err := sentry.Init(sentry.ClientOptions{
			Dsn:              sentryDsn,
			TracesSampleRate: 1.0,
		})

		if err != nil {
			log.Printf("Sentry initialization failed: %v\n", err)
		} else {
			log.Print("Initialized Sentry")
		}
	}
}

func initializeEchoServer(relayConfig *api.RelayConfig, port string, webhookHMAC string) *echo.Echo {
	e := echo.New()
	e.Use(func(e echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			gitstatefetteContext := &gcontext.GitstafetteContext{
				Context:     c,
				WebhookHMAC: webhookHMAC,
				Relay:       relayConfig,
			}
			return e(gitstatefetteContext)
		}
	})

	e.Use(middleware.Logger())
	e.Use(sentryecho.New(sentryecho.Options{}))

	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "Hello, World!")
	})
	e.POST("/v1/github/", internal_api.HandleGitHubPost)
	e.GET("/v1/watchlist", internal_api.HandleWatchListGet)
	e.GET("/v1/events/:repo", internal_api.HandleRetrieveEventsForRepository)

	// Start Echo server
	go func(echoPort string) {
		if err := e.Start(":" + echoPort); err != nil && err != http.ErrServerClosed {
			e.Logger.Fatal("shutting down the Echo server")
		}
	}(port)
	return e
}

var kaep = keepalive.EnforcementPolicy{
	MinTime:             3 * time.Second, // If a client pings more than once every 5 seconds, terminate the connection
	PermitWithoutStream: true,            // Allow pings even when there are no active streams
}

var kasp = keepalive.ServerParameters{
	MaxConnectionIdle:     15 * time.Second, // If a client is idle for 15 seconds, send a GOAWAY
	MaxConnectionAge:      60 * time.Second, // If any connection is alive for more than 60 seconds, send a GOAWAY
	MaxConnectionAgeGrace: 5 * time.Second,  // Allow 5 seconds for pending RPCs to complete before forcibly closing connections
	Time:                  5 * time.Second,  // Ping the client if it is idle for 5 seconds to ensure the connection is still active
	Timeout:               10 * time.Second, // Wait 1 second for the ping ack before assuming the connection is dead
}

func initializeGRPCHealthServer(grpcPort string) *grpc.Server {
	grpcServer := grpc.NewServer(grpc.KeepaliveEnforcementPolicy(kaep), grpc.KeepaliveParams(kasp), grpc.UnaryInterceptor(otelgrpc.UnaryServerInterceptor()))

	go func(s *grpc.Server) {
		grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
		if err != nil {
			log.Fatal().Err(err).Msg("failed to listen")
		}

		grpc_health_v1.RegisterHealthServer(s, &HealthCheckService{})
		if err := s.Serve(grpcListener); err != nil {
			log.Fatal().Err(err).Msg("failed to serve")
		}
		log.Info().Msg("Shutdown GRPC health server")
	}(grpcServer)
	return grpcServer
}

func initializeGRPCServer(grpcPort string, tlsConfig *tls.Config, healthServer *grpc.Server, ctx context.Context) *grpc.Server {
	grpcServer := grpc.NewServer(
		//grpc.KeepaliveEnforcementPolicy(kaep),
		//grpc.KeepaliveParams(kasp),
		grpc.ChainStreamInterceptor(grpc_internal.ValidateToken, grpc_internal.EventsServerStreamInterceptor),
	)

	if tlsConfig != nil {
		serverCredentials := credentials.NewTLS(tlsConfig)
		grpcServer = grpc.NewServer(
			//grpc.KeepaliveEnforcementPolicy(kaep),
			//grpc.KeepaliveParams(kasp),
			grpc.Creds(serverCredentials),
			grpc.ChainStreamInterceptor(grpc_internal.ValidateToken, grpc_internal.EventsServerStreamInterceptor),
		)
	}

	go func(s *grpc.Server) {
		otel_util.SetupOTelSDK(context.Background(), "gsf-server", "0.0.1")

		grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
		if err != nil {
			log.Fatal().Err(err).Msg("failed to listen")
		}

		log.Printf("Starting GRPC server")
		api.RegisterGitstafetteServer(s, &server{})
		if healthServer == nil {
			log.Info().Msg("GRPC HealthCheck server is empty, running service with normal GRPC server")
			grpc_health_v1.RegisterHealthServer(s, &HealthCheckService{})
		} else {
			log.Printf("Running GRPC HealthCheck server standalone\n", s.GetServiceInfo())
		}

		if err := s.Serve(grpcListener); err != nil {
			log.Fatal().Err(err).Msg("failed to serve")
		}
		log.Info().Msg("Shutdown GRPC gitstafette server")
	}(grpcServer)
	return grpcServer
}

func (s server) WebhookEventStatus(ctx context.Context, req *api.WebhookEventStatusRequest) (*api.WebhookEventStatusResponse, error) {
	response := &api.WebhookEventStatusResponse{
		ServerId:     "Gitstafette",
		Count:        0,
		RepositoryId: req.RepositoryId,
		Status:       "OK",
	}
	return response, nil
}
func (s server) WebhookEventStatuses(request *api.WebhookEventStatusesRequest, srv api.Gitstafette_WebhookEventStatusesServer) error {

	status01 := &api.WebhookEventStatusResponse{
		ServerId:     "Gitstafette",
		Count:        1,
		RepositoryId: "12345",
		Status:       "OK",
	}

	status02 := &api.WebhookEventStatusResponse{
		ServerId:     "Gitstafette",
		Count:        2,
		RepositoryId: "7891",
		Status:       "OK",
	}

	status03 := &api.WebhookEventStatusResponse{
		ServerId:     "Gitstafette",
		Count:        3,
		RepositoryId: "7892",
		Status:       "FAILED",
	}

	finish := time.Now().Add(time.Second * 30)
	ctx, stop := signal.NotifyContext(srv.Context(), os.Interrupt, syscall.SIGTERM)

	defer stop()

	waitInterval := time.Second * 5

	log.Printf("Wait Interval is: %v", waitInterval)

	spanCtx, span := otel.Tracer("Server").Start(ctx, "WebhookEventStatuses", trace.WithSpanKind(trace.SpanKindServer))
	span.AddEvent("Start")
	events := []*api.WebhookEventStatusResponse{status01, status02, status03}
	lastEvent := len(events) - 1
	currentEvent := 0

	for time.Now().Before(finish) {
		closed := false
		select {
		case <-time.After(waitInterval):
			eventInfo := fmt.Sprintf("Sent event %v of %v", currentEvent, lastEvent)
			_, span := otel.Tracer("Server").Start(spanCtx, eventInfo, trace.WithSpanKind(trace.SpanKindServer))
			if currentEvent > lastEvent {
				closed = true
				break
			}
			eventStatus := events[currentEvent]
			currentEvent++

			span.AddEvent("Send", trace.WithAttributes(attribute.Int("eventCounter", currentEvent)))
			if err := srv.Send(eventStatus); err != nil {
				return err
			}
			span.End()
			break
		case <-srv.Context().Done(): // Activated when ctx.Done() closes
			log.Printf("Closing WebhookEventStatuses (client context %s closed)", request.ClientId)
			closed = true
			break
		case <-ctx.Done(): // Activated when ctx.Done() closes
			log.Info().Msg("Closing WebhookEventStatuses (main context closed)")
			closed = true
			break
		}
		if closed {
			log.Info().Msg("Context is already closed")
			break
		}
	}
	log.Printf("Reached %v, so closed context %s", finish, request.ClientId)
	span.AddEvent("Finished", trace.WithAttributes(attribute.String("reason", "timeout")))
	span.End()
	return nil
}

func (s server) WebhookEventPush(ignoredContext context.Context, request *api.WebhookEventPushRequest) (*api.WebhookEventPushResponse, error) {
	response := &api.WebhookEventPushResponse{
		ResponseCode:        "200", // TODO implement a response code system
		ResponseDescription: "depends",
		Accepted:            false,
	}

	err := cache.Event(request.RepositoryId, request.WebhookEvent)
	if err == nil {
		response.Accepted = true
		log.Printf("Accepted Webhook Event Push for Repo %v: %v", request.RepositoryId, request.WebhookEvent.EventId)
	}
	return response, err
}

func (s server) FetchWebhookEvents(request *api.WebhookEventsRequest, srv api.Gitstafette_FetchWebhookEventsServer) error {
	log.Printf("Relaying webhook events for repository %s", request.RepositoryId)
	meter := mp.Meter("gitstafette")
	counter, _ := meter.Int64Counter(
		"webhook_events_relayed",
		otelapi.WithDescription("Number of webhook events relayed"),
	)

	durationSeconds := request.GetDurationSecs()
	finish := time.Now().Add(time.Second * time.Duration(durationSeconds))
	log.Printf("Stream is alive from %v to %v", time.Now(), finish)

	ctx, stop := signal.NotifyContext(srv.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	span := trace.SpanFromContext(srv.Context())
	sublogger := log.With().
		Str("span_id", span.SpanContext().SpanID().String()).
		Str("trace_id", span.SpanContext().TraceID().String()).
		Logger()

	log.Printf("Response Interval is: %v", responseInterval)

	for time.Now().Before(finish) {
		closed := false
		select {
		case <-time.After(responseInterval):
			_, span := otel.Tracer("Server").Start(srv.Context(), "retrieveCachedEventsForRepository", trace.WithSpanKind(trace.SpanKindServer))
			sublogger.Info().Msgf("Fetching events for repo %v (with Span)", request.RepositoryId)

			events, err := retrieveCachedEventsForRepository(request.RepositoryId)

			if err != nil {
				sublogger.Info().Msgf("Could not get events for Repo: %v\n", err)
				span.SetStatus(codes2.Error, "Could not get events for Repo")
				return err
			}
			response := &api.WebhookEventsResponse{
				WebhookEvents: events,
			}

			if err := srv.Send(response); err != nil {
				sublogger.Info().Msgf("Error sending stream: %v\n", err)
				span.SetStatus(codes2.Error, "Error sending stream")
				return err
			}
			sublogger.Info().Msgf("Send %v events to client (%v) for repo %v", len(events), request.ClientId, request.RepositoryId)

			counter.Add(srv.Context(), int64(len(events)))
			updateRelayStatus(events, request.RepositoryId)
			span.AddEvent("SendEvents", trace.WithAttributes(attribute.Int("events", len(events))))
			span.End()
		case <-srv.Context().Done(): // Activated when ctx.Done() closes
			sublogger.Info().Msgf("Closing FetchWebhookEvents (client context %s closed)", request.ClientId)
			closed = true
			break
		case <-ctx.Done(): // Activated when ctx.Done() closes
			sublogger.Info().Msg("Closing FetchWebhookEvents (main context closed)")
			closed = true
			break
		}
		if closed {
			sublogger.Info().Msg("Context is already closed")
			break
		}
	}
	sublogger.Info().Msgf("Reached %v, so closed context %s", finish, request.ClientId)
	span.AddEvent("Finished", trace.WithAttributes(attribute.String("reason", "timeout")))
	span.End()
	return nil
}

func updateRelayStatus(events []*api.WebhookEvent, repositoryId string) {
	cachedEvents := cache.Store.RetrieveEventsForRepository(repositoryId)
	for _, event := range events {
		eventId := event.EventId
		for _, cachedEvent := range cachedEvents {
			if cachedEvent.ID == eventId {
				cachedEvent.IsRelayed = true
				cachedEvent.TimeRelayed = time.Now()
			}
		}
	}
}

func retrieveCachedEventsForRepository(repositoryId string) ([]*api.WebhookEvent, error) {
	events := make([]*api.WebhookEvent, 0)
	if !cache.Repositories.RepositoryIsWatched(repositoryId) {
		return events, fmt.Errorf("cannot fetch events for empty repository id")
	}
	cachedEvents := cache.Store.RetrieveEventsForRepository(repositoryId)
	for _, cachedEvent := range cachedEvents {
		if cachedEvent.IsRelayed {
			log.Printf("Event is already relayed: %v", cachedEvent)
			continue
		}
		event := api.InternalToExternalEvent(cachedEvent)
		events = append(events, event)
	}
	return events, nil
}

// HealthCheckService implements grpc_health_v1.HealthServer
type HealthCheckService struct{}

// TODO add actual health checks
func (s *HealthCheckService) Check(ctx context.Context, req *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	return &grpc_health_v1.HealthCheckResponse{
		Status: grpc_health_v1.HealthCheckResponse_SERVING,
	}, nil
}

// TODO add actual health checks
func (s *HealthCheckService) Watch(req *grpc_health_v1.HealthCheckRequest, server grpc_health_v1.Health_WatchServer) error {
	return server.Send(&grpc_health_v1.HealthCheckResponse{
		Status: grpc_health_v1.HealthCheckResponse_SERVING,
	})
}
