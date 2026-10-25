package persistencetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/diagnostics"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/platform/ids"
	diagport "orchestrator/internal/ports/diagnostics"
	"orchestrator/internal/ports/persistence"
)

// DiagnosticsStore is a persistence.Store that also holds diagnostic reports.
type DiagnosticsStore interface {
	persistence.Store
	diagport.Repository
}

// DiagnosticsFixture names what Diagnostics created, for reopen checks.
type DiagnosticsFixture struct {
	Organization, Account  string
	Completed, Interrupted diagnostics.Report
	AccountVersion         int64
}

func diagReport(org, account, user string, version int64, at time.Time) diagnostics.Report {
	return diagnostics.Report{
		ID: ids.New(), OrganizationKey: org, AccountKey: account, CreatedBy: user, AccountVersion: version,
		Purpose: diagnostics.PurposeGeneral, Mode: diagnostics.ModeConditions,
		Scope:  diagnostics.Scope{Purpose: diagnostics.PurposeGeneral, Mode: diagnostics.ModeConditions, AccountKey: account, AccountName: "Cluster", AccountType: "INTERNAL", AccountVersion: version, Identity: map[string]string{"cluster": "c"}},
		Status: diagnostics.StatusQueued, Revision: 1, Owner: "owner-" + ids.New(), LeaseExpiresAt: ptr(at.Add(time.Minute)),
		TotalChecks: 2, CleanupStatus: diagnostics.CleanupNotRequired, CreatedAt: at, UpdatedAt: at,
	}
}

func ptr[T any](v T) *T { return &v }

func finding(id string, v diagnostics.Verdict, at time.Time) diagnostics.Finding {
	return diagnostics.Finding{ID: id, Code: id, Group: "cluster", Verdict: v, Reason: "OBSERVED", Facts: map[string]string{"nodes": "3"}, ObservedAt: at}
}

func runUpdate(r diagnostics.Report, status diagnostics.Status, findings []diagnostics.Finding, total int, resources []diagnostics.ProbeResource, cleanup diagnostics.CleanupStatus, lease, now time.Time) diagnostics.RunUpdate {
	return diagnostics.RunUpdate{OrganizationKey: r.OrganizationKey, AccountKey: r.AccountKey, ID: r.ID, Owner: r.Owner, ExpectedRevision: r.Revision, Status: status,
		Findings: findings, CompletedChecks: len(findings), TotalChecks: total, ProbeResources: resources, CleanupStatus: cleanup, LeaseExpiresAt: lease, Now: now}
}

