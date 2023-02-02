package cache

import (
	"encoding/json"
	"fmt"
	"github.com/getsentry/sentry-go"
	"github.com/go-redis/redis"
	api "github.com/joostvdg/gitstafette/api/v1"
	"log"
	"strconv"
)

type RedisConfig struct {
	Host     string
	Port     string
	Password string
	Database string
}

type redisStore struct {
	redisClient *redis.Client
	isConnected bool
}

func NewRedisStore(config *RedisConfig) *redisStore {
	database, err := strconv.Atoi(config.Database)
	if err != nil {
		log.Printf("Warning: no (valid) database provided, selecting '0'")
		database = 0
	}
	redisConnected := false
	redisClient := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", config.Host, config.Port),
		Password: config.Password,
		DB:       database,
	})

	pong, err := redisClient.Ping().Result()
	if err != nil {
		log.Printf("Could not connect to Redis: %v\n", err)
	} else {
		log.Printf("What does Redis say: %v\n", pong)
		redisConnected = true
	}
	return &redisStore{
		redisClient: redisClient,
		isConnected: redisConnected,
	}
}

func (r *redisStore) IsConnected() bool {
	return r.isConnected
}

func (r *redisStore) Store(repositoryId string, event *api.WebhookEventInternal) bool {
	jsonRepresentation, err := json.Marshal(event)
	if err != nil {
		errorMessage := fmt.Sprintf("Could not parse event: %v", err)
		log.Print(errorMessage)
		sentry.CaptureMessage(errorMessage)
		return false
	}

	response := r.redisClient.LPush(repositoryId, string(jsonRepresentation))
	numAffected, err := response.Result()
	if err != nil || numAffected <= 0 {
		errorMessage := fmt.Sprintf("Could not store event in RedisStore for Repo %v: %v", repositoryId, err)
		log.Print(errorMessage)
		sentry.CaptureMessage(errorMessage)
		return false
	}
	return true
}

func (r *redisStore) RetrieveEventsForRepository(repositoryId string) []*api.WebhookEventInternal {
	events := make([]*api.WebhookEventInternal, 0)
	numberOfRecords := r.CountEventsForRepository(repositoryId)
	for i := 0; i < numberOfRecords; i++ {
		result := r.redisClient.RPop(repositoryId)
		jsonEvent, err := result.Result()
		if err != nil {
			log.Printf("Could get events in RedisStore for Repo %v", repositoryId)
			break
		}
		var event api.WebhookEventInternal
		err = json.Unmarshal([]byte(jsonEvent), &event)
		if err != nil {
			log.Printf("Could not parse event from RedisStore for %v: %v", repositoryId, err)
		}
	}

	return events
}

func (r *redisStore) CountEventsForRepository(repositoryId string) int {
	response := r.redisClient.LLen(repositoryId)
	numberOfItems, err := response.Result()
	if err != nil {
		log.Printf("Could not count events in RedisStore for Repo %v: %v", repositoryId, err)
		return 0
	}
	return int(numberOfItems)
}

func (r *redisStore) Remove(repositoryId string, event *api.WebhookEventInternal) bool {
	return false
}
