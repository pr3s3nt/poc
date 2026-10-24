package accountdiag

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"orchestrator/internal/domain/application"
	domain "orchestrator/internal/domain/diagnostics"
	probes "orchestrator/internal/ports/diagnostics"
)

// plan lists the provider's checks followed by the platform's own use-case
// checks, each with the selected use cases it supports.
func (s *Service) plan(provider probes.Provider, scope domain.Scope, need *probes.Requirements) []probes.Check {
	checks := append([]probes.Check(nil), provider.Plan(scope, need)...)
	checks = append(checks, platformChecks(scope, checks)...)
	known := map[string]bool{}
	for i := range checks {
		known[checks[i].ID] = true
	}
	for i := range checks {
		var deps []string
		for _, d := range checks[i].DependsOn {
			if known[d] {
				deps = append(deps, d)
			}
		}
		checks[i].DependsOn = deps
	}
	return checks
}

// worker is the state of one run owned by this process.
type worker struct {
	svc      *Service
	scope    domain.Scope
	acct     application.Connection
	provider probes.Provider
	plan     []probes.Check
	need     *probes.Requirements

	mu        sync.Mutex // serializes persistence and the fields below
	report    domain.Report
	findings  []domain.Finding
	resources []domain.ProbeResource
	byID      map[string]domain.Finding
	lost      bool
	cancelled bool // a cancellation request was observed
	failures  int  // consecutive progress write failures
	stopRun   context.CancelFunc
	handle    *runHandle
}

func (s *Service) execute(ctx context.Context, h *runHandle, report domain.Report, acct application.Connection, provider probes.Provider, plan []probes.Check, need *probes.Requirements) {
	w := &worker{svc: s, scope: report.Scope, acct: acct, provider: provider, plan: plan, need: need, report: report, byID: map[string]domain.Finding{}, handle: h}
	w.run(ctx)
}

func (w *worker) now() time.Time { return w.svc.cfg.Now().UTC() }

