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
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health/grpc_health_v1"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	api "github.com/joostvdg/gitstafette/api/v1"
	"github.com/labstack/echo/v4"
)

// TODO add flags for target for Relay

const (
	envSentry = "SENTRY_DSN"
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
	relayConfig, err := api.CreateRelayConfig(*relayEnabled, *relayHost, *relayPort, *relayProtocol, *relayInsecure)
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
	grpcHealthServer := initializeGRPCHealthServer(*grpcHealthPort)
	grpcServer := initializeGRPCServer(*grpcPort, tlsConfig)
	echoServer := initializeEchoServer(relayConfig, *port, *webhookHMAC)
	log.Printf("Started http server on: %s, grpc server on: %s, and grpc health server on: %s\n", *port, *grpcPort, *grpcHealthPort)

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
	grpcHealthServer.GracefulStop()
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

func initializeGRPCHealthServer(grpcPort string) *grpc.Server {
	grpcServer := grpc.NewServer()

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

func initializeGRPCServer(grpcPort string, tlsConfig *tls.Config) *grpc.Server {
	serverCredentials := credentials.NewTLS(tlsConfig)
	grpcServer := grpc.NewServer(grpc.Creds(serverCredentials))

	go func(s *grpc.Server) {
		grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
		if err != nil {
			log.Fatalf("failed to listen: %v", err)
		}

		api.RegisterGitstafetteServer(s, server{})
		if err := s.Serve(grpcListener); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
		log.Println("Shutdown GRPC gitstafette server")
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
	}
	return response, err
}

func (s server) FetchWebhookEvents(request *api.WebhookEventsRequest, srv api.Gitstafette_FetchWebhookEventsServer) error {
	log.Printf("Relaying webhook events for repository %s", request.RepositoryId)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	clock := time.NewTicker(5 * time.Second)
	for {
		select {
		case <-clock.C:
			events, err := retrieveCachedEventsForRepository(request.RepositoryId)

			// TODO properly handleerror
			if err != nil {
				log.Printf("Could not get events for Repo: %v\n", err)
			}
			response := &api.WebhookEventsResponse{
				WebhookEvents: events,
			}
			srv.Send(response)
			updateRelayStatus(events, request.RepositoryId)

		case <-ctx.Done(): // Activated when ctx.Done() closes
			log.Println("Closing FetchWebhookEvents")
			return nil
		}
	}
}

func updateRelayStatus(events []*api.WebhookEvent, repositoryId string) {
	cachedEvents := cache.Store.RetrieveEventsForRepository(repositoryId)
	for _, event := range events {
		eventId := event.EventId
		for _, cachedEvent := range cachedEvents {
			if cachedEvent.ID == eventId {
				cachedEvent.IsRelayed = true
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
