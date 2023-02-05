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
	"github.com/joostvdg/gitstafette/internal/relay"
	"github.com/labstack/echo/v4/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	api "github.com/joostvdg/gitstafette/api/v1"
	"github.com/labstack/echo/v4"
)

// TODO add flags for target for Relay

const (
	envSentry        = "SENTRY_DSN"
	envOauthToken    = "OAUTH_TOKEN"
	responseInterval = time.Second * 3
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

	tlsConfig, err := config.NewTLSConfig(*caFileLocation, *certFileLocation, *certKeyFileLocation, true)
	if err != nil {
		log.Fatal("Invalid certificate configuration: ", err.Error())
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
		log.Fatal("Malformed URL: ", err.Error())
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
	log.Println("Shutting down Echo server")
	if err := echoServer.Shutdown(ctx); err != nil {
		echoServer.Logger.Fatal(err)
	}
	log.Println("Shutting down GRPC gitstafette server")
	grpcServer.GracefulStop()
	log.Println("Shutting down GRPC health server")
	if *grpcHealthPort != *grpcPort {
		grpcHealthServer.GracefulStop()
	}
	cache.PrepareForShutdown()
	log.Printf("Shutting down!\n")
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
	MinTime:             5 * time.Second, // If a client pings more than once every 5 seconds, terminate the connection
	PermitWithoutStream: true,            // Allow pings even when there are no active streams
}

var kasp = keepalive.ServerParameters{
	MaxConnectionIdle:     15 * time.Second, // If a client is idle for 15 seconds, send a GOAWAY
	MaxConnectionAge:      30 * time.Second, // If any connection is alive for more than 30 seconds, send a GOAWAY
	MaxConnectionAgeGrace: 5 * time.Second,  // Allow 5 seconds for pending RPCs to complete before forcibly closing connections
	Time:                  5 * time.Second,  // Ping the client if it is idle for 5 seconds to ensure the connection is still active
	Timeout:               1 * time.Second,  // Wait 1 second for the ping ack before assuming the connection is dead
}

func initializeGRPCHealthServer(grpcPort string) *grpc.Server {
	grpcServer := grpc.NewServer(grpc.KeepaliveEnforcementPolicy(kaep), grpc.KeepaliveParams(kasp))

	go func(s *grpc.Server) {
		grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
		if err != nil {
			log.Fatalf("failed to listen: %v", err)
		}

		grpc_health_v1.RegisterHealthServer(s, &HealthCheckService{})
		if err := s.Serve(grpcListener); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
		log.Println("Shutdown GRPC health server")
	}(grpcServer)
	return grpcServer
}

func initializeGRPCServer(grpcPort string, tlsConfig *tls.Config, healthServer *grpc.Server) *grpc.Server {
	grpcServer := grpc.NewServer(grpc.KeepaliveEnforcementPolicy(kaep), grpc.KeepaliveParams(kasp), grpc.StreamInterceptor(validateToken))
	if tlsConfig != nil {
		serverCredentials := credentials.NewTLS(tlsConfig)
		grpcServer = grpc.NewServer(
			grpc.KeepaliveEnforcementPolicy(kaep),
			grpc.KeepaliveParams(kasp),
			grpc.Creds(serverCredentials),
			grpc.StreamInterceptor(validateToken),
		)
	}

	go func(s *grpc.Server) {
		grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
		if err != nil {
			log.Fatalf("failed to listen: %v", err)
		}

		log.Printf("Starting GRPC server")
		api.RegisterGitstafetteServer(s, &server{})
		if healthServer == nil {
			log.Printf("GRPC HealthCheck server is empty, running service with normal GRPC server\n")
			grpc_health_v1.RegisterHealthServer(s, &HealthCheckService{})
		} else {
			log.Printf("Running GRPC HealthCheck server standalone\n", s.GetServiceInfo())
		}

		if err := s.Serve(grpcListener); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
		log.Println("Shutdown GRPC gitstafette server")
	}(grpcServer)
	return grpcServer
}

func validateToken(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	log.Printf("Validating token for GRPC Stream Request")
	oauthToken, oauthOk := os.LookupEnv(envOauthToken)
	if oauthOk {
		log.Printf("Validating token for GRPC Stream Request -> TOKEN FOUND")
		md, ok := metadata.FromIncomingContext(ss.Context())
		if !ok {
			errorMessage := "missing metadata when validating OAuth Token"
			log.Println(errorMessage)
			return status.Error(codes.InvalidArgument, errorMessage)
		}

		if !valid(md["authorization"], oauthToken) {
			errorMessage := "OAuth Token Missing Or Not Valid"
			log.Println(errorMessage)
			return status.Error(codes.Unauthenticated, errorMessage)
		} else {
			log.Printf("Validating token for GRPC Stream Request -> TOKEN VALID")
		}
	} else {
		log.Println("Validating token for GRPC Stream Request -> TOKEN MISSING")
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

	for time.Now().Before(finish) {
		select {
		case <-time.After(responseInterval):
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
		case <-srv.Context().Done(): // Activated when ctx.Done() closes
			log.Printf("Closing FetchWebhookEvents (client context %s closed)", request.ClientId)
			return nil
		case <-ctx.Done(): // Activated when ctx.Done() closes
			log.Println("Closing FetchWebhookEvents (main context closed)")
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
