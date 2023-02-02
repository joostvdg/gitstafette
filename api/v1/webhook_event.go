package gitstafette_v1

import (
	"bytes"
	"log"
	"time"
)

const DeliveryIdHeader = "X-Github-Delivery"

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

func ExternalToInternalEvent(event *WebhookEvent) *WebhookEventInternal {
	webhookEventHeaders := make([]WebhookEventHeader, 0)
	deliveryId := ""
	for _, header := range event.Headers {
		key := header.Name
		value := header.Values
		if key != "" && value != "" {
			webhookEventHeader := WebhookEventHeader{
				Key:        key,
				FirstValue: value,
			}
			if key == DeliveryIdHeader {
				deliveryId = value
			}
			webhookEventHeaders = append(webhookEventHeaders, webhookEventHeader)
		}
	}

	log.Printf("webhookEventHeaders: %v\n", webhookEventHeaders)
	eventBody := bytes.NewBuffer(event.Body).String()
	return &WebhookEventInternal{
		ID:        deliveryId,
		IsRelayed: false,
		Timestamp: time.Now(),
		Headers:   webhookEventHeaders,
		EventBody: eventBody,
	}
}

func InternalToExternalEvent(internalEvent *WebhookEventInternal) *WebhookEvent {
	headers := make([]*Header, len(internalEvent.Headers))
	for _, header := range internalEvent.Headers {
		header := &Header{
			Name:   header.Key,
			Values: header.FirstValue,
		}
		headers = append(headers, header)
	}

	return &WebhookEvent{
		EventId: internalEvent.ID,
		Body:    []byte(internalEvent.EventBody),
		Headers: headers,
	}
}
