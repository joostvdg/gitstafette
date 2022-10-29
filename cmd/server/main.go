package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/joostvdg/gitstafette/internal/api/v1"
	"github.com/joostvdg/gitstafette/internal/cache"
	gcontext "github.com/joostvdg/gitstafette/internal/context"
	"github.com/joostvdg/gitstafette/internal/relay"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	api "github.com/joostvdg/gitstafette/api/v1"
	"github.com/labstack/echo/v4"
)

const delimiter = ","

// TODO add flags for target for Relay

func main() {
	port := flag.String("port", "1323", "Port used for hosting the server")
	repositoryIDs := flag.String("repositories", "", "Comma separated list of GitHub repository IDs to listen for")
	relayEndpoint := flag.String("relayEndpoint", "", "URL of the Relay Endpoint to deliver the captured events to")
	flag.Parse()

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
	relay.RelayHealthCheck(serviceContext)
	relay.RelayCachedEvents(serviceContext)

	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "Hello, World!")
	})
	e.POST("/v1/github/", v1.HandleGitHubPost)
	e.GET("/v1/watchlist/", v1.HandleWatchListGet)
	e.GET("/v1/events/:repo", v1.HandleRetrieveEventsForRepository)
	e.Logger.Fatal(e.Start(":" + *port))
	fmt.Printf("Shutting down!\n")
}
