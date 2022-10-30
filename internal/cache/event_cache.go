package cache

import (
	"fmt"
	v1 "github.com/joostvdg/gitstafette/api/v1"
	"net/http"
	"net/url"
	"time"
)

// TODO register delivery endpoint for Repository
// TODO configure Repositories with TOML
// TODO delivery endpoint contains payloadEndpoint and healthCheckEndpoint
// TODO periodically send events to relay endpoint -> if alive
// TODO support custom CA/Certs for relay endpoint

// TODO cleanup cached events if they are Relayed
// TODO add write protection for the Events

// TODO add lock
var CachedEvents map[*v1.Repository][]*v1.WebhookEventInternal

func init() {
	CachedEvents = make(map[*v1.Repository][]*v1.WebhookEventInternal)
}

// TODO remove endpoint url/relay function from here

func Event(targetRepositoryID string, eventBody []byte, headers http.Header, endpoint *url.URL) error {

	webhookEvent := &v1.WebhookEventInternal{
		IsRelayed: false,
		Timestamp: time.Now(),
		Headers:   headers,
		EventBody: eventBody,
	}
	repository := RepoWatcher.GetRepository(targetRepositoryID)
	repositoryEvents := CachedEvents[repository]
	if repositoryEvents == nil {
		repositoryEvents = make([]*v1.WebhookEventInternal, 0)
	}
	repositoryEvents = append(repositoryEvents, webhookEvent)
	CachedEvents[repository] = repositoryEvents

	fmt.Printf("Cached event for repository %v, current holding %d events for the repository",
		targetRepositoryID, len(repositoryEvents))
	return nil
}
