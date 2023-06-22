package relay

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/go-resty/resty/v2"
	v1 "github.com/joostvdg/gitstafette/api/v1"
	"github.com/joostvdg/gitstafette/internal/cache"
	gcontext "github.com/joostvdg/gitstafette/internal/context"
	"github.com/joostvdg/gitstafette/internal/otel_util"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	otelapi "go.opentelemetry.io/otel/metric"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	"net/http"
	"net/url"
	"time"
)

// TODO periodically relay message to relay endpoint
// TODO health check on relay endpoint
// TODO remove relayed messages from cache

type Status struct {
	LastCheckWasSuccessfull     bool
	CounterOfFailedHealthChecks int
	TimeOfLastCheck             time.Time
	TimeOfLastFailure           time.Time
}

var sublogger zerolog.Logger

func init() {
	sublogger = log.With().Str("component", "relay").Logger()
}

func InitiateRelay(serviceContext *gcontext.ServiceContext, repositoryId string) {
	relayConfig := serviceContext.Relay
	if relayConfig.Enabled {
		go RelayHealthCheck(serviceContext)
		go RelayCachedEvents(serviceContext, repositoryId)
	} else {
		sublogger.Info().Msg("Relay is disabled")
	}
}

// TODO API should have a function To/From internal/external rep
func eventHeadersToHTTPHeaders(eventHeaders []v1.WebhookEventHeader) http.Header {
	var headers http.Header
	headers = make(map[string][]string)

	for _, header := range eventHeaders {
		key := header.Key
		value := header.FirstValue
		values := make([]string, 1)
		values[0] = value
		headers[key] = values
	}
	return headers
}

// HTTPRelay testing the relay functionality
func HTTPRelay(event *v1.WebhookEventInternal, relayEndpoint *url.URL) {
	client := resty.New()
	// TODO: handle this based on secure/insecure and TLS config
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}
	client.SetTLSClientConfig(tlsConfig)
	request := client.R().SetBody(event.EventBody)
	request.Header = eventHeadersToHTTPHeaders(event.Headers)
	response, err := request.Post(relayEndpoint.String())
	if err != nil {
		sublogger.Warn().Msgf("Encountered an error when relaying {event: %v, endpoint: %v}: %v\n",
			event.ID, relayEndpoint, err)
		sublogger.Warn().Msgf("Request: %v\n", request)
		sublogger.Warn().Msgf("Request Headers: %v\n", request.Header)
	} else {
		sublogger.Info().Msgf("[relay] Valid Relay Response (%v) - {event: %v, endpoint: %v}: %v\n", response.StatusCode(),
			event.ID, relayEndpoint, response)
	}

}

func RelayCachedEvents(serviceContext *gcontext.ServiceContext, repositoryId string) {
	ctx := serviceContext.Context
	relay := serviceContext.Relay
	clock := time.NewTicker(10 * time.Second)

	metricsProvider := otel_util.InitMeterProvider(context.Background())
	meter := metricsProvider.Meter("Gitstafette-Client")
	// TODO: make this a histogram per repository
	histogram, err := meter.Int64Histogram("CachedEvents", otelapi.WithDescription("a very nice histogram"))
	if err != nil {
		sublogger.Warn().Err(err).Msg("Encountered an error when creating histogram")
	}

	for {
		select {
		case <-clock.C:
			// TODO handle properly
			events := cache.Store.RetrieveEventsForRepository(repositoryId)
			for _, webhookEvent := range events {
				if !webhookEvent.IsRelayed {
					if relay.Protocol == "grpc" {
						GRPCRelay(webhookEvent, relay, repositoryId)
					} else {
						HTTPRelay(webhookEvent, relay.Endpoint)
					}

					// TODO add check on relay, so that we only set IsRelayed if we actually did
					webhookEvent.IsRelayed = true
					histogram.Record(ctx, 1)
				}
			}
		case <-ctx.Done(): // Activated when ctx.Done() closes
			sublogger.Info().Msg("Closing RelayCachedEvents")
			return
		}
	}
}

