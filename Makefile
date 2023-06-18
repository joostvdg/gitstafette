CONFIG_PATH=${HOME}/.gitstafette/
LOCAL_VERSION = $(shell git describe --tags --always)
PACKAGE_VERSION ?= "0.1.0-$(LOCAL_VERSION)"
NAME := gitstafette
PROJECT_ID := kearos-gcp
MAIN_GO :=

.PHONY: init
init:
	mkdir -p ${CONFIG_PATH}

.PHONY: docs
docs:
	mkdocs serve

.PHONY: publish-docs
publish-docs: 
	mkdocs gh-deploy -b gh-pages


.PHONY: proto
proto:
	protoc api/v1/*.proto \
	--go_out=. \
	--go_opt=paths=source_relative \
	--proto_path=.
	protoc api/health/v1/*.proto \
	--go_out=. \
	--go_opt=paths=source_relative \
	--proto_path=.

.PHONY: compile
compile:
	protoc api/v1/*.proto \
	--go_out=. \
	--go-grpc_out=. \
	--go_opt=paths=source_relative \
	--go-grpc_opt=paths=source_relative \
	--proto_path=.
	protoc api/health/v1/*.proto \
	--go_out=. \
	--go-grpc_out=. \
	--go_opt=paths=source_relative \
	--go-grpc_opt=paths=source_relative \
	--proto_path=.

.PHONY: sbom-server
sbom-server:
	cyclonedx-gomod app -json -output app-server.bom.json -licenses -main cmd/server .

.PHONY: sbom-client
sbom-client:
	cyclonedx-gomod app -json -output app-client.bom.json -licenses -main cmd/client .

.PHONY: probe-1
probe-1:
	grpc-health-probe -addr=localhost:50051

.PHONY: probe-1-tls
probe-1-tls:
	grpc-health-probe -addr=localhost:50051 \
		-tls \
		-tls-ca-cert /mnt/d/Projects/homelab-rpi/certs/ca.pem \
		-tls-client-cert /mnt/d/Projects/homelab-rpi/certs/gitstafette/client-local.pem \
		-tls-client-key /mnt/d/Projects/homelab-rpi/certs/gitstafette/client-local-key.pem

.PHONY: probe-1-aws
probe-1-aws:
	grpc-health-probe -addr=events.gitstafette.joostvdg.net:50051 -tls

.PHONY: gcurl-aws
gcurl-aws:
	grpcurl \
	  -proto api/health/v1/healthcheck.proto \
	  -d '{"client_id": "local-grpcurl", "repository_id": "537845873", "last_received_event_id": 1}' \
	  events.gitstafette.joostvdg.net:50051 \
	  gitstafette.v1.Gitstafette.FetchWebhookEvents

.PHONY: gcurl-gcr-hc
gcurl-gcr-hc:
	grpcurl \
	  -proto api/health/v1/healthcheck.proto \
	  gitstafette-server-qad46fd4qq-ez.a.run.app:443 \
	  grpc.health.v1.Health/Check

.PHONY: gcurl-local-1-tls
gcurl-local-1-tls:
	grpcurl \
	  -proto api/v1/gitstafette.proto \
	  -d '{"client_id": "me", "repository_id": "537845873", "last_received_event_id": 1}' \
	  -cacert /mnt/d/Projects/homelab-rpi/certs/ca.pem \
	  -cert /mnt/d/Projects/homelab-rpi/certs/gitstafette/client-local.pem \
	  -key /mnt/d/Projects/homelab-rpi/certs/gitstafette/client-local-key.pem \
	  localhost:50051 \
	  gitstafette.v1.Gitstafette.FetchWebhookEvents


.PHONY: ghz-local-2
ghz-local-2:
	ghz --insecure \
		--proto api/v1/gitstafette.proto \
		--call gitstafette.v1.Gitstafette.FetchWebhookEvents \
		--data '{"client_id": "me", "repository_id": "537845873", "last_received_event_id": 1}' \
		localhost:50052

.PHONY: server-1
server-1:
	export OTEL_SERVICE_NAME=GSF-Server-1; export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317; go \
		run cmd/server/main.go --repositories 537845873 \
		--port 1323 --grpcPort 50051 --grpcHealthPort 50051 \

	#    export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317;\
	#    export OTEL_SERVICE_NAME=Server-1;\
	#    export OTEL_RESOURCE_ATTRIBUTES="service.namespace=gitstafette,service.name=Server,service.instance.id=Server-1";\
	#    export PUBLIC_OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=http://localhost:4318/v1/traces;\


.PHONY: server-1-hmac
server-1-hmac:
	go run cmd/server/main.go --repositories 537845873 \
		--port 1323 --grpcPort 50051 \
		--webhookHMAC=${GITSTAFETTE_HMAC}

.PHONY: server-1-tls
server-1-tls:
	OAUTH_TOKEN="Q4HEg0ODGuie0wraqUn4" go run cmd/server/main.go --repositories "537845873,478599060" --port 1323 --grpcPort 50051 --grpcHealthPort 50051 \
		--caFileLocation /mnt/d/Projects/homelab-rpi/certs/ca.pem \
		--certFileLocation /mnt/d/Projects/homelab-rpi/certs/gitstafette/server-local.pem \
		--certKeyFileLocation /mnt/d/Projects/homelab-rpi/certs/gitstafette/server-local-key.pem

.PHONY: server-2
server-2:
	go run cmd/server/main.go --repositories 537845873 --port 1324 --grpcPort 50052

.PHONY: server-relay
server-relay:
	OAUTH_TOKEN="abc" go run cmd/server/main.go --repositories 537845873 --port 1325 --grpcPort 50053\
		--relayEnabled=true --relayHost=127.0.0.1 --relayPort=50051 \
		--caFileLocation /mnt/d/Projects/homelab-rpi/certs/ca.pem \
		--certFileLocation /mnt/d/Projects/homelab-rpi/certs/gitstafette/server-local.pem \
		--certKeyFileLocation /mnt/d/Projects/homelab-rpi/certs/gitstafette/server-local-key.pem

.PHONY: server-relay-gcr
server-relay-gcr:
	go run cmd/server/main.go --repositories 537845873 --port 1325 --grpcPort 50053\
		--relayEnabled=true --relayHost=gitstafette.joostvdg.net --relayPort=443


.PHONY: test-client-1
test-client-1:
	export OTEL_SERVICE_NAME=GSF-TEST-CLIENT-1; go run cmd/test-client/main.go --server "localhost" --port 50051

.PHONY: client-1
client-1:
	export OTEL_SERVICE_NAME=GSF-Client-1; export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317; go \
 	    run cmd/client/main.go --repo 537845873 --server "localhost" \
		--port 50051 --insecure=true \
		--streamWindow 15

    # export OTEL_SERVICE_NAME=Client-1;\
    # export OTEL_RESOURCE_ATTRIBUTES="service.namespace=gitstafette,service.name=Client,service.instance.id=Client-1";\
    # export PUBLIC_OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=http://localhost:4318/v1/traces;\

.PHONY: client-1-tls
client-1-tls:
	export OAUTH_TOKEN="Q4HEg0ODGuie0wraqUn4";export OTEL_SERVICE_NAME=GSF-CLIENT-1; go run cmd/client/main.go --repo 537845873 --server "127.0.0.1" --port 50051 \
		--secure \
		--streamWindow 100 \
		--healthCheckPort=8081 \
		--clientId="local-1" \
		--caFileLocation /mnt/d/Projects/homelab-rpi/certs/ca.pem \
		--certFileLocation /mnt/d/Projects/homelab-rpi/certs/gitstafette/client-local.pem \
		--certKeyFileLocation /mnt/d/Projects/homelab-rpi/certs/gitstafette/client-local-key.pem

client-aws:
	OAUTH_TOKEN="Q4HEg0ODGuie0wraqUn4" go run cmd/client/main.go --repo 537845873 --server "events.gitstafette.joostvdg.net" --port 50051 \
		--secure \
		--streamWindow 3600 \
		--healthCheckPort=8081 \
		--clientId="local-1" \
		--caFileLocation /mnt/d/Projects/homelab-rpi/certs/ca.pem \
		--certFileLocation /mnt/d/Projects/homelab-rpi/certs/gitstafette/client-local.pem \
		--certKeyFileLocation /mnt/d/Projects/homelab-rpi/certs/gitstafette/client-local-key.pem


.PHONY: client-2-tls
client-2-tls:
	OAUTH_TOKEN="Q4HEg0ODGuie0wraqUn4" go run cmd/client/main.go --repo 478599060 --server "127.0.0.1" --port 50051 \
		--secure \
		--streamWindow 100 \
		--healthCheckPort=8082 \
		--clientId="local-2" \
		--caFileLocation /mnt/d/Projects/homelab-rpi/certs/ca.pem \
		--certFileLocation /mnt/d/Projects/homelab-rpi/certs/gitstafette/client-local.pem \
		--certKeyFileLocation /mnt/d/Projects/homelab-rpi/certs/gitstafette/client-local-key.pem


.PHONY: client-2
client-2:
	go run cmd/client/main.go --repo 537845873 --server "127.0.0.1" --port 50051 --healthCheckPort=8081 --relayEndpoint http://localhost:1324/v1/github/

.PHONY: client-3
client-3:
	go run cmd/client/main.go --repo 537845873 --server "127.0.0.1" -port- 50051 --relayEndpoint http://localhost:1324/v1/github/

.PHONY: client-cluster-send
client-cluster-send:
	go run cmd/client/main.go --repo 537845873 --server gitstafette-server-qad46fd4qq-ez.a.run.app --port 443

.PHONY: client-cluster-receive
client-cluster-receive:
	go run cmd/client/main.go --repo 537845873 --server "gitstafette.home.lab" --port 443 --insecure

.PHONY: client-cluster-receive-tls
client-cluster-receive-tls:
	go run cmd/client/main.go --repo 583684693 --server "gitstafette.home.lab" --port 443 \
		--secure \
		--caFileLocation /mnt/d/Projects/homelab-rpi/certs/ca.pem \
		--certFileLocation /mnt/d/Projects/homelab-rpi/certs/gitstafette/client-local.pem \
		--certKeyFileLocation /mnt/d/Projects/homelab-rpi/certs/gitstafette/client-local-key.pem

.PHONY: client-gcr-receive
client-gcr-receive:
	go run cmd/client/main.go \
 	--repo 537845873 \
 	--server "gitstafette-server-qad46fd4qq-ez.a.run.app" \
 	--port 443 --secure

.PHONY: client-local-server-relay-jenkins
client-local-server-relay-jenkins:
	go run cmd/client/main.go --repo 537845873 --server "127.0.0.1" --port 50051 \
	--secure \
	--caFileLocation /mnt/d/Projects/homelab-rpi/certs/ca.pem \
	--certFileLocation /mnt/d/Projects/homelab-rpi/certs/gitstafette/client-local.pem \
	--certKeyFileLocation /mnt/d/Projects/homelab-rpi/certs/gitstafette/client-local-key.pem \
	--relayEnabled=true \
	--relayHost="cranberry.fritz.box" \
	--relayPath="/github-webhook/" \
	--relayHealthCheckPath="/login" \
	--relayPort=443 \
	--relayProtocol=https \
	--relayInsecure=true \
	--clientId="test-local"

# gitstafette.joostvdg.net
# gitstafette-server-qad46fd4qq-ez.a.run.app
.PHONY: client-gcr-server-relay-jenkins
client-gcr-server-relay-jenkins:
	go run cmd/client/main.go --repo 537845873 --server gitstafette.joostvdg.net  --port 443 \
	--secure \
	--caFileLocation /mnt/d/Projects/homelab-rpi/certs/ca.pem \
	--certFileLocation /mnt/d/Projects/homelab-rpi/certs/gitstafette/client-local.pem \
	--certKeyFileLocation /mnt/d/Projects/homelab-rpi/certs/gitstafette/client-local-key.pem \
	--relayEnabled=true \
	--relayHost="cranberry.fritz.box" \
	--relayPath="/github-webhook/" \
	--relayHealthCheckPath="/login" \
	--relayPort=443 \
	--relayProtocol=https \
	--relayInsecure=true \
	--webhookHMAC=${GITSTAFETTE_HMAC} \
	--clientId="test-local"

go-build-server:
	CGO_ENABLED=0 go build -o bin/$(NAME) cmd/server/main.go

go-build-client:
	CGO_ENABLED=0 go build -o bin/$(NAME) cmd/client/main.go

dxbuild-server:
	docker buildx build . --platform linux/arm64,linux/amd64 --tag ghcr.io/joostvdg/gitstafette/server:${PACKAGE_VERSION}

dxpush-server:
	docker buildx build . --platform linux/arm64,linux/amd64 --tag ghcr.io/joostvdg/gitstafette/server:${PACKAGE_VERSION} --push

dxbuild-client:
	docker buildx build . -f ./Dockerfile.client --platform linux/arm64,linux/amd64 --tag ghcr.io/joostvdg/gitstafette/client:${PACKAGE_VERSION}

dxpush-client:
	docker buildx build . -f ./Dockerfile.client --platform linux/arm64,linux/amd64 --tag ghcr.io/joostvdg/gitstafette/client:${PACKAGE_VERSION} --push


gpush: dxpush-server
	docker pull ghcr.io/joostvdg/gitstafette/server:${PACKAGE_VERSION}
	docker tag ghcr.io/joostvdg/gitstafette/server:${PACKAGE_VERSION} gcr.io/${PROJECT_ID}/${NAME}-server:${PACKAGE_VERSION}
	docker push gcr.io/${PROJECT_ID}/${NAME}-server:${PACKAGE_VERSION}


gdeploy: gpush
	gcloud run deploy gitstafette-server-http --image=gcr.io/${PROJECT_ID}/${NAME}-server:${PACKAGE_VERSION} \
		--memory=128Mi --max-instances=1 --timeout=30 --project=${PROJECT_ID} \
		--platform managed --allow-unauthenticated --region=europe-west4 \
		--args="--repositories=537845873,478599060" \
		--args="--port=8080" \
		--args="--grpcPort=50051" \
		--args="--relayEnabled=true" \
		--args="--relayHost=gitstafette.joostvdg.net" \
		--args="--relayPort=443" \
		--args="--webhookHMAC=${GITSTAFETTE_HMAC}"

	gcloud run deploy gitstafette-server \
		--use-http2 \
		--image=gcr.io/${PROJECT_ID}/${NAME}-server:${PACKAGE_VERSION} \
		--memory=128Mi --max-instances=1 --timeout=300 --project=${PROJECT_ID}\
		--platform managed --allow-unauthenticated --region=europe-west4 \
		--args="--repositories=537845873,478599060" \
		--args="--port=1323"\
		--args="--grpcPort=8080" \
		--args="--grpcHealthPort=8080"

helm-server:
	helm upgrade gitstafette-server --install -n gitstafette --create-namespace ./helm/gitstafette-server --values ./helm/server-values.yaml

helm-client-local:
	helm upgrade gitstafette-client-local --install -n gitstafette --create-namespace ./helm/gitstafette-client --values ./helm/client-local-values.yaml

helm-client-gcr:
	helm upgrade gitstafette-client-gcr --install -n gitstafette --create-namespace ./helm/gitstafette-client --values ./helm/client-gcr-values.yaml