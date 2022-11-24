# gitstafette

Git Webhook Relay demo app

## TODO


* Host server in Google Cloud Run
  * use personal domain
  * support grpc streaming
  * suport http + grpc streaming
* Carvel package
  * personal carvel package repository
  * on GHCR
  * Client
  * Server
* Support Webhook with secret/token
* Mutual TLS with self-signed certs / Custom CA
* Helm support
  * Helm chart for Client
* Add Sentry support for client
* OpenTelemetry metrics
* OpenTracing metrics
* Expose State with GraphQL
  * with authentication
* CI/CD In Kubernetes
  * Build with Tekton / CloudNative BuildPacks
  * generate SBOM/SPDX
  * Scan with Snyk?
  * Testcontainers?
  * combine steps with Cartographer?
* Kubernetes Controller + CR for generating clients
  * Metacontroller?
  * Operator?
  * (GRPC) Server should support multiple clients
  * add CR to cluster for individual Repo's, then spawn a client
* Clients in multiple languages?
  * Java
  * Rust

## Testing Kubernetes

### HTTP

```shell
kubectl port-forward -n gitstafette svc/gitstafette-server 7777:1323
```

```shell
http :7777
```

### GRPC

```shell
kubectl port-forward -n gitstafette svc/gitstafette-server 7777:50051
```

```shell
grpc-health-probe -addr=localhost:7777
```

## Resources

* https://developer.redis.com/develop/golang/

### GRPC

* https://github.com/fullstorydev/grpcurl

### GRPC HealthCheck

* https://projectcontour.io/guides/grpc/
* https://github.com/projectcontour/yages
* https://stackoverflow.com/questions/59352845/how-to-implement-go-grpc-go-health-check
* https://github.com/grpc-ecosystem/grpc-health-probe
* https://github.com/grpc/grpc/blob/master/doc/health-checking.md

## Testing Webhooks Locally

```shell
http POST http://localhost:1323/v1/github/ \
  X-Github-Delivery:d4049330-377e-11ed-9c2e-1ae286aab35f \
  X-Github-Hook-Installation-Target-Id:537845873 \
  X-Github-Hook-Installation-Target-Type:repository \
  Test=True
```

## Google Cloud Run

* https://cloud.google.com/run/docs/configuring/containers
* https://cloud.google.com/run/docs/deploying#terraform
* https://ahmet.im/blog/cloud-run-multiple-processes-easy-way/
* https://github.com/ahmetb/multi-process-container-lazy-solution/blob/master/start.sh
* https://cloud.google.com/blog/products/serverless/cloud-run-healthchecks

### Envoy Setup

We can only use ***one*** port with Cloud Run.
But, we can use an Envoy proxy to route between the http and grpc servers.

* https://gruchalski.com/posts/2022-02-20-keycloak-1700-with-tls-behind-envoy/