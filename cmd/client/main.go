package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	api "github.com/joostvdg/gitstafette/api/v1"
	"github.com/joostvdg/gitstafette/internal/cache"
	gcontext "github.com/joostvdg/gitstafette/internal/context"
	"github.com/joostvdg/gitstafette/internal/relay"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// TODO do not close if we have not relayed our events yet!

func main() {
	serverPort := flag.String("port", "50051", "Port used for connecting to the GRPC Server")
	serverAddress := flag.String("server", "127.0.0.1", "Server address to connect to")
	repositoryId := flag.String("repo", "", "GitHub Repository ID to receive webhook events for")
	relayEnabled := flag.Bool("relayEnabled", false, "If the server should relay received events, rather than caching them for clients")
	relayHost := flag.String("relayHost", "127.0.0.1", "Host address to relay events to")
	relayPort := flag.String("relayPort", "50051", "The port of the relay address")
	relayProtocol := flag.String("relayProtocol", "grpc", "The protocol for the relay address (grpc, or http)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	relayConfig, err := api.CreateRelayConfig(*relayEnabled, *relayHost, *relayPort, *relayProtocol)
	if err != nil {
		log.Fatal("Malformed URL: ", err.Error())
	}
	serviceContext := &gcontext.ServiceContext{
		Context: ctx,
		Relay:   relayConfig,
	}
	relay.InitiateRelay(serviceContext, *repositoryId)
	cache.InitCache(*repositoryId, nil)

	stream := initializeWebhookEventStreamOrDie(*repositoryId, *serverAddress, *serverPort, ctx)
	handleWebhookEventStream(stream, *repositoryId, ctx)
	log.Printf("Closing client\n")
}

func handleWebhookEventStream(stream api.Gitstafette_FetchWebhookEventsClient, repositoryId string, ctx context.Context) {
	serverClosed := make(chan bool)
	go func() {
		clock := time.NewTicker(5 * time.Second)
		for {
			select {
			case <-clock.C:
				response, err := stream.Recv()
				if err == io.EOF {
					log.Println("Server send end of stream, closing")
					serverClosed <- true // server has ended the stream
					continue
				}
				if err != nil {
					log.Printf("Error resceiving stream: %v\n", err) // is this recoverable or not?
					continue
				}

				log.Printf("Received %d WebhookEvents", len(response.WebhookEvents))
				for _, event := range response.WebhookEvents {
					log.Printf("InternalEvent: %d, body size: %d, number of headers:  %d\n", event.EventId, len(event.Body), len(event.Headers))
					cache.Event(repositoryId, event)
				}
			case <-ctx.Done(): // Activated when ctx.Done() closes
				log.Println("Closing FetchWebhookEvents")
				serverClosed <- true
				return
			}
		}
	}()
	<-serverClosed
}

func initializeWebhookEventStreamOrDie(repositoryId string, serverHost string, serverPort string, ctx context.Context) api.Gitstafette_FetchWebhookEventsClient {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithAuthority(serverHost))

	systemRoots, err := x509.SystemCertPool()
	if err != nil {
		log.Printf("cannot load root CA certs: %v", err)
	}
	creds := credentials.NewTLS(&tls.Config{
		RootCAs: systemRoots,
	})
	opts = append(opts, grpc.WithTransportCredentials(creds))

	// https://www.googlecloudcommunity.com/gc/Serverless/Unable-to-connect-to-Cloud-Run-gRPC-server/m-p/422280/highlight/true#M345
	//opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	server := fmt.Sprintf("%s:%s", serverHost, serverPort)
	conn, err := grpc.Dial(server, opts...)

	if err != nil {
		log.Fatalf("cannot connect to the server %s: %v\n", server, err)
	}

	client := api.NewGitstafetteClient(conn)
	request := &api.WebhookEventsRequest{
		ClientId:            "myself",
		RepositoryId:        repositoryId,
		LastReceivedEventId: 0,
	}

	stream, err := client.FetchWebhookEvents(ctx, request)
	if err != nil {
		log.Fatalf("could not open stream to %s: %v\n", server, err)
	}
	return stream
}
