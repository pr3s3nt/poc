package execution

import (
	"context"
	"orchestrator/internal/domain/endpoint"
)

// GatewayReadiness verifies that existing infrastructure admits the route's
// workload namespace. It is read-only and is not a Gateway lifecycle owner.
type GatewayReadiness interface {
	CheckGateway(context.Context, Target, endpoint.Desired) error
}
