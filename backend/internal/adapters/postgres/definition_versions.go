package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/persistence"
)

func (s *Store) GetDefinitionHistory(ctx context.Context, org, key string) (persistence.DefinitionHistory, error) {
	h := persistence.DefinitionHistory{DefinitionKey: key, Versions: []resource.DefinitionRevision{}}
	err := s.q(ctx).QueryRow(ctx, `SELECT p.generation,p.active_revision FROM resource_definition_policies p JOIN organizations o ON o.id=p.organization_id WHERE o.organization_key=$1 AND p.definition_key=$2`, org, key).Scan(&h.Generation, &h.ActiveID)
	if errors.Is(err, pgx.ErrNoRows) {
		return h, nil
	}
	if err != nil {
		return h, translate(err)
	}
	rows, err := s.q(ctx).Query(ctx, `SELECT v.revision_id,v.number,v.snapshot,v.created_at,v.archived FROM resource_definition_versions v JOIN organizations o ON o.id=v.organization_id WHERE o.organization_key=$1 AND v.definition_key=$2 ORDER BY v.number`, org, key)
	if err != nil {
		return h, err
	}
	defer rows.Close()
	for rows.Next() {
		var v resource.DefinitionRevision
		var raw []byte
		if err := rows.Scan(&v.ID, &v.Number, &raw, &v.CreatedAt, &v.Archived); err != nil {
			return h, err
		}
		if err := json.Unmarshal(raw, &v.Definition); err != nil {
			return h, err
		}
		h.Versions = append(h.Versions, v)
	}
	return h, rows.Err()
}
func (s *Store) SaveDefinitionHistory(ctx context.Context, org string, expected int64, h persistence.DefinitionHistory) error {
	return s.Transact(ctx, func(ctx context.Context) error {
		var family string
		if err := s.q(ctx).QueryRow(ctx, `SELECT rd.id::text FROM resource_definitions rd JOIN organizations o ON o.id=rd.organization_id WHERE o.organization_key=$1 AND rd.definition_key=$2 FOR UPDATE OF rd`, org, h.DefinitionKey).Scan(&family); err != nil {
			return translate(err)
		}
		old, err := s.GetDefinitionHistory(ctx, org, h.DefinitionKey)
		if err != nil {
			return err
		}
		if old.Generation != expected || h.Generation != expected+1 {
			return persistence.ErrVersionConflict
		}
		if err := persistence.ValidateDefinitionHistory(old, h); err != nil {
			return err
		}
		for _, v := range h.Versions {
			raw, err := json.Marshal(v.Definition)
			if err != nil {
				return err
			}
			_, err = s.q(ctx).Exec(ctx, `INSERT INTO resource_definition_versions(organization_id,definition_key,revision_id,number,snapshot,created_at,archived) SELECT o.id,$2,$3,$4,$5,$6,$7 FROM organizations o WHERE o.organization_key=$1 ON CONFLICT(organization_id,revision_id) DO UPDATE SET archived=EXCLUDED.archived`, org, h.DefinitionKey, v.ID, v.Number, raw, v.CreatedAt, v.Archived)
			if err != nil {
				return translate(err)
			}
		}
		_, err = s.q(ctx).Exec(ctx, `INSERT INTO resource_definition_policies(organization_id,definition_key,generation,active_revision) SELECT o.id,$2,$3,$4 FROM organizations o WHERE o.organization_key=$1 ON CONFLICT(organization_id,definition_key) DO UPDATE SET generation=EXCLUDED.generation,active_revision=EXCLUDED.active_revision`, org, h.DefinitionKey, h.Generation, h.ActiveID)
		return translate(err)
	})
}
func (s *Store) GetDefinitionSelection(ctx context.Context, app, env string) (persistence.DefinitionSelection, error) {
	var raw []byte
	err := s.q(ctx).QueryRow(ctx, `SELECT s.document FROM environment_definition_selections s JOIN environments e ON e.id=s.environment_id JOIN applications a ON a.id=e.application_id WHERE a.application_key=$1 AND e.environment_key=$2`, app, env).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err := s.GetEnvironment(ctx, app, env); err != nil {
			return persistence.DefinitionSelection{}, err
		}
		return persistence.DefinitionSelection{ApplicationKey: app, EnvironmentKey: env}, nil
	}
	return catalogDecode[persistence.DefinitionSelection](raw, err)
}
func (s *Store) FreezeDefinitionSelection(ctx context.Context, v persistence.DefinitionSelection) error {
	v.Frozen = true
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.q(ctx).Exec(ctx, `INSERT INTO environment_definition_selections(environment_id,revision,document) SELECT e.id,$3,$4 FROM environments e JOIN applications a ON a.id=e.application_id WHERE a.application_key=$1 AND e.environment_key=$2 ON CONFLICT(environment_id) DO NOTHING`, v.ApplicationKey, v.EnvironmentKey, v.Revision, raw)
	return translate(err)
}
func (s *Store) SelectDefinitionSelection(ctx context.Context, org string, expectedEnv, expected int64, v persistence.DefinitionSelection) error {
	return s.fenced(ctx, func(ctx context.Context) error {
		env, err := scanEnv(s.q(ctx).QueryRow(ctx, envSelect+` WHERE a.application_key=$1 AND e.environment_key=$2 AND a.organization_id=(SELECT id FROM organizations WHERE organization_key=$3) AND a.status='ACTIVE' FOR UPDATE OF e`, v.ApplicationKey, v.EnvironmentKey, org))
		if err != nil {
			return translate(err)
		}
		if env.Busy() && env.ActiveOperationID != ownerArg(ctx) {
			return persistence.ErrEnvironmentBusy
		}
		if env.Version != expectedEnv {
			return persistence.ErrVersionConflict
		}
		old, err := s.GetDefinitionSelection(ctx, v.ApplicationKey, v.EnvironmentKey)
		if err != nil {
			return err
		}
		if old.Revision != expected || v.Revision != expected+1 {
			return persistence.ErrVersionConflict
		}
		v.Frozen = true
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		if _, err := s.q(ctx).Exec(ctx, `INSERT INTO environment_definition_selections(environment_id,revision,document) VALUES($1,$2,$3) ON CONFLICT(environment_id) DO UPDATE SET revision=EXCLUDED.revision,document=EXCLUDED.document`, env.ID, v.Revision, raw); err != nil {
			return translate(err)
		}
		_, err = s.q(ctx).Exec(ctx, `UPDATE environments SET version=version+1 WHERE id=$1`, env.ID)
		return translate(err)
	})
}
func (s *Store) ApplyDefinitionSelection(ctx context.Context, app, env string, expected int64, used []resource.Definition, pin string) error {
	if _, ok := persistence.OwnerFrom(ctx); !ok {
		return persistence.ErrOperationLost
	}
	return s.fenced(ctx, func(ctx context.Context) error {
		current, err := s.GetEnvironment(ctx, app, env)
		if err != nil {
			return err
		}
		var owner bool
		if err := s.q(ctx).QueryRow(ctx, `SELECT active_operation_id=$2::uuid FROM environments WHERE id=$1`, current.ID, ownerArg(ctx)).Scan(&owner); err != nil {
			return err
		}
		if !owner {
			return persistence.ErrOperationLost
		}
		v, err := s.GetDefinitionSelection(ctx, app, env)
		if err != nil {
			return err
		}
		if v.Revision != expected {
			return persistence.ErrVersionConflict
		}
		if !v.Frozen {
			v.Baseline = used
		}
		v.Frozen = true
		v.Desired = used
		v.Applied = used
		v.AppliedVerified = true
		v.AppliedRevision = expected
		v.CatalogPin = pin
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		_, err = s.q(ctx).Exec(ctx, `INSERT INTO environment_definition_selections(environment_id,revision,document) VALUES($1,$2,$3) ON CONFLICT(environment_id) DO UPDATE SET document=EXCLUDED.document`, current.ID, expected, raw)
		return translate(err)
	})
}
