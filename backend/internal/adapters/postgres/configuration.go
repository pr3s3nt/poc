package postgres

import (
	"context"
	"fmt"

	"orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/persistence"
)

func (s *Store) GetConfigurationScope(ctx context.Context, app, env string) (configuration.Scope, error) {
	var v configuration.Scope
	v.ApplicationKey = app
	v.EnvironmentKey = env
	err := s.q(ctx).QueryRow(ctx, `SELECT COALESCE(e.desired_config_revision_id::text,''),COALESCE(cr.version,0) FROM environments e JOIN applications a ON a.id=e.application_id LEFT JOIN configuration_revisions cr ON cr.id=e.desired_config_revision_id WHERE a.application_key=$1 AND e.environment_key=$2`, app, env).Scan(&v.DesiredRevisionID, &v.Version)
	if err != nil {
		return v, translate(err)
	}
	return v, nil
}
func (s *Store) GetConfigurationRevision(ctx context.Context, id string) (configuration.Revision, error) {
	var v configuration.Revision
	v.Entries = map[string]configuration.Entry{}
	err := s.q(ctx).QueryRow(ctx, `SELECT cr.id::text,a.application_key,e.environment_key,cr.version FROM configuration_revisions cr JOIN environments e ON e.id=cr.environment_id JOIN applications a ON a.id=e.application_id WHERE cr.id=$1::uuid`, id).Scan(&v.ID, &v.ApplicationKey, &v.EnvironmentKey, &v.Version)
	if err != nil {
		return v, translate(err)
	}
	rows, err := s.q(ctx).Query(ctx, `SELECT key_name,kind,COALESCE(value_ref,''),store_key,COALESCE(variable_value,'') FROM configuration_revision_entries WHERE revision_id=$1::uuid`, id)
	if err != nil {
		return v, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var e configuration.Entry
		if err = rows.Scan(&name, &e.Kind, &e.ValueRef, &e.StoreKey, &e.Value); err != nil {
			return v, err
		}
		v.Entries[name] = e
	}
	return v, rows.Err()
}
func (s *Store) CommitConfigurationRevision(ctx context.Context, expected int64, v configuration.Revision) error {
	return s.fenced(ctx, func(ctx context.Context) error { return s.fencedCommitConfigurationRevision(ctx, expected, v) })
}

func (s *Store) fencedCommitConfigurationRevision(ctx context.Context, expected int64, v configuration.Revision) error {
	if err := v.Validate(); err != nil {
		return err
	}
	return s.Transact(ctx, func(ctx context.Context) error {
		var envID, active string
		var current int64
		err := s.q(ctx).QueryRow(ctx, `SELECT e.id::text,COALESCE(cr.version,0),COALESCE(e.active_operation_id::text,'') FROM environments e JOIN applications a ON a.id=e.application_id LEFT JOIN configuration_revisions cr ON cr.id=e.desired_config_revision_id WHERE a.application_key=$1 AND e.environment_key=$2 FOR UPDATE OF e`, v.ApplicationKey, v.EnvironmentKey).Scan(&envID, &current, &active)
		if err != nil {
			return translate(err)
		}
		if active != "" && active != ownerArg(ctx) {
			return persistence.Busy(v.ApplicationKey, v.EnvironmentKey)
		}
		if current != expected || v.Version != expected+1 {
			return persistence.ErrVersionConflict
		}
		if _, err = s.q(ctx).Exec(ctx, `INSERT INTO configuration_revisions(id,environment_id,version) VALUES($1::uuid,$2::uuid,$3)`, v.ID, envID, v.Version); err != nil {
			return translate(err)
		}
		for name, e := range v.Entries {
			if _, err = s.q(ctx).Exec(ctx, `INSERT INTO configuration_revision_entries(revision_id,key_name,kind,value_ref,store_key,variable_value) VALUES($1::uuid,$2,$3,NULLIF($4,''),$5,NULLIF($6,''))`, v.ID, name, e.Kind, e.ValueRef, e.StoreKey, e.Value); err != nil {
				return translate(err)
			}
		}
		_, err = s.q(ctx).Exec(ctx, `UPDATE environments SET desired_config_revision_id=$1::uuid WHERE id=$2::uuid`, v.ID, envID)
		return translate(err)
	})
}