// write persists the current progress with the owner token. It reloads and
// retries when a cancel request or sweep changed the revision, and stops the
// run when ownership is gone. Callers hold w.mu.
func (w *worker) write(status domain.Status) error {
	if status == domain.StatusCancelling {
		status = domain.StatusRunning
	}
	for attempt := 0; attempt < 6; attempt++ {
		now := w.now()
		cleanup := domain.CleanupNotRequired
		if len(w.resources) > 0 {
			cleanup = domain.CleanupPending
			if status.Terminal() {
				cleanup = w.finalCleanup()
			}
		}
		update := domain.RunUpdate{
			OrganizationKey: w.report.OrganizationKey, AccountKey: w.report.AccountKey, ID: w.report.ID, Owner: w.report.Owner,
			ExpectedRevision: w.report.Revision, Status: status, Findings: w.findings, CompletedChecks: len(w.findings), TotalChecks: w.report.TotalChecks,
			ProbeResources: w.resources, CleanupStatus: cleanup, LeaseExpiresAt: now.Add(w.svc.cfg.LeaseTTL), Now: now,
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		next, err := w.svc.repo.UpdateRun(ctx, update)
		cancel()
		switch {
		case err == nil:
			w.report = next
			if next.Status == domain.StatusCancelling {
				w.cancelled = true
				w.stopRun()
			}
			return nil
		case errors.Is(err, domain.ErrRevisionConflict):
			rctx, rcancel := context.WithTimeout(context.Background(), 15*time.Second)
			cur, gerr := w.svc.repo.GetReport(rctx, w.report.OrganizationKey, w.report.AccountKey, w.report.ID)
			rcancel()
			if gerr != nil {
				return gerr
			}
			w.report = cur
			if cur.Status == domain.StatusCancelling {
				w.cancelled = true
				w.stopRun()
			}
			if cur.Status.Terminal() || cur.Owner != update.Owner {
				w.lost = true
				w.stopRun()
				return domain.ErrLeaseLost
			}
		case errors.Is(err, domain.ErrLeaseLost):
			w.lost = true
			w.stopRun()
			return err
		default:
			return err
		}
	}
	return domain.ErrRevisionConflict
}

func (w *worker) finalCleanup() domain.CleanupStatus {
	switch {
	case len(w.resources) == 0:
		return domain.CleanupNotRequired
	case domain.AllSettled(w.resources):
		return domain.CleanupVerified
	}
	return domain.CleanupUnknown
}

// RunID implements probes.Recorder.
func (w *worker) RunID() string { return w.report.ID }

// Record implements probes.Recorder: a resource is persisted as intended
// before the create call and again with its observed UID afterwards.
func (w *worker) Record(ctx context.Context, resource domain.ProbeResource) error {
	// Cancellation and lost ownership are observed before anything is created.
	if err := ctx.Err(); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.lost {
		return domain.ErrLeaseLost
	}
	replaced := false
	for i := range w.resources {
		if w.resources[i].Kind == resource.Kind && w.resources[i].Namespace == resource.Namespace && w.resources[i].Name == resource.Name {
			w.resources[i], replaced = resource, true
		}
	}
	if !replaced {
		w.resources = append(w.resources, resource)
	}
	if err := resource.Validate(); err != nil {
		return err
	}
	return w.write(domain.StatusRunning)
}

func (w *worker) run(parent context.Context) {
	svc := w.svc
	runCtx, stop := context.WithTimeout(parent, svc.cfg.RunTimeout)
	defer stop()
	runCtx, w.stopRun = context.WithCancel(runCtx)
	w.handle.setProbes(w.stopRun)
	cleanupCtx := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.Background(), svc.cfg.CleanupTimeout)
	}

	w.mu.Lock()
	bg, bgCancel := context.WithTimeout(context.Background(), 15*time.Second)
	now := w.now()
	claimed, err := svc.repo.ClaimRun(bg, w.report.OrganizationKey, w.report.AccountKey, w.report.ID, w.report.Owner, w.report.Revision, now.Add(svc.cfg.LeaseTTL), now)
	bgCancel()
	if err != nil {
		w.mu.Unlock()
		return
	}
	w.report = claimed
	w.mu.Unlock()

	beat := time.NewTicker(svc.cfg.Heartbeat)
	defer beat.Stop()
	done := make(chan struct{})
	var beats sync.WaitGroup
	beats.Add(1)
	go func() {
		defer beats.Done()
		for {
			select {
			case <-done:
				return
			case <-beat.C:
				w.mu.Lock()
				if !w.lost && !w.report.Status.Terminal() {
					_ = w.write(w.report.Status)
				}
				w.mu.Unlock()
			}
		}
	}()
	stopBeats := func() { close(done); beats.Wait() }

	// Credential resolution is bounded and covered by the heartbeat above, so
	// a slow credential store cannot expire a legitimate run's lease.
	openCtx, openCancel := context.WithTimeout(runCtx, svc.cfg.CheckTimeout)
	session := w.provider.Open(openCtx, w.acct, w.scope)
	openCancel()
	defer session.Close()

	var always []probes.Check
	for _, c := range w.plan {
		if c.Always {
			always = append(always, c)
		}
	}
	timedOut := false
	for _, c := range w.plan {
		if c.Always {
			continue
		}
		if runCtx.Err() != nil {
			timedOut = errors.Is(runCtx.Err(), context.DeadlineExceeded) && parent.Err() == nil
			break
		}
		finding, ok := w.evaluate(runCtx, session, c)
		if !ok {
			break
		}
		w.mu.Lock()
		if w.lost {
			w.mu.Unlock()
			stopBeats()
			return
		}
		w.append(finding)
		if err := w.write(domain.StatusRunning); err != nil && !w.lost {
			w.failures++
			if w.failures >= 3 {
				// Progress cannot be stored: stop before any further external
				// action; the lease sweep records the interruption.
				w.lost = true
				w.stopRun()
			}
		} else if err == nil {
			w.failures = 0
		}
		lost := w.lost
		w.mu.Unlock()
		if lost {
			stopBeats()
			return
		}
	}

	w.mu.Lock()
	lost, cancelled := w.lost, w.cancelled
	w.mu.Unlock()
	if lost || (parent.Err() != nil && !cancelled) {
		// Ownership gone or the process is stopping: leave the report to the
		// lease sweep, which records an honest INTERRUPTED state.
		if len(w.resources) > 0 {
			ctx, cancel := cleanupCtx()
			w.cleanup(ctx, session)
			cancel()
		}
		stopBeats()
		return
	}
	if timedOut {
		w.fill(domain.VerdictUnknown, "RUN_TIMEOUT", always)
	}

	// Run-owned cleanup uses its own bounded context, after cancellation too.
	cctx, ccancel := cleanupCtx()
	w.cleanup(cctx, session)
	for _, c := range always {
		w.mu.Lock()
		finding := w.cleanupFinding(c)
		w.append(finding)
		_ = w.write(domain.StatusRunning)
		w.mu.Unlock()
	}
	ccancel()
	stopBeats()

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.lost {
		return
	}
	// A cancel request may not have been seen yet: this write reloads on a
	// revision conflict and records it before the final status is chosen.
	_ = w.write(domain.StatusRunning)
	if w.lost {
		return
	}
	status := domain.StatusCompleted
	if len(w.findings) < w.report.TotalChecks {
		if w.cancelled {
			status = domain.StatusCancelled
		} else {
			w.fillLocked(domain.VerdictUnknown, "NOT_RUN", nil)
		}
	}
	_ = w.write(status)
}

