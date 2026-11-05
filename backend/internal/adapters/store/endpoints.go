package store

import (
	"context"

	"orchestrator/internal/domain/endpoint"
	"orchestrator/internal/ports/persistence"
)

// endpointReservation is one hostname path claim of the in-memory ledger.
type endpointReservation struct {
	Claim endpoint.Claim `json:"claim"`
	Owner endpoint.Owner `json:"owner"`
}

func claimKey(claim endpoint.Claim) string { return claim.Hostname + " " + claim.Path }

func (s *Store) endpointConflict(claim endpoint.Claim, owner endpoint.Owner) bool {
	for _, held := range s.state.EndpointReservations {
		if held.Claim.Hostname == claim.Hostname && (!held.Owner.SameHost(owner) || (held.Claim.Path == claim.Path && held.Owner != owner)) {
			return true
		}
	}
	return false
}

func (s *Store) CheckEndpoint(ctx context.Context, claim endpoint.Claim, owner endpoint.Owner) error {
	defer s.rlock(ctx)()
	if err := owner.Validate(); err != nil {
		return err
	}
	if s.endpointConflict(claim, owner) {
		return persistence.ErrHostnameTaken
	}
	return nil
}

func (s *Store) ReserveEndpoint(ctx context.Context, claim endpoint.Claim, owner endpoint.Owner) error {
	defer s.lock(ctx)()
	if err := owner.Validate(); err != nil {
		return err
	}
	if s.endpointConflict(claim, owner) {
		return persistence.ErrHostnameTaken
	}
	if s.state.EndpointReservations == nil {
		s.state.EndpointReservations = map[string]endpointReservation{}
	}
	s.state.EndpointReservations[claimKey(claim)] = endpointReservation{Claim: claim, Owner: owner}
	return nil
}

func (s *Store) RetainEndpoints(ctx context.Context, owner endpoint.Owner, keep []endpoint.Claim) error {
	defer s.lock(ctx)()
	if err := owner.Validate(); err != nil {
		return err
	}
	retained := map[string]bool{}
	for _, claim := range keep {
		retained[claimKey(claim)] = true
	}
	for key, held := range s.state.EndpointReservations {
		if held.Owner == owner && !retained[key] {
			delete(s.state.EndpointReservations, key)
		}
	}
	return nil
}

var _ persistence.EndpointRepository = (*Store)(nil)
