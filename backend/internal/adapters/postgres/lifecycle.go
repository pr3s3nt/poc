package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/configuration"
	"orchestrator/internal/ports/persistence"
)

var _ persistence.ApplicationLifecycleRepository = (*Store)(nil)

func deletionJSON(d *application.Deletion) ([]byte, error) {
	if d == nil {
		return []byte(`{}`), nil
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return nil, fmt.Errorf("postgres: encode application deletion: %w", err)
	}
	return raw, nil
}

func decodeDeletion(raw []byte, v *application.Application) error {
	if len(raw) == 0 || string(raw) == "{}" {
		return nil
	}
	var d application.Deletion
	if err := json.Unmarshal(raw, &d); err != nil {
		return fmt.Errorf("postgres: decode application deletion: %w", err)
	}
	v.Deletion = &d
	return nil
}

// TransitionApplication is the UC-18 lifecycle compare-and-set.
func (s *Store) TransitionApplication(ctx context.Context, key string, expected int64, status application.LifecycleStatus, deletion *application.Deletion) (application.Application, error) {
	switch status {
	case application.StatusActive, application.StatusDeleting, application.StatusDeleted:
	default:
		return application.Application{}, fmt.Errorf("postgres: invalid application status %q", status)
	}
	raw, err := deletionJSON(deletion)
	if err != nil {
		return application.Application{}, err
	}
	var id string
	err = s.q(ctx).QueryRow(ctx, `UPDATE applications SET status=$3,deletion=$4::jsonb,version=version+1 WHERE application_key=$1 AND version=$2 RETURNING application_key`, key, expected, string(status), raw).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, gerr := s.GetApplication(ctx, key); gerr != nil {
			return application.Application{}, gerr
		}
		return application.Application{}, persistence.ErrVersionConflict
	}
	if err != nil {
		return application.Application{}, fmt.Errorf("postgres: transition application: %w", translate(err))
	}
	return s.GetApplication(ctx, key)
}

// ListConfigurationRevisions returns every revision of one Environment.
func (s *Store) ListConfigurationRevisions(ctx context.Context, app, env string) ([]configuration.Revision, error) {
	rows, err := s.q(ctx).Query(ctx, `SELECT cr.id::text FROM configuration_revisions cr JOIN environments e ON e.id=cr.environment_id JOIN applications a ON a.id=e.application_id WHERE a.application_key=$1 AND e.environment_key=$2 ORDER BY cr.version`, app, env)
	if err != nil {
		return nil, err
	}
	var revisionIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		revisionIDs = append(revisionIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]configuration.Revision, 0, len(revisionIDs))
	for _, id := range revisionIDs {
		revision, err := s.GetConfigurationRevision(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, revision)
	}
	return out, nil
}
