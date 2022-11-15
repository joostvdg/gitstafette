# gitstafette

Git Webhook Relay demo app

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