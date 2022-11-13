package cache

import (
	api "github.com/joostvdg/gitstafette/api/v1"
	"log"
)

type inMemoryStore struct {
	events map[string][]*api.WebhookEventInternal
}

func NewInMemoryStore() *inMemoryStore {
	i := new(inMemoryStore)
	i.events = make(map[string][]*api.WebhookEventInternal)
	return i
}

// TODO need a lock of some sort
func (i *inMemoryStore) Store(repositoryId string, event *api.WebhookEventInternal) bool {
	events := i.events[repositoryId]
	if events == nil {
		events = make([]*api.WebhookEventInternal, 0)
	}
	events = append(events, event)
	i.events[repositoryId] = events
	log.Printf("Cached event for repository %v, currently holding %d events for the repository",
		repositoryId, len(events))
	return true
}

func (i *inMemoryStore) RetrieveEventsForRepository(repositoryId string) []*api.WebhookEventInternal {
	events := make([]*api.WebhookEventInternal, 0)
	if i.CountEventsForRepository(repositoryId) > 0 {
		events = i.events[repositoryId]
	}
	return events
}

func (i *inMemoryStore) CountEventsForRepository(repositoryId string) int {
	events := i.events[repositoryId]
	if events == nil {
		return 0
	}
	return len(events)
}

func (i *inMemoryStore) IsConnected() bool {
	return true
}
