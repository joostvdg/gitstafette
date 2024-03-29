package cache

import (
	"bytes"
	api "github.com/joostvdg/gitstafette/api/v1"
	"net/http"
	"strings"
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

const delimiter = ","

var Store EventStore
var Repositories RepositoryWatcher

type EventStore interface {
	Store(repositoryId string, event *api.WebhookEventInternal) bool
	Remove(repositoryId string, event *api.WebhookEventInternal) bool
	RetrieveEventsForRepository(repositoryId string) []*api.WebhookEventInternal
	CountEventsForRepository(repositoryId string) int
	IsConnected() bool
}

func InitCache(repositoryIDs string, redisConfig *RedisConfig) []string {
	if repositoryIDs == "" || len(repositoryIDs) <= 1 {
		sublogger.Fatal().Msg("Did not receive any RepositoryID to watch")
	}

	var repoIds []string
	if strings.Contains(repositoryIDs, delimiter) {
		repoIds = strings.Split(repositoryIDs, delimiter)
	} else {
		repoIds = []string{repositoryIDs}
	}

	Repositories = createRepositoryWatcher()
	for _, repoId := range repoIds {
		Repositories.AddRepository(repoId)
	}
	Store = initializeStore(redisConfig)
	return repoIds
}

func initializeStore(config *RedisConfig) EventStore {
	var store EventStore
	store = NewInMemoryStore()

	if config != nil {
		redisStore := NewRedisStore(config)
		if redisStore.isConnected {
			store = redisStore
		}
	}
	return store
}

// TODO should probably have some logic for closing the stores
// for example, disconnecting the Redis client if it is connected
func PrepareForShutdown() {

}

func Event(targetRepositoryID string, event *api.WebhookEvent) error {
	webhookEvent := api.ExternalToInternalEvent(event)
	Store.Store(targetRepositoryID, webhookEvent)
	return nil
}

func InternalEvent(targetRepositoryID string, eventBodyBytes []byte, headers http.Header) (bool, error) {
	deliveryId := headers.Get(api.DeliveryIdHeader)

	webhookEventHeaders := make([]api.WebhookEventHeader, len(headers))
	for key, value := range headers {
		webhookEventHeader := api.WebhookEventHeader{
			Key:        key,
			FirstValue: value[0],
		}
		webhookEventHeaders = append(webhookEventHeaders, webhookEventHeader)
	}

	eventBody := bytes.NewBuffer(eventBodyBytes).String()
	webhookEvent := &api.WebhookEventInternal{
		ID:           deliveryId,
		IsRelayed:    false,
		TimeReceived: time.Now(),
		Headers:      webhookEventHeaders,
		EventBody:    eventBody,
	}

	return Store.Store(targetRepositoryID, webhookEvent), nil
}
