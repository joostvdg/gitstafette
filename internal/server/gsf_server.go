package server

import (
	"context"
	"fmt"
	"github.com/joostvdg/gitstafette/internal/cache"
	"github.com/joostvdg/gitstafette/internal/otel_util"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otelmetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	//semconv "go.opentelemetry.io/otel_util/semconv/v1.18.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/rs/zerolog/log"
	"os"
	"os/signal"
	"syscall"
	"time"

	api "github.com/joostvdg/gitstafette/api/v1"
)

type GitstafetteServer struct {
	api.UnimplementedGitstafetteServer
	Tracer           trace.Tracer
	MeterProvider    *sdkmetric.MeterProvider
	ResponseInterval time.Duration
}

func (s GitstafetteServer) WebhookEventStatus(ctx context.Context, req *api.WebhookEventStatusRequest) (*api.WebhookEventStatusResponse, error) {
	response := &api.WebhookEventStatusResponse{
		ServerId:     "Gitstafette",
		Count:        0,
		RepositoryId: req.RepositoryId,
		Status:       "OK",
	}
	return response, nil
}
func (s GitstafetteServer) WebhookEventStatuses(request *api.WebhookEventStatusesRequest, srv api.Gitstafette_WebhookEventStatusesServer) error {
	tracer := s.Tracer
	status01 := &api.WebhookEventStatusResponse{
		ServerId:     "Gitstafette",
		Count:        1,
		RepositoryId: "12345",
		Status:       "OK",
	}

	status02 := &api.WebhookEventStatusResponse{
		ServerId:     "Gitstafette",
		Count:        2,
		RepositoryId: "7891",
		Status:       "OK",
	}

	status03 := &api.WebhookEventStatusResponse{
		ServerId:     "Gitstafette",
		Count:        3,
		RepositoryId: "7892",
		Status:       "FAILED",
	}

	finish := time.Now().Add(time.Second * 30)
	ctx, stop := signal.NotifyContext(srv.Context(), os.Interrupt, syscall.SIGTERM)

	defer stop()

	waitInterval := time.Second * 5

	log.Printf("Wait Interval is: %v", waitInterval)

	spanCtx, span := tracer.Start(ctx, "WebhookEventStatuses", trace.WithSpanKind(trace.SpanKindServer))
	span.AddEvent("Start")
	events := []*api.WebhookEventStatusResponse{status01, status02, status03}
	lastEvent := len(events) - 1
	currentEvent := 0

timed:
	for time.Now().Before(finish) {
		select {
		case <-time.After(waitInterval):
			eventInfo := fmt.Sprintf("Sent event %v of %v", currentEvent, lastEvent)
			_, span := tracer.Start(spanCtx, eventInfo, trace.WithSpanKind(trace.SpanKindServer))
			if currentEvent > lastEvent {
				break timed
			}
			eventStatus := events[currentEvent]
			currentEvent++

			span.AddEvent("Send", trace.WithAttributes(attribute.Int("eventCounter", currentEvent)))
			if err := srv.Send(eventStatus); err != nil {
				return err
			}
			span.End()
			break timed
		case <-srv.Context().Done(): // Activated when ctx.Done() closes
			log.Printf("Closing WebhookEventStatuses (client context %s closed)", request.ClientId)
			break timed
		case <-ctx.Done(): // Activated when ctx.Done() closes
			log.Info().Msg("Closing WebhookEventStatuses (main context closed)")
			break timed
		}
	}
	log.Printf("Reached %v, so closed context %s", finish, request.ClientId)
	span.AddEvent("Finished", trace.WithAttributes(attribute.String("reason", "timeout")))
	span.End()
	return nil
}

func (s GitstafetteServer) WebhookEventPush(ignoredContext context.Context, request *api.WebhookEventPushRequest) (*api.WebhookEventPushResponse, error) {
	response := &api.WebhookEventPushResponse{
		ResponseCode:        "200", // TODO implement a response code system
		ResponseDescription: "depends",
		Accepted:            false,
	}

	err := cache.Event(request.RepositoryId, request.WebhookEvent)
	if err == nil {
		response.Accepted = true
		log.Printf("Accepted Webhook Event Push for Repo %v: %v", request.RepositoryId, request.WebhookEvent.EventId)
	}
	return response, err
}