// fill records the remaining non-always checks with one honest reason.
func (w *worker) fill(verdict domain.Verdict, reason string, skip []probes.Check) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.fillLocked(verdict, reason, skip)
}

func (w *worker) fillLocked(verdict domain.Verdict, reason string, skip []probes.Check) {
	skipID := map[string]bool{}
	for _, c := range skip {
		skipID[c.ID] = true
	}
	for _, c := range w.plan {
		if _, done := w.byID[c.ID]; done || skipID[c.ID] {
			continue
		}
		w.append(w.finish(c, domain.Finding{Verdict: verdict, Reason: reason}, w.now(), 0))
	}
}

func (w *worker) append(f domain.Finding) {
	w.findings = append(w.findings, f)
	w.byID[f.ID] = f
}

// cleanup deletes the run-owned resources and records what was observed.
func (w *worker) cleanup(ctx context.Context, session probes.Session) {
	w.mu.Lock()
	pending := append([]domain.ProbeResource(nil), w.resources...)
	w.mu.Unlock()
	if len(pending) == 0 || domain.AllSettled(pending) {
		return
	}
	result := session.Cleanup(ctx, pending, w.report.ID)
	w.mu.Lock()
	if !w.lost {
		w.resources = mergeResources(w.resources, result)
		_ = w.write(w.report.Status)
	}
	w.mu.Unlock()
}

func mergeResources(current, observed []domain.ProbeResource) []domain.ProbeResource {
	out := append([]domain.ProbeResource(nil), current...)
	for _, o := range observed {
		found := false
		for i := range out {
			if out[i].Kind == o.Kind && out[i].Namespace == o.Namespace && out[i].Name == o.Name {
				out[i], found = o, true
			}
		}
		if !found && len(out) < domain.MaxProbeItems {
			// A resource discovered during cleanup (a bound volume) is recorded.
			out = append(out, o)
		}
	}
	return out
}

// cleanupFinding derives the cleanup verdict from the observed resources only.
func (w *worker) cleanupFinding(c probes.Check) domain.Finding {
	f := domain.Finding{Facts: map[string]string{}}
	var unknown, preserved, removed int
	for _, r := range w.resources {
		switch r.State {
		case domain.ResourceDeleted:
			removed++
		case domain.ResourcePreserved:
			preserved++
		case domain.ResourceAbsent:
		default:
			unknown++
		}
	}
	f.Facts["found"] = domain.Itoa(len(w.resources))
	switch {
	case len(w.resources) == 0:
		f.Verdict, f.Reason = domain.VerdictNA, "NO_PROBE_RESOURCES"
	case unknown > 0:
		f.Verdict, f.Reason, f.Remediation = domain.VerdictUnknown, "CLEANUP_UNCONFIRMED", "cleanup-confirm"
	case preserved > 0:
		f.Verdict, f.Reason, f.Remediation = domain.VerdictFail, "FOREIGN_OBJECT_PRESERVED", "cleanup-foreign"
	default:
		f.Verdict, f.Reason = domain.VerdictPass, "CLEANUP_VERIFIED"
		f.Facts["denied"] = "0"
	}
	return w.finish(c, f, w.now(), 0)
}

