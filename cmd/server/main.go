package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/getsentry/sentry-go"
	sentryecho "github.com/getsentry/sentry-go/echo"
	internal_api "github.com/joostvdg/gitstafette/internal/api/v1"
	"github.com/joostvdg/gitstafette/internal/cache"
	"github.com/joostvdg/gitstafette/internal/config"
	gcontext "github.com/joostvdg/gitstafette/internal/context"
	grpc_internal "github.com/joostvdg/gitstafette/internal/grpc"
	"github.com/joostvdg/gitstafette/internal/info"
	"github.com/joostvdg/gitstafette/internal/otel_util"
	"github.com/joostvdg/gitstafette/internal/relay"
	"github.com/joostvdg/gitstafette/internal/server"
	"github.com/labstack/echo/v4/middleware"
	"github.com/rs/zerolog"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	//semconv "go.opentelemetry.io/otel_util/semconv/v1.18.0"
	"go.opentelemetry.io/otel/trace"

	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	infoapi "github.com/joostvdg/gitstafette/api/info"
	api "github.com/joostvdg/gitstafette/api/v1"
	"github.com/labstack/echo/v4"
)

// TODO add flags for target for Relay

const (
	envSentry        = "SENTRY_DSN"
	responseInterval = time.Second * 5
)

var (
	mp                 *sdkmetric.MeterProvider
	tracer             trace.Tracer
	otelEnabled        bool
	errMissingMetadata = status.Errorf(codes.InvalidArgument, "missing metadata")
	errInvalidToken    = status.Errorf(codes.Unauthenticated, "invalid token")
)