func (s *Store) ListWorkloadDrafts(ctx context.Context, app, env string) ([]environment.WorkloadDraft, error) {
	rows, err := s.q(ctx).Query(ctx, `SELECT w.workload_id,w.state,w.score_document FROM workload_drafts w JOIN environments e ON e.id=w.environment_id JOIN applications a ON a.id=e.application_id WHERE a.application_key=$1 AND e.environment_key=$2 ORDER BY w.workload_id`, app, env)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []environment.WorkloadDraft
	for rows.Next() {
		v := environment.WorkloadDraft{ApplicationKey: app, EnvironmentKey: env}
		var b []byte
		if err = rows.Scan(&v.WorkloadID, &v.State, &b); err != nil {
			return nil, err
		}
		if len(b) > 0 {
			if err = unmarshalJSON(b, &v.Score); err != nil {
				return nil, err
			}
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) GetWorkloadDraft(ctx context.Context, app, env, workload string) (environment.WorkloadDraft, error) {
	v := environment.WorkloadDraft{ApplicationKey: app, EnvironmentKey: env, WorkloadID: workload}
	var b []byte
	err := s.q(ctx).QueryRow(ctx, `SELECT w.state,w.score_document FROM workload_drafts w JOIN environments e ON e.id=w.environment_id JOIN applications a ON a.id=e.application_id WHERE a.application_key=$1 AND e.environment_key=$2 AND w.workload_id=$3`, app, env, workload).Scan(&v.State, &b)
	if err != nil {
		return v, translate(err)
	}
	if len(b) > 0 {
		err = unmarshalJSON(b, &v.Score)
	}
	return v, err
}
func (s *Store) SaveWorkloadDraft(ctx context.Context, expected int64, v environment.WorkloadDraft) error {
	return s.fenced(ctx, func(ctx context.Context) error { return s.fencedSaveWorkloadDraft(ctx, expected, v) })
}

func (s *Store) fencedSaveWorkloadDraft(ctx context.Context, expected int64, v environment.WorkloadDraft) error {
	if v.WorkloadID == "" || (v.State != environment.DraftUpsert && v.State != environment.DraftDelete) || (v.State == environment.DraftUpsert && v.Score == nil) {
		return fmt.Errorf("postgres: invalid workload draft")
	}
	var score any
	if v.Score != nil {
		b, err := jsonBytes(v.Score)
		if err != nil {
			return err
		}
		score = b
	}
	tag, err := s.q(ctx).Exec(ctx, `WITH bumped AS (UPDATE environments e SET draft_version=draft_version+1 FROM applications a WHERE e.application_id=a.id AND a.application_key=$1 AND e.environment_key=$2 AND e.draft_version=$3 AND (e.active_operation_id IS NULL OR e.active_operation_id=NULLIF($7,'')::uuid) RETURNING e.id) INSERT INTO workload_drafts(environment_id,workload_id,score_document,state) SELECT id,$4,$5,$6 FROM bumped ON CONFLICT(environment_id,workload_id) DO UPDATE SET score_document=EXCLUDED.score_document,state=EXCLUDED.state,updated_at=now()`, v.ApplicationKey, v.EnvironmentKey, expected, v.WorkloadID, score, v.State, ownerArg(ctx))
	if err != nil {
		return translate(err)
	}
	if tag.RowsAffected() == 0 {
		return s.classifyEnvWrite(ctx, v.ApplicationKey, v.EnvironmentKey, 0, false, persistence.ErrVersionConflict)
	}
	return nil
}
func (s *Store) DeleteWorkloadDraft(ctx context.Context, app, env, workload string, expected int64) error {
	return s.fenced(ctx, func(ctx context.Context) error { return s.fencedDeleteWorkloadDraft(ctx, app, env, workload, expected) })
}

func (s *Store) fencedDeleteWorkloadDraft(ctx context.Context, app, env, workload string, expected int64) error {
	return s.Transact(ctx, func(ctx context.Context) error {
		tag, err := s.q(ctx).Exec(ctx, `UPDATE environments e SET draft_version=draft_version+1 FROM applications a WHERE e.application_id=a.id AND a.application_key=$1 AND e.environment_key=$2 AND e.draft_version=$3 AND (e.active_operation_id IS NULL OR e.active_operation_id=NULLIF($4,'')::uuid)`, app, env, expected, ownerArg(ctx))
		if err != nil {
			return translate(err)
		}
		if tag.RowsAffected() == 0 {
			return s.classifyEnvWrite(ctx, app, env, 0, false, persistence.ErrVersionConflict)
		}
		tag, err = s.q(ctx).Exec(ctx, `DELETE FROM workload_drafts w USING environments e,applications a WHERE w.environment_id=e.id AND e.application_id=a.id AND a.application_key=$1 AND e.environment_key=$2 AND w.workload_id=$3`, app, env, workload)
		if err != nil {
			return translate(err)
		}
		if tag.RowsAffected() == 0 {
			return persistence.ErrNotFound
		}
		return nil
	})
}
