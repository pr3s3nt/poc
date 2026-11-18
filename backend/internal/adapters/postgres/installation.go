package postgres

import (
	"context"
	"orchestrator/internal/domain/application"
)

func (s *Store) ListOrganizations(ctx context.Context) ([]application.Organization, error) {
	rows, err := s.q(ctx).Query(ctx, `SELECT id::text,organization_key,name FROM organizations ORDER BY organization_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []application.Organization{}
	for rows.Next() {
		var o application.Organization
		if err := rows.Scan(&o.ID, &o.Key, &o.Name); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
func (s *Store) LockInstallationSetup(ctx context.Context) error {
	_, err := s.q(ctx).Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(current_database() || ':' || current_schema() || ':installation-setup',0))`)
	return err
}
