package grpcclient

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// CheckHealth performs a gRPC health check against a target service.
// Uses the standard grpc.health.v1.Health/Check protocol.
// Returns nil if the service is SERVING, error otherwise.
func CheckHealth(ctx context.Context, conn *grpc.ClientConn) error {
	client := grpc_health_v1.NewHealthClient(conn)
	resp, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		return fmt.Errorf("grpcclient: health check: %w", err)
	}
	if resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		return fmt.Errorf("grpcclient: service not serving (status: %s)", resp.GetStatus())
	}
	return nil
}
