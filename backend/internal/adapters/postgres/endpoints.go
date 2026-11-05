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

// CheckEndpoint is a read-only, advisory availability check for Preview: the
// hostname must be free or held by the same Application Environment generation,
// and the path free or held by the same workload (EP-07).
func (s *Store) CheckEndpoint(ctx context.Context, claim endpoint.Claim, owner endpoint.Owner) error {
	if err := owner.Validate(); err != nil {
		return err
	}
	var host endpoint.Owner
	err := s.q(ctx).QueryRow(ctx, `SELECT o.organization_key,a.application_key,e.environment_key,h.target_generation
 FROM endpoint_hosts h JOIN organizations o ON o.id=h.organization_id
 JOIN applications a ON a.id=h.application_id JOIN environments e ON e.id=h.environment_id
 WHERE h.hostname=$1`, claim.Hostname).Scan(&host.Organization, &host.Application, &host.Environment, &host.Generation)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return translate(err)
	}
	if !host.SameHost(owner) {
		return persistence.ErrHostnameTaken
	}
	var workload string
	err = s.q(ctx).QueryRow(ctx, `SELECT workload_id FROM endpoint_paths WHERE hostname=$1 AND path=$2`, claim.Hostname, claim.Path).Scan(&workload)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return translate(err)
	}
	if workload != owner.Workload {
		return persistence.ErrHostnameTaken
	}
	return nil
}

// ReserveEndpoint atomically claims the hostname for the owner's Application
// Environment generation, then the path for its workload. Primary keys arbitrate
// across processes, Organizations and concurrent transactions; same-owner
// retries are idempotent and never replace another owner.
func (s *Store) ReserveEndpoint(ctx context.Context, claim endpoint.Claim, owner endpoint.Owner) error {
	if err := owner.Validate(); err != nil {
		return err
	}
	ids, err := s.endpointOwner(ctx, owner)
	if err != nil {
		return err
	}
	return s.Transact(ctx, func(ctx context.Context) error {
		var accepted string
		err := s.q(ctx).QueryRow(ctx, `INSERT INTO endpoint_hosts(hostname,organization_id,application_id,environment_id,target_generation)
 VALUES($1,$2,$3,$4,$5) ON CONFLICT(hostname) DO UPDATE SET hostname=EXCLUDED.hostname
 WHERE endpoint_hosts.organization_id=EXCLUDED.organization_id AND endpoint_hosts.application_id=EXCLUDED.application_id
 AND endpoint_hosts.environment_id=EXCLUDED.environment_id AND endpoint_hosts.target_generation=EXCLUDED.target_generation RETURNING hostname`,
			claim.Hostname, ids.organization, ids.application, ids.environment, owner.Generation).Scan(&accepted)
		if errors.Is(err, pgx.ErrNoRows) {
			return persistence.ErrHostnameTaken
		}
		if err != nil {
			return translate(err)
		}
		err = s.q(ctx).QueryRow(ctx, `INSERT INTO endpoint_paths(hostname,path,workload_id) VALUES($1,$2,$3)
 ON CONFLICT(hostname,path) DO UPDATE SET path=EXCLUDED.path WHERE endpoint_paths.workload_id=EXCLUDED.workload_id RETURNING hostname`,
			claim.Hostname, claim.Path, owner.Workload).Scan(&accepted)
		if errors.Is(err, pgx.ErrNoRows) {
			return persistence.ErrHostnameTaken
		}
		return translate(err)
	})
}

// RetainEndpoints releases the owner's path claims not listed in keep, then any
// hostname of its Application Environment generation left without a path.
// Callers invoke it only after acknowledged delivery, pruning or removal.
func (s *Store) RetainEndpoints(ctx context.Context, owner endpoint.Owner, keep []endpoint.Claim) error {
	if err := owner.Validate(); err != nil {
		return err
	}
	ids, err := s.endpointOwner(ctx, owner)
	if errors.Is(err, persistence.ErrNotFound) {
		return nil // an owner that never existed holds no reservation
	}
	if err != nil {
		return err
	}
	hosts, paths := []string{}, []string{}
	for _, claim := range keep {
		hosts, paths = append(hosts, claim.Hostname), append(paths, claim.Path)
	}
	return s.Transact(ctx, func(ctx context.Context) error {
		if _, err := s.q(ctx).Exec(ctx, `DELETE FROM endpoint_paths p USING endpoint_hosts h
 WHERE p.hostname=h.hostname AND h.organization_id=$1 AND h.application_id=$2 AND h.environment_id=$3 AND h.target_generation=$4
 AND p.workload_id=$5 AND (p.hostname,p.path) NOT IN (SELECT * FROM unnest($6::text[],$7::text[]))`,
			ids.organization, ids.application, ids.environment, owner.Generation, owner.Workload, hosts, paths); err != nil {
			return translate(err)
		}
		_, err := s.q(ctx).Exec(ctx, `DELETE FROM endpoint_hosts h WHERE h.organization_id=$1 AND h.application_id=$2 AND h.environment_id=$3
 AND h.target_generation=$4 AND NOT EXISTS (SELECT 1 FROM endpoint_paths p WHERE p.hostname=h.hostname)`,
			ids.organization, ids.application, ids.environment, owner.Generation)
		return translate(err)
	})
}

var _ persistence.EndpointRepository = (*Store)(nil)
