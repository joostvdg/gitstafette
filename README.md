# gitstafette

Git Webhook Relay demo app

## TODO

* Alternative with AWS
  * Status Page
    * https://www.statuscake.com/statuscake-long-page/?a_aid=5eef0535df77b&a_bid=b0850064
    * https://cronitor.io/pricing
    * https://uptimerobot.com/pricing/
    * https://www.checklyhq.com/pricing
  * Web fronted by Envoy with AWS Cert
    * like Keycloak on VMware laptop
* set Kubernetes security
  * SecurityContext: https://snyk.io/blog/10-kubernetes-security-context-settings-you-should-understand/
  * Seccomp profiles: https://itnext.io/seccomp-in-kubernetes-part-i-7-things-you-should-know-before-you-even-start-97502ad6b6d6
    * https://www.pulumi.com/resources/kubernetes-seccomp-profiles/
  * Secrity Admission: https://kubernetes.io/blog/2022/08/25/pod-security-admission-stable/
  * Network policies: https://kubernetes.io/docs/concepts/services-networking/network-policies/
* CI/CD In Kubernetes
  * Build with Tekton / CloudNative BuildPacks
  * generate SBOM/SPDX
  * deploy via Crossplane
    * https://marketplace.upbound.io/providers/upbound/provider-gcp/v0.26.0/resources/cloudrun.gcp.upbound.io/Service/v1beta1
* Add Sentry support for client
* OpenTelemetry metrics
* OpenTracing metrics
* Expose State with GraphQL
  * with authentication
  * Gitstafette Explorer?
* track relay status per client
* alternative setup with CIVO cloud
  * https://www.civo.com/docs/kubernetes/load-balancers
* Fly.io
  * https://fly.io/docs/reference/configuration/#services-ports-tls_options
  * https://fly.io/docs/app-guides/multiple-processes/#maybe-you-don-t-need-multiple-processes
* CI/CD In Kubernetes
  * Scan with Snyk?
  * Testcontainers?
  * combine steps with Cartographer?
* Kubernetes Controller + CR for generating clients
  * Metacontroller?
  * Operator?
  * (GRPC) Server should support multiple clients --> Does!
  * add CR to cluster for individual Repo's, then spawn a client
  * https://betterprogramming.pub/build-a-kubernetes-operator-in-10-minutes-11eec1492d30
* Clients in multiple languages?
  * Java (20, spring boot 3, native?)
  * Rust: https://blog.ediri.io/creating-a-microservice-in-rust-using-grpc?s=31


### HMAC Support

* https://golangcode.com/generate-sha256-hmac/
* https://docs.github.com/en/developers/webhooks-and-events/webhooks/securing-your-webhooks

## Testing Kubernetes

### HTTP

```shell
kubectl port-forward -n gitstafette svc/gitstafette-config 7777:1323
```

```shell
http :7777
```

### GRPC

```shell
kubectl port-forward -n gitstafette svc/gitstafette-config 7777:50051
```

```shell
grpc-health-probe -addr=localhost:7777
```

## Resources

* https://developer.redis.com/develop/golang/

### GRPC

* https://github.com/fullstorydev/grpcurl
* https://github.com/grpc-ecosystem/grpc-cloud-run-example/tree/master/golang
* https://github.com/grpc/grpc-go/blob/master/Documentation/keepalive.md
* https://github.com/grpc/grpc-go/tree/master/examples/features/keepalive
* https://github.com/GoogleCloudPlatform/golang-samples/tree/main/run/grpc-server-streaming
* http://www.inanzzz.com/index.php/post/cvjx/using-oauth-authentication-tokens-for-grpc-client-and-server-communications-in-golang

### Test GRPC

* running server without TLS

```shell
grpcurl \
  -plaintext \
  -proto api/v1/gitstafette.proto \
  -d '{"client_id": "me", "repository_id": "537845873", "last_received_event_id": 1}' \
  localhost:50051 \
  gitstafette.v1.Gitstafette.FetchWebhookEvents
```

