package gitstafette_v1

import (
	"time"
)

type WebhookEventInternal struct {
	ID        string               `json:"id"`
	IsRelayed bool                 `json:"isRelayed"`
	Timestamp time.Time            `json:"timestamp"`
	Headers   []WebhookEventHeader `json:"headers"`
	EventBody string               `json:"eventBody"`
}

type WebhookEventHeader struct {
	Key        string `json:"key"`
	FirstValue string `json:"firstValue"`
}
