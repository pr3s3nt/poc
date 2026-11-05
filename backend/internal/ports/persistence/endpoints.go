package persistence

import (
	"context"
	"errors"
	"orchestrator/internal/domain/endpoint"
)

var ErrHostnameTaken = errors.New("endpoint: hostname is already reserved")

// EndpointRepository is an atomic reservation ledger, independent of mutable
// ActiveResource outputs. Failed/unknown publication never deletes reservations.
type EndpointRepository interface {
	CheckEndpoint(context.Context, string, endpoint.Owner) error
	ReserveEndpoint(context.Context, string, endpoint.Owner) error
	// RetainEndpoints is called ONLY after acknowledged delivery/pruning/removal.
	RetainEndpoints(context.Context, endpoint.Owner, []string) error
}
