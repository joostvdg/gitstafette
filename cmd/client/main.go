package main

import (
	"context"
	"flag"
	"fmt"
	api "github.com/joostvdg/gitstafette/api/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	serverPort := flag.String("port", "50051", "Port used for connecting to the GRPC Server")
	serverAddress := flag.String("server", "127.0.0.1", "Server address to connect to")
	repositoryId := flag.String("repo", "", "GitHub Repository ID to receive webhook events for")
	flag.Parse()

	server := fmt.Sprintf("%s:%s", *serverAddress, *serverPort)
	insecureCredentials := insecure.NewCredentials()
	conn, err := grpc.Dial(server, grpc.WithTransportCredentials(insecureCredentials))

	if err != nil {
		log.Fatalf("cannot connect to the server %s: %v\n", server, err)
	}

	client := api.NewGitstafetteClient(conn)
	request := &api.WebhookEventsRequest{
		ClientId:            "myself",
		RepositoryId:        *repositoryId,
		LastReceivedEventId: 0,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	stream, err := client.FetchWebhookEvents(ctx, request)
	if err != nil {
		log.Fatalf("could not open stream: %v\n", err)
	}

	serverClosed := make(chan bool)
	go func() {
		clock := time.NewTicker(5 * time.Second)
		for {
			select {
			case <-clock.C:
				response, err := stream.Recv()
				if err == io.EOF {
					serverClosed <- true // server has ended the stream
					continue
				}
				if err != nil {
					log.Printf("Error resceiving stream: %v\n", err) // is this recoverable or not?
					continue
				}

				log.Printf("Received %d WebhookEvents", len(response.WebhookEvents))
				for _, event := range response.WebhookEvents {
					log.Printf("Event: %d, body size: %d, number of headers:  %d\n", event.EventId, len(event.Body), len(event.Headers))
				}
			case <-ctx.Done(): // Activated when ctx.Done() closes
				fmt.Println("Closing FetchWebhookEvents")
				return
			}
		}
	}()
	<-serverClosed
	log.Printf("Closing client\n")
}
