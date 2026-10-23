package effectiveness

import (
	"context"
	"errors"

	domain "orchestrator/internal/domain/effectiveness"
	"orchestrator/internal/ports/persistence"
)

// BaselineVersion is one stored version with its author's username.
type BaselineVersion struct {
	domain.Baseline
	Author string
}

// BaselineList is the history of one scope, newest first.
type BaselineList struct {
	CurrentVersion int64
	Versions       []BaselineVersion
}

// AddBaseline is the ALT-01 command. ApplicationKey empty = Organization.
type AddBaseline struct {
	OrganizationKey string
	AuthorID        string
	ExpectedVersion int64
	Baseline        domain.Baseline
}

func (s *Service) repository() (persistence.EffectivenessBaselineRepository, error) {
	repo, ok := s.store.(persistence.EffectivenessBaselineRepository)
	if !ok {
		return nil, ErrBaselinesUnavailable
	}
	return repo, nil
}

func (s *Service) checkApplication(ctx context.Context, organizationKey, applicationKey string) error {
	if applicationKey == "" {
		return nil
	}
	app, err := s.store.GetApplication(ctx, applicationKey)
	if err != nil {
		return err
	}
	if app.OrganizationKey != organizationKey {
		return persistence.ErrNotFound
	}
	return nil
}

// Baselines lists the work-duration baseline versions of a scope.
func (s *Service) Baselines(ctx context.Context, organizationKey, applicationKey string) (BaselineList, error) {
	repo, err := s.repository()
	if err != nil {
		return BaselineList{}, err
	}
	if err := s.checkApplication(ctx, organizationKey, applicationKey); err != nil {
		return BaselineList{}, err
	}
	versions, err := repo.ListEffectivenessBaselines(ctx, organizationKey, domain.MetricWorkDuration, applicationKey)
	if err != nil {
		return BaselineList{}, err
	}
	authors := newActorResolver(s.store, organizationKey)
	out := BaselineList{CurrentVersion: domain.CurrentVersion(versions), Versions: make([]BaselineVersion, 0, len(versions))}
	for _, v := range versions {
		out.Versions = append(out.Versions, BaselineVersion{Baseline: v, Author: authors.username(ctx, v.CreatedBy)})
	}
	return out, nil
}

// AddBaselineVersion appends one version; older versions never change.
func (s *Service) AddBaselineVersion(ctx context.Context, cmd AddBaseline) (BaselineVersion, error) {
	repo, err := s.repository()
	if err != nil {
		return BaselineVersion{}, err
	}
	b := cmd.Baseline
	b.OrganizationKey, b.CreatedBy, b.CreatedAt = cmd.OrganizationKey, cmd.AuthorID, s.now().UTC()
	if !b.Retired {
		b.Statistic = domain.StatisticMedian
	}
	if err := b.Validate(); err != nil {
		var fe *domain.FieldError
		if errors.As(err, &fe) {
			return BaselineVersion{}, &FieldError{Field: fe.Field}
		}
		return BaselineVersion{}, err
	}
	if cmd.ExpectedVersion < 0 {
		return BaselineVersion{}, &FieldError{Field: "expectedVersion"}
	}
	if err := s.checkApplication(ctx, cmd.OrganizationKey, b.ApplicationKey); err != nil {
		return BaselineVersion{}, err
	}
	stored, err := repo.AppendEffectivenessBaseline(ctx, b, cmd.ExpectedVersion)
	if err != nil {
		return BaselineVersion{}, err
	}
	return BaselineVersion{Baseline: stored, Author: newActorResolver(s.store, cmd.OrganizationKey).username(ctx, stored.CreatedBy)}, nil
}
