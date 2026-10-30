package persistence

import (
	"context"
	"orchestrator/internal/domain/application"
)

// OrganizationIdentityReader resolves original ID pins for private worker calls.
// It is not an HTTP authorization boundary.
type OrganizationIdentityReader interface {
	GetOrganizationByID(context.Context, string) (application.Organization, error)
}
