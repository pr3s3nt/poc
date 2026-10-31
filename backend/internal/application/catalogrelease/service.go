// Package catalogrelease coordinates complete-set publication, never deployment.
package catalogrelease

import (
	"context"
	"errors"
	"time"

	domain "orchestrator/internal/domain/catalogrelease"
	"orchestrator/internal/ports/catalogsource"
	"orchestrator/internal/ports/persistence"
)

type Service struct {
	Store     persistence.Store
	Discovery catalogsource.Discovery
	Inspector catalogsource.Inspector
}

// Save preserves server-owned observations. Once observed, source identity is immutable.
func (s *Service) Save(ctx context.Context, expected int64, source domain.Source) (persistence.CatalogSource, error) {
	if source.Validate() != nil || expected < 0 || source.Version != expected+1 {
		return persistence.CatalogSource{}, domain.ErrInvalid
	}
	value := persistence.CatalogSource{Source: source, ObservedTags: map[string]string{}}
	err := s.Store.Transact(ctx, func(ctx context.Context) error {
		old, err := s.Store.GetCatalogSource(ctx, source.Organization, source.ID)
		if err != nil && !(expected == 0 && errors.Is(err, persistence.ErrNotFound)) {
			return err
		}
		if old.Source.Version != expected {
			return persistence.ErrVersionConflict
		}
		if len(old.ObservedTags) > 0 && (old.Source.Repository != source.Repository || old.Source.Branch != source.Branch || old.Source.ManifestPath != source.ManifestPath) {
			return domain.ErrConflict
		}
		value.ObservedTags = old.ObservedTags
		value.LastChecked = old.LastChecked
		value.Code = old.Code
		return s.Store.SaveCatalogSource(ctx, expected, value)
	})
	return value, err
}

// Refresh performs remote I/O outside the transaction. The final CAS ensures
// a concurrent source edit/refresh cannot publish against a different config.
func (s *Service) Refresh(ctx context.Context, org, id string) (persistence.CatalogSource, error) {
	value, err := s.Store.GetCatalogSource(ctx, org, id)
	if err != nil {
		return value, err
	}
	if s.Discovery == nil || s.Inspector == nil {
		return value, domain.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	observation, fetchErr := s.Discovery.Discover(ctx, value.Source)
	value.LastChecked = time.Now().UTC()
	value.Code = "READY"
	releases := []domain.Release{}
	if fetchErr != nil {
		value.Code = "SOURCE_UNAVAILABLE"
	} else {
		if !domain.ValidCommit(observation.BranchTip) || observation.ObservedAt.IsZero() {
			return value, domain.ErrUnavailable
		}
		value.LastChecked = observation.ObservedAt
		if value.ObservedTags == nil {
			value.ObservedTags = map[string]string{}
		}
		for _, tag := range observation.Tags {
			if tag.Name == "" || !domain.ValidCommit(tag.Commit) {
				return value, domain.ErrInvalid
			}
			if old, ok := value.ObservedTags[tag.Name]; ok && old != tag.Commit {
				value.Code = "TAG_CONFLICT"
				continue
			}
			value.ObservedTags[tag.Name] = tag.Commit
			if !tag.InBranch {
				if value.Code == "READY" {
					value.Code = "TAG_OUTSIDE_BRANCH"
				}
				continue
			}
			existing, err := s.Store.GetCatalogRelease(ctx, org, id, tag.Name)
			if err == nil {
				if existing.Commit != tag.Commit {
					value.Code = "TAG_CONFLICT"
				}
				continue
			}
			if !errors.Is(err, persistence.ErrNotFound) {
				return value, err
			}
			release, err := s.Inspector.InspectRelease(ctx, value.Source, tag, observation)
			if err != nil || release.Validate() != nil || release.Organization != org || release.SourceID != id || release.Tag != tag.Name || release.Commit != tag.Commit || release.BranchTip != observation.BranchTip {
				if value.Code == "READY" {
					value.Code = "RELEASE_INVALID"
				}
				continue
			}
			releases = append(releases, release)
		}
	}
	expected := value.Source.Version
	value.Source.Version++
	err = s.Store.Transact(ctx, func(ctx context.Context) error {
		if err := s.Store.SaveCatalogSource(ctx, expected, value); err != nil {
			return err
		}
		for _, r := range releases {
			if err := s.Store.InsertCatalogRelease(ctx, r); err != nil {
				return err
			}
		}
		return nil
	})
	return value, err
}

// Run periodically discovers all configured sources, serially and with bounded
// operations. Callers supply organizations from authenticated platform storage.
func (s *Service) Run(ctx context.Context, organizations func(context.Context) ([]string, error)) {
	timer := time.NewTicker(15 * time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			orgs, err := organizations(ctx)
			if err != nil {
				continue
			}
			for _, org := range orgs {
				sources, err := s.Store.ListCatalogSources(ctx, org)
				if err != nil {
					continue
				}
				for _, source := range sources {
					if ctx.Err() != nil {
						return
					}
					_, _ = s.Refresh(ctx, org, source.Source.ID)
				}
			}
		}
	}
}
