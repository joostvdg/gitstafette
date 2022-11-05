package cache

import (
	api "github.com/joostvdg/gitstafette/api/v1"
	"log"
	"net/http"
	"time"
)

// TODO register delivery endpoint for Repository
// TODO configure Repositories with TOML
// TODO delivery endpoint contains payloadEndpoint and healthCheckEndpoint
// TODO periodically send events to relay endpoint -> if alive
// TODO support custom CA/Certs for relay endpoint

// TODO cleanup cached events if they are Relayed
// TODO add write protection for the Events
// TODO use Redis for caching if we can
// TODO so, we get an interface -> InMemory and Redis impls?

// TODO add lock
var CachedEvents map[*api.Repository][]*api.WebhookEventInternal

func init() {
	CachedEvents = make(map[*api.Repository][]*api.WebhookEventInternal)
}

// TODO do this properly
func PrepareForShutdown() {
	CachedEvents = make(map[*api.Repository][]*api.WebhookEventInternal)
}

func Event(targetRepositoryID string, event *api.WebhookEvent) error {
	var headers http.Header
	headers = make(map[string][]string)

	for _, header := range event.Headers {
		key := header.Name
		value := header.Values
		values := make([]string, 1)
		values[0] = value
		headers[key] = values
	}

	webhookEvent := &api.WebhookEventInternal{
		IsRelayed: false,
		Timestamp: time.Now(),
		Headers:   headers,
		EventBody: event.Body,
	}
	return processEvent(targetRepositoryID, webhookEvent)

}

func InternalEvent(targetRepositoryID string, eventBody []byte, headers http.Header) error {
	webhookEvent := &api.WebhookEventInternal{
		IsRelayed: false,
		Timestamp: time.Now(),
		Headers:   headers,
		EventBody: eventBody,
	}
	return processEvent(targetRepositoryID, webhookEvent)
}

func processEvent(targetRepositoryID string, event *api.WebhookEventInternal) error {
	repository := RepoWatcher.GetRepository(targetRepositoryID)
	repositoryEvents := CachedEvents[repository]
	if repositoryEvents == nil {
		repositoryEvents = make([]*api.WebhookEventInternal, 0)
	}
	repositoryEvents = append(repositoryEvents, event)
	CachedEvents[repository] = repositoryEvents

	log.Printf("Cached event for repository %v, current holding %d events for the repository",
		targetRepositoryID, len(repositoryEvents))
	return nil
}
