package relay

import (
	"fmt"
	"github.com/go-resty/resty/v2"
	v1 "github.com/joostvdg/gitstafette/api/v1"
	"github.com/joostvdg/gitstafette/internal/cache"
	"github.com/joostvdg/gitstafette/internal/context"
	"time"

	"net/url"
)

// TODO periodically relay message to relay endpoint
// TODO health check on relay endpoint
// TODO remove relayed messages from cache

type Status struct {
	LastCheckWasSuccessfull     bool
	CounterOfFailedHealthChecks int
	TimeOfLastCheck             time.Time
	TimeOfLastFailure           time.Time
}

// BasicRelay testing the relay functionality
func BasicRelay(event *v1.WebhookEventInternal, relayEndpoint *url.URL) {
	client := resty.New()

	request := client.R().SetBody(event.EventBody)
	request.Header = event.Headers
	response, err := request.Post(relayEndpoint.String())
	if err != nil {
		fmt.Printf("Encountered an error when relaying: %v\n", err)
	} else {
		fmt.Printf("Response: %v\n", response)
	}

}

func RelayCachedEvents(serviceContext *context.ServiceContext) {
	ctx := serviceContext.Context
	clock := time.NewTicker(30 * time.Second)
	for {
		select {
		case <-clock.C:
			// TODO handle properly
			events := cache.CachedEvents
			for _, webhookEvents := range events {
				for _, webhookEvent := range webhookEvents {
					if !webhookEvent.IsRelayed {
						BasicRelay(webhookEvent, serviceContext.RelayEndpoint)
						// TODO add check on relay, so that we only set IsRelayed if we actually did
						webhookEvent.IsRelayed = true
					}
				}

			}
		case <-ctx.Done(): // Activated when ctx.Done() closes
			fmt.Println("Closing RelayCachedEvents")
			return
		}
	}
}

/**
This is a GitHub Ping Header Set
Request URL: https://smee.io/3l3edGAqmbBJ9x9
Request method: POST
content-type: application/json
User-Agent: GitHub-Hookshot/ede37db
X-GitHub-Delivery: d4049330-377e-11ed-9c2e-1ae286aab35f
X-GitHub-Event: ping
X-GitHub-Hook-ID: 380052596
X-GitHub-Hook-Installation-Target-ID: 537845873
X-GitHub-Hook-Installation-Target-Type: repository
*/

func RelayHealthCheck(serviceContext *context.ServiceContext) {
	status := Status{
		LastCheckWasSuccessfull:     false,
		CounterOfFailedHealthChecks: 0,
		TimeOfLastCheck:             time.Now(),
	}
	ctx := serviceContext.Context
	clock := time.NewTicker(30 * time.Second)
	for {
		select {
		case <-clock.C:
			// TODO do healthcheck
			keys := cache.RepoWatcher.WatchedRepositories()
			status.TimeOfLastCheck = time.Now()
			healthy, err := doHealthCheck(serviceContext.RelayEndpoint, keys[0])

			if err != nil {
				fmt.Printf("Encountered an error doing healthcheck on relay: %v\n", err)
			}

			if !healthy {
				status.CounterOfFailedHealthChecks = status.CounterOfFailedHealthChecks + 1
				status.TimeOfLastFailure = time.Now()
				status.LastCheckWasSuccessfull = false
			} else {
				status.LastCheckWasSuccessfull = true
				status.CounterOfFailedHealthChecks = 0
			}
		case <-ctx.Done(): // Activated when ctx.Done() closes
			fmt.Println("Closing RelayHealthCheck")
			return
		}
	}
}

// TODO verify healthcheck with Jenkins or something similar
func doHealthCheck(relayEndpoint *url.URL, repositoryId string) (bool, error) {
	fmt.Printf("Doing healthcheck for relay %v (using repo %v)\n", relayEndpoint.String(), repositoryId)
	client := resty.New()
	response, err := client.R().
		SetHeader("X-GitHub-Event", "ping").
		SetHeader("X-GitHub-Hook-Installation-Target-Type", "repository").
		SetHeader("X-GitHub-Hook-Installation-Target-ID", repositoryId).
		SetHeader("User-Agent", "Gitstafette").
		SetBody(`{"zen": "Design for failure.","repository": {"id": ` + repositoryId + `}}`).
		Post(relayEndpoint.String())
	if err != nil {
		fmt.Printf("Encountered an error when relaying: %v\n", err)
		return false, err
	}
	fmt.Printf("Response: %v\n", response)
	return true, nil
}
