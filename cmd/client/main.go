package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	api "github.com/joostvdg/gitstafette/api/v1"
	v1 "github.com/joostvdg/gitstafette/internal/api/v1"
	"github.com/joostvdg/gitstafette/internal/cache"
	"github.com/joostvdg/gitstafette/internal/config"
	gcontext "github.com/joostvdg/gitstafette/internal/context"
	"github.com/joostvdg/gitstafette/internal/relay"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/credentials/oauth"
	"google.golang.org/grpc/keepalive"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// TODO do not close if we have not relayed our events yet!

var requestInterval = time.Second * 45

const envOauthToken = "OAUTH_TOKEN"

var (
	resource          *sdkresource.Resource
	initResourcesOnce sync.Once
)

type ServerState struct {
	HasError     bool
	ErrorMessage string
}

func main() {
	// TODO retrieve only events for specific repository
	// TODO so we need a repositories flag, like with the config
	grpcServerPort := flag.String("port", "50051", "Port used for connecting to the GRPC Server")
	grpcServerHost := flag.String("server", "127.0.0.1", "Server host to connect to")
	grpcServerInsecure := flag.Bool("insecure", false, "If the grpc streaming config should be handled insecurely, must provide either `secure` or `insecure` flag")
	grpcServerSecure := flag.Bool("secure", false, "If the grpc streaming config should be handled securely, must provide either `secure` or `insecure` flag")
	repositoryId := flag.String("repo", "", "GitHub Repository ID to receive webhook events for")
	relayEnabled := flag.Bool("relayEnabled", false, "If the config should relay received events, rather than caching them for clients")
	relayHost := flag.String("relayHost", "127.0.0.1", "Host address to relay events to")
	relayPath := flag.String("relayPath", "/", "Path on the host address to relay events to")
	relayHealthCheckPath := flag.String("relayHealthCheckPath", "/", "Path on the host address to do health check on, for relay target")
	relayPort := flag.String("relayPort", "50051", "The port of the relay address")
	relayProtocol := flag.String("relayProtocol", "grpc", "The protocol for the relay address (grpc, or http)")
	relayInsecure := flag.Bool("relayInsecure", false, "If the relay config should be handled insecurely")
	caFileLocation := flag.String("caFileLocation", "", "The root CA file for trusting clients using TLS connection")
	certFileLocation := flag.String("certFileLocation", "", "The certificate file for trusting clients using TLS connection")
	certKeyFileLocation := flag.String("certKeyFileLocation", "", "The certificate key file for trusting clients using TLS connection")
	clientId := flag.String("clientId", "gitstafette-client", "The id of the client to identify connections")
	streamWindow := flag.Int("streamWindow", 180, "The time we spend streaming with the server, in seconds")
	healthCheckPort := flag.String("healthCheckPort", "8080", "Port used for a http health check server, used for running in container environments")
	webhookHMAC := flag.String("webhookHMAC", "", "The hmac token used to verify the webhook events")
	flag.Parse()

	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	sublogger := log.With().Str("component", "init").Logger()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	relayConfig, err := api.CreateRelayConfig(*relayEnabled, *relayHost, *relayPath, *relayHealthCheckPath, *relayPort, *relayProtocol, *relayInsecure)
	if err != nil {
		sublogger.Fatal().Err(err).Msg("Malformed Relay URL")
	}

	serviceContext := &gcontext.ServiceContext{
		Context: ctx,
		Relay:   relayConfig,
	}
	relay.InitiateRelay(serviceContext, *repositoryId)
	cache.InitCache(*repositoryId, nil)
	go initHealthCheckServer(ctx, *healthCheckPort)

	insecure := *grpcServerInsecure
	if *grpcServerSecure {
		insecure = false
	}

	oauthToken, oauthOk := os.LookupEnv(envOauthToken)
	if !oauthOk {
		oauthToken = ""
	}

	tlsConfig, err := config.NewTLSConfig(*caFileLocation, *certFileLocation, *certKeyFileLocation, false)
	if err != nil {
		sublogger.Fatal().Err(err).Msg("Invalid certificate configuration")
	}

	grpcServerConfig := api.CreateServerConfig(*grpcServerHost, *grpcServerPort, *streamWindow, insecure, oauthToken, tlsConfig)
	grpcClientConfig := api.CreateClientConfig(*clientId, *repositoryId, *streamWindow, *webhookHMAC)

	for {
		stream := initializeWebhookEventStreamOrDie(grpcClientConfig, grpcServerConfig, ctx)
		err := handleWebhookEventStream(stream, grpcClientConfig, ctx)
		if err != nil {
			sublogger.Fatal().Err(err).Msg("Error streaming from server")
		}
	}

	sublogger.Info().Msg("Closing client")
}

