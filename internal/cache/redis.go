package cache

import (
	"encoding/json"
	"fmt"
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
		log.Printf("Could not parse event")
		return false
	}

	response := r.redisClient.LPush(repositoryId, string(jsonRepresentation))
	numAffected, err := response.Result()
	if err != nil || numAffected <= 0 {
		log.Printf("Could not store event in RedisStore for Repo %v: %v", repositoryId, err)
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

//func redisAddRepository(repo api.Repository) {
//	webhookEventHeaders := make([]api.WebhookEventHeader, 2)
//	webhookEventHeader1 := api.WebhookEventHeader{
//		Key:        "ABC",
//		FirstValue: "XYZ",
//	}
//	webhookEventHeader2 := api.WebhookEventHeader{
//		Key:        "DEF",
//		FirstValue: "XYZ",
//	}
//	webhookEventHeaders = append(webhookEventHeaders, webhookEventHeader1)
//	webhookEventHeaders = append(webhookEventHeaders, webhookEventHeader2)
//	testData := api.WebhookEventInternal{
//		ID:        "0",
//		IsRelayed: false,
//		Timestamp: time.Now(),
//		Headers:   webhookEventHeaders,
//		EventBody: "{}",
//	}
//	jsonRepresentation, err := json.Marshal(testData)
//
//	if err != nil {
//		log.Printf("Ran into an error parsing webhook event: %v\n", err)
//	} else {
//		redisClient.SAdd(repo.ID, string(jsonRepresentation))
//		//redisClient.Set(repo.ID, string(jsonRepresentation), time.Minute*10)
//	}
//}
