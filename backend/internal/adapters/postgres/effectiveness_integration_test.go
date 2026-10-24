package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	effectivenessapp "orchestrator/internal/application/effectiveness"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/effectiveness"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

func baselineVersion(value int64, from effectiveness.Date) effectiveness.Baseline {
	size := 8
	start, end := effectiveness.Date("2026-09-01"), effectiveness.Date("2026-09-30")
	return effectiveness.Baseline{OrganizationKey: "acme", Metric: effectiveness.MetricWorkDuration, Statistic: effectiveness.StatisticMedian,
		ValueSeconds: value, Source: "Đo tay 8 yêu cầu", SampleSize: &size, MeasuredFrom: &start, MeasuredTo: &end, EffectiveFrom: from,
		CreatedBy: seed.FixedPlatformEngineerAccountID, CreatedAt: time.Now().UTC()}
}

// UC-17 baselines are append-only versions that survive a restart; the
// Console login cannot rewrite them, and the report reads the persisted
// history of the Organization (Deployments and cleanup operations).
func TestEffectivenessBaselinesPersistAndStayAppendOnly(t *testing.T) {
	db := freshDatabase(t)
	ctx := context.Background()
	first, err := Open(ctx, db.Console)
	if err != nil {
		t.Fatal(err)
	}
	opts := seed.Defaults()
	if err := seedtest.Apply(ctx, first, opts); err != nil {
		t.Fatal(err)
	}
	v1, err := first.AppendEffectivenessBaseline(ctx, baselineVersion(3600, "2026-10-01"), 0)
	if err != nil || v1.Version != 1 || v1.SampleSize == nil || *v1.SampleSize != 8 || *v1.MeasuredTo != "2026-09-30" || v1.EffectiveFrom != "2026-10-01" {
		t.Fatalf("first version = %+v, %v", v1, err)
	}
	if _, err := first.AppendEffectivenessBaseline(ctx, baselineVersion(1800, "2026-10-05"), 0); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("stale expected version = %v", err)
	}
	retired := effectiveness.Baseline{OrganizationKey: "acme", Metric: effectiveness.MetricWorkDuration, Retired: true, Source: "Ngừng áp dụng",
		EffectiveFrom: "2026-11-01", CreatedBy: seed.FixedPlatformEngineerAccountID, CreatedAt: time.Now().UTC()}
	if v2, err := first.AppendEffectivenessBaseline(ctx, retired, 1); err != nil || v2.Version != 2 || !v2.Retired || v2.ValueSeconds != 0 {
		t.Fatalf("retired version = %+v, %v", v2, err)
	}
	scoped := baselineVersion(900, "2026-10-01")
	scoped.ApplicationKey = opts.ApplicationKey
	if v, err := first.AppendEffectivenessBaseline(ctx, scoped, 0); err != nil || v.Version != 1 || v.ApplicationKey != opts.ApplicationKey {
		t.Fatalf("application scope has its own versions: %+v, %v", v, err)
	}
	foreign := baselineVersion(900, "2026-10-01")
	foreign.ApplicationKey = "missing"
	if _, err := first.AppendEffectivenessBaseline(ctx, foreign, 0); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("unknown application = %v", err)
	}
	env, err := first.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC().Add(-time.Hour)
	record := deployment.Deployment{ID: ids.New(), EnvironmentID: env.ID, OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey,
		Action: deployment.ActionDeploy, WorkloadID: "api", ActorRef: seed.FixedPlatformEngineerAccountID, Status: deployment.StatusPlanning, BaseEnvironmentVersion: env.Version, StartedAt: started}
	if err := first.SaveDeployment(ctx, record); err != nil {
		t.Fatal(err)
	}
	record.Status, record.FailureReason = deployment.StatusFailed, "planning rejected"
	done := started.Add(time.Minute)
	record.FinishedAt = &done
	if err := first.SaveDeployment(ctx, record); err != nil {
		t.Fatal(err)
	}
	first.Close()

	console, err := pgx.Connect(ctx, db.Console)
	if err != nil {
		t.Fatal(err)
	}
	defer console.Close(ctx)
	for _, sql := range []string{`UPDATE effectiveness_baselines SET value_seconds=1`, `DELETE FROM effectiveness_baselines`} {
		_, err := console.Exec(ctx, sql)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Fatalf("%s as Console = %v, want insufficient privilege", sql, err)
		}
	}

	second, err := Open(ctx, db.Console)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	versions, err := second.ListEffectivenessBaselines(ctx, "acme", effectiveness.MetricWorkDuration, "")
	if err != nil || len(versions) != 2 || versions[0].Version != 2 || versions[1].ValueSeconds != 3600 || versions[1].Source != "Đo tay 8 yêu cầu" {
		t.Fatalf("versions after restart = %+v, %v", versions, err)
	}
	if runs, err := second.ListCleanupRuns(ctx, "acme"); err != nil || len(runs) != 0 {
		t.Fatalf("cleanup history = %+v, %v", runs, err)
	}
	svc := effectivenessapp.New(second, effectivenessapp.OriginOf(opts.Profile))
	today := effectiveness.DateOf(time.Now())
	report, err := svc.Report(ctx, effectivenessapp.Query{OrganizationKey: "acme", From: string(today.AddDays(-6)), To: string(today)})
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range report.Sources {
		if !src.Available {
			t.Fatalf("PostgreSQL provides every source: %+v", report.Sources)
		}
	}
	var attempts *effectiveness.Result
	for i := range report.Metrics {
		if report.Metrics[i].ID == "execution_attempts" {
			attempts = &report.Metrics[i]
		}
	}
	if attempts == nil || *attempts.Value != 1 || attempts.Details["failedRuns"] != 1 {
		t.Fatalf("persisted run not reported: %+v", attempts)
	}
}

// Cleanup operations store the Console username as actor_ref. Resolving it
// must not abort the report's read transaction: a non-UUID reference is an
// unknown User Account ID, then looked up by username.
func TestEffectivenessReportResolvesUsernameActors(t *testing.T) {
	db := freshDatabase(t)
	ctx := context.Background()
	store, err := Open(ctx, db.Console)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	opts := seed.Defaults()
	if err := seedtest.Apply(ctx, store, opts); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetUserAccount(ctx, "platform-engineer"); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("non-UUID account ID = %v, want not found", err)
	}
	env, err := store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC().Add(-time.Hour)
	record := deployment.Deployment{ID: ids.New(), EnvironmentID: env.ID, OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey,
		Action: deployment.ActionDeploy, WorkloadID: "api", ActorRef: "platform-engineer", Status: deployment.StatusPlanning, BaseEnvironmentVersion: env.Version, StartedAt: started}
	if err := store.SaveDeployment(ctx, record); err != nil {
		t.Fatal(err)
	}
	record.Status, record.FailureReason = deployment.StatusFailed, "planning rejected"
	done := started.Add(time.Minute)
	record.FinishedAt = &done
	if err := store.SaveDeployment(ctx, record); err != nil {
		t.Fatal(err)
	}
	svc := effectivenessapp.New(store, effectivenessapp.OriginOf(opts.Profile))
	today := effectiveness.DateOf(time.Now())
	report, err := svc.Report(ctx, effectivenessapp.Query{OrganizationKey: "acme", From: string(today.AddDays(-6)), To: string(today)})
	if err != nil {
		t.Fatalf("report with a username actor: %v", err)
	}
	for _, m := range report.Metrics {
		if m.ID == "active_users" && (m.Value == nil || *m.Value != 1) {
			t.Fatalf("username actor not counted as a person: %+v", m)
		}
	}
}
