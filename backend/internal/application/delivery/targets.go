package delivery

import (
	"context"

	"orchestrator/internal/domain/application"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
)

// Targets resolves original Organization/account ID pins for private worker calls.
// It never uses host cluster state.
type Targets struct {
	Store    persistence.Store
	Identity persistence.OrganizationIdentityReader
}

func (t Targets) ResolveManagementTarget(ctx context.Context, b port.BindingVersion) (execution.Target, error) {
	if t.Identity == nil || t.Store == nil {
		return execution.Target{}, persistence.ErrNotFound
	}
	org, err := t.Identity.GetOrganizationByID(ctx, b.OrganizationID)
	if err != nil || org.ID != b.OrganizationID {
		return execution.Target{}, persistence.ErrNotFound
	}
	accounts, err := t.Store.ListConnections(ctx, org.Key)
	if err != nil {
		return execution.Target{}, err
	}
	for _, a := range accounts {
		if a.ID == b.ManagementAccountID && a.Version == b.ManagementAccountVersion && a.Kind == application.ConnectionKubernetes && a.Status == application.ConnectionReady {
			return execution.Target{Organization: org.Key, Connection: a.Key}, nil
		}
	}
	return execution.Target{}, persistence.ErrNotFound
}
func (t Targets) ResolveWorkloadTarget(ctx context.Context, b port.BindingVersion, key string) (execution.Target, error) {
	if t.Identity == nil || t.Store == nil {
		return execution.Target{}, persistence.ErrNotFound
	}
	org, err := t.Identity.GetOrganizationByID(ctx, b.OrganizationID)
	if err != nil || org.ID != b.OrganizationID {
		return execution.Target{}, persistence.ErrNotFound
	}
	a, err := t.Store.GetConnection(ctx, org.Key, key)
	if err != nil || a.Kind != application.ConnectionKubernetes || a.Status != application.ConnectionReady {
		return execution.Target{}, persistence.ErrNotFound
	}
	return execution.Target{Organization: org.Key, Connection: a.Key}, nil
}
