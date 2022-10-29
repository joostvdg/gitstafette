package v1

import (
	"net/http"
	"time"
)

type WebhookEvent struct {
	IsRelayed bool
	Timestamp time.Time
	Headers   http.Header
	EventBody []byte
}