func initHealthCheckServer(ctx context.Context, port string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthCheck)
	muxServer := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
		BaseContext: func(l net.Listener) context.Context {
			return ctx
		},
	}

	go func() {
		err := muxServer.ListenAndServe()
		if err != nil {
			log.Fatal().Err(err).Msg("Could not start health check service")
		}
	}()
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "OK\n")
}

func handleWebhookEventStream(stream api.Gitstafette_FetchWebhookEventsClient, grpcClientConfig *api.GRPCClientConfig, ctx context.Context) error {
	serverClosed := make(chan bool)
	serverError := &ServerState{}
	go func(serverResponse api.Gitstafette_FetchWebhookEventsClient, serverErrorState *ServerState) {

		// because Google Cloud Run's Envoy only handles streams for up to X amount of seconds,
		// 	we have to connect to the server for at most the durationSeconds
		finish := time.Now().Add(time.Second * time.Duration(grpcClientConfig.StreamWindow))

		for time.Now().Before(finish) {
			select {
			case <-time.After(requestInterval):
				response, err := stream.Recv()
				if err == io.EOF {
					log.Info().Msg("Server send end of stream, closing")
					serverClosed <- true // config has ended the stream
					return
				}
				if err != nil {
					errorMessage := fmt.Sprintf("Error receiving stream: %v\n", err)
					log.Warn().Msg(errorMessage) // is this recoverable or not?
					serverClosed <- true
					serverErrorState.HasError = true
					serverErrorState.ErrorMessage = errorMessage
					return
				}

				log.Printf("Received %d WebhookEvents", len(response.WebhookEvents))
				for _, event := range response.WebhookEvents {
					log.Printf("[grpc] InternalEvent: %d, body size: %d, number of headers:  %d\n", event.EventId, len(event.Body), len(event.Headers))
					eventIsValid := v1.ValidateEvent(grpcClientConfig.WebhookHMAC, event)
					messageAddition := ""
					if grpcClientConfig.WebhookHMAC != "" {
						messageAddition = " against hmac token on digest header"
					}
					log.Printf("[grpc] Event %v is validated"+messageAddition+", valid: %v",
						event.EventId, eventIsValid)
					cache.Event(grpcClientConfig.RepositoryId, event)
				}
			case <-ctx.Done(): // Activated when ctx.Done() closes
				log.Info().Msg("[grpc] Closing FetchWebhookEvents")
				serverClosed <- true
				return
			}
		}
		serverClosed <- true
	}(stream, serverError)
	<-serverClosed
	stream.Context().Done()
	if serverError.HasError {
		fmt.Errorf(serverError.ErrorMessage)
	}
	return nil
}

var kacp = keepalive.ClientParameters{
	Time:                10 * time.Second, // send pings every 10 seconds if there is no activity
	Timeout:             time.Second,      // wait 1 second for ping ack before considering the connection dead
	PermitWithoutStream: true,             // send pings even without active streams
}

