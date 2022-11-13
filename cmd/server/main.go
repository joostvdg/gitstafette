package main

import (
	"context"
	"flag"
	"fmt"
	internal_api "github.com/joostvdg/gitstafette/internal/api/v1"
	"github.com/joostvdg/gitstafette/internal/cache"
	gcontext "github.com/joostvdg/gitstafette/internal/context"
	"github.com/joostvdg/gitstafette/internal/relay"
	"google.golang.org/grpc"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	api "github.com/joostvdg/gitstafette/api/v1"
	"github.com/labstack/echo/v4"
)

// TODO add flags for target for Relay

type server struct {
	api.UnimplementedGitstafetteServer
}

func main() {
	port := flag.String("port", "1323", "Port used for hosting the server")
	grpcPort := flag.String("grpcPort", "50051", "Port used for hosting the grpc server")
	repositoryIDs := flag.String("repositories", "", "Comma separated list of GitHub repository IDs to listen for")
	relayEndpoint := flag.String("relayEndpoint", "", "URL of the Relay Endpoint to deliver the captured events to")
	redisDatabase := flag.String("redisDatabase", "0", "Database used for redis")
	redisHost := flag.String("redisHost", "localhost", "Host of the Redis server")
	redisPort := flag.String("redisPort", "6379", "Port of the Redis server")
	redisPassword := flag.String("redisPassword", "", "Password of the Redis server (default is no password")
	flag.Parse()

	log.Printf("Starting server, http port: %s, grpc port: %s\n", *port, *grpcPort)

	redisConfig := &cache.RedisConfig{
		Host:     *redisHost,
		Port:     *redisPort,
		Password: *redisPassword,
		Database: *redisDatabase,
	}
	cache.InitCache(*repositoryIDs, redisConfig)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serviceContext := &gcontext.ServiceContext{
		Context: ctx,
	}

	var relayEndpointURL *url.URL
	if *relayEndpoint == "" {
		relayEndpointURL = relay.InitiateRelayOrDie(*relayEndpoint, serviceContext)
	}
	grpcServer := initializeGRPCServer(*grpcPort)
	echoServer := initializeEchoServer(relayEndpointURL, *port)

	// Wait for interrupt signal to gracefully shut down the server with a timeout of 10 seconds.
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
	log.Println("Shutting down GRPC server")
	grpcServer.GracefulStop()
	cache.PrepareForShutdown()
	log.Printf("Shutting down!\n")
}

func initializeEchoServer(relayEndpointURL *url.URL, port string) *echo.Echo {
	e := echo.New()
	e.Use(func(e echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			gitstatefetteContext := &gcontext.GitstafetteContext{
				Context:       c,
				RelayEndpoint: relayEndpointURL,
			}
			return e(gitstatefetteContext)
		}
	})

	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "Hello, World!")
	})
	e.POST("/v1/github/", internal_api.HandleGitHubPost)
	e.GET("/v1/watchlist/", internal_api.HandleWatchListGet)
	e.GET("/v1/events/:repo", internal_api.HandleRetrieveEventsForRepository)

	// Start Echo server
	go func(echoPort string) {
		if err := e.Start(":" + echoPort); err != nil && err != http.ErrServerClosed {
			e.Logger.Fatal("shutting down the server")
		}
	}(port)
	return e
}

func initializeGRPCServer(grpcPort string) *grpc.Server {
	grpcServer := grpc.NewServer()
	go func(s *grpc.Server) {
		grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
		if err != nil {
			log.Fatalf("failed to listen: %v", err)
		}

		api.RegisterGitstafetteServer(s, server{})
		if err := s.Serve(grpcListener); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
		log.Println("Shutdown GRPC server")
	}(grpcServer)
	return grpcServer
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

		headers := make([]*api.Header, len(cachedEvent.Headers))
		for _, header := range cachedEvent.Headers {
			header := &api.Header{
				Name:   header.Key,
				Values: header.FirstValue,
			}
			headers = append(headers, header)
		}

		event := &api.WebhookEvent{
			EventId: cachedEvent.ID,
			Body:    []byte(cachedEvent.EventBody),
			Headers: headers,
		}
		events = append(events, event)
	}
	return events, nil
}
