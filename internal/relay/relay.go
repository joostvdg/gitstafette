package relay

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/go-resty/resty/v2"
	v1 "github.com/joostvdg/gitstafette/api/v1"
	"github.com/joostvdg/gitstafette/internal/cache"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"context"
	gcontext "github.com/joostvdg/gitstafette/internal/context"
	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"log"
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

func InitiateRelay(serviceContext *gcontext.ServiceContext, repositoryId string) {
	relayConfig := serviceContext.Relay
	if relayConfig.Enabled {
		go RelayHealthCheck(serviceContext)
		go RelayCachedEvents(serviceContext, repositoryId)
	} else {
		log.Println("Relay is disabled")
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
	log.Printf("Doing HTTPRelay\n")
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
		log.Printf("Encountered an error when relaying: %v\n", err)
		log.Printf("Request: %v\n", request)
		log.Printf("Request Headers: %v\n", request.Header)
	} else {
		log.Printf("Response (%v): %v\n", response.StatusCode(), response)
	}

}

func RelayCachedEvents(serviceContext *gcontext.ServiceContext, repositoryId string) {
	ctx := serviceContext.Context
	relay := serviceContext.Relay
	clock := time.NewTicker(10 * time.Second)
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
				}
			}
		case <-ctx.Done(): // Activated when ctx.Done() closes
			fmt.Println("Closing RelayCachedEvents")
			return
		}
	}
}

func GRPCRelay(internalEvent *v1.WebhookEventInternal, relay *v1.RelayConfig, repositoryId string) {
	systemRoots, err := x509.SystemCertPool()
	if err != nil {
		log.Printf("cannot load root CA certs: %v", err)
	}
	creds := credentials.NewTLS(&tls.Config{
		RootCAs: systemRoots,
	})
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithAuthority(relay.Host))
	opts = append(opts, grpc.WithTransportCredentials(creds))

	server := fmt.Sprintf("%s:%s", relay.Host, relay.Port)
	log.Printf("GRPCRelay to config: %s\n", server)
	conn, err := grpc.Dial(server, opts...)

	if err != nil {
		log.Fatalf("cannot connect to the config %s: %v\n", server, err)
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
		log.Fatalf("could not open stream: %v\n", err)
	}
	fmt.Printf("GRPC Push response: %v\n", response)
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
	clock := time.NewTicker(5 * time.Second)
	for {
		select {
		case <-clock.C:
			// TODO do healthcheck
			repoIds := cache.Repositories.Repositories
			log.Printf("We have %v repositories (%v)", len(repoIds), repoIds)
			status.TimeOfLastCheck = time.Now()
			healthy := false
			var err error
			if relay.Protocol == "grpc" {
				healthy, err = doGrpcHealthcheck(serviceContext)
			} else if relay.Protocol == "http" || relay.Protocol == "https" {
				healthy, err = doHttpHealthcheck(relay.HealthEndpoint, repoIds[0])
				if err != nil {
					fmt.Printf("Encountered an error doing healthcheck on relay: %v\n", err)
				}
			} else {
				fmt.Printf("Invalid relay protocol %s\n", relay.Protocol)
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
			fmt.Println("Closing RelayHealthCheck")
			return
		}
	}
}

func doGrpcHealthcheck(serviceContext *gcontext.ServiceContext) (bool, error) {
	//ctx, cancel := context.WithCancel(context.Background())
	//defer cancel()
	//flConnTimeout := time.Second * 5
	//flAddr := serviceContext.Relay.Endpoint.String()
	//dialCtx, dialCancel := context.WithTimeout(ctx, flConnTimeout)
	//defer dialCancel()
	//opts := []grpc.DialOption{
	//	grpc.WithUserAgent("gitstafette"),
	//	grpc.WithBlock(),
	//	grpc.WithTransportCredentials(insecure.NewCredentials()),
	//}
	//conn, err := grpc.DialContext(dialCtx, flAddr, opts...)
	//if err != nil {
	//	if err == context.DeadlineExceeded {
	//		log.Printf("timeout: failed to connect service %q within %v", flAddr, flConnTimeout)
	//	} else {
	//		log.Printf("error: failed to connect service at %q: %+v", flAddr, err)
	//	}
	//}
	//defer conn.Close()
	//flRPCTimeout := time.Second * 5
	//rpcCtx, rpcCancel := context.WithTimeout(ctx, flRPCTimeout)
	//defer rpcCancel()

	systemRoots, err := x509.SystemCertPool()
	if err != nil {
		log.Printf("cannot load root CA certs: %v", err)
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
		log.Fatalf("cannot connect to the config %s: %v\n", server, err)
	}

	client := healthpb.NewHealthClient(conn)

	request := &healthpb.HealthCheckRequest{}
	ctx := context.Background()
	resp, err := client.Check(ctx, request)

	if err != nil {
		if stat, ok := status.FromError(err); ok && stat.Code() == codes.Unimplemented {
			log.Printf("error: this config does not implement the grpc health protocol (grpc.health.v1.Health): %s\n", stat.Message())
		} else if stat, ok := status.FromError(err); ok && stat.Code() == codes.DeadlineExceeded {
			log.Printf("timeout: health rpc did not complete within time\n")
		} else {
			log.Printf("error: health rpc failed: %+v", err)
		}
	}
	if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		log.Printf("service unhealthy (responded with %q)", resp.GetStatus().String())
	} else {
		log.Printf("Response from healthcheck: %v", resp.GetStatus())
	}

	return false, nil
}

// TODO verify healthcheck with Jenkins or something similar
func doHttpHealthcheck(relayEndpoint *url.URL, repositoryId string) (bool, error) {
	log.Printf("Doing healthcheck for relay %v (using repo %v)\n", relayEndpoint.String(), repositoryId)
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
		SetBody(`{"zen": "Design for failure.","repository": {"id": ` + repositoryId + `}}`).
		Get(relayEndpoint.String())
	// TODO: add POST option
	//Post(relayEndpoint.String())
	if err != nil {
		log.Printf("Encountered an error when relaying: %v\n", err)
		return false, err
	}
	// TODO: set behind debug flag
	//fmt.Printf("Response: %v\n", response)
	return response.IsSuccess(), nil
}
