package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/effectiveness"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/ports/persistence"
)

func TestEffectivenessBaselinesSurviveSnapshotReload(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveOrganization(ctx, application.Organization{ID: "o1", Key: "acme", Name: "Acme"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveUserAccount(ctx, identity.UserAccount{ID: "u1", OrganizationKey: "acme", Username: "pe", PasswordHash: "x", Role: identity.RolePlatformEngineer, Status: identity.AccountActive}); err != nil {
		t.Fatal(err)
	}
	b := effectiveness.Baseline{OrganizationKey: "acme", Metric: effectiveness.MetricWorkDuration, Statistic: effectiveness.StatisticMedian, ValueSeconds: 600,
		Source: "Đo tay", EffectiveFrom: "2026-10-01", CreatedBy: "u1", CreatedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	if v, err := s.AppendEffectivenessBaseline(ctx, b, 0); err != nil || v.Version != 1 || v.ID == "" {
		t.Fatalf("append = %+v, %v", v, err)
	}
	if _, err := s.AppendEffectivenessBaseline(ctx, b, 0); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("stale version = %v", err)
	}
	b.ApplicationKey = "missing"
	if _, err := s.AppendEffectivenessBaseline(ctx, b, 0); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("missing application = %v", err)
	}
	reloaded, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := reloaded.ListEffectivenessBaselines(ctx, "acme", effectiveness.MetricWorkDuration, "")
	if err != nil || len(versions) != 1 || versions[0].ValueSeconds != 600 || versions[0].EffectiveFrom != "2026-10-01" {
		t.Fatalf("after reload = %+v, %v", versions, err)
	}
}
