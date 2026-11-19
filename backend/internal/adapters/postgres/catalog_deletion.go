package postgres

import (
	"context"
)

// DeleteCatalogEntry serializes catalog reference writes with the check and
// delete. Existing FK constraints also guard the persisted Definition and
// active resource identities.
func (s *Store) DeleteCatalogEntry(ctx context.Context, org, key string, definition bool, check func(context.Context) error) error {
	table, column := "resource_types", "resource_type_key"
	if definition {
		table, column = "resource_definitions", "definition_key"
	}
	return s.Transact(ctx, func(ctx context.Context) error {
		if _, err := s.q(ctx).Exec(ctx, `LOCK TABLE applications, environments, resource_types,
			resource_definitions, active_resources, environment_definition_selections,
			workload_drafts, deployment_sets IN SHARE ROW EXCLUSIVE MODE`); err != nil {
			return translate(err)
		}
		var id string
		err := s.q(ctx).QueryRow(ctx, "SELECT entry.id::text FROM "+table+" entry JOIN organizations o ON o.id=entry.organization_id WHERE o.organization_key=$1 AND entry."+column+"=$2", org, key).Scan(&id)
		if err != nil {
			return translate(err)
		}
		if err := check(ctx); err != nil {
			return err
		}
		if _, err = s.q(ctx).Exec(ctx, "DELETE FROM "+table+" WHERE id=$1::uuid", id); err != nil {
			return translate(err)
		}
		// Invalidate previews and writes that validated against the old catalog
		// before this transaction took the Environment lock.
		_, err = s.q(ctx).Exec(ctx, `UPDATE environments e SET version=e.version+1, draft_version=e.draft_version+1
			FROM applications a, organizations o WHERE a.id=e.application_id AND o.id=a.organization_id
			AND o.organization_key=$1 AND a.status='ACTIVE'`, org)
		return translate(err)
	})
}
