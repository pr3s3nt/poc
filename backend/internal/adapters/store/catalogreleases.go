package store

import (
	"context"
	"encoding/json"
	"orchestrator/internal/domain/catalogrelease"
	"orchestrator/internal/ports/persistence"
	"sort"
)

func copyCatalog[T any](v T) T {
	raw, _ := json.Marshal(v)
	var result T
	_ = json.Unmarshal(raw, &result)
	return result
}
func catalogReleaseKey(org, id string) string { return org + "/" + id }
func (s *Store) ListCatalogSources(ctx context.Context, org string) ([]persistence.CatalogSource, error) {
	defer s.rlock(ctx)()
	out := []persistence.CatalogSource{}
	for _, v := range s.state.CatalogSources {
		if v.Source.Organization == org {
			out = append(out, copyCatalog(v))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Source.ID < out[j].Source.ID })
	return out, nil
}
func (s *Store) GetCatalogSource(ctx context.Context, org, id string) (persistence.CatalogSource, error) {
	defer s.rlock(ctx)()
	v, ok := s.state.CatalogSources[catalogReleaseKey(org, id)]
	if !ok {
		return v, persistence.ErrNotFound
	}
	return copyCatalog(v), nil
}
func (s *Store) SaveCatalogSource(ctx context.Context, expected int64, v persistence.CatalogSource) error {
	defer s.lock(ctx)()
	if v.Source.Validate() != nil {
		return catalogrelease.ErrInvalid
	}
	if _, ok := s.state.Organizations[v.Source.Organization]; !ok {
		return persistence.ErrNotFound
	}
	key := catalogReleaseKey(v.Source.Organization, v.Source.ID)
	if expected < 0 || s.state.CatalogSources[key].Source.Version != expected || v.Source.Version != expected+1 {
		return persistence.ErrVersionConflict
	}
	if s.state.CatalogSources == nil {
		s.state.CatalogSources = map[string]persistence.CatalogSource{}
	}
	s.state.CatalogSources[key] = copyCatalog(v)
	return nil
}
func (s *Store) ListCatalogReleases(ctx context.Context, org, id string) ([]catalogrelease.Release, error) {
	defer s.rlock(ctx)()
	out := []catalogrelease.Release{}
	for _, v := range s.state.CatalogReleases {
		if v.Organization == org && v.SourceID == id {
			out = append(out, copyCatalog(v))
		}
	}
	catalogrelease.SortReleases(out)
	return out, nil
}
func (s *Store) GetCatalogRelease(ctx context.Context, org, id, tag string) (catalogrelease.Release, error) {
	defer s.rlock(ctx)()
	v, ok := s.state.CatalogReleases[catalogReleaseKey(org, id)+"/"+tag]
	if !ok {
		return v, persistence.ErrNotFound
	}
	return copyCatalog(v), nil
}
func (s *Store) InsertCatalogRelease(ctx context.Context, v catalogrelease.Release) error {
	defer s.lock(ctx)()
	if v.Validate() != nil {
		return catalogrelease.ErrIncomplete
	}
	if _, ok := s.state.CatalogSources[catalogReleaseKey(v.Organization, v.SourceID)]; !ok {
		return persistence.ErrNotFound
	}
	key := catalogReleaseKey(v.Organization, v.SourceID) + "/" + v.Tag
	if old, ok := s.state.CatalogReleases[key]; ok {
		if old.Fingerprint() != v.Fingerprint() {
			return catalogrelease.ErrConflict
		}
		return nil
	}
	if s.state.CatalogReleases == nil {
		s.state.CatalogReleases = map[string]catalogrelease.Release{}
	}
	s.state.CatalogReleases[key] = copyCatalog(v)
	return nil
}
func (s *Store) ListCatalogSourceOrganizations(ctx context.Context) ([]string, error) {
	defer s.rlock(ctx)()
	seen := map[string]bool{}
	for _, v := range s.state.CatalogSources {
		seen[v.Source.Organization] = true
	}
	out := []string{}
	for org := range seen {
		out = append(out, org)
	}
	sort.Strings(out)
	return out, nil
}
func (s *Store) GetCatalogEnvironment(ctx context.Context, app, env string) (persistence.CatalogEnvironment, error) {
	defer s.rlock(ctx)()
	key := envKey(app, env)
	if _, ok := s.state.Environments[key]; !ok {
		return persistence.CatalogEnvironment{}, persistence.ErrNotFound
	}
	value, ok := s.state.CatalogEnvironments[key]
	if !ok {
		return persistence.CatalogEnvironment{ApplicationKey: app, EnvironmentKey: env, Status: "LEGACY"}, nil
	}
	return copyCatalog(value), nil
}
func (s *Store) SelectCatalog(ctx context.Context, cmd persistence.CatalogSelectionCommand) (out persistence.CatalogEnvironment, err error) {
	err = s.catalogTransaction(ctx, func(ctx context.Context) error {
		if cmd.Selection.Validate() != nil {
			return catalogrelease.ErrInvalid
		}
		app, ok := s.state.Applications[cmd.ApplicationKey]
		if !ok || app.OrganizationKey != cmd.Organization {
			return persistence.ErrNotFound
		}
		key := envKey(cmd.ApplicationKey, cmd.EnvironmentKey)
		env, ok := s.state.Environments[key]
		if !ok {
			return persistence.ErrNotFound
		}
		if env.Busy() {
			return persistence.ErrEnvironmentBusy
		}
		old := s.state.CatalogEnvironments[key]
		if env.Version != cmd.ExpectedEnvironmentVersion || old.Revision != cmd.ExpectedRevision {
			return persistence.ErrVersionConflict
		}
		release, ok := s.state.CatalogReleases[catalogReleaseKey(cmd.Organization, cmd.Selection.SourceID)+"/"+cmd.Selection.Tag]
		if !ok || release.Fingerprint() != cmd.Selection.Fingerprint {
			return persistence.ErrNotFound
		}
		if _, err := release.Definitions(cmd.Selection); err != nil {
			return err
		}
		if old.Applied != nil && old.Applied.SourceID != cmd.Selection.SourceID {
			return catalogrelease.ErrInvalid
		}
		if old.Applied != nil && old.Applied.SourceID == cmd.Selection.SourceID {
			previous := s.state.CatalogReleases[catalogReleaseKey(cmd.Organization, old.Applied.SourceID)+"/"+old.Applied.Tag]
			if catalogrelease.IsDowngrade(previous.Tag, release.Tag) {
				return catalogrelease.ErrInvalid
			}
		}
		old.ApplicationKey = cmd.ApplicationKey
		old.EnvironmentKey = cmd.EnvironmentKey
		old.Revision++
		old.Desired = &cmd.Selection
		old.Status = "PENDING"
		if s.state.CatalogEnvironments == nil {
			s.state.CatalogEnvironments = map[string]persistence.CatalogEnvironment{}
		}
		s.state.CatalogEnvironments[key] = copyCatalog(old)
		env.Version++
		s.state.Environments[key] = env
		out = copyCatalog(old)
		return nil
	})
	return
}
func (s *Store) ActivateCatalog(ctx context.Context, app, env string, revision int64, status string) error {
	return s.catalogTransaction(ctx, func(ctx context.Context) error {
		if _, ok := persistence.OwnerFrom(ctx); !ok {
			return persistence.ErrOperationLost
		}
		key := envKey(app, env)
		e, ok := s.state.Environments[key]
		if !ok {
			return persistence.ErrNotFound
		}
		if e.ActiveOperationID == "" || e.ActiveOperationID != persistence.OperationFrom(ctx) {
			return persistence.ErrOperationLost
		}
		if err := s.ownerCheck(ctx, e); err != nil {
			return err
		}
		v := s.state.CatalogEnvironments[key]
		if v.Revision != revision || v.Desired == nil {
			return persistence.ErrVersionConflict
		}
		if status != "APPLIED" && status != "PARTIAL" {
			return catalogrelease.ErrInvalid
		}
		v.Status = status
		if status == "APPLIED" {
			v.Applied = copyCatalog(v.Desired)
		}
		s.state.CatalogEnvironments[key] = v
		return nil
	})
}

// Catalog methods can participate in the caller's atomic selection transaction.
func (s *Store) catalogTransaction(ctx context.Context, fn func(context.Context) error) error {
	if s.inTx(ctx) {
		return fn(ctx)
	}
	return s.Transact(ctx, fn)
}
