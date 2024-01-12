package grpc

import (
	"context"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// HealthCheckService implements grpc_health_v1.HealthServer
type HealthCheckService struct{}

// TODO add actual health checks
func (s *HealthCheckService) Check(ctx context.Context, req *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	return &grpc_health_v1.HealthCheckResponse{
		Status: grpc_health_v1.HealthCheckResponse_SERVING,
	}, nil
}

// TODO add actual health checks
func (s *HealthCheckService) Watch(req *grpc_health_v1.HealthCheckRequest, server grpc_health_v1.Health_WatchServer) error {
	return server.Send(&grpc_health_v1.HealthCheckResponse{
		Status: grpc_health_v1.HealthCheckResponse_SERVING,
	})
}
