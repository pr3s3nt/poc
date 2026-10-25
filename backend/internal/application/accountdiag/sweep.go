package accountdiag

import (
	"context"
	"errors"
	"time"

	"orchestrator/internal/domain/application"
	domain "orchestrator/internal/domain/diagnostics"
	"orchestrator/internal/platform/ids"
	probes "orchestrator/internal/ports/diagnostics"
	"orchestrator/internal/ports/persistence"
)

// Run sweeps until ctx ends: expired leases become INTERRUPTED (findings kept,
// in-flight cleanup UNKNOWN) and unresolved run-owned resources of finished or
// interrupted execution runs are re-checked. A live lease of another instance
// is never touched.
func (s *Service) Run(ctx context.Context) {
	s.Sweep(ctx)
	ticker := time.NewTicker(s.cfg.SweepEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Sweep(ctx)
		}
	}
}

// Sweep performs one reconciliation pass.
func (s *Service) Sweep(ctx context.Context) {
	if _, err := s.repo.ExpireLeases(ctx, s.cfg.Now().UTC()); err != nil {
		return
	}
	for i := 0; i < 8 && ctx.Err() == nil; i++ {
		now := s.cfg.Now().UTC()
		owner := s.cfg.Instance + "/recovery/" + ids.New()
		report, ok, err := s.repo.ClaimCleanup(ctx, owner, now.Add(s.cfg.CleanupTimeout+s.cfg.LeaseTTL), now)
		if err != nil || !ok {
			return
		}
		s.recover(ctx, report)
	}
}

// recover re-checks the recorded resources of one report against the pinned
// account target and records only what is observed.
func (s *Service) recover(ctx context.Context, report domain.Report) {
	resources := append([]domain.ProbeResource(nil), report.ProbeResources...)
	status := domain.CleanupUnknown
	acct, err := s.accounts.GetAccount(ctx, report.OrganizationKey, report.AccountKey)
	provider := s.providers[report.Scope.AccountType]
	if err == nil && provider != nil && acct.Kind != "" {
		resources = s.recoverResources(ctx, acct, provider, report, resources)
		if domain.AllSettled(resources) {
			status = domain.CleanupVerified
		}
	} else if err != nil && !errors.Is(err, persistence.ErrNotFound) {
		// A transient read error keeps the unresolved state for the next sweep.
		resources = report.ProbeResources
	}
	update := domain.CleanupUpdate{
		OrganizationKey: report.OrganizationKey, AccountKey: report.AccountKey, ID: report.ID, Owner: report.Owner, ExpectedRevision: report.Revision,
		ProbeResources: resources, CleanupStatus: status, Now: s.cfg.Now().UTC(), Release: true,
	}
	wctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, _ = s.repo.UpdateCleanup(wctx, update)
}

func (s *Service) recoverResources(ctx context.Context, acct application.Connection, provider probes.Provider, report domain.Report, resources []domain.ProbeResource) []domain.ProbeResource {
	cctx, cancel := context.WithTimeout(ctx, s.cfg.CleanupTimeout)
	defer cancel()
	session := provider.Open(cctx, acct, report.Scope)
	defer session.Close()
	return mergeResources(resources, session.Cleanup(cctx, resources, report.ID))
}
