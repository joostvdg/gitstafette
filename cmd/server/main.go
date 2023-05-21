package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"github.com/getsentry/sentry-go"
	sentryecho "github.com/getsentry/sentry-go/echo"
	internal_api "github.com/joostvdg/gitstafette/internal/api/v1"
	"github.com/joostvdg/gitstafette/internal/cache"
	"github.com/joostvdg/gitstafette/internal/config"
	gcontext "github.com/joostvdg/gitstafette/internal/context"
	grpc_internal "github.com/joostvdg/gitstafette/internal/grpc"
	"github.com/joostvdg/gitstafette/internal/relay"
	"github.com/labstack/echo/v4/middleware"
	"github.com/rs/zerolog"
	"sync"

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

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	api "github.com/joostvdg/gitstafette/api/v1"
	"github.com/labstack/echo/v4"
)

// TODO add flags for target for Relay

const (
	envSentry        = "SENTRY_DSN"
	responseInterval = time.Second * 30
)

var (
	resource          *sdkresource.Resource
	initResourcesOnce sync.Once
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
	relayConfig, err := api.CreateRelayConfig(*relayEnabled, *relayHost, *relayPath, *relayHealthCheckPath, *relayPort, *relayProtocol, *relayInsecure)
	if err != nil {
		log.Fatal().Err(err).Msg("Malformed URL")
	}

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
	initSentry() // has to happen before we init Echo
	var grpcHealthServer *grpc.Server
	if *grpcHealthPort != *grpcPort {
		grpcHealthServer = initializeGRPCHealthServer(*grpcHealthPort)
	}
	grpcServer := initializeGRPCServer(*grpcPort, tlsConfig, grpcHealthServer)
	echoServer := initializeEchoServer(relayConfig, *port, *webhookHMAC)
	log.Printf("Started http server on: %s, grpc server on: %s, and grpc health server on: %s\n", *port, *grpcPort, *grpcHealthPort)

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
	Timeout:               10 * time.Second,  // Wait 1 second for the ping ack before assuming the connection is dead
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

func initializeGRPCServer(grpcPort string, tlsConfig *tls.Config, healthServer *grpc.Server) *grpc.Server {
	tp := initTracerProvider()
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Fatal().Err(err).Msg("Tracer Provider Shutdown")
		}
	}()

	mp := initMeterProvider()
	defer func() {
		if err := mp.Shutdown(context.Background()); err != nil {
			log.Fatal().Err(err).Msg("Error shutting down meter provider")
		}
	}()

	grpcServer := grpc.NewServer(
		grpc.KeepaliveEnforcementPolicy(kaep),
		grpc.KeepaliveParams(kasp),
		grpc.ChainStreamInterceptor(grpc_internal.ValidateToken,grpc_internal.EventsServerStreamInterceptor,otelgrpc.StreamServerInterceptor()),
		grpc.UnaryInterceptor(otelgrpc.UnaryServerInterceptor()),
	)

	if tlsConfig != nil {
		serverCredentials := credentials.NewTLS(tlsConfig)
		grpcServer = grpc.NewServer(
			grpc.KeepaliveEnforcementPolicy(kaep),
			grpc.KeepaliveParams(kasp),
			grpc.Creds(serverCredentials),
			grpc.ChainStreamInterceptor(grpc_internal.ValidateToken, grpc_internal.EventsServerStreamInterceptor, otelgrpc.StreamServerInterceptor()),
			grpc.UnaryInterceptor(otelgrpc.UnaryServerInterceptor()),
		)
	}

	go func(s *grpc.Server) {
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

	durationSeconds := request.GetDurationSecs()
	finish := time.Now().Add(time.Second * time.Duration(durationSeconds))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	waitInterval := responseInterval
	durationIntervalCalc := (durationSeconds / 3) - 5
	if time.Duration(durationIntervalCalc) > waitInterval {
		waitInterval = time.Duration(durationIntervalCalc)
	}
	log.Printf("Wait Interval is: %v", waitInterval)

	for time.Now().Before(finish) {
		select {
		case <-time.After(waitInterval):
			log.Printf("Fetching events for repo %v (with Span)", request.RepositoryId)
			_, span := otel.Tracer("Server").Start(srv.Context() , "FetchWebhookEvents")
			events, err := retrieveCachedEventsForRepository(request.RepositoryId)

			// TODO properly handleerror
			if err != nil {
				log.Printf("Could not get events for Repo: %v\n", err)
			}
			response := &api.WebhookEventsResponse{
				WebhookEvents: events,
			}
			log.Printf("Send %v events to client (%v) for repo %v", len(events), request.ClientId, request.RepositoryId)

			if err := srv.Send(response); err != nil {
				log.Printf("Error sending stream: %v\n", err)
				return err
			}

			updateRelayStatus(events, request.RepositoryId)
			log.Info().Msg("Closing Span")
			span.End()
		case <-srv.Context().Done(): // Activated when ctx.Done() closes
			log.Printf("Closing FetchWebhookEvents (client context %s closed)", request.ClientId)
			return nil
		case <-ctx.Done(): // Activated when ctx.Done() closes
			log.Info().Msg("Closing FetchWebhookEvents (main context closed)")
			return nil
		}
	}
	log.Printf("Reached %v, so closed context %s", finish, request.ClientId)
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


func initResource() *sdkresource.Resource {
	initResourcesOnce.Do(func() {
		extraResources, _ := sdkresource.New(
			context.Background(),
			sdkresource.WithOS(),
			sdkresource.WithProcess(),
			sdkresource.WithContainer(),
			sdkresource.WithHost(),
		)
		resource, _ = sdkresource.Merge(
			sdkresource.Default(),
			extraResources,
		)
	})
	return resource
}

func initTracerProvider() *sdktrace.TracerProvider {
	ctx := context.Background()

	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure())
	if err != nil {
		log.Fatal().Err(err).Msg("OTLP Trace gRPC Creation failed")
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(initResource()),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp
}

func initMeterProvider() *sdkmetric.MeterProvider {
	ctx := context.Background()

	exporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithInsecure())
	if err != nil {
		log.Fatal().Err(err).Msg("new otlp metric grpc exporter failed")
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
		sdkmetric.WithResource(initResource()),
	)
	global.SetMeterProvider(mp)
	return mp
}

