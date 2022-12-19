# gitstafette

Git Webhook Relay demo app

## TODO

* Carvel package
  * personal carvel package repository
  * on GHCR
  * Client
  * Server
* CI/CD In Kubernetes
  * Build with Tekton / CloudNative BuildPacks
  * generate SBOM/SPDX
* Add Sentry support for client
* OpenTelemetry metrics
* OpenTracing metrics
* Support Webhook with secret/token
* Host server in Google Cloud Run
  * use personal domain
* Mutual TLS with self-signed certs / Custom CA
* Expose State with GraphQL
  * with authentication
* set Kubernetes security
  * SecurityContext: https://snyk.io/blog/10-kubernetes-security-context-settings-you-should-understand/
  * Seccomp profiles: https://itnext.io/seccomp-in-kubernetes-part-i-7-things-you-should-know-before-you-even-start-97502ad6b6d6
    * https://www.pulumi.com/resources/kubernetes-seccomp-profiles/
  * Secrity Admission: https://kubernetes.io/blog/2022/08/25/pod-security-admission-stable/
  * Network policies: https://kubernetes.io/docs/concepts/services-networking/network-policies/
* CI/CD In Kubernetes
  * Scan with Snyk?
  * Testcontainers?
  * combine steps with Cartographer?
* Kubernetes Controller + CR for generating clients
  * Metacontroller?
  * Operator?
  * (GRPC) Server should support multiple clients
  * add CR to cluster for individual Repo's, then spawn a client
  * https://betterprogramming.pub/build-a-kubernetes-operator-in-10-minutes-11eec1492d30
* Clients in multiple languages?
  * Java (20, spring boot 3, native?)
  * Rust: https://blog.ediri.io/creating-a-microservice-in-rust-using-grpc?s=31

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

## Netshoot

* https://github.com/nicolaka/netshoot


```shell
kubectl run tmp-shell --rm -i --tty --image nicolaka/netshoot
```

## Carvel Package

### Carvel Repository

```yaml
apiVersion: packaging.carvel.dev/v1alpha1
kind: PackageRepository
metadata:
  annotations:
    kctrl.carvel.dev/repository-version: 0.0.0-08ddea6
  creationTimestamp: "2022-12-11T19:31:21Z"
  name: carvel.kearos.net
spec:
  fetch:
    imgpkgBundle:
      image: index.docker.io/caladreas/carvel-repo@sha256:328ce1a61054c6fb1aa8f291b3d32ca1b92407ad159cb1e266556d931d1cc771
```

### Server Package

```yaml
apiVersion: packaging.carvel.dev/v1alpha1
kind: PackageInstall
metadata:
  name: gitstafette-server
  namespace: gitstafette
spec:
  serviceAccountName: default
  packageRef:
    refName: server.gitstafette.kearos.net
    versionSelection:
      constraints: 0.0.0-08ddea6
```