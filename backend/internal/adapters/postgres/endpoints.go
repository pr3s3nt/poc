package postgres

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"orchestrator/internal/domain/endpoint"
	"orchestrator/internal/ports/persistence"
)

func (s *Store) CheckEndpoint(ctx context.Context, host string, owner endpoint.Owner) error {
	if err := owner.Validate(); err != nil {
		return err
	}
	var existing endpoint.Owner
	err := s.q(ctx).QueryRow(ctx, `SELECT organization_key,application_key,environment_key,target_generation,workload_id FROM endpoint_reservations WHERE hostname=$1`, host).Scan(&existing.Organization, &existing.Application, &existing.Environment, &existing.Generation, &existing.Workload)
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
func (s *Store) ReserveEndpoint(ctx context.Context, host string, owner endpoint.Owner) error {
	if err := owner.Validate(); err != nil {
		return err
	}
	// The unique hostname key arbitrates across processes, Organizations and
	// concurrent transactions. Same-owner retries never replace another owner.
	var accepted string
	err := s.q(ctx).QueryRow(ctx, `INSERT INTO endpoint_reservations(hostname,organization_key,application_key,environment_key,target_generation,workload_id)
 VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(hostname) DO UPDATE SET hostname=EXCLUDED.hostname
 WHERE endpoint_reservations.organization_key=EXCLUDED.organization_key AND endpoint_reservations.application_key=EXCLUDED.application_key
 AND endpoint_reservations.environment_key=EXCLUDED.environment_key AND endpoint_reservations.target_generation=EXCLUDED.target_generation
 AND endpoint_reservations.workload_id=EXCLUDED.workload_id RETURNING hostname`, host, owner.Organization, owner.Application, owner.Environment, owner.Generation, owner.Workload).Scan(&accepted)
	if errors.Is(err, pgx.ErrNoRows) {
		return persistence.ErrHostnameTaken
	}
	return translate(err)
}
func (s *Store) RetainEndpoints(ctx context.Context, owner endpoint.Owner, keep []string) error {
	if err := owner.Validate(); err != nil {
		return err
	}
	if keep == nil {
		keep = []string{}
	}
	_, err := s.q(ctx).Exec(ctx, `DELETE FROM endpoint_reservations WHERE organization_key=$1 AND application_key=$2 AND environment_key=$3 AND target_generation=$4 AND workload_id=$5 AND NOT(hostname=ANY($6::text[]))`, owner.Organization, owner.Application, owner.Environment, owner.Generation, owner.Workload, keep)
	return translate(err)
}

var _ persistence.EndpointRepository = (*Store)(nil)
