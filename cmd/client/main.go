package main

import (
	"context"
	"flag"
	"fmt"
	api "github.com/joostvdg/gitstafette/api/v1"
	"github.com/joostvdg/gitstafette/internal/cache"
	gcontext "github.com/joostvdg/gitstafette/internal/context"
	"github.com/joostvdg/gitstafette/internal/relay"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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
	relayEndpoint := flag.String("relayEndpoint", "", "URL of the Relay Endpoint to deliver the captured events to")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serviceContext := &gcontext.ServiceContext{
		Context: ctx,
	}
	relay.InitiateRelayOrDie(*relayEndpoint, serviceContext)
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

func initializeWebhookEventStreamOrDie(repositoryId string, serverAddress string, serverPort string, ctx context.Context) api.Gitstafette_FetchWebhookEventsClient {
	server := fmt.Sprintf("%s:%s", serverAddress, serverPort)
	insecureCredentials := insecure.NewCredentials()
	conn, err := grpc.Dial(server, grpc.WithTransportCredentials(insecureCredentials))

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
		log.Fatalf("could not open stream: %v\n", err)
	}
	return stream
}
