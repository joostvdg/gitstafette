package cache

import (
	"fmt"
	v1 "github.com/joostvdg/gitstafette/api/v1"
	"net/http"
	"time"
)

// TODO cache events for Repository
// TODO register delivery endpoint for Repository
// TODO configure Repositories with TOML
// TODO delivery endpoint contains payloadEndpoint and healthCheckEndpoint

// TODO add lock
var CachedEvents map[*v1.Repository][]*v1.WebhookEvent

func init() {
	CachedEvents = make(map[*v1.Repository][]*v1.WebhookEvent)
}

func Event(targetRepositoryID string, eventBody []byte, headers http.Header) error {
	webhookEvent := &v1.WebhookEvent{
		Timestamp: time.Now(),
		Headers:   headers,
		EventBody: eventBody,
	}
	repository := RepoWatcher.GetRepository(targetRepositoryID)
	repositoryEvents := CachedEvents[repository]
	if repositoryEvents == nil {
		repositoryEvents = make([]*v1.WebhookEvent, 0)
	}
	repositoryEvents = append(repositoryEvents, webhookEvent)
	CachedEvents[repository] = repositoryEvents

	fmt.Printf("Cached event for repository %v, current holding %d events for the repository",
		targetRepositoryID, len(repositoryEvents))
	return nil
}
