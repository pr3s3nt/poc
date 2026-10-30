package delivery

import (
	"context"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/ports/persistence"
)

type Usage struct {
	ApplicationKey string `json:"applicationKey"`
	EnvironmentKey string `json:"environmentKey"`
	Desired        bool   `json:"desired"`
	Applied        bool   `json:"applied"`
}
type BindingDetails struct {
	Binding              domain.BindingVersion
	ManagementAccountKey string
	CredentialConfigured bool
	Usage                []Usage
}

func (s *Bindings) Details(ctx context.Context, orgKey, id string, version int64) (BindingDetails, error) {
	org, err := s.store.GetOrganization(ctx, orgKey)
	if err != nil {
		return BindingDetails{}, err
	}
	b, err := s.store.GetDeliveryBinding(ctx, org.ID, id, version)
	if err != nil {
		return BindingDetails{}, err
	}
	out := BindingDetails{Binding: b, CredentialConfigured: b.WriterCredentialRef != "", Usage: []Usage{}}
	accounts, err := s.store.ListConnections(ctx, orgKey)
	if err != nil {
		return out, err
	}
	for _, a := range accounts {
		if a.ID == b.ManagementAccountID {
			out.ManagementAccountKey = a.Key
		}
	}
	if out.ManagementAccountKey == "" {
		return out, persistence.ErrNotFound
	}
	apps, err := s.store.ListApplications(ctx)
	if err != nil {
		return out, err
	}
	for _, app := range apps {
		if app.OrganizationKey != orgKey {
			continue
		}
		envs, err := s.store.ListEnvironments(ctx, app.Key)
		if err != nil {
			return out, err
		}
		for _, env := range envs {
			desired, applied := env.DesiredDelivery.Effective(), env.AppliedDelivery.Effective()
			d := desired.BindingID == id && desired.BindingVersion == version
			a := applied.BindingID == id && applied.BindingVersion == version
			if d || a {
				out.Usage = append(out.Usage, Usage{ApplicationKey: app.Key, EnvironmentKey: env.Key, Desired: d, Applied: a})
			}
		}
	}
	return out, nil
}
