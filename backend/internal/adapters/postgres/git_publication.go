package postgres

import (
	"context"
	"encoding/hex"
	"orchestrator/internal/ports/persistence"
)

func (s *Store) publicationOwner(ctx context.Context, l persistence.GitPublicationLease) error {
	current, ok := persistence.OwnerFrom(ctx)
	if !ok || current != l.Owner || !validUUID(l.OrganizationID) {
		return persistence.ErrOperationLost
	}
	digest, err := hex.DecodeString(l.Destination)
	if err != nil || len(digest) != 32 {
		return persistence.ErrImmutable
	}
	var valid bool
	err = s.q(ctx).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM environment_operations op JOIN environments e ON e.id=op.environment_id JOIN applications a ON a.id=e.application_id WHERE op.id=$1 AND op.owner_instance=$2 AND op.fence=$3 AND op.status IN ('ACTIVE','RECOVERING') AND e.active_operation_id=op.id AND a.organization_id=$4)`, current.OperationID, current.Owner, current.Fence, l.OrganizationID).Scan(&valid)
	if err != nil {
		return translate(err)
	}
	if !valid {
		return persistence.ErrOperationLost
	}
	return nil
}
func (s *Store) ClaimGitPublication(ctx context.Context, l persistence.GitPublicationLease) error {
	return s.fenced(ctx, func(ctx context.Context) error {
		if err := s.publicationOwner(ctx, l); err != nil {
			return err
		}
		_, err := s.q(ctx).Exec(ctx, `INSERT INTO git_publication_leases(destination_digest,organization_id,environment_operation_id,owner,fence) VALUES($1,$2,$3,$4,$5) ON CONFLICT(destination_digest) DO NOTHING`, l.Destination, l.OrganizationID, l.Owner.OperationID, l.Owner.Owner, l.Owner.Fence)
		if err != nil {
			return translate(err)
		}
		return s.CheckGitPublication(ctx, l)
	})
}
func (s *Store) CheckGitPublication(ctx context.Context, l persistence.GitPublicationLease) error {
	if err := s.publicationOwner(ctx, l); err != nil {
		return err
	}
	var matches bool
	err := s.q(ctx).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM git_publication_leases WHERE destination_digest=$1 AND organization_id=$2 AND environment_operation_id=$3 AND owner=$4 AND fence=$5)`, l.Destination, l.OrganizationID, l.Owner.OperationID, l.Owner.Owner, l.Owner.Fence).Scan(&matches)
	if err != nil {
		return translate(err)
	}
	if !matches {
		return persistence.ErrEnvironmentBusy
	}
	return nil
}
func (s *Store) ReleaseGitPublication(ctx context.Context, l persistence.GitPublicationLease) error {
	return s.fenced(ctx, func(ctx context.Context) error {
		if err := s.publicationOwner(ctx, l); err != nil {
			return err
		}
		tag, err := s.q(ctx).Exec(ctx, `DELETE FROM git_publication_leases WHERE destination_digest=$1 AND organization_id=$2 AND environment_operation_id=$3 AND owner=$4 AND fence=$5`, l.Destination, l.OrganizationID, l.Owner.OperationID, l.Owner.Owner, l.Owner.Fence)
		if err != nil {
			return translate(err)
		}
		if tag.RowsAffected() != 1 {
			return persistence.ErrOperationLost
		}
		return nil
	})
}