// finish stamps the planned identity on a probe result so a probe cannot
// choose its own ID, dependencies or affected use cases.
func (w *worker) finish(c probes.Check, f domain.Finding, at time.Time, took time.Duration) domain.Finding {
	f.ID, f.Code, f.Group = c.ID, c.ID, c.Group
	f.AffectedUseCases, f.DependsOn = c.AffectedUseCases, c.DependsOn
	f.ObservedAt, f.DurationMillis = at, took.Milliseconds()
	f = f.Sanitized()
	if f.Reason == "" || !f.Verdict.Valid() || f.Validate() != nil {
		f.Verdict, f.Reason, f.Facts, f.Remediation = domain.VerdictUnknown, "PROBE_ERROR", nil, ""
	}
	return f
}

// evaluate runs one check. The bool is false when the run itself was stopped
// during the check, in which case no finding is recorded.
func (w *worker) evaluate(ctx context.Context, session probes.Session, c probes.Check) (domain.Finding, bool) {
	start := w.now()
	w.mu.Lock()
	prior := make(map[string]domain.Finding, len(w.byID))
	for k, v := range w.byID {
		prior[k] = v.Clone()
	}
	w.mu.Unlock()
	for _, dep := range c.DependsOn {
		if p, ok := prior[dep]; ok && (p.Verdict == domain.VerdictFail || p.Verdict == domain.VerdictBlocked) {
			return w.finish(c, domain.Finding{Verdict: domain.VerdictBlocked, Reason: "PREREQUISITE_FAILED"}, start, 0), true
		}
	}
	// A changed account version invalidates the remaining probes.
	cur, err := w.svc.accounts.GetAccount(ctx, w.report.OrganizationKey, w.report.AccountKey)
	switch {
	case ctx.Err() != nil:
		return domain.Finding{}, false
	case err != nil:
		// The saved identity is no longer confirmed: never keep probing with it.
		return w.finish(c, domain.Finding{Verdict: domain.VerdictUnknown, Reason: "ACCOUNT_UNAVAILABLE", Remediation: "rerun"}, start, 0), true
	case cur.Version != w.scope.AccountVersion:
		return w.finish(c, domain.Finding{Verdict: domain.VerdictUnknown, Reason: "ACCOUNT_CHANGED", Remediation: "rerun"}, start, 0), true
	}
	timeout := w.svc.cfg.CheckTimeout
	if c.Execution {
		timeout = w.svc.cfg.ExecutionTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var f domain.Finding
	if strings.HasPrefix(c.ID, "uc.") {
		f = w.svc.evalPlatform(cctx, c, w.scope, w.acct, w.need, w.report.OrganizationKey, prior)
	} else {
		f = w.runProbe(cctx, session, c, prior)
	}
	if ctx.Err() != nil {
		return domain.Finding{}, false
	}
	if cctx.Err() != nil {
		f = domain.Finding{Verdict: domain.VerdictUnknown, Reason: "CHECK_TIMEOUT", Remediation: "rerun"}
	}
	return w.finish(c, f, start, w.now().Sub(start)), true
}

func (w *worker) runProbe(ctx context.Context, session probes.Session, c probes.Check, prior map[string]domain.Finding) (f domain.Finding) {
	defer func() {
		if recover() != nil {
			f = domain.Finding{Verdict: domain.VerdictUnknown, Reason: "PROBE_ERROR"}
		}
	}()
	return session.Run(ctx, c, prior, w.need, w)
}

// sortedKeys is used by platform checks for stable list facts.
func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
