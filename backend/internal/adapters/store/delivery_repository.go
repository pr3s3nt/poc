package store

import (
	"context"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/ports/persistence"
)

func (s *Store) GetPlatformDeliveryDefault(ctx context.Context, org string) (domain.PlatformDefault, error) {
	defer s.rlock(ctx)()
	d, ok := s.state.PlatformDeliveryDefaults[org]
	if !ok {
		return d, persistence.ErrNotFound
	}
	return d, nil
}
func (s *Store) SetPlatformDeliveryDefault(ctx context.Context, in domain.PlatformDefault, expected int64) (out domain.PlatformDefault, err error) {
	err = s.Transact(ctx, func(ctx context.Context) error {
		if in.OrganizationID == "" || in.BindingID == "" || in.BindingVersion < 1 || expected < 0 {
			return domain.ErrInvalid
		}
		b, err := s.GetDeliveryBinding(ctx, in.OrganizationID, in.BindingID, in.BindingVersion)
		if err != nil {
			return err
		}
		if b.Status != domain.BindingReady {
			return domain.ErrUnsupported
		}
		if s.state.PlatformDeliveryDefaults == nil {
			s.state.PlatformDeliveryDefaults = map[string]domain.PlatformDefault{}
		}
		prior := s.state.PlatformDeliveryDefaults[in.OrganizationID]
		if prior.Revision != expected {
			return persistence.ErrVersionConflict
		}
		in.Revision = expected + 1
		s.state.PlatformDeliveryDefaults[in.OrganizationID] = in
		out = in
		return nil
	})
	return
}
func repositoryKey(org, app string) string { return org + "/" + app }
func (s *Store) GetApplicationDeliveryRepository(ctx context.Context, org, app string) (domain.ApplicationRepository, error) {
	defer s.rlock(ctx)()
	v, ok := s.state.ApplicationDeliveryRepositories[repositoryKey(org, app)]
	if !ok {
		return v, persistence.ErrNotFound
	}
	return v, nil
}
func (s *Store) ReserveApplicationDeliveryRepository(ctx context.Context, in domain.ApplicationRepository) (out domain.ApplicationRepository, err error) {
	err = s.Transact(ctx, func(ctx context.Context) error {
		if err := s.fence(ctx); err != nil {
			return err
		}
		if in.Validate() != nil || in.Status != "PENDING" || in.Revision != 1 {
			return domain.ErrInvalid
		}
		found := false
		for _, a := range s.state.Applications {
			org, err := s.GetOrganization(ctx, a.OrganizationKey)
			if err == nil && a.ID == in.ApplicationID && org.ID == in.OrganizationID && a.Lifecycle() == "ACTIVE" {
				found = true
			}
		}
		if !found {
			return persistence.ErrNotFound
		}
		if _, err := s.GetDeliveryBinding(ctx, in.OrganizationID, in.BindingID, in.BindingVersion); err != nil {
			return err
		}
		if s.state.ApplicationDeliveryRepositories == nil {
			s.state.ApplicationDeliveryRepositories = map[string]domain.ApplicationRepository{}
		}
		k := repositoryKey(in.OrganizationID, in.ApplicationID)
		if prior, ok := s.state.ApplicationDeliveryRepositories[k]; ok {
			if prior.BindingID != in.BindingID || prior.BindingVersion != in.BindingVersion || prior.Name != in.Name {
				return persistence.ErrVersionConflict
			}
			out = prior
			return nil
		}
		s.state.ApplicationDeliveryRepositories[k] = in
		out = in
		return nil
	})
	return
}
func (s *Store) AcknowledgeApplicationDeliveryRepository(ctx context.Context, in domain.ApplicationRepository, expected int64) (out domain.ApplicationRepository, err error) {
	err = s.Transact(ctx, func(ctx context.Context) error {
		if err := s.fence(ctx); err != nil {
			return err
		}
		if in.Validate() != nil || in.Status != "READY" {
			return domain.ErrInvalid
		}
		k := repositoryKey(in.OrganizationID, in.ApplicationID)
		prior, ok := s.state.ApplicationDeliveryRepositories[k]
		if !ok {
			return persistence.ErrNotFound
		}
		if !prior.SameAllocation(in) {
			return persistence.ErrImmutable
		}
		if prior.Status == "READY" {
			if prior.RemoteID != in.RemoteID || prior.CloneURL != in.CloneURL || prior.Branch != in.Branch {
				return persistence.ErrImmutable
			}
			out = prior
			return nil
		}
		if prior.Revision != expected {
			return persistence.ErrVersionConflict
		}
		in.Revision = expected + 1
		s.state.ApplicationDeliveryRepositories[k] = in
		out = in
		return nil
	})
	return
}
