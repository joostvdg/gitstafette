# copied from: https://github.com/GoogleCloudPlatform/golang-samples/blob/main/run/grpc-server-streaming/Dockerfile
FROM golang:1.19-buster as builder
ARG TARGETARCH
ARG TARGETOS
WORKDIR /go/src/gitstafette
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make go-build-server

FROM debian:buster-slim
EXPOSE 1323
EXPOSE 50051
ENV PORT=8080
RUN set -x && apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y \
    ca-certificates && \
    rm -rf /var/lib/apt/lists/*
ENTRYPOINT ["/usr/bin/gitstafette"]
COPY --from=builder /go/src/gitstafette/bin/gitstafette /usr/bin/gitstafette
