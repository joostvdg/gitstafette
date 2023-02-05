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
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// TODO do not close if we have not relayed our events yet!

const requestInterval = time.Second * 5

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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	relayConfig, err := api.CreateRelayConfig(*relayEnabled, *relayHost, *relayPath, *relayHealthCheckPath, *relayPort, *relayProtocol, *relayInsecure)
	if err != nil {
		log.Fatal("Malformed URL: ", err.Error())
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

	tlsConfig, err := config.NewTLSConfig(*caFileLocation, *certFileLocation, *certKeyFileLocation, false)
	if err != nil {
		log.Fatal("Invalid certificate configuration: ", err.Error())
	}

	grpcServerConfig := api.CreateServerConfig(*grpcServerHost, *grpcServerPort, *streamWindow, insecure, tlsConfig)
	grpcClientConfig := api.CreateClientConfig(*clientId, *repositoryId, *streamWindow, *webhookHMAC)

	for {
		stream := initializeWebhookEventStreamOrDie(grpcClientConfig, grpcServerConfig, ctx)
		err := handleWebhookEventStream(stream, grpcClientConfig, ctx)
		if err != nil {
			log.Fatalf("Error streaming from server: %v\n", err)
		}
	}

	log.Printf("Closing client\n")
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
			log.Fatalf("Could not start health check service: %v", err)
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
					log.Println("Server send end of stream, closing")
					serverClosed <- true // config has ended the stream
					return
				}
				if err != nil {
					errorMessage := fmt.Sprintf("Error resceiving stream: %v\n", err)
					log.Println(errorMessage) // is this recoverable or not?
					serverClosed <- true
					serverErrorState.HasError = true
					serverErrorState.ErrorMessage = errorMessage
					return
				}

				log.Printf("Received %d WebhookEvents", len(response.WebhookEvents))
				for _, event := range response.WebhookEvents {
					log.Printf("InternalEvent: %d, body size: %d, number of headers:  %d\n", event.EventId, len(event.Body), len(event.Headers))
					eventIsValid := v1.ValidateEvent(grpcClientConfig.WebhookHMAC, event)
					messageAddition := ""
					if grpcClientConfig.WebhookHMAC != "" {
						messageAddition = " against hmac token on digest header"
					}
					log.Printf("Event %v is validated"+messageAddition+", valid: %v",
						event.EventId, eventIsValid)
					cache.Event(grpcClientConfig.RepositoryId, event)
				}
			case <-ctx.Done(): // Activated when ctx.Done() closes
				log.Println("Closing FetchWebhookEvents")
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

func initializeWebhookEventStreamOrDie(grpcClientConfig *api.GRPCClientConfig, serverConfig *api.GRPCServerConfig, ctx context.Context) api.Gitstafette_FetchWebhookEventsClient {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithAuthority(serverConfig.Host))
	opts = append(opts, grpc.WithKeepaliveParams(kacp))

	if serverConfig.Insecure {
		log.Printf("Not using TLS for GRPC server connection (insecure set)")
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else if serverConfig.TLSConfig.RootCAs != nil { // TODO verify if this is al that is required
		log.Printf("Using provided TLS certificates for GRPC server connection (RootCA's set)")
		clientCreds := credentials.NewTLS(serverConfig.TLSConfig)
		opts = append(opts, grpc.WithTransportCredentials(clientCreds))
	} else {
		log.Printf("Using default system TLS certificates for GRPC server connection (secure, but no RootCA)")
		// https://www.googlecloudcommunity.com/gc/Serverless/Unable-to-connect-to-Cloud-Run-gRPC-server/m-p/422280/highlight/true#M345
		systemRoots, err := x509.SystemCertPool()
		if err != nil {
			log.Printf("cannot load root CA certs: %v", err)
		}
		creds := credentials.NewTLS(&tls.Config{
			RootCAs: systemRoots,
		})
		opts = append(opts, grpc.WithTransportCredentials(creds))
	}

	server := fmt.Sprintf("%s:%s", serverConfig.Host, serverConfig.Port)
	conn, err := grpc.Dial(server, opts...)

	if err != nil {
		log.Fatalf("cannot connect to the config %s: %v\n", server, err)
	}

	client := api.NewGitstafetteClient(conn)
	request := &api.WebhookEventsRequest{
		ClientId:            grpcClientConfig.ClientID,
		RepositoryId:        grpcClientConfig.RepositoryId,
		LastReceivedEventId: 0,
		DurationSecs:        uint32(serverConfig.StreamWindow),
	}

	stream, err := client.FetchWebhookEvents(ctx, request)
	if err != nil {
		log.Fatalf("could not open stream to %s: %v\n", server, err)
	}
	return stream
}
