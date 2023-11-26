package cache

import (
	"context"
	api "github.com/joostvdg/gitstafette/api/v1"
	"github.com/joostvdg/gitstafette/internal/otel_util"
	otelapi "go.opentelemetry.io/otel/metric"
	"sync"
)

type inMemoryStore struct {
	mu              sync.Mutex
	events          map[string][]*api.WebhookEventInternal
	eventsHistogram otelapi.Int64Histogram
}

func NewInMemoryStore() *inMemoryStore {
	i := new(inMemoryStore)
	i.events = make(map[string][]*api.WebhookEventInternal)
	_, err, mp := otel_util.SetupOTelSDK(context.Background(), "gsf-inmemory-store", "0.0.1")
	if err != nil {
		sublogger.Warn().Err(err).Msg("Encountered an error when setting up OTEL SDK")
	}
	meter := mp.Meter("gsf-inmemory-store")
	// TODO: make this a histogram per repository
	histogram, err := meter.Int64Histogram("CachedEvents", otelapi.WithDescription("a very nice histogram"))
	if err != nil {
		sublogger.Warn().Err(err).Msg("Encountered an error when creating histogram")
	} else {
		i.eventsHistogram = histogram
	}

	return i
}

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

	// TODO optimize this, as it is not very efficient
	if i.eventsHistogram != nil {
		totalEvents := 0
		for repo := range i.events {
			totalEvents += len(i.events[repo])
		}
		i.eventsHistogram.Record(context.Background(), int64(totalEvents))
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
	if events == nil {
		return false // nothing to remove
	}
	var updatedEvents []*api.WebhookEventInternal
	for _, storedEvent := range events {
		if storedEvent.ID != event.ID {
			updatedEvents = append(updatedEvents, storedEvent)
		}
	}
	i.events[repositoryId] = updatedEvents

	// TODO optimize this, as it is not very efficient
	totalEvents := 0
	for repo := range i.events {
		totalEvents += len(i.events[repo])
	}
	if i.eventsHistogram != nil {
		i.eventsHistogram.Record(context.Background(), int64(totalEvents))
	}

	return true
}
