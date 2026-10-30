package store

import (
	"context"
	"encoding/hex"
	"orchestrator/internal/ports/persistence"
)

func (s *Store) publicationOwner(ctx context.Context, l persistence.GitPublicationLease) error {
	owner, ok := persistence.OwnerFrom(ctx)
	if !ok || owner != l.Owner {
		return persistence.ErrOperationLost
	}
	digest, err := hex.DecodeString(l.Destination)
	if err != nil || len(digest) != 32 {
		return persistence.ErrImmutable
	}
	op, ok := s.state.Operations[owner.OperationID]
	if !ok || op.Owner != owner.Owner || op.Fence != owner.Fence || !op.Status.Holds() {
		return persistence.ErrOperationLost
	}
	app, ok := s.state.Applications[op.ApplicationKey]
	if !ok {
		return persistence.ErrNotFound
	}
	org, ok := s.state.Organizations[app.OrganizationKey]
	if !ok || org.ID != l.OrganizationID {
		return persistence.ErrNotFound
	}
	return nil
}
func (s *Store) ClaimGitPublication(ctx context.Context, l persistence.GitPublicationLease) error {
	return s.Transact(ctx, func(ctx context.Context) error {
		if err := s.publicationOwner(ctx, l); err != nil {
			return err
		}
		if prior, ok := s.state.PublicationLeases[l.Destination]; ok && prior != l {
			return persistence.ErrEnvironmentBusy
		}
		s.state.PublicationLeases[l.Destination] = l
		return nil
	})
}
func (s *Store) CheckGitPublication(ctx context.Context, l persistence.GitPublicationLease) error {
	defer s.rlock(ctx)()
	if err := s.publicationOwner(ctx, l); err != nil {
		return err
	}
	if prior, ok := s.state.PublicationLeases[l.Destination]; !ok || prior != l {
		return persistence.ErrOperationLost
	}
	return nil
}
func (s *Store) ReleaseGitPublication(ctx context.Context, l persistence.GitPublicationLease) error {
	return s.Transact(ctx, func(ctx context.Context) error {
		if err := s.publicationOwner(ctx, l); err != nil {
			return err
		}
		if prior, ok := s.state.PublicationLeases[l.Destination]; !ok || prior != l {
			return persistence.ErrOperationLost
		}
		delete(s.state.PublicationLeases, l.Destination)
		return nil
	})
}
