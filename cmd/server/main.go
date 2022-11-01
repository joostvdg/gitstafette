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
	"strings"
	"syscall"
	"time"

	api "github.com/joostvdg/gitstafette/api/v1"
	"github.com/labstack/echo/v4"
)

const delimiter = ","

// TODO add flags for target for Relay

type server struct {
	api.UnimplementedGitstafetteServer
}

func main() {
	port := flag.String("port", "1323", "Port used for hosting the server")
	grpcPort := flag.String("grpcPort", "50051", "Port used for hosting the grpc server")
	repositoryIDs := flag.String("repositories", "", "Comma separated list of GitHub repository IDs to listen for")
	relayEndpoint := flag.String("relayEndpoint", "", "URL of the Relay Endpoint to deliver the captured events to")
	flag.Parse()

	fmt.Printf("Starting server, http port: %s, grpc port: %s\n", *port, *grpcPort)

	relayEndpointURL, err := url.Parse(*relayEndpoint)
	if err != nil {
		fmt.Println("Malformed URL: ", err.Error())
		return
	}

	if *repositoryIDs == "" || len(*repositoryIDs) <= 1 {
		log.Fatal("Did not receive any RepositoryID to watch")
	} else if strings.Contains(*repositoryIDs, delimiter) {
		repoIds := strings.Split(*repositoryIDs, delimiter)
		for _, repoID := range repoIds {
			repository := api.Repository{ID: repoID}
			cache.RepoWatcher.AddRepository(&repository)
		}
	} else {
		repository := api.Repository{ID: *repositoryIDs}
		cache.RepoWatcher.AddRepository(&repository)
	}

	cache.RepoWatcher.ReportWatchedRepositories()

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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serviceContext := &gcontext.ServiceContext{
		Context:       ctx,
		RelayEndpoint: relayEndpointURL,
	}

	if *relayEndpoint == "" {
		fmt.Printf("RelayEndpoint is empty, disabling relay push\n")
	} else {
		go relay.RelayHealthCheck(serviceContext)
		go relay.RelayCachedEvents(serviceContext)
	}

	grpcServer := grpc.NewServer()
	go func(s *grpc.Server) {
		grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%s", *grpcPort))
		if err != nil {
			log.Fatalf("failed to listen: %v", err)
		}

		api.RegisterGitstafetteServer(s, server{})
		if err := s.Serve(grpcListener); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}(grpcServer)

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
	}(*port)

	// Wait for interrupt signal to gracefully shutdown the server with a timeout of 10 seconds.
	// Use a buffered channel to avoid missing signals as recommended for signal.Notify
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	fmt.Println("Shutdown Echo server")
	if err := e.Shutdown(ctx); err != nil {
		e.Logger.Fatal(err)
	}
	fmt.Println("Shutdown GRPC server")
	grpcServer.GracefulStop()
	fmt.Printf("Shutting down!\n")

}

func (s server) FetchWebhookEvents(request *api.WebhookEventsRequest, srv api.Gitstafette_FetchWebhookEventsServer) error {
	log.Printf("Relaying webhook events for repository %s", request.RepositoryId)

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
		}
	}
	return nil
}

func retrieveCachedEventsForRepository(repositoryId string) ([]*api.WebhookEvent, error) {
	repository := cache.RepoWatcher.GetRepository(repositoryId)
	events := make([]*api.WebhookEvent, 0)
	if repository == nil {
		return events, fmt.Errorf("cannot fetch events for empty repository id")
	}
	cachedEvents := cache.CachedEvents[repository]
	for _, cachedEvent := range cachedEvents {
		headers := make([]*api.Header, len(cachedEvent.Headers))
		for name, values := range cachedEvent.Headers {
			value := values[0]
			header := &api.Header{
				Name:   name,
				Values: value,
			}
			headers = append(headers, header)
		}

		event := &api.WebhookEvent{
			EventId: 0, // TDODO handle eventIDs
			Body:    cachedEvent.EventBody,
			Headers: headers,
		}
		events = append(events, event)
	}
	return events, nil
}