func GRPCRelay(internalEvent *v1.WebhookEventInternal, relay *v1.RelayConfig, repositoryId string) {
	systemRoots, err := x509.SystemCertPool()
	if err != nil {
		sublogger.Warn().Err(err).Msg("cannot load root CA certs")
	}
	creds := credentials.NewTLS(&tls.Config{
		RootCAs: systemRoots,
	})
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithAuthority(relay.Host))
	opts = append(opts, grpc.WithTransportCredentials(creds))

	server := fmt.Sprintf("%s:%s", relay.Host, relay.Port)
	sublogger.Info().
		Str("server", server).
		Msg("[relay] GRPCRelay to config")
	conn, err := grpc.Dial(server, opts...)

	if err != nil {
		sublogger.Fatal().Err(err).Str("server", server).Msg("cannot connect to the config")
	}

	client := v1.NewGitstafetteClient(conn)
	event := v1.InternalToExternalEvent(internalEvent)
	request := &v1.WebhookEventPushRequest{
		CliendId:     "myself", // TODO generate unique client ids (maybe from env vars in K8S/GCR?)
		RepositoryId: repositoryId,
		WebhookEvent: event,
	}

	ctx := context.Background()
	response, err := client.WebhookEventPush(ctx, request)
	if err != nil {
		sublogger.Fatal().Err(err).Msg("could not open stream")
	}
	sublogger.Info().Msgf("GRPC Push response: %v\n", response)
}

/**
This is a GitHub Ping Header Set
Request URL: https://smee.io/3l3edGAqmbBJ9x9
Request method: POST
content-type: application/json
User-Agent: GitHub-Hookshot/ede37db
X-GitHub-Delivery: d4049330-377e-11ed-9c2e-1ae286aab35f
X-GitHub-InternalEvent: ping
X-GitHub-Hook-ID: 380052596
X-GitHub-Hook-Installation-Target-ID: 537845873
X-GitHub-Hook-Installation-Target-Type: repository
*/

func RelayHealthCheck(serviceContext *gcontext.ServiceContext) {
	status := Status{
		LastCheckWasSuccessfull:     false,
		CounterOfFailedHealthChecks: 0,
		TimeOfLastCheck:             time.Now(),
	}
	ctx := serviceContext.Context
	relay := serviceContext.Relay
	clock := time.NewTicker(60 * time.Second)
	for {
		select {
		case <-clock.C:
			// TODO do healthcheck
			repoIds := cache.Repositories.Repositories
			log.Printf("[relay] We have %v repositories (%v)", len(repoIds), repoIds)
			status.TimeOfLastCheck = time.Now()
			healthy := false
			var err error
			if relay.Protocol == "grpc" {
				healthy, err = doGrpcHealthcheck(serviceContext)
			} else if relay.Protocol == "http" || relay.Protocol == "https" {
				healthy, err = doHttpHealthcheck(relay.HealthEndpoint, repoIds[0])
				if err != nil {
					log.Printf("[relay] Encountered an error doing healthcheck on relay: %v\n", err)
				}
			} else {
				log.Printf("[relay] Invalid relay protocol %s\n", relay.Protocol)
			}

			if !healthy {
				status.CounterOfFailedHealthChecks = status.CounterOfFailedHealthChecks + 1
				status.TimeOfLastFailure = time.Now()
				status.LastCheckWasSuccessfull = false
			} else {
				status.LastCheckWasSuccessfull = true
				status.CounterOfFailedHealthChecks = 0
			}
		case <-ctx.Done(): // Activated when ctx.Done() closes
			fmt.Println("[relay] Closing RelayHealthCheck")
			return
		}
	}
}