func (s GitstafetteServer) FetchWebhookEvents(request *api.WebhookEventsRequest, srv api.Gitstafette_FetchWebhookEventsServer) error {
	log.Printf("Relaying webhook events for repository %s", request.RepositoryId)
	tracer := s.Tracer
	var counter otelmetric.Int64Counter
	otelEnabled := otel_util.IsOTelEnabled()
	if otelEnabled {
		meter := s.MeterProvider.Meter("gitstafette")
		counter, _ = meter.Int64Counter(
			"webhook_events_relayed",
			otelmetric.WithDescription("Number of webhook events relayed"),
		)
	}

	durationSeconds := request.GetDurationSecs()
	finish := time.Now().Add(time.Second * time.Duration(durationSeconds))
	log.Printf("Stream is alive from %v to %v", time.Now(), finish)
	log.Printf("Response Interval is: %v", s.ResponseInterval)

	ctx, stop := signal.NotifyContext(srv.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sublogger := log.With().Logger()
	var parentSpanContext context.Context
	var span trace.Span
	if otelEnabled {
		traceContext, spanContext, tmpSpan := otel_util.StartServerSpanFromClientContext(srv.Context(), tracer, "FetchWebhookEvents", trace.SpanKindServer)
		span = tmpSpan
		defer span.End()
		parentSpanContext = spanContext

		sublogger = log.With().
			Str("span_id", span.SpanContext().SpanID().String()).
			Str("trace_id", span.SpanContext().TraceID().String()).
			Str("incoming_trace_id", traceContext.TraceID().String()).
			Logger()
	}

timed:
	for time.Now().Before(finish) {
		select {
		case <-time.After(s.ResponseInterval):
			var childSpan trace.Span
			if otelEnabled {
				_, childSpan = tracer.Start(parentSpanContext, "retrieveCachedEventsForRepository", trace.WithSpanKind(trace.SpanKindServer))
			}

			sublogger.Info().Msgf("Fetching events for repo %v (with Span)", request.RepositoryId)

			events, err := retrieveCachedEventsForRepository(request.RepositoryId)

			if err != nil {
				sublogger.Info().Msgf("Could not get events for Repo: %v\n", err)
				otel_util.SetSpanStatus(childSpan, codes.Error, "Could not get events for Repo")
				return err
			}
			response := &api.WebhookEventsResponse{
				WebhookEvents: events,
			}

			if err := srv.Send(response); err != nil {
				sublogger.Info().Msgf("Error sending stream: %v\n", err)
				otel_util.SetSpanStatus(childSpan, codes.Error, "Error sending stream")
				return err
			}
			sublogger.Info().Msgf("Send %v events to client (%v) for repo %v", len(events), request.ClientId, request.RepositoryId)

			if otelEnabled {
				counter.Add(srv.Context(), int64(len(events)))
			}
			updateRelayStatus(events, request.RepositoryId)
			otel_util.AddSpanEventWithOption(childSpan, "SendEvents", trace.WithAttributes(attribute.Int("events", len(events))))

			if otelEnabled {
				childSpan.End()
			}

		case <-srv.Context().Done(): // Activated when ctx.Done() closes
			sublogger.Info().Msgf("Closing FetchWebhookEvents (client context %s closed)", request.ClientId)
			break timed
		case <-ctx.Done(): // Activated when ctx.Done() closes
			sublogger.Info().Msg("Closing FetchWebhookEvents (main context closed)")
			break timed
		}
	}
	sublogger.Info().Msgf("Reached %v, so closed context %s", finish, request.ClientId)
	if otelEnabled {
		span.AddEvent("Finished", trace.WithAttributes(attribute.String("reason", "timeout")))
		span.SetStatus(codes.Ok, "Finished")
	}
	return nil
}

func retrieveCachedEventsForRepository(repositoryId string) ([]*api.WebhookEvent, error) {
	events := make([]*api.WebhookEvent, 0)
	if !cache.Repositories.RepositoryIsWatched(repositoryId) {
		return events, fmt.Errorf("cannot fetch events for empty repository id")
	}
	cachedEvents := cache.Store.RetrieveEventsForRepository(repositoryId)
	for _, cachedEvent := range cachedEvents {
		if cachedEvent.IsRelayed {
			log.Printf("Event is already relayed: %v", cachedEvent)
			continue
		}
		event := api.InternalToExternalEvent(cachedEvent)
		events = append(events, event)
	}
	return events, nil
}

func updateRelayStatus(events []*api.WebhookEvent, repositoryId string) {
	cachedEvents := cache.Store.RetrieveEventsForRepository(repositoryId)
	for _, event := range events {
		eventId := event.EventId
		for _, cachedEvent := range cachedEvents {
			if cachedEvent.ID == eventId {
				cachedEvent.IsRelayed = true
				cachedEvent.TimeRelayed = time.Now()
			}
		}
	}
}
