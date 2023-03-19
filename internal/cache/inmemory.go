package cache

import (
	api "github.com/joostvdg/gitstafette/api/v1"
	"golang.org/x/exp/slices"
	"sync"
)

type inMemoryStore struct {
	mu     sync.RWMutex
	events map[string][]*api.WebhookEventInternal
}

func NewInMemoryStore() *inMemoryStore {
	i := new(inMemoryStore)
	i.events = make(map[string][]*api.WebhookEventInternal)
	return i
}

// TODO need a lock of some sort
func (i *inMemoryStore) Store(repositoryId string, event *api.WebhookEventInternal) bool {
	sublogger.Debug().Msg("Claiming lock")
	i.mu.Lock()
	defer i.mu.Unlock()
	events := i.events[repositoryId]
	if events == nil {
		events = make([]*api.WebhookEventInternal, 0)
	}
	for _, storedEvent := range events {
		if storedEvent.ID == event.ID {
			sublogger.Warn().Str("repo", repositoryId).Str("event", event.ID).Msg("Already stored this event, skipping")
			return false
		}
	}

	event.IsRelayed = false
	events = append(events, event)
	i.events[repositoryId] = events
	sublogger.Info().Msgf("Cached event for repository %v, currently holding %d events for the repository",
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

func (i *inMemoryStore) Remove(repositoryId string, event *api.WebhookEventInternal) bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	events := i.events[repositoryId]
	indexToRemove := slices.Index(events, event)
	updatedEvents := removeElementByIndex(events, indexToRemove)
	i.events[repositoryId] = updatedEvents
	return true
}

// removes element while preserving order
// copied from here: https://golangprojectstructure.com/removing-elements-from-slice-array/
// TODO: do we need to preserve order?
func removeElementByIndex[T any](slice []T, index int) []T {
	return append(slice[:index], slice[index+1:]...)
}
