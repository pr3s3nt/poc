package postgres

import (
	"context"
	"errors"

	"orchestrator/internal/domain/endpoint"
	"orchestrator/internal/ports/persistence"

	"github.com/jackc/pgx/v5"
)

// endpointOwnerIDs resolves the owner's keys to the referenced row identities.
const endpointOwnerIDs = `SELECT o.id,a.id,e.id FROM organizations o
 JOIN applications a ON a.organization_id=o.id
 JOIN environments e ON e.application_id=a.id
 WHERE o.organization_key=$1 AND a.application_key=$2 AND e.environment_key=$3`

type endpointOwnerRow struct{ organization, application, environment string }

func (s *Store) endpointOwner(ctx context.Context, owner endpoint.Owner) (endpointOwnerRow, error) {
	var row endpointOwnerRow
	err := s.q(ctx).QueryRow(ctx, endpointOwnerIDs, owner.Organization, owner.Application, owner.Environment).Scan(&row.organization, &row.application, &row.environment)
	return row, translate(err)
}

// CheckEndpoint is a read-only, advisory availability check for Preview.
func (s *Store) CheckEndpoint(ctx context.Context, host string, owner endpoint.Owner) error {
	if err := owner.Validate(); err != nil {
		return err
	}
	var existing endpoint.Owner
	err := s.q(ctx).QueryRow(ctx, `SELECT o.organization_key,a.application_key,e.environment_key,r.target_generation,r.workload_id
 FROM endpoint_reservations r JOIN organizations o ON o.id=r.organization_id
 JOIN applications a ON a.id=r.application_id JOIN environments e ON e.id=r.environment_id
 WHERE r.hostname=$1`, host).Scan(&existing.Organization, &existing.Application, &existing.Environment, &existing.Generation, &existing.Workload)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return translate(err)
	}
	if existing != owner {
		return persistence.ErrHostnameTaken
	}
	return nil
}

// ReserveEndpoint atomically claims host. The hostname primary key arbitrates
// across processes, Organizations and concurrent transactions; a same-owner
// retry is idempotent and never replaces another owner.
func (s *Store) ReserveEndpoint(ctx context.Context, host string, owner endpoint.Owner) error {
	if err := owner.Validate(); err != nil {
		return err
	}
	ids, err := s.endpointOwner(ctx, owner)
	if err != nil {
		return err
	}
	var accepted string
	err = s.q(ctx).QueryRow(ctx, `INSERT INTO endpoint_reservations(hostname,organization_id,application_id,environment_id,target_generation,workload_id)
 VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(hostname) DO UPDATE SET hostname=EXCLUDED.hostname
 WHERE endpoint_reservations.organization_id=EXCLUDED.organization_id AND endpoint_reservations.application_id=EXCLUDED.application_id
 AND endpoint_reservations.environment_id=EXCLUDED.environment_id AND endpoint_reservations.target_generation=EXCLUDED.target_generation
 AND endpoint_reservations.workload_id=EXCLUDED.workload_id RETURNING hostname`,
		host, ids.organization, ids.application, ids.environment, owner.Generation, owner.Workload).Scan(&accepted)
	if errors.Is(err, pgx.ErrNoRows) {
		return persistence.ErrHostnameTaken
	}
	return translate(err)
}

// RetainEndpoints releases the owner's claims not listed in keep. Callers
// invoke it only after acknowledged delivery, pruning or removal.
func (s *Store) RetainEndpoints(ctx context.Context, owner endpoint.Owner, keep []string) error {
	if err := owner.Validate(); err != nil {
		return err
	}
	if keep == nil {
		keep = []string{}
	}
	ids, err := s.endpointOwner(ctx, owner)
	if errors.Is(err, persistence.ErrNotFound) {
		return nil // an owner that never existed holds no reservation
	}
	if err != nil {
		return err
	}
	_, err = s.q(ctx).Exec(ctx, `DELETE FROM endpoint_reservations WHERE organization_id=$1 AND application_id=$2 AND environment_id=$3
 AND target_generation=$4 AND workload_id=$5 AND NOT(hostname=ANY($6::text[]))`,
		ids.organization, ids.application, ids.environment, owner.Generation, owner.Workload, keep)
	return translate(err)
}

var _ persistence.EndpointRepository = (*Store)(nil)