* running server with TLS

```shell
grpcurl \                                                                                                                               ─╯
  -proto api/v1/gitstafette.proto \
  -d '{"client_id": "me", "repository_id": "537845873", "last_received_event_id": 1}' \
  localhost:50051 \
  gitstafette.v1.Gitstafette.FetchWebhookEvents
```

```shell
grpcurl \
  -proto api/v1/gitstafette.proto \
  -d '{"client_id": "me", "repository_id": "537845873", "last_received_event_id": 1}' \
  -cacert /mnt/d/Projects/homelab-rpi/certs/ca.pem \
  -cert /mnt/d/Projects/homelab-rpi/certs/gitstafette/client-local.pem \
  -key /mnt/d/Projects/homelab-rpi/certs/gitstafette/client-local-key.pem \
  localhost:50051 \
  gitstafette.v1.Gitstafette.FetchWebhookEvents 
```

### GRPC HealthCheck

* When insecure
  ```shell
  grpc-health-probe -addr=localhost:50051
  ```

* When secure
  ```shell
  grpc-health-probe -addr=localhost:50051 \
      -tls \
      -tls-ca-cert /mnt/d/Projects/homelab-rpi/certs/ca.pem \
      -tls-client-cert /mnt/d/Projects/homelab-rpi/certs/gitstafette/client-local.pem \
      -tls-client-key /mnt/d/Projects/homelab-rpi/certs/gitstafette/client-local-key.pem
  ```



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
  X-GitHub-Event:push \
  Test=True
```

```shell
http POST http://localhost:1323/v1/github/ \
  X-Github-Delivery:d4049330-377e-11ed-9c2e-1ae286aab35f \
  X-Github-Hook-Installation-Target-Id:478599060 \
  X-Github-Hook-Installation-Target-Type:repository \
  X-GitHub-Event:push \
  Test=True
```

### GCR

```shell
http POST https://gitstafette-server-http-qad46fd4qq-ez.a.run.app/v1/github/ \
  X-Github-Delivery:d4049330-377e-11ed-9c2e-1ae286aab35f \
  X-Github-Hook-Installation-Target-Id:537845873 \
  X-Github-Hook-Installation-Target-Type:repository \
  X-GitHub-Event:push \
  Test=True
```


### Invalid HMAC

```shell
http POST http://localhost:1323/v1/github/ \
  X-Github-Delivery:d4049330-377e-11ed-9c2e-1ae286aab35f \
  X-Github-Hook-Installation-Target-Id:537845873 \
  X-Github-Hook-Installation-Target-Type:repository \
  X-GitHub-Event:push \
  x-hub-signature-256:sha256=b101fdde955cb8809872eaa41d56838c9fbaa7aace134743cfd1fea7b87dc74e \
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
  name: gitstafette-config
  namespace: gitstafette
spec:
  serviceAccountName: default
  packageRef:
    refName: config.gitstafette.kearos.net
    versionSelection:
      constraints: 0.0.0-08ddea6
```

## Docker Compose

* https://gruchalski.com/posts/2022-02-20-keycloak-1700-with-tls-behind-envoy/
* https://github.com/envoyproxy/envoy/blob/main/examples/front-proxy/docker-compose.yaml
* https://docs.docker.com/compose/compose-file/#command

### Test Connection Via Envoy HTTPS

```shell
http POST https://localhost/v1/github/ \
  Host:events.gitstafette.joostvdg.net \
  X-Github-Delivery:d4049330-377e-11ed-9c2e-1ae286aab35f \
  X-Github-Hook-Installation-Target-Id:537845873 \
  X-Github-Hook-Installation-Target-Type:repository \
  X-GitHub-Event:push \
  Test=True --verify=false
```

## Running On AWS

* https://cloud-images.ubuntu.com/locator/ec2/
* https://developer.hashicorp.com/packer/tutorials/aws-get-started/aws-get-started-build-image
* https://docs.docker.com/engine/install/ubuntu/