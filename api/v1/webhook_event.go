package gitstafette_v1

import (
	"net/http"
	"time"
)

type WebhookEventInternal struct {
	IsRelayed bool
	Timestamp time.Time
	Headers   http.Header
	EventBody []byte
}
