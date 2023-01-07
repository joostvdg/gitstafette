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

.PHONY: server-1
server-1:
	go run cmd/server/main.go --repositories 537845873 --port 1323 --grpcPort 50051

.PHONY: server-1-hmac
server-1-hmac:
	go run cmd/server/main.go --repositories 537845873 \
		--port 1323 --grpcPort 50051 \
		--webhookHMAC=${GITSTAFETTE_HMAC}

.PHONY: server-1-tls
server-1-tls:
	go run cmd/server/main.go --repositories 537845873 --port 1323 --grpcPort 50051 \
		--caFileLocation /mnt/d/Projects/homelab-rpi/certs/ca.pem \
		--certFileLocation /mnt/d/Projects/homelab-rpi/certs/gitstafette/server-local.pem \
		--certKeyFileLocation /mnt/d/Projects/homelab-rpi/certs/gitstafette/server-local-key.pem

.PHONY: server-2
server-2:
	go run cmd/server/main.go --repositories 537845873 --port 1324 --grpcPort 50052

.PHONY: server-relay
server-relay:
	go run cmd/server/main.go --repositories 537845873 --port 1325 --grpcPort 50053\
		--relayEnabled=true --relayHost=127.0.0.1 --relayPort=50051

.PHONY: client-1
client-1:
	go run cmd/client/main.go --repo 537845873 --server "127.0.0.1" --port 50051

.PHONY: client-1-tls
client-1-tls:
	go run cmd/client/main.go --repo 537845873 --server "127.0.0.1" --port 50051 \
		--secure \
		--caFileLocation /mnt/d/Projects/homelab-rpi/certs/ca.pem \
		--certFileLocation /mnt/d/Projects/homelab-rpi/certs/gitstafette/client-local.pem \
		--certKeyFileLocation /mnt/d/Projects/homelab-rpi/certs/gitstafette/client-local-key.pem

.PHONY: client-2
client-2:
	go run cmd/client/main.go --repo 537845873 --server "127.0.0.1" --port 50051 --relayEndpoint http://localhost:1324/v1/github/

.PHONY: client-3
client-3:
	go run cmd/client/main.go --repo 537845873 --server "127.0.0.1" -port- 50051 --relayEndpoint http://localhost:1324/v1/github/

.PHONY: client-cluster-send
client-cluster-send:
	go run cmd/client/main.go --repo 537845873 --server gitstafette-server-qad46fd4qq-ez.a.run.app --port 443

.PHONY: client-cluster-receive
client-cluster-receive:
	go run cmd/client/main.go --repo 537845873 --server "lemon.fritz.box" --port 80 --insecure

.PHONY: client-cluster-receive-tls
client-cluster-receive-tls:
	go run cmd/client/main.go --repo 537845873 --server "lemon.fritz.box" --port 80 \
		--secure \
		--caFileLocation /mnt/d/Projects/homelab-rpi/certs/ca.pem \
		--certFileLocation /mnt/d/Projects/homelab-rpi/certs/gitstafette/client-local.pem \
		--certKeyFileLocation /mnt/d/Projects/homelab-rpi/certs/gitstafette/client-local-key.pem

.PHONY: client-gcr-receive
client-gcr-receive:
	go run cmd/client/main.go --repo 537845873 --server "gitstafette-server-qad46fd4qq-ez.a.run.app" --port 443 --secure

go-build-server:
	CGO_ENABLED=0 go build -o bin/$(NAME) cmd/server/main.go

go-build-client:
	CGO_ENABLED=0 go build -o bin/$(NAME) cmd/client/main.go

dxbuild-server:
	docker buildx build . -f ./docker/server/Dockerfile --platform linux/arm64,linux/amd64 --tag ghcr.io/joostvdg/gitstafette/server:${PACKAGE_VERSION}

dxpush-server:
	docker buildx build . -f ./docker/server/Dockerfile --platform linux/arm64,linux/amd64 --tag ghcr.io/joostvdg/gitstafette/server:${PACKAGE_VERSION} --push

dxbuild-client:
	docker buildx build . -f ./docker/client/Dockerfile --platform linux/arm64,linux/amd64 --tag ghcr.io/joostvdg/gitstafette/client:${PACKAGE_VERSION}

dxpush-client:
	docker buildx build . -f ./docker/client/Dockerfile --platform linux/arm64,linux/amd64 --tag ghcr.io/joostvdg/gitstafette/client:${PACKAGE_VERSION} --push


gpush: dxpush-server
	docker pull caladreas/gitstafette-server:${PACKAGE_VERSION}
	docker tag caladreas/gitstafette-server:${PACKAGE_VERSION} gcr.io/${PROJECT_ID}/${NAME}-server:${PACKAGE_VERSION}
	docker push gcr.io/${PROJECT_ID}/${NAME}-server:${PACKAGE_VERSION}


gdeploy: gpush
	gcloud run deploy gitstafette-server-http --image=gcr.io/${PROJECT_ID}/${NAME}-server:${PACKAGE_VERSION} \
		--memory=128Mi --max-instances=1 --timeout=30 --project=$(PROJECT_ID)\
		--platform managed --allow-unauthenticated --region=europe-west4 \
		--args="--repositories" --args="537845873"\
		--args="--port=8080"\
      	--args="--grpcPort=50051" \
		--args="--relayEnabled=true"\
		--args="--relayHost=gitstafette-server-qad46fd4qq-ez.a.run.app"\
		--args="--relayPort=443"

	gcloud beta run deploy gitstafette-server \
		--use-http2 \
		--image=gcr.io/${PROJECT_ID}/${NAME}-server:${PACKAGE_VERSION} \
		--memory=128Mi --max-instances=1 --timeout=30 --project=$(PROJECT_ID)\
		--platform managed --allow-unauthenticated --region=europe-west4 \
		--args="--repositories=537845873" \
		--args="--port=1323"\
      	--args="--grpcPort=8080"

helm-server:
	helm upgrade gitstafette-server --install -n gitstafette --create-namespace ./helm/gitstafette-server --values ./helm/server-values.yaml

helm-client-local:
	helm upgrade gitstafette-client-local --install -n gitstafette --create-namespace ./helm/gitstafette-client --values ./helm/client-local-values.yaml

helm-client-gcr:
	helm upgrade gitstafette-client-gcr --install -n gitstafette --create-namespace ./helm/gitstafette-client --values ./helm/client-gcr-values.yaml