func doGrpcHealthcheck(serviceContext *gcontext.ServiceContext) (bool, error) {
	systemRoots, err := x509.SystemCertPool()
	if err != nil {
		sublogger.Info().Err(err).Msg("cannot load root CA certs")
	}
	creds := credentials.NewTLS(&tls.Config{
		RootCAs: systemRoots,
	})

	relayConfig := serviceContext.Relay
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(creds))
	opts = append(opts, grpc.WithAuthority(relayConfig.Host))

	// https://www.googlecloudcommunity.com/gc/Serverless/Unable-to-connect-to-Cloud-Run-gRPC-server/m-p/422280/highlight/true#M345
	server := fmt.Sprintf("%s:%s", relayConfig.Host, relayConfig.Port)
	conn, err := grpc.Dial(server, opts...)
	if err != nil {
		sublogger.Fatal().Err(err).Str("server", server).Msg("cannot connect to the config")
	}

	client := healthpb.NewHealthClient(conn)

	request := &healthpb.HealthCheckRequest{}
	ctx := context.Background()
	resp, err := client.Check(ctx, request)

	if err != nil {
		if stat, ok := status.FromError(err); ok && stat.Code() == codes.Unimplemented {
			sublogger.Warn().Msgf("this config does not implement the grpc health protocol (grpc.health.v1.Health): %s\n", stat.Message())
		} else if stat, ok := status.FromError(err); ok && stat.Code() == codes.DeadlineExceeded {
			sublogger.Warn().Msgf("timeout: health rpc did not complete within time")
		} else {
			sublogger.Warn().Err(err).Msg("grpc relay healthcheck failed")
		}
	}
	if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		sublogger.Warn().Str("status", resp.GetStatus().String()).Msg("grpc relay service unhealthy")
	} else {
		sublogger.Info().Str("status", resp.GetStatus().String()).Msg("grpc relay service ok")
	}

	return false, nil
}

// TODO verify healthcheck with Jenkins or something similar
func doHttpHealthcheck(relayEndpoint *url.URL, repositoryId string) (bool, error) {
	sublogger.Info().Msgf("Doing healthcheck for relay %v (using repo %v)\n", relayEndpoint.String(), repositoryId)
	client := resty.New()
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}
	client.SetTLSClientConfig(tlsConfig)
	response, err := client.R().
		SetHeader("X-GitHub-InternalEvent", "ping").
		SetHeader("X-GitHub-Hook-Installation-Target-Type", "repository").
		SetHeader("X-GitHub-Hook-Installation-Target-ID", repositoryId).
		SetHeader("User-Agent", "Gitstafette").
		SetHeader("Centent-Type", "application/json").
		SetBody(`{"zen": "Design for failure.","repository": {"id": ` + repositoryId + `}}`).
		Get(relayEndpoint.String())
	// TODO: add POST option
	//Post(relayEndpoint.String())
	if err != nil {
		sublogger.Warn().Err(err).Msg("Encountered an error when relaying")
		return false, err
	}
	// TODO: set behind debug flag
	//fmt.Printf("Response: %v\n", response)
	return response.IsSuccess(), nil
}

func CleanupRelayedEvents(serviceContext *gcontext.ServiceContext) {
	ctx := serviceContext.Context
	clock := time.NewTicker(5 * time.Second)
	timeAfterWhichWeCleanup := time.Minute * 2

	metricsProvider := otel_util.InitMeterProvider(ctx)
	meter := metricsProvider.Meter("Gitstafette")
	cleanupRelayedEventsCounter, err := meter.Int64Counter("cleanup_relayed_events")
	if err != nil {
		sublogger.Warn().Err(err).Msg("Encountered an error when creating histogram")
	}

	for {
		select {
		case <-clock.C:
			repoIds := cache.Repositories.Repositories
			for _, repositoryId := range repoIds {
				cachedEvents := cache.Store.RetrieveEventsForRepository(repositoryId)
				for _, cachedEvent := range cachedEvents {
					relayTime := cachedEvent.TimeRelayed.Add(timeAfterWhichWeCleanup)
					if cachedEvent.IsRelayed && time.Now().After(relayTime) {
						sublogger.Info().Msgf("Event (%v::%v) was relayed %s ago, removing",
							repositoryId, cachedEvent.ID, time.Since(cachedEvent.TimeRelayed).Round(time.Second))
						cache.Store.Remove(repositoryId, cachedEvent)
						cleanupRelayedEventsCounter.Add(ctx, 1)
					}
				}
			}
		case <-ctx.Done(): // Activated when ctx.Done() closes
			sublogger.Info().Msg("Closing CleanupRelayedEvents")
			return
		}
	}
}
