CONFIG_PATH=${HOME}/.gitstafette/
LOCAL_VERSION = $(shell git describe --tags --always)
PACKAGE_VERSION ?= "0.1.0-${LOCAL_VERSION}"
NAME := gitstafette
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
	protoc api/**/v1/*.proto \
	--go_out=. \
	--go_opt=paths=source_relative \
	--proto_path=.

.PHONY: compile
compile:
	protoc api/**/v1/*.proto \
	--go_out=. \
	--go-grpc_out=. \
	--go_opt=paths=source_relative \
	--go-grpc_opt=paths=source_relative \
	--proto_path=.

.PHONY: server-1
server-1:
	go run cmd/server/main.go --repositories 537845873 --port 1323 --grpcPort 50051

.PHONY: server-2
server-2:
	go run cmd/server/main.go --repositories 537845873 --port 1324 --grpcPort 50052

.PHONY: client-1
client-1:
	go run cmd/client/main.go --repo 537845873 --server "127.0.0.1" --port 50051

.PHONY: client-2
client-2:
	go run cmd/client/main.go --repo 537845873 --server "127.0.0.1" --port 50051 --relayEndpoint http://localhost:1324/v1/github/

go-build-server:
	CGO_ENABLED=0 go build -o bin/$(NAME) cmd/server/main.go

go-build-client:
	CGO_ENABLED=0 go build -o bin/$(NAME) cmd/client/main.go

dxbuild-server:
	docker buildx build . --platform linux/arm64,linux/amd64 --tag caladreas/gitstafette-server:${PACKAGE_VERSION}

dxpush-server:
	docker buildx build . --platform linux/arm64,linux/amd64 --tag caladreas/gitstafette-server:${PACKAGE_VERSION} --push

dxbuild-client:
	docker buildx build . --platform linux/arm64,linux/amd64 --tag caladreas/gitstafette-server:${PACKAGE_VERSION}

dxpush-client:
	docker buildx build . --platform linux/arm64,linux/amd64 --tag caladreas/gitstafette-server:${PACKAGE_VERSION} --push