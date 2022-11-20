# gitstafette

Git Webhook Relay demo app

## TODO

* Helm support
  * Helm chart for Server
  * Helm chart for Client
  * improve health checks
  * define sensible resources
* Carvel package
  * personal carvel package repository
  * on GHCR
* Kubernetes Controller + CR for generating clients
  * Metacontroller?
  * Operator?
  * (GRPC) Server should support multiple clients
  * add CR to cluster for individual Repo's, then spawn a client
* Mutual TLS with self-signed certs / Custom CA
* OpenTelemetry metrics
* OpenTracing metrics
* CI/CD In Kubernetes
  * Build with Tekton / CloudNative BuildPacks
  * generate SBOM
  * Testcontainers?
  * combine steps with Cartographer?
* Clients in multiple languages?
  * Java
  * Rust
* Host server in Google Cloud Run

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