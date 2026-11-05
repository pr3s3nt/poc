package persistence

import (
	"context"
	"errors"

	"orchestrator/internal/domain/endpoint"
)

var ErrHostnameTaken = errors.New("endpoint: hostname is already reserved")

// EndpointRepository is an atomic reservation ledger, independent of mutable
// ActiveResource outputs (EP-03/05/07). A hostname belongs to one Application
// Environment generation; each hostname path belongs to one of its workloads.
// Failed/unknown publication never deletes reservations.
type EndpointRepository interface {
	CheckEndpoint(context.Context, endpoint.Claim, endpoint.Owner) error
	ReserveEndpoint(context.Context, endpoint.Claim, endpoint.Owner) error
	// RetainEndpoints is called ONLY after acknowledged delivery/pruning/removal.
	// It releases the workload's other paths and hostnames left without a path.
	RetainEndpoints(context.Context, endpoint.Owner, []endpoint.Claim) error
}
