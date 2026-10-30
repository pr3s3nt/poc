package delivery

import (
	"context"
	"orchestrator/internal/domain/application"
	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/persistence"
)

func (s *Bindings) Inspect(ctx context.Context, orgKey, providerKind, accountKey string, version int64, config map[string]string) (port.ConfigurationDiscovery, error) {
	org, err := s.store.GetOrganization(ctx, orgKey)
	if err != nil {
		return port.ConfigurationDiscovery{}, err
	}
	account, err := s.store.GetConnection(ctx, orgKey, accountKey)
	if err != nil || account.Kind != application.ConnectionKubernetes || account.Status != application.ConnectionReady || account.Version != version {
		return port.ConfigurationDiscovery{}, persistence.ErrNotFound
	}
	provider, err := s.registry.Resolve(providerKind)
	if err != nil {
		return port.ConfigurationDiscovery{}, err
	}
	inspector, ok := provider.(port.ConfigurationInspector)
	if !ok {
		return port.ConfigurationDiscovery{}, domain.ErrUnsupported
	}
	return inspector.InspectConfiguration(ctx, domain.BindingVersion{OrganizationID: org.ID, ManagementAccountID: account.ID, ManagementAccountVersion: account.Version, Config: config})
}
