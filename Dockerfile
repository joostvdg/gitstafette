FROM golang:1.17 AS build
WORKDIR /go/src/gitstafette
ARG TARGETARCH
ARG TARGETOS
COPY go.* ./
RUN go mod download
COPY . ./
RUN make go-build-server

FROM alpine:3
RUN apk --no-cache add ca-certificates
EXPOSE 1323
EXPOSE 50051
ENV PORT=8080
ENTRYPOINT ["/usr/bin/gitstafette"]
COPY --from=build /go/src/gitstafette/bin/gitstafette /usr/bin/gitstafette