package delivery

import (
	"context"
	"errors"
	"strings"
	"time"

	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/persistence"
	hosting "orchestrator/internal/ports/repositoryhosting"
)

var ErrPlatformDefault = errors.New("delivery: platform default CD must be configured")

type RepositoryStore interface {
	persistence.Store
	persistence.DeliveryPlatformRepository
	persistence.DeliveryBindingRepository
}
type Repositories struct {
	Store RepositoryStore
	Hosts hosting.Factory
}

func (s *Repositories) Resolve(ctx context.Context, b domain.BindingVersion, p port.ApplicationRepositoryBinding, env environment.Environment, create bool) (domain.BindingVersion, error) {
	policy, err := p.RepositoryPolicy(b)
	if err != nil {
		return b, err
	}
	if !policy.Enabled {
		return b, nil
	}
	if s == nil || s.Store == nil || s.Hosts == nil {
		return b, ErrUnavailable
	}
	app, err := s.Store.GetApplication(ctx, env.ApplicationKey)
	if err != nil {
		return b, err
	}
	org, err := s.Store.GetOrganization(ctx, app.OrganizationKey)
	if err != nil || org.ID != b.OrganizationID {
		return b, persistence.ErrNotFound
	}
	allocation, err := s.Store.GetApplicationDeliveryRepository(ctx, org.ID, app.ID)
	if errors.Is(err, persistence.ErrNotFound) && create {
		owner, ok := persistence.OwnerFrom(ctx)
		if !ok {
			return b, persistence.ErrOperationLost
		}
		op, ownerErr := s.Store.GetOperation(ctx, owner.OperationID)
		if ownerErr != nil || op.Owner != owner.Owner || op.Fence != owner.Fence || !op.Status.Holds() || op.ApplicationKey != app.Key || op.EnvironmentKey != env.Key {
			return b, persistence.ErrOperationLost
		}
		id := ids.New()
		allocation, err = s.Store.ReserveApplicationDeliveryRepository(ctx, domain.ApplicationRepository{ID: id, OrganizationID: org.ID, ApplicationID: app.ID, BindingID: b.ID, BindingVersion: b.Version, Name: policy.Prefix + strings.ReplaceAll(app.ID, "-", ""), Marker: "orchestrator application " + app.ID + " allocation " + id, Status: "PENDING", Revision: 1, CreatedAt: time.Now().UTC().Truncate(time.Microsecond)})
	}
	if err != nil {
		return b, err
	}
	if allocation.BindingID != b.ID || allocation.BindingVersion != b.Version {
		return b, domain.ErrUnsupported
	}
	if create {
		owner, ok := persistence.OwnerFrom(ctx)
		if !ok {
			return b, persistence.ErrOperationLost
		}
		op, err := s.Store.GetOperation(ctx, owner.OperationID)
		if err != nil || op.Owner != owner.Owner || op.Fence != owner.Fence || !op.Status.Holds() || op.ApplicationKey != app.Key || op.EnvironmentKey != env.Key {
			return b, persistence.ErrOperationLost
		}
		host, err := s.Hosts.Open(ctx, b, policy)
		if err != nil {
			return b, err
		}
		acknowledged, err := host.Ensure(ctx, allocation)
		if err != nil {
			return b, err
		}
		op, err = s.Store.GetOperation(ctx, owner.OperationID)
		if err != nil || op.Owner != owner.Owner || op.Fence != owner.Fence || !op.Status.Holds() || op.ApplicationKey != app.Key || op.EnvironmentKey != env.Key {
			return b, persistence.ErrOperationLost
		}
		allocation, err = s.Store.AcknowledgeApplicationDeliveryRepository(ctx, acknowledged, allocation.Revision)
		if err != nil {
			return b, err
		}
	}
	if allocation.Status != "READY" {
		return b, ErrUnavailable
	}
	return p.BindApplicationRepository(b, allocation, env.Key)
}
func (s *Bindings) Default(ctx context.Context, orgKey string) (domain.PlatformDefault, error) {
	org, err := s.store.GetOrganization(ctx, orgKey)
	if err != nil {
		return domain.PlatformDefault{}, err
	}
	repo, ok := s.store.(persistence.DeliveryPlatformRepository)
	if !ok {
		return domain.PlatformDefault{}, ErrUnavailable
	}
	out, err := repo.GetPlatformDeliveryDefault(ctx, org.ID)
	if errors.Is(err, persistence.ErrNotFound) {
		return domain.PlatformDefault{OrganizationID: org.ID}, nil
	}
	return out, err
}
func (s *Bindings) SetDefault(ctx context.Context, orgKey, id string, version, expected int64) (domain.PlatformDefault, error) {
	org, err := s.store.GetOrganization(ctx, orgKey)
	if err != nil {
		return domain.PlatformDefault{}, err
	}
	repo, ok := s.store.(persistence.DeliveryPlatformRepository)
	if !ok {
		return domain.PlatformDefault{}, ErrUnavailable
	}
	b, err := s.store.GetDeliveryBinding(ctx, org.ID, id, version)
	if err != nil {
		return domain.PlatformDefault{}, err
	}
	if b.Status != domain.BindingReady {
		return domain.PlatformDefault{}, domain.ErrUnsupported
	}
	provider, err := s.registry.Resolve(b.ProviderKind)
	if err != nil {
		return domain.PlatformDefault{}, ErrUnavailable
	}
	managed, ok := provider.(port.ApplicationRepositoryBinding)
	if !ok {
		return domain.PlatformDefault{}, domain.ErrUnsupported
	}
	policy, err := managed.RepositoryPolicy(b)
	if err != nil || !policy.Enabled {
		return domain.PlatformDefault{}, domain.ErrUnsupported
	}
	return repo.SetPlatformDeliveryDefault(ctx, domain.PlatformDefault{OrganizationID: org.ID, BindingID: b.ID, BindingVersion: b.Version}, expected)
}

// DefaultSelection is called inside the Application creation transaction. It
// pins existing verified platform facts only; no remote repository is created.
func (s *Bindings) DefaultSelection(ctx context.Context, orgKey string) (domain.Selection, error) {
	d, err := s.Default(ctx, orgKey)
	if err != nil {
		return domain.Selection{}, err
	}
	if d.Revision == 0 {
		return domain.Selection{}, ErrPlatformDefault
	}
	b, err := s.store.GetDeliveryBinding(ctx, d.OrganizationID, d.BindingID, d.BindingVersion)
	if err != nil || b.Status != domain.BindingReady {
		return domain.Selection{}, ErrPlatformDefault
	}
	selection := domain.Selection{Mode: domain.CD, BindingID: b.ID, BindingVersion: b.Version}
	if b.Capabilities.ValidateSelection(selection) != nil {
		return domain.Selection{}, ErrPlatformDefault
	}
	return selection, nil
}
