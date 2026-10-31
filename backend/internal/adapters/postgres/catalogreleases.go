package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"orchestrator/internal/domain/catalogrelease"
	"orchestrator/internal/ports/persistence"
)

func catalogDecode[T any](raw []byte, err error) (T, error) {
	var out T
	if errors.Is(err, pgx.ErrNoRows) {
		return out, persistence.ErrNotFound
	}
	if err != nil {
		return out, fmt.Errorf("postgres: catalog read (%s)", sqlState(err))
	}
	if json.Unmarshal(raw, &out) != nil {
		return out, errors.New("postgres: invalid stored catalog")
	}
	return out, nil
}
func (s *Store) GetCatalogSource(ctx context.Context, org, id string) (persistence.CatalogSource, error) {
	var raw []byte
	err := s.q(ctx).QueryRow(ctx, `SELECT c.document FROM catalog_sources c JOIN organizations o ON o.id=c.organization_id WHERE o.organization_key=$1 AND c.source_key=$2`, org, id).Scan(&raw)
	return catalogDecode[persistence.CatalogSource](raw, err)
}
func (s *Store) ListCatalogSources(ctx context.Context, org string) ([]persistence.CatalogSource, error) {
	rows, err := s.q(ctx).Query(ctx, `SELECT c.document FROM catalog_sources c JOIN organizations o ON o.id=c.organization_id WHERE o.organization_key=$1 ORDER BY c.source_key`, org)
	if err != nil {
		return nil, fmt.Errorf("postgres: catalog list (%s)", sqlState(err))
	}
	defer rows.Close()
	out := []persistence.CatalogSource{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		v, err := catalogDecode[persistence.CatalogSource](raw, nil)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) SaveCatalogSource(ctx context.Context, expected int64, v persistence.CatalogSource) error {
	if v.Source.Validate() != nil {
		return catalogrelease.ErrInvalid
	}
	if expected < 0 || v.Source.Version != expected+1 {
		return persistence.ErrVersionConflict
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tag, err := s.q(ctx).Exec(ctx, `INSERT INTO catalog_sources(organization_id,source_key,version,document)
 SELECT id,$2,$3,$4 FROM organizations WHERE organization_key=$1 AND ($5::bigint=0 OR EXISTS(SELECT 1 FROM catalog_sources WHERE organization_id=organizations.id AND source_key=$2))
 ON CONFLICT(organization_id,source_key) DO UPDATE SET version=EXCLUDED.version,document=EXCLUDED.document WHERE catalog_sources.version=$5`, v.Source.Organization, v.Source.ID, v.Source.Version, raw, expected)
	if err != nil {
		return fmt.Errorf("postgres: catalog save (%s)", sqlState(err))
	}
	if tag.RowsAffected() != 1 {
		return persistence.ErrVersionConflict
	}
	return nil
}
func (s *Store) GetCatalogRelease(ctx context.Context, org, id, tag string) (catalogrelease.Release, error) {
	var raw []byte
	err := s.q(ctx).QueryRow(ctx, `SELECT c.document FROM catalog_releases c JOIN organizations o ON o.id=c.organization_id WHERE o.organization_key=$1 AND c.source_key=$2 AND c.tag=$3`, org, id, tag).Scan(&raw)
	return catalogDecode[catalogrelease.Release](raw, err)
}
func (s *Store) ListCatalogReleases(ctx context.Context, org, id string) ([]catalogrelease.Release, error) {
	rows, err := s.q(ctx).Query(ctx, `SELECT c.document FROM catalog_releases c JOIN organizations o ON o.id=c.organization_id WHERE o.organization_key=$1 AND c.source_key=$2 ORDER BY c.tag`, org, id)
	if err != nil {
		return nil, fmt.Errorf("postgres: catalog list (%s)", sqlState(err))
	}
	defer rows.Close()
	out := []catalogrelease.Release{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		v, err := catalogDecode[catalogrelease.Release](raw, nil)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	catalogrelease.SortReleases(out)
	return out, rows.Err()
}
func (s *Store) InsertCatalogRelease(ctx context.Context, v catalogrelease.Release) error {
	if v.Validate() != nil {
		return catalogrelease.ErrIncomplete
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tag, err := s.q(ctx).Exec(ctx, `INSERT INTO catalog_releases(organization_id,source_key,tag,fingerprint,document) SELECT id,$2,$3,$4,$5 FROM organizations WHERE organization_key=$1 ON CONFLICT DO NOTHING`, v.Organization, v.SourceID, v.Tag, v.Fingerprint(), raw)
	if err != nil {
		return fmt.Errorf("postgres: catalog publish (%s)", sqlState(err))
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	old, err := s.GetCatalogRelease(ctx, v.Organization, v.SourceID, v.Tag)
	if err != nil {
		return err
	}
	if old.Fingerprint() != v.Fingerprint() {
		return catalogrelease.ErrConflict
	}
	return nil
}
func (s *Store) ListCatalogSourceOrganizations(ctx context.Context) ([]string, error) {
	rows, err := s.q(ctx).Query(ctx, `SELECT DISTINCT o.organization_key FROM catalog_sources c JOIN organizations o ON o.id=c.organization_id ORDER BY o.organization_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var org string
		if err := rows.Scan(&org); err != nil {
			return nil, err
		}
		out = append(out, org)
	}
	return out, rows.Err()
}
func (s *Store) GetCatalogEnvironment(ctx context.Context, app, env string) (persistence.CatalogEnvironment, error) {
	var raw []byte
	err := s.q(ctx).QueryRow(ctx, `SELECT b.document FROM environment_catalog_bindings b JOIN environments e ON e.id=b.environment_id JOIN applications a ON a.id=e.application_id WHERE a.application_key=$1 AND e.environment_key=$2`, app, env).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err := s.GetEnvironment(ctx, app, env); err != nil {
			return persistence.CatalogEnvironment{}, err
		}
		return persistence.CatalogEnvironment{ApplicationKey: app, EnvironmentKey: env, Status: "LEGACY"}, nil
	}
	return catalogDecode[persistence.CatalogEnvironment](raw, err)
}
func (s *Store) SelectCatalog(ctx context.Context, cmd persistence.CatalogSelectionCommand) (out persistence.CatalogEnvironment, err error) {
	err = s.Transact(ctx, func(ctx context.Context) error {
		if cmd.Selection.Validate() != nil {
			return catalogrelease.ErrInvalid
		}
		env, err := scanEnv(s.q(ctx).QueryRow(ctx, envSelect+` WHERE a.application_key=$1 AND e.environment_key=$2 AND a.organization_id=(SELECT id FROM organizations WHERE organization_key=$3) FOR UPDATE OF e`, cmd.ApplicationKey, cmd.EnvironmentKey, cmd.Organization))
		if err != nil {
			return translate(err)
		}
		if env.Busy() {
			return persistence.ErrEnvironmentBusy
		}
		if env.Version != cmd.ExpectedEnvironmentVersion {
			return persistence.ErrVersionConflict
		}
		old, err := s.GetCatalogEnvironment(ctx, cmd.ApplicationKey, cmd.EnvironmentKey)
		if err != nil {
			return err
		}
		if old.Revision != cmd.ExpectedRevision {
			return persistence.ErrVersionConflict
		}
		release, err := s.GetCatalogRelease(ctx, cmd.Organization, cmd.Selection.SourceID, cmd.Selection.Tag)
		if err != nil {
			return err
		}
		if _, err = release.Definitions(cmd.Selection); err != nil {
			return err
		}
		if old.Applied != nil && old.Applied.SourceID != cmd.Selection.SourceID {
			return catalogrelease.ErrInvalid
		}
		if old.Applied != nil && old.Applied.SourceID == cmd.Selection.SourceID && catalogrelease.IsDowngrade(old.Applied.Tag, cmd.Selection.Tag) {
			return catalogrelease.ErrInvalid
		}
		old.Revision++
		old.Desired = &cmd.Selection
		old.Status = "PENDING"
		raw, err := json.Marshal(old)
		if err != nil {
			return err
		}
		var appliedSource, appliedTag any
		if old.Applied != nil {
			appliedSource = old.Applied.SourceID
			appliedTag = old.Applied.Tag
		}
		_, err = s.q(ctx).Exec(ctx, `INSERT INTO environment_catalog_bindings(environment_id,organization_id,desired_source,desired_tag,applied_source,applied_tag,revision,document) SELECT $1,o.id,$3,$4,$5,$6,$7,$8 FROM organizations o WHERE o.organization_key=$2 ON CONFLICT(environment_id) DO UPDATE SET desired_source=EXCLUDED.desired_source,desired_tag=EXCLUDED.desired_tag,revision=EXCLUDED.revision,document=EXCLUDED.document`, env.ID, cmd.Organization, cmd.Selection.SourceID, cmd.Selection.Tag, appliedSource, appliedTag, old.Revision, raw)
		if err != nil {
			return translate(err)
		}
		_, err = s.q(ctx).Exec(ctx, `UPDATE environments SET version=version+1 WHERE id=$1`, env.ID)
		out = old
		return translate(err)
	})
	return
}
func (s *Store) ActivateCatalog(ctx context.Context, app, env string, revision int64, status string) error {
	if _, ok := persistence.OwnerFrom(ctx); !ok {
		return persistence.ErrOperationLost
	}
	if status != "APPLIED" && status != "PARTIAL" {
		return catalogrelease.ErrInvalid
	}
	return s.fenced(ctx, func(ctx context.Context) error {
		v, err := s.GetCatalogEnvironment(ctx, app, env)
		if err != nil {
			return err
		}
		if v.Revision != revision || v.Desired == nil {
			return persistence.ErrVersionConflict
		}
		v.Status = status
		if status == "APPLIED" {
			v.Applied = v.Desired
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		var appliedSource, appliedTag any
		if v.Applied != nil {
			appliedSource = v.Applied.SourceID
			appliedTag = v.Applied.Tag
		}
		tag, err := s.q(ctx).Exec(ctx, `UPDATE environment_catalog_bindings b SET document=$3,applied_source=$4,applied_tag=$5 FROM environments e JOIN applications a ON a.id=e.application_id WHERE b.environment_id=e.id AND a.application_key=$1 AND e.environment_key=$2 AND e.active_operation_id=$6 AND b.revision=$7`, app, env, raw, appliedSource, appliedTag, ownerArg(ctx), revision)
		if err != nil {
			return translate(err)
		}
		if tag.RowsAffected() != 1 {
			return persistence.ErrVersionConflict
		}
		return nil
	})
}
