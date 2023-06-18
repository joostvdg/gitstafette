package main

import (
	"context"
	"flag"
	"fmt"
	api "github.com/joostvdg/gitstafette/api/v1"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
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
	grpcServerPort := flag.String("port", "50051", "Port used for connecting to the GRPC Server")
	grpcServerHost := flag.String("server", "127.0.0.1", "Server host to connect to")
	flag.Parse()

	log.Printf("Connecting to GRPC Server at %s:%s", *grpcServerHost, *grpcServerPort)

	tp := initTracerProvider()
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Fatal().Err(err).Msg("Error shutting down Tracer provider")
		}
	}()

	mp := initMeterProvider()
	defer func() {
		if err := mp.Shutdown(context.Background()); err != nil {
			log.Fatal().Err(err).Msg("Error shutting down Meter provider")
		}
	}()

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
		ClientId: "test-client",
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

func initResource() *sdkresource.Resource {
	initResourcesOnce.Do(func() {
		extraResources, _ := sdkresource.New(
			context.Background(),
			sdkresource.WithOS(),
			sdkresource.WithProcess(),
			sdkresource.WithContainer(),
			sdkresource.WithHost(),
		)
		resource, _ = sdkresource.Merge(
			sdkresource.Default(),
			extraResources,
		)
	})
	return resource
}

func initTracerProvider() *sdktrace.TracerProvider {
	ctx := context.Background()

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint("localhost:4317"),
	)
	if err != nil {
		log.Fatal().Err(err).Msg("OTLP Trace gRPC Creation")
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(initResource()),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp
}

func initMeterProvider() *sdkmetric.MeterProvider {
	ctx := context.Background()

	exporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithInsecure(), otlpmetricgrpc.WithEndpoint("localhost:4317"))
	if err != nil {
		log.Fatal().Err(err).Msg("new otlp metric grpc exporter failed")
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
		sdkmetric.WithResource(initResource()),
	)
	global.SetMeterProvider(mp)
	return mp
}

