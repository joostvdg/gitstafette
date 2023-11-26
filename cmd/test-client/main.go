package main

import (
	"context"
	"flag"
	"fmt"
	api "github.com/joostvdg/gitstafette/api/v1"
	"github.com/joostvdg/gitstafette/internal/otel_util"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"io"
	"sync"
	"time"
)

var (
	resource          *sdkresource.Resource
	initResourcesOnce sync.Once
)

func main() {
	otel_util.SetupOTelSDK(context.Background(), "test-client", "0.0.1")
	grpcServerPort := flag.String("port", "50051", "Port used for connecting to the GRPC Server")
	grpcServerHost := flag.String("server", "127.0.0.1", "Server host to connect to")
	flag.Parse()

	log.Printf("Connecting to GRPC Server at %s:%s", *grpcServerHost, *grpcServerPort)

	address := *grpcServerHost + ":" + *grpcServerPort
	fetchWebhookStatus(address)

	for {
		err := fetchWebhookStatuses(address)
		if err != nil {
			log.Fatal().Err(err).Msg("Error streaming from server")
		}
	}

}

func fetchWebhookStatus(address string) error {
	grpcOpts := createGrpcOptions()
	conn, err := grpc.Dial(address, grpcOpts...)
	if err != nil {
		log.Fatal().Err(err).Msg("did not connect")
	}
	defer conn.Close()

	client := api.NewGitstafetteClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	md := metadata.Pairs("timestamp", time.Now().Format(time.Stamp), "kn", "vn")
	ctx = metadata.NewOutgoingContext(ctx, md)

	request := &api.WebhookEventStatusRequest{
		ClientId:     "test-client",
		RepositoryId: "test-repo",
	}

	statusResponse, err := client.WebhookEventStatus(ctx, request)
	if err == nil {
		log.Info().Msgf("Status Result: %v\n", statusResponse)
	}
	return err

}

func fetchWebhookStatuses(address string) error {
	grpcOpts := createGrpcOptions()
	conn, err := grpc.Dial(address, grpcOpts...)
	if err != nil {
		log.Fatal().Err(err).Msg("did not connect")
	}
	defer conn.Close()

	client := api.NewGitstafetteClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	md := metadata.Pairs("timestamp", time.Now().Format(time.Stamp), "kn", "vn")
	ctx = metadata.NewOutgoingContext(ctx, md)

	request := &api.WebhookEventStatusesRequest{
		ClientId: "test-client",
	}

	statusStream, err := client.WebhookEventStatuses(ctx, request)
	for {
		var header, trailer metadata.MD
		status, err := statusStream.Recv()
		if err == io.EOF {
			log.Printf("End of fetchWebhookStatuses")
			break
		}
		if err != nil {
			log.Fatal().Err(err).Msg("fetchWebhookStatuses failed")
		}
		log.Printf("Status Result: %v\n", status)
		header, err = statusStream.Header()
		if err != nil {
			log.Printf("Failed to get header: %v", err)
		}
		headerInfo := fmt.Sprintf("%v", header)
		log.Info().Msgf("Header: %s", headerInfo)

		trailer = statusStream.Trailer()
		trailerInfo := fmt.Sprintf("%v", trailer)
		log.Info().Msgf("Trailer: %s", trailerInfo)
	}
	if err == io.EOF {
		log.Info().Msg("Server send end of stream, closing")
		return nil
	}

	return err
}

func createGrpcOptions() []grpc.DialOption {
	noCreds := insecure.NewCredentials()
	grpcOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(noCreds),
		grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
		grpc.WithStreamInterceptor(otelgrpc.StreamClientInterceptor()),
	}
	return grpcOpts
}