func initializeWebhookEventStreamOrDie(clientConfig *api.GRPCClientConfig, serverConfig *api.GRPCServerConfig, ctx context.Context) api.Gitstafette_FetchWebhookEventsClient {
	sublogger := log.With().Str("component", "grpc-init").Logger()

	tp := initTracerProvider(ctx)
	defer func() {
		if err := tp.Shutdown(ctx); err != nil {
			sublogger.Fatal().Err(err).Msg("Tracer Provider Shutdown")
		}
	}()
	mp := initMeterProvider(ctx)
	defer func() {
		if err := mp.Shutdown(ctx); err != nil {
			sublogger.Fatal().Err(err).Msg("Error shutting down meter provider")
		}
	}()


	var opts []grpc.DialOption
	opts = append(opts, grpc.WithAuthority(serverConfig.Host))
	opts = append(opts, grpc.WithKeepaliveParams(kacp))
	opts = append(opts, grpc.WithChainStreamInterceptor(otelgrpc.StreamClientInterceptor(), ClientStreamInterceptor))
	opts = append(opts, grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()))

	if serverConfig.OAuthToken != "" {
		rpcCreds := oauth.NewOauthAccess(&oauth2.Token{AccessToken: serverConfig.OAuthToken})
		opts = append(opts, grpc.WithPerRPCCredentials(rpcCreds))
	}

	if serverConfig.Insecure {
		sublogger.Info().Msg("Not using TLS for GRPC server connection (insecure set)")
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else if serverConfig.TLSConfig != nil && serverConfig.TLSConfig.RootCAs != nil { // TODO verify if this is al that is required
		sublogger.Info().Msg("Using provided TLS certificates for GRPC server connection (RootCA's set)")
		clientCreds := credentials.NewTLS(serverConfig.TLSConfig)
		opts = append(opts, grpc.WithTransportCredentials(clientCreds))
	} else {
		sublogger.Info().Msg("Using default system TLS certificates for GRPC server connection (secure, but no RootCA)")
		// https://www.googlecloudcommunity.com/gc/Serverless/Unable-to-connect-to-Cloud-Run-gRPC-server/m-p/422280/highlight/true#M345
		systemRoots, err := x509.SystemCertPool()
		if err != nil {
			sublogger.Warn().Err(err).Msg("cannot load root CA certs")
		}
		creds := credentials.NewTLS(&tls.Config{
			RootCAs: systemRoots,
		})
		opts = append(opts, grpc.WithTransportCredentials(creds))
	}

	server := fmt.Sprintf("%s:%s", serverConfig.Host, serverConfig.Port)
	conn, err := grpc.Dial(server, opts...)

	if err != nil {
		sublogger.Fatal().Err(err).Str("server", server).Msg("cannot connect to the config")
	}

	client := api.NewGitstafetteClient(conn)
	request := &api.WebhookEventsRequest{
		ClientId:            clientConfig.ClientID,
		RepositoryId:        clientConfig.RepositoryId,
		LastReceivedEventId: 0,
		DurationSecs:        uint32(serverConfig.StreamWindow),
	}

	stream, err := client.FetchWebhookEvents(ctx, request)
	if err != nil {
		sublogger.Fatal().Err(err).Str("server", server).Msg("could not open stream")
	}
	return stream
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

func initTracerProvider(ctx context.Context) *sdktrace.TracerProvider {
	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure())
	if err != nil {
		log.Fatal().Err(err).Msg("OTLP Trace gRPC Creation failed")
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(initResource()),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp
}

func initMeterProvider(ctx context.Context) *sdkmetric.MeterProvider {
	exporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithInsecure())
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

func ClientStreamInterceptor(
	ctx context.Context,
	desc *grpc.StreamDesc,
	cc *grpc.ClientConn,
	method string,
	streamer grpc.Streamer,
	opts ...grpc.CallOption) (grpc.ClientStream, error) {

	newCtx, span := otel.Tracer("Gitstafette-Client").Start(ctx, method)
	s, err := streamer(newCtx, desc, cc, method, opts...)
	if err != nil {
		return nil, err
	}
	span.SetAttributes(attribute.String("grpc.stream.type", "client"))
	return s, nil
}

