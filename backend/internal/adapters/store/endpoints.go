package store

import (
	"context"
	"orchestrator/internal/domain/endpoint"
	"orchestrator/internal/ports/persistence"
)

func (s *Store) CheckEndpoint(ctx context.Context, host string, owner endpoint.Owner) error {
	defer s.rlock(ctx)()
	if err := owner.Validate(); err != nil {
		return err
	}
	if prior, ok := s.state.EndpointReservations[host]; ok && prior != owner {
		return persistence.ErrHostnameTaken
	}
	return nil
}
func (s *Store) ReserveEndpoint(ctx context.Context, host string, owner endpoint.Owner) error {
	defer s.lock(ctx)()
	if err := owner.Validate(); err != nil {
		return err
	}
	if prior, ok := s.state.EndpointReservations[host]; ok && prior != owner {
		return persistence.ErrHostnameTaken
	}
	if s.state.EndpointReservations == nil {
		s.state.EndpointReservations = map[string]endpoint.Owner{}
	}
	s.state.EndpointReservations[host] = owner
	return nil
}
func (s *Store) RetainEndpoints(ctx context.Context, owner endpoint.Owner, keep []string) error {
	defer s.lock(ctx)()
	if err := owner.Validate(); err != nil {
		return err
	}
	retained := map[string]bool{}
	for _, host := range keep {
		retained[host] = true
	}
	for host, prior := range s.state.EndpointReservations {
		if prior == owner && !retained[host] {
			delete(s.state.EndpointReservations, host)
		}
	}
	return nil
}

var _ persistence.EndpointRepository = (*Store)(nil)