// Diagnostics checks the report repository contract: Organization/account
// isolation, one active run per account, owner+lease+revision guarded worker
// writes, cancellation, lease expiry, immutability of recorded history,
// deep-copy isolation, strict facts, paging and independence from the
// Connection version (ADR-022).
func Diagnostics(t *testing.T, st DiagnosticsStore) DiagnosticsFixture {
	t.Helper()
	ctx := context.Background()
	base := time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)
	users := map[string]string{}
	for _, org := range []string{"diag-acme", "diag-globex"} {
		if err := st.SaveOrganization(ctx, application.Organization{Key: org, Name: org}); err != nil {
			t.Fatal(err)
		}
		users[org] = ids.New()
		if err := st.SaveUserAccount(ctx, identity.UserAccount{ID: users[org], OrganizationKey: org, Username: "u-" + org, PasswordHash: "x", Role: identity.RolePlatformEngineer, Status: identity.AccountActive}); err != nil {
			t.Fatal(err)
		}
	}
	for _, key := range []string{"k8s-a", "k8s-b"} {
		if err := st.SaveConnection(ctx, application.Connection{ID: ids.New(), Key: key, Name: key, OrganizationKey: "diag-acme", Kind: application.ConnectionKubernetes,
			AuthenticationType: application.AuthKubeconfig, Config: map[string]any{"cluster": "c"}, SecretRef: "ref", Status: application.ConnectionReady, Verification: map[string]any{"verified": true}, Version: 4}); err != nil {
			t.Fatal(err)
		}
	}
	before, err := st.GetConnection(ctx, "diag-acme", "k8s-a")
	if err != nil {
		t.Fatal(err)
	}

	first := diagReport("diag-acme", "k8s-a", users["diag-acme"], 4, base)
	stored, err := st.CreateReport(ctx, first)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if stored.AccountID != before.ID || stored.Status != diagnostics.StatusQueued {
		t.Fatalf("stored report %+v", stored)
	}
	// Invalid shapes are refused at the boundary.
	for name, mutate := range map[string]func(*diagnostics.Report){
		"scope version":  func(r *diagnostics.Report) { r.Scope.AccountVersion = 99 },
		"scope account":  func(r *diagnostics.Report) { r.Scope.AccountKey = "other" },
		"general detail": func(r *diagnostics.Report) { r.Scope.UseCases = []string{"deploy"} },
		"bad identity":   func(r *diagnostics.Report) { r.Scope.Identity = map[string]string{"kubeconfig": "apiVersion: v1"} },
		"fact key": func(r *diagnostics.Report) {
			r.Findings = []diagnostics.Finding{finding("k8s.api", diagnostics.VerdictPass, base)}
			r.Findings[0].Facts = map[string]string{"stderr": "x"}
			r.CompletedChecks = 1
		},
		"fact value": func(r *diagnostics.Report) {
			r.Findings = []diagnostics.Finding{finding("k8s.api", diagnostics.VerdictPass, base)}
			r.Findings[0].Facts = map[string]string{"nodes": "3 token=abc"}
			r.CompletedChecks = 1
		},
		"progress": func(r *diagnostics.Report) { r.CompletedChecks = 1 },
	} {
		bad := diagReport("diag-acme", "k8s-b", users["diag-acme"], 4, base)
		mutate(&bad)
		if _, err := st.CreateReport(ctx, bad); !errors.Is(err, diagnostics.ErrInvalidReport) && !errors.Is(err, diagnostics.ErrInvalidScope) {
			t.Fatalf("%s accepted: %v", name, err)
		}
	}
	if _, err := st.CreateReport(ctx, diagReport("diag-acme", "k8s-a", users["diag-acme"], 4, base.Add(time.Second))); !errors.Is(err, diagnostics.ErrBusy) {
		t.Fatalf("second active run: %v", err)
	}
	if _, err := st.CreateReport(ctx, diagReport("diag-acme", "k8s-b", users["diag-acme"], 4, base.Add(time.Second))); err != nil {
		t.Fatalf("another account may run: %v", err)
	}
	if _, err := st.CreateReport(ctx, diagReport("diag-globex", "k8s-a", users["diag-globex"], 4, base)); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("foreign organization account: %v", err)
	}
	if _, err := st.CreateReport(ctx, diagReport("diag-acme", "k8s-a", users["diag-globex"], 4, base)); !errors.Is(err, persistence.ErrNotFound) && !errors.Is(err, diagnostics.ErrBusy) {
		t.Fatalf("foreign author: %v", err)
	}
	if _, err := st.GetReport(ctx, "diag-globex", "k8s-a", first.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("foreign organization read: %v", err)
	}
	if _, err := st.GetReport(ctx, "diag-acme", "k8s-b", first.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("wrong account read: %v", err)
	}

	// Worker lifecycle: owner, live lease and revision guards.
	lease := base.Add(time.Minute)
	if _, err := st.ClaimRun(ctx, "diag-acme", "k8s-a", first.ID, "someone-else", 1, lease, base); !errors.Is(err, diagnostics.ErrLeaseLost) {
		t.Fatalf("claim by another owner: %v", err)
	}
	if _, err := st.ClaimRun(ctx, "diag-acme", "k8s-a", first.ID, first.Owner, 7, lease, base); !errors.Is(err, diagnostics.ErrRevisionConflict) {
		t.Fatalf("claim at a stale revision: %v", err)
	}
	running, err := st.ClaimRun(ctx, "diag-acme", "k8s-a", first.ID, first.Owner, 1, lease, base.Add(time.Second))
	if err != nil || running.Status != diagnostics.StatusRunning || running.StartedAt == nil || running.Revision != 2 {
		t.Fatalf("claim: %+v %v", running, err)
	}
	one := []diagnostics.Finding{finding("k8s.api", diagnostics.VerdictPass, base)}
	progress, err := st.UpdateRun(ctx, runUpdate(running, diagnostics.StatusRunning, one, 2, nil, diagnostics.CleanupNotRequired, lease.Add(time.Minute), base.Add(2*time.Second)))
	if err != nil || progress.CompletedChecks != 1 || len(progress.Findings) != 1 || progress.Revision != 3 {
		t.Fatalf("progress: %+v %v", progress, err)
	}
	stale := runUpdate(progress, diagnostics.StatusRunning, one, 2, nil, diagnostics.CleanupNotRequired, lease.Add(time.Minute), base.Add(2*time.Second))
	stale.Owner = "stale-worker"
	if _, err := st.UpdateRun(ctx, stale); !errors.Is(err, diagnostics.ErrLeaseLost) {
		t.Fatalf("stale worker write: %v", err)
	}
	sameOwnerStale := runUpdate(running, diagnostics.StatusRunning, one, 2, nil, diagnostics.CleanupNotRequired, lease.Add(time.Minute), base.Add(2*time.Second))
	if _, err := st.UpdateRun(ctx, sameOwnerStale); !errors.Is(err, diagnostics.ErrRevisionConflict) {
		t.Fatalf("same-owner write at an old revision: %v", err)
	}
	regress := runUpdate(progress, diagnostics.StatusRunning, nil, 2, nil, diagnostics.CleanupNotRequired, lease.Add(time.Minute), base.Add(2*time.Second))
	if _, err := st.UpdateRun(ctx, regress); !errors.Is(err, diagnostics.ErrInvalidReport) {
		t.Fatalf("findings were removed: %v", err)
	}
	rewrite := runUpdate(progress, diagnostics.StatusRunning, []diagnostics.Finding{finding("k8s.api", diagnostics.VerdictFail, base)}, 2, nil, diagnostics.CleanupNotRequired, lease.Add(time.Minute), base.Add(2*time.Second))
	if _, err := st.UpdateRun(ctx, rewrite); !errors.Is(err, diagnostics.ErrInvalidReport) {
		t.Fatalf("a recorded finding was rewritten: %v", err)
	}
	badFact := finding("k8s.nodes", diagnostics.VerdictPass, base)
	badFact.Facts = map[string]string{"rawError": "x509: certificate signed by unknown authority"}
	if _, err := st.UpdateRun(ctx, runUpdate(progress, diagnostics.StatusRunning, append(append([]diagnostics.Finding{}, one...), badFact), 2, nil, diagnostics.CleanupNotRequired, lease.Add(time.Minute), base.Add(2*time.Second))); !errors.Is(err, diagnostics.ErrInvalidReport) {
		t.Fatalf("unregistered fact accepted: %v", err)
	}
	dangling := finding("k8s.nodes", diagnostics.VerdictBlocked, base)
	dangling.DependsOn = []string{"never-ran"}
	if _, err := st.UpdateRun(ctx, runUpdate(progress, diagnostics.StatusRunning, append(append([]diagnostics.Finding{}, one...), dangling), 2, nil, diagnostics.CleanupNotRequired, lease.Add(time.Minute), base.Add(2*time.Second))); !errors.Is(err, diagnostics.ErrInvalidReport) {
		t.Fatalf("dangling dependency accepted: %v", err)
	}
	if _, err := st.UpdateRun(ctx, runUpdate(progress, diagnostics.StatusRunning, append(append([]diagnostics.Finding{}, one...), one[0]), 2, nil, diagnostics.CleanupNotRequired, lease.Add(time.Minute), base.Add(2*time.Second))); !errors.Is(err, diagnostics.ErrInvalidReport) {
		t.Fatalf("duplicate finding accepted: %v", err)
	}
	if _, err := st.UpdateRun(ctx, runUpdate(progress, diagnostics.StatusRunning, one, 3, nil, diagnostics.CleanupNotRequired, lease.Add(time.Minute), base.Add(2*time.Second))); !errors.Is(err, diagnostics.ErrInvalidReport) {
		t.Fatalf("the planned check count changed: %v", err)
	}
	if _, err := st.UpdateRun(ctx, runUpdate(progress, diagnostics.StatusCompleted, one, 2, nil, diagnostics.CleanupNotRequired, lease.Add(time.Minute), base.Add(2*time.Second))); !errors.Is(err, diagnostics.ErrInvalidReport) {
		t.Fatalf("completed with pending checks: %v", err)
	}

	// Returned values never alias persisted history.
	read, err := st.GetReport(ctx, "diag-acme", "k8s-a", first.ID)
	if err != nil {
		t.Fatal(err)
	}
	read.Findings[0].Facts["nodes"] = "999"
	read.Findings[0].Verdict = diagnostics.VerdictFail
	read.Scope.Identity["cluster"] = "tampered"
	*read.LeaseExpiresAt = base.Add(100 * time.Hour)
	read.Scope.AccountName = "tampered"
	again0, err := st.GetReport(ctx, "diag-acme", "k8s-a", first.ID)
	if err != nil || again0.Findings[0].Facts["nodes"] != "3" || again0.Findings[0].Verdict != diagnostics.VerdictPass || again0.Scope.Identity["cluster"] != "c" ||
		!again0.LeaseExpiresAt.Equal(lease.Add(time.Minute)) || again0.Scope.AccountName != "Cluster" {
		t.Fatalf("persisted report was changed through a returned value: %+v", again0)
	}
	one[0].Facts["nodes"] = "777"
	if got, _ := st.GetReport(ctx, "diag-acme", "k8s-a", first.ID); got.Findings[0].Facts["nodes"] != "3" {
		t.Fatal("persisted report aliases the caller's input")
	}
	one[0].Facts["nodes"] = "3"

	cancelling, err := st.RequestCancel(ctx, "diag-acme", "k8s-a", first.ID, base.Add(3*time.Second))
	if err != nil || cancelling.Status != diagnostics.StatusCancelling || cancelling.Revision != 4 {
		t.Fatalf("cancel: %+v %v", cancelling, err)
	}
	again, err := st.RequestCancel(ctx, "diag-acme", "k8s-a", first.ID, base.Add(4*time.Second))
	if err != nil || again.Status != diagnostics.StatusCancelling || again.Revision != 4 {
		t.Fatalf("idempotent cancel: %+v %v", again, err)
	}
	if _, err := st.UpdateRun(ctx, runUpdate(progress, diagnostics.StatusRunning, one, 2, nil, diagnostics.CleanupNotRequired, lease.Add(time.Minute), base.Add(5*time.Second))); !errors.Is(err, diagnostics.ErrRevisionConflict) {
		t.Fatalf("a write that missed the cancel request: %v", err)
	}
	still, err := st.UpdateRun(ctx, runUpdate(cancelling, diagnostics.StatusRunning, one, 2, nil, diagnostics.CleanupNotRequired, lease.Add(time.Minute), base.Add(5*time.Second)))
	if err != nil || still.Status != diagnostics.StatusCancelling {
		t.Fatalf("running must not overwrite a cancel request: %+v %v", still, err)
	}
	done, err := st.UpdateRun(ctx, runUpdate(still, diagnostics.StatusCancelled, one, 2, nil, diagnostics.CleanupNotRequired, lease.Add(time.Minute), base.Add(6*time.Second)))
	if err != nil || done.Status != diagnostics.StatusCancelled || done.FinishedAt == nil || done.Owner != "" || done.LeaseExpiresAt != nil {
		t.Fatalf("cancelled: %+v %v", done, err)
	}
	if _, err := st.UpdateRun(ctx, runUpdate(still, diagnostics.StatusCancelled, one, 2, nil, diagnostics.CleanupNotRequired, lease.Add(time.Minute), base.Add(7*time.Second))); !errors.Is(err, diagnostics.ErrLeaseLost) {
		t.Fatalf("terminal report accepted a worker write: %v", err)
	}
	if got, err := st.RequestCancel(ctx, "diag-acme", "k8s-a", first.ID, base.Add(7*time.Second)); err != nil || got.Status != diagnostics.StatusCancelled || got.Revision != done.Revision {
		t.Fatalf("cancel of terminal report: %+v %v", got, err)
	}

	// A cancelled QUEUED run never starts.
	queued, err := st.CreateReport(ctx, diagReport("diag-acme", "k8s-a", users["diag-acme"], 4, base.Add(10*time.Second)))
	if err != nil {
		t.Fatalf("new run after the previous ended: %v", err)
	}
	cancelled, err := st.RequestCancel(ctx, "diag-acme", "k8s-a", queued.ID, base.Add(11*time.Second))
	if err != nil || cancelled.Status != diagnostics.StatusCancelled {
		t.Fatalf("cancel queued: %+v %v", cancelled, err)
	}
	if _, err := st.ClaimRun(ctx, "diag-acme", "k8s-a", queued.ID, queued.Owner, 1, lease, base.Add(12*time.Second)); !errors.Is(err, diagnostics.ErrLeaseLost) {
		t.Fatalf("claim of cancelled run: %v", err)
	}

	// An expired lease refuses writes before any sweep ran.
	lapsed := diagReport("diag-acme", "k8s-a", users["diag-acme"], 4, base.Add(15*time.Second))
	lapsed.LeaseExpiresAt = ptr(base.Add(16 * time.Second))
	if _, err := st.CreateReport(ctx, lapsed); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ClaimRun(ctx, "diag-acme", "k8s-a", lapsed.ID, lapsed.Owner, 1, base.Add(time.Hour), base.Add(17*time.Second)); !errors.Is(err, diagnostics.ErrLeaseLost) {
		t.Fatalf("claim after the lease ended: %v", err)
	}
	lapsedRunning, err := st.ClaimRun(ctx, "diag-acme", "k8s-a", lapsed.ID, lapsed.Owner, 1, base.Add(18*time.Second), base.Add(15*time.Second+500*time.Millisecond))
	if err != nil {
		t.Fatalf("claim inside the lease: %v", err)
	}
	if _, err := st.UpdateRun(ctx, runUpdate(lapsedRunning, diagnostics.StatusRunning, one, 2, nil, diagnostics.CleanupNotRequired, base.Add(time.Hour), base.Add(19*time.Second))); !errors.Is(err, diagnostics.ErrLeaseLost) {
		t.Fatalf("a worker with an expired lease wrote before the sweep: %v", err)
	}
	if n, err := st.ExpireLeases(ctx, base.Add(19*time.Second)); err != nil || n != 1 {
		t.Fatalf("sweep: %d %v", n, err)
	}

	// Completed run, then an interrupted run with unresolved probe resources.
	completedStart := diagReport("diag-acme", "k8s-a", users["diag-acme"], 4, base.Add(20*time.Second))
	completedStart.LeaseExpiresAt = ptr(base.Add(time.Hour))
	if _, err := st.CreateReport(ctx, completedStart); err != nil {
		t.Fatal(err)
	}
	cRun, err := st.ClaimRun(ctx, "diag-acme", "k8s-a", completedStart.ID, completedStart.Owner, 1, base.Add(time.Hour), base.Add(21*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	two := []diagnostics.Finding{finding("k8s.api", diagnostics.VerdictPass, base), finding("k8s.nodes", diagnostics.VerdictFail, base)}
	two[1].DependsOn = []string{"k8s.api"}
	completed, err := st.UpdateRun(ctx, runUpdate(cRun, diagnostics.StatusCompleted, two, 2, nil, diagnostics.CleanupNotRequired, base.Add(time.Hour), base.Add(22*time.Second)))
	if err != nil || completed.Status != diagnostics.StatusCompleted || completed.Ready() {
		t.Fatalf("complete: %+v %v", completed, err)
	}

	execReport := diagReport("diag-acme", "k8s-a", users["diag-acme"], 4, base.Add(30*time.Second))
	execReport.Purpose, execReport.Mode = diagnostics.PurposeUseCase, diagnostics.ModeExecution
	execReport.Scope.Purpose, execReport.Scope.Mode, execReport.Scope.UseCases = diagnostics.PurposeUseCase, diagnostics.ModeExecution, []string{"deploy"}
	execReport.LeaseExpiresAt, execReport.TotalChecks = ptr(base.Add(31*time.Second)), 3
	if _, err := st.CreateReport(ctx, execReport); err != nil {
		t.Fatal(err)
	}
	eRun, err := st.ClaimRun(ctx, "diag-acme", "k8s-a", execReport.ID, execReport.Owner, 1, base.Add(31*time.Second), base.Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	resources := []diagnostics.ProbeResource{{Kind: "namespace", Name: "diag-probe-abc", UID: "11111111-2222-3333-4444-555555555555", State: diagnostics.ResourceCreated}}
	ePart, err := st.UpdateRun(ctx, runUpdate(eRun, diagnostics.StatusRunning, []diagnostics.Finding{finding("k8s.exec.namespace", diagnostics.VerdictPass, base)}, 3, resources, diagnostics.CleanupPending, base.Add(31*time.Second), base.Add(30*time.Second+time.Millisecond)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateRun(ctx, runUpdate(ePart, diagnostics.StatusRunning, ePart.Findings, 3, nil, diagnostics.CleanupPending, base.Add(31*time.Second), base.Add(30*time.Second+2*time.Millisecond))); !errors.Is(err, diagnostics.ErrInvalidReport) {
		t.Fatalf("a recorded probe resource was dropped: %v", err)
	}
	if _, err := st.UpdateRun(ctx, runUpdate(ePart, diagnostics.StatusRunning, ePart.Findings, 3, resources, diagnostics.CleanupVerified, base.Add(31*time.Second), base.Add(30*time.Second+2*time.Millisecond))); !errors.Is(err, diagnostics.ErrInvalidReport) {
		t.Fatalf("cleanup verified with a live resource: %v", err)
	}
	if n, err := st.ExpireLeases(ctx, base.Add(20*time.Second)); err != nil || n != 0 {
		t.Fatalf("live lease expired: %d %v", n, err)
	}
	if n, err := st.ExpireLeases(ctx, base.Add(time.Minute)); err != nil || n != 1 {
		t.Fatalf("expire: %d %v", n, err)
	}
	interrupted, err := st.GetReport(ctx, "diag-acme", "k8s-a", execReport.ID)
	if err != nil || interrupted.Status != diagnostics.StatusInterrupted || interrupted.CleanupStatus != diagnostics.CleanupUnknown || len(interrupted.Findings) != 1 || interrupted.Owner != "" {
		t.Fatalf("interrupted: %+v %v", interrupted, err)
	}
	if _, err := st.UpdateRun(ctx, runUpdate(ePart, diagnostics.StatusCompleted, ePart.Findings, 3, resources, diagnostics.CleanupVerified, base.Add(time.Hour), base.Add(time.Minute))); !errors.Is(err, diagnostics.ErrLeaseLost) {
		t.Fatalf("stale worker rewrote an interrupted report: %v", err)
	}

	// Cleanup recovery claim is exclusive, revision guarded and never touches findings.
	claimed, ok, err := st.ClaimCleanup(ctx, "recovery-1", base.Add(2*time.Minute), base.Add(time.Minute))
	if err != nil || !ok || claimed.ID != execReport.ID || claimed.Owner != "recovery-1" {
		t.Fatalf("claim cleanup: %+v %v %v", claimed, ok, err)
	}
	if _, ok, err := st.ClaimCleanup(ctx, "recovery-2", base.Add(2*time.Minute), base.Add(time.Minute+time.Second)); err != nil || ok {
		t.Fatalf("a live recovery lease was taken: %v %v", ok, err)
	}
	cu := diagnostics.CleanupUpdate{OrganizationKey: "diag-acme", AccountKey: "k8s-a", ID: execReport.ID, Owner: "recovery-2", ExpectedRevision: claimed.Revision, CleanupStatus: diagnostics.CleanupVerified, ProbeResources: resources, Now: base.Add(time.Minute + time.Second)}
	if _, err := st.UpdateCleanup(ctx, cu); !errors.Is(err, diagnostics.ErrLeaseLost) {
		t.Fatalf("cleanup by a non-owner: %v", err)
	}
	cu.Owner = "recovery-1"
	if _, err := st.UpdateCleanup(ctx, cu); !errors.Is(err, diagnostics.ErrInvalidReport) {
		t.Fatalf("cleanup verified while the namespace still exists: %v", err)
	}
	cu.ExpectedRevision = 1
	if _, err := st.UpdateCleanup(ctx, cu); !errors.Is(err, diagnostics.ErrRevisionConflict) {
		t.Fatalf("cleanup at a stale revision: %v", err)
	}
	cu.ExpectedRevision = claimed.Revision
	cu.Now = base.Add(3 * time.Minute)
	if _, err := st.UpdateCleanup(ctx, cu); !errors.Is(err, diagnostics.ErrLeaseLost) {
		t.Fatalf("cleanup after the recovery lease ended: %v", err)
	}
	resolved := []diagnostics.ProbeResource{{Kind: "namespace", Name: "diag-probe-abc", UID: "11111111-2222-3333-4444-555555555555", State: diagnostics.ResourceDeleted}}
	cu.Now, cu.ProbeResources, cu.Release = base.Add(70*time.Second), resolved, true
	after, err := st.UpdateCleanup(ctx, cu)
	if err != nil || after.CleanupStatus != diagnostics.CleanupVerified || len(after.Findings) != 1 || after.Status != diagnostics.StatusInterrupted || after.Owner != "" {
		t.Fatalf("cleanup update: %+v %v", after, err)
	}
	if _, ok, _ := st.ClaimCleanup(ctx, "recovery-3", base.Add(5*time.Minute), base.Add(3*time.Minute)); ok {
		t.Fatal("a verified cleanup was claimed again")
	}

	// Paging, filtering and latest summary.
	page, err := st.ListReports(ctx, "diag-acme", "k8s-a", diagport.ListQuery{Limit: 2})
	if err != nil || len(page.Reports) != 2 || page.NextCursor == "" || page.Reports[0].ID != execReport.ID {
		t.Fatalf("page 1: %+v %v", page, err)
	}
	next, err := st.ListReports(ctx, "diag-acme", "k8s-a", diagport.ListQuery{Limit: 2, Cursor: page.NextCursor})
	if err != nil || len(next.Reports) != 2 || next.Reports[0].ID == page.Reports[0].ID {
		t.Fatalf("page 2: %+v %v", next, err)
	}
	if _, err := st.ListReports(ctx, "diag-acme", "k8s-a", diagport.ListQuery{Cursor: "not-a-cursor"}); err == nil {
		t.Fatal("malformed cursor accepted")
	}
	general, err := st.ListReports(ctx, "diag-acme", "k8s-a", diagport.ListQuery{Purpose: diagnostics.PurposeGeneral, Limit: 100})
	if err != nil || len(general.Reports) != 4 {
		t.Fatalf("purpose filter: %d %v", len(general.Reports), err)
	}
	if _, err := st.ListReports(ctx, "diag-globex", "k8s-a", diagport.ListQuery{}); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("foreign list: %v", err)
	}
	latest, err := st.LatestReports(ctx, "diag-acme")
	if err != nil || latest["k8s-a"].ID != execReport.ID || latest["k8s-b"].Status != diagnostics.StatusQueued {
		t.Fatalf("latest: %+v %v", latest, err)
	}
	if foreign, err := st.LatestReports(ctx, "diag-globex"); err != nil || len(foreign) != 0 {
		t.Fatalf("foreign latest: %+v %v", foreign, err)
	}

	// Diagnostics never touch the Connection record.
	afterConn, err := st.GetConnection(ctx, "diag-acme", "k8s-a")
	if err != nil || afterConn.Version != before.Version || afterConn.Status != before.Status || len(afterConn.Verification) != len(before.Verification) {
		t.Fatalf("connection changed by diagnostics: %+v %v", afterConn, err)
	}
	return DiagnosticsFixture{Organization: "diag-acme", Account: "k8s-a", Completed: completed, Interrupted: after, AccountVersion: before.Version}
}

// AssertDiagnostics checks that a reopened store still holds the fixture.
func AssertDiagnostics(t *testing.T, st DiagnosticsStore, f DiagnosticsFixture) {
	t.Helper()
	ctx := context.Background()
	got, err := st.GetReport(ctx, f.Organization, f.Account, f.Completed.ID)
	if err != nil || got.Status != diagnostics.StatusCompleted || len(got.Findings) != 2 || got.Findings[1].Verdict != diagnostics.VerdictFail || got.Scope.AccountVersion != f.AccountVersion {
		t.Fatalf("completed report after reopen: %+v %v", got, err)
	}
	interrupted, err := st.GetReport(ctx, f.Organization, f.Account, f.Interrupted.ID)
	if err != nil || interrupted.Status != diagnostics.StatusInterrupted || interrupted.CleanupStatus != diagnostics.CleanupVerified || len(interrupted.ProbeResources) != 1 {
		t.Fatalf("interrupted report after reopen: %+v %v", interrupted, err)
	}
}