func main() {
	name := flag.String("name", "GSF-Server", "Name of the GitstafetteServer")
	port := flag.String("port", "1323", "Port used for hosting the GitstafetteServer")
	grpcPort := flag.String("grpcPort", "50051", "Port used for hosting the grpc streaming GitstafetteServer")
	grpcHealthPort := flag.String("grpcHealthPort", "50052", "Port used for hosting the grpc health checks")
	repositoryIDs := flag.String("repositories", "", "Comma separated list of GitHub repository IDs to listen for")
	redisDatabase := flag.String("redisDatabase", "0", "Database used for redis")
	redisHost := flag.String("redisHost", "localhost", "Host of the Redis GitstafetteServer")
	redisPort := flag.String("redisPort", "6379", "Port of the Redis GitstafetteServer")
	redisPassword := flag.String("redisPassword", "", "Password of the Redis GitstafetteServer (default is no password")
	relayEnabled := flag.Bool("relayEnabled", false, "If the GitstafetteServer should relay received events, rather than caching them for clients")
	relayHost := flag.String("relayHost", "127.0.0.1", "Host address to relay events to")
	relayPath := flag.String("relayPath", "/", "Path on the host address to relay events to")
	relayHealthCheckPath := flag.String("relayHealthCheckPath", "/", "Path on the host address to do health check on, for relay target")
	relayPort := flag.String("relayPort", "50051", "The port of the relay address")
	relayProtocol := flag.String("relayProtocol", "grpc", "The protocol for the relay address (grpc, or http)")
	relayInsecure := flag.Bool("relayInsecure", false, "If the relay GitstafetteServer should be handled insecurely")
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

	otelEnabled = otel_util.IsOTelEnabled()
	if otelEnabled {
		log.Info().Msg("OTEL is enabled")
		var otelShutdown func(context.Context) error
		otelShutdown, err, metricProvider, tp := otel_util.SetupOTelSDK(ctx, "gsf-client", "0.0.1")
		mp = metricProvider
		tracer = tp.Tracer("gsf-GitstafetteServer")
		if err != nil {
			log.Fatal().Err(err).Msg("Could not configure OTEL URL")
		}
		// Handle shutdown properly so nothing leaks.
		defer func() {
			err = errors.Join(err, otelShutdown(context.Background()))
		}()
	} else {
		log.Info().Msg("OTEL is disabled")
	}

	relayConfig, err := api.CreateRelayConfig(*relayEnabled, *relayHost, *relayPath, *relayHealthCheckPath, *relayPort, *relayProtocol, *relayInsecure)
	if err != nil {
		log.Fatal().Err(err).Msg("Malformed URL")
	}
	serverConfig := &api.ServerConfig{
		Name:         *name,
		Host:         "localhost",
		Port:         *port,
		GrpcPort:     *grpcPort,
		Repositories: repoIds,
	}

	initSentry() // has to happen before we init Echo
	var grpcHealthServer *grpc.Server
	if *grpcHealthPort != *grpcPort {
		grpcHealthServer = initializeGRPCHealthServer(*grpcHealthPort)
	}

	grpcServer := initializeGRPCServer(*grpcPort, tlsConfig, grpcHealthServer, ctx, serverConfig, relayConfig)
	echoServer := initializeEchoServer(relayConfig, *port, *webhookHMAC)
	log.Printf("Started http GitstafetteServer on: %s, grpc GitstafetteServer on: %s, and grpc health GitstafetteServer on: %s\n", *port, *grpcPort, *grpcHealthPort)

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
	log.Info().Msg("Shutting down Echo GitstafetteServer")
	if err := echoServer.Shutdown(ctx); err != nil {
		echoServer.Logger.Fatal(err)
	}
	log.Info().Msg("Shutting down GRPC gitstafette GitstafetteServer")
	grpcServer.GracefulStop()
	log.Info().Msg("Shutting down GRPC health GitstafetteServer")
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

	// Start Echo GitstafetteServer
	go func(echoPort string) {
		if err := e.Start(":" + echoPort); err != nil && err != http.ErrServerClosed {
			e.Logger.Fatal("shutting down the Echo GitstafetteServer")
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

// valid validates the authorization.
func valid(authorization []string) bool {
	if len(authorization) < 1 {
		return false
	}
	token := strings.TrimPrefix(authorization[0], "Bearer ")
	// Perform the token validation here. For the sake of this example, the code
	// here forgoes any of the usual OAuth2 token validation and instead checks
	// for a token matching an arbitrary string.
	return token == "some-secret-token"
}

func unaryInterceptor(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	// authentication (token verification)
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, errMissingMetadata
	}
	if !valid(md["authorization"]) {
		return nil, errInvalidToken
	}
	m, err := handler(ctx, req)
	if err != nil {
		log.Error().Err(err).Msg("RPC failed")
	}
	return m, err
}

func initializeGRPCHealthServer(grpcPort string) *grpc.Server {
	grpcServer := grpc.NewServer(grpc.KeepaliveEnforcementPolicy(kaep), grpc.KeepaliveParams(kasp), grpc.UnaryInterceptor(unaryInterceptor))

	go func(s *grpc.Server) {
		grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
		if err != nil {
			log.Fatal().Err(err).Msg("failed to listen")
		}

		grpc_health_v1.RegisterHealthServer(s, &grpc_internal.HealthCheckService{})
		if err := s.Serve(grpcListener); err != nil {
			log.Fatal().Err(err).Msg("failed to serve")
		}
		log.Info().Msg("Shutdown GRPC health GitstafetteServer")
	}(grpcServer)
	return grpcServer
}

func initializeGRPCServer(grpcPort string, tlsConfig *tls.Config, healthServer *grpc.Server, ctx context.Context, serverConfig *api.ServerConfig, relayConfig *api.RelayConfig) *grpc.Server {
	grpcServer := grpc.NewServer(
		grpc.ChainStreamInterceptor(grpc_internal.ValidateToken),
	)

	if tlsConfig != nil {
		serverCredentials := credentials.NewTLS(tlsConfig)
		grpcServer = grpc.NewServer(
			grpc.Creds(serverCredentials),
			grpc.ChainStreamInterceptor(grpc_internal.ValidateToken),
		)
	}

	go func(s *grpc.Server) {
		grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
		if err != nil {
			log.Fatal().Err(err).Msg("failed to listen")
		}

		log.Printf("Starting GRPC GitstafetteServer")
		api.RegisterGitstafetteServer(s, &server.GitstafetteServer{
			Tracer:           tracer,
			MeterProvider:    mp,
			ResponseInterval: responseInterval,
		})
		infoapi.RegisterInfoServer(s, &info.InfoServer{
			RelayConfig:  relayConfig,
			ServerConfig: serverConfig,
			Tracer:       tracer,
			Type:         infoapi.InstanceType_SERVER,
		})
		if healthServer == nil {
			log.Info().Msg("GRPC HealthCheck GitstafetteServer is empty, running service with normal GRPC GitstafetteServer")
			grpc_health_v1.RegisterHealthServer(s, &grpc_internal.HealthCheckService{})
		} else {
			log.Printf("Running GRPC HealthCheck GitstafetteServer standalone\n", s.GetServiceInfo())
		}

		if err := s.Serve(grpcListener); err != nil {
			log.Fatal().Err(err).Msg("failed to serve")
		}
		log.Info().Msg("Shutdown GRPC gitstafette GitstafetteServer")
	}(grpcServer)
	return grpcServer
}
