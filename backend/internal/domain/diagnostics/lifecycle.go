package diagnostics

import (
	"errors"
	"fmt"
	"reflect"
	"time"
)

// ErrRevisionConflict means another writer (a cancel request, a sweep) changed
// the report since the caller read it. The caller reloads and decides again.
var ErrRevisionConflict = errors.New("diagnostics: the report changed; reload it")

// Clone deep-copies the scope.
func (s Scope) Clone() Scope {
	s.UseCases = append([]string(nil), s.UseCases...)
	if len(s.UseCases) == 0 {
		s.UseCases = nil
	}
	if s.Identity != nil {
		identity := make(map[string]string, len(s.Identity))
		for k, v := range s.Identity {
			identity[k] = v
		}
		s.Identity = identity
	}
	if s.Context != nil {
		c := *s.Context
		c.Definitions = append([]DefinitionPin(nil), s.Context.Definitions...)
		s.Context = &c
	}
	return s
}

func cloneTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	c := *t
	return &c
}

// Clone deep-copies the report so a stored value can never be changed through
// a returned one.
func (r Report) Clone() Report {
	r.Scope = r.Scope.Clone()
	if r.Findings != nil {
		findings := make([]Finding, len(r.Findings))
		for i, f := range r.Findings {
			findings[i] = f.Clone()
		}
		r.Findings = findings
	}
	r.ProbeResources = append([]ProbeResource(nil), r.ProbeResources...)
	if len(r.ProbeResources) == 0 {
		r.ProbeResources = nil
	}
	r.LeaseExpiresAt, r.HeartbeatAt = cloneTime(r.LeaseExpiresAt), cloneTime(r.HeartbeatAt)
	r.StartedAt, r.FinishedAt = cloneTime(r.StartedAt), cloneTime(r.FinishedAt)
	return r
}

// RunUpdate is a guarded worker write. The worker names the revision it last
// read; a cancel request or sweep in between makes it reload first.
type RunUpdate struct {
	OrganizationKey, AccountKey, ID, Owner string
	ExpectedRevision                       int64
	Status                                 Status
	Findings                               []Finding
	CompletedChecks, TotalChecks           int
	ProbeResources                         []ProbeResource
	CleanupStatus                          CleanupStatus
	LeaseExpiresAt                         time.Time
	Now                                    time.Time
}

// CleanupUpdate is a guarded write to a terminal report whose probe resources
// were unresolved. Only resources, cleanup status and recovery ownership move.
type CleanupUpdate struct {
	OrganizationKey, AccountKey, ID, Owner string
	ExpectedRevision                       int64
	ProbeResources                         []ProbeResource
	CleanupStatus                          CleanupStatus
	Now                                    time.Time
	// Release clears the recovery ownership; otherwise LeaseExpiresAt extends it.
	Release        bool
	LeaseExpiresAt time.Time
}

func leaseLive(r Report, now time.Time) bool {
	return r.LeaseExpiresAt != nil && r.LeaseExpiresAt.After(now)
}

// ApplyClaim moves a QUEUED report owned by owner to RUNNING.
func (r Report) ApplyClaim(owner string, expectedRevision int64, lease, now time.Time) (Report, error) {
	if r.Status != StatusQueued || r.Owner == "" || r.Owner != owner || !leaseLive(r, now) {
		return Report{}, ErrLeaseLost
	}
	if r.Revision != expectedRevision {
		return Report{}, ErrRevisionConflict
	}
	next := r.Clone()
	started := now
	next.Status, next.StartedAt, next.UpdatedAt = StatusRunning, &started, now
	next.LeaseExpiresAt, next.HeartbeatAt = &lease, &started
	next.Revision++
	return next, next.Validate()
}

// ApplyUpdate applies a worker write after checking ownership, a live lease,
// the revision and every transition invariant.
func (r Report) ApplyUpdate(u RunUpdate) (Report, error) {
	if r.Owner == "" || r.Owner != u.Owner || (r.Status != StatusRunning && r.Status != StatusCancelling) || !leaseLive(r, u.Now) {
		return Report{}, ErrLeaseLost
	}
	if r.Revision != u.ExpectedRevision {
		return Report{}, ErrRevisionConflict
	}
	status := u.Status
	switch {
	case status == StatusRunning && r.Status == StatusCancelling:
		status = StatusCancelling
	case status == StatusRunning, status == StatusCompleted:
	case status == StatusCancelled && r.Status == StatusCancelling:
	default:
		return Report{}, fmt.Errorf("%w: status transition", ErrInvalidReport)
	}
	next := r.Clone()
	next.Status, next.CompletedChecks, next.TotalChecks, next.CleanupStatus = status, u.CompletedChecks, u.TotalChecks, u.CleanupStatus
	next.Findings = make([]Finding, len(u.Findings))
	for i, f := range u.Findings {
		next.Findings[i] = f.Clone()
	}
	next.ProbeResources = append([]ProbeResource(nil), u.ProbeResources...)
	now := u.Now
	next.UpdatedAt, next.HeartbeatAt = now, &now
	lease := u.LeaseExpiresAt
	next.LeaseExpiresAt = &lease
	if status.Terminal() {
		next.FinishedAt, next.Owner, next.LeaseExpiresAt = &now, "", nil
		if status == StatusCompleted && next.CompletedChecks != next.TotalChecks {
			return Report{}, fmt.Errorf("%w: a completed run has pending checks", ErrInvalidReport)
		}
	}
	next.Revision++
	if err := r.validateProgress(next); err != nil {
		return Report{}, err
	}
	return next, nil
}

// validateProgress checks that next only extends r.
func (r Report) validateProgress(next Report) error {
	if err := next.Validate(); err != nil {
		return err
	}
	if r.TotalChecks != 0 && next.TotalChecks != r.TotalChecks {
		return fmt.Errorf("%w: the planned check count is fixed", ErrInvalidReport)
	}
	if len(next.Findings) < len(r.Findings) {
		return fmt.Errorf("%w: findings are append-only", ErrInvalidReport)
	}
	for i, f := range r.Findings {
		if !reflect.DeepEqual(f, next.Findings[i]) {
			return fmt.Errorf("%w: recorded findings are immutable", ErrInvalidReport)
		}
	}
	return validateResourceProgress(r.ProbeResources, next.ProbeResources)
}

func validateResourceProgress(prev, next []ProbeResource) error {
	index := map[string]ProbeResource{}
	for _, p := range next {
		index[resourceKey(p)] = p
	}
	for _, old := range prev {
		cur, ok := index[resourceKey(old)]
		if !ok {
			return fmt.Errorf("%w: a recorded probe resource cannot be removed", ErrInvalidReport)
		}
		if old.UID != "" && cur.UID != old.UID {
			return fmt.Errorf("%w: a recorded probe resource identity cannot change", ErrInvalidReport)
		}
		if old.Settled() && cur.State != old.State {
			return fmt.Errorf("%w: a settled probe resource cannot change", ErrInvalidReport)
		}
	}
	return nil
}

// ApplyCancel records a cancellation request. The bool reports a change.
func (r Report) ApplyCancel(now time.Time) (Report, bool) {
	next := r.Clone()
	switch r.Status {
	case StatusQueued:
		next.Status, next.Owner, next.LeaseExpiresAt = StatusCancelled, "", nil
		t := now
		next.FinishedAt = &t
	case StatusRunning:
		next.Status = StatusCancelling
	default:
		return next, false
	}
	next.UpdatedAt = now
	next.Revision++
	return next, true
}

// ApplyExpire interrupts an active report whose lease ended, keeping findings.
// Cleanup that was in flight becomes UNKNOWN; nothing is ever assumed clean.
func (r Report) ApplyExpire(now time.Time) (Report, bool) {
	if !r.Status.Active() || r.LeaseExpiresAt == nil || r.LeaseExpiresAt.After(now) {
		return r, false
	}
	next := r.Clone()
	next.Status, next.Owner, next.LeaseExpiresAt = StatusInterrupted, "", nil
	t := now
	next.FinishedAt, next.UpdatedAt = &t, now
	if next.CleanupStatus == CleanupPending {
		next.CleanupStatus = CleanupUnknown
		if len(next.ProbeResources) == 0 {
			next.CleanupStatus = CleanupNotRequired
		}
	}
	next.Revision++
	return next, true
}

// CleanupEligible reports whether recovery may claim the report now.
func (r Report) CleanupEligible(now time.Time) bool {
	if !r.Status.Terminal() || r.Mode != ModeExecution || len(r.ProbeResources) == 0 || AllSettled(r.ProbeResources) && r.CleanupStatus == CleanupVerified {
		return false
	}
	if r.CleanupStatus != CleanupPending && r.CleanupStatus != CleanupUnknown {
		return false
	}
	return r.Owner == "" || !leaseLive(r, now)
}

// ApplyCleanupClaim gives owner the recovery lease.
func (r Report) ApplyCleanupClaim(owner string, lease, now time.Time) (Report, bool) {
	if !r.CleanupEligible(now) {
		return r, false
	}
	next := r.Clone()
	next.Owner, next.LeaseExpiresAt, next.UpdatedAt = owner, &lease, now
	next.Revision++
	return next, true
}

// ApplyCleanupUpdate records recovery progress on a terminal report.
func (r Report) ApplyCleanupUpdate(u CleanupUpdate) (Report, error) {
	if !r.Status.Terminal() || r.Owner == "" || r.Owner != u.Owner || !leaseLive(r, u.Now) {
		return Report{}, ErrLeaseLost
	}
	if r.Revision != u.ExpectedRevision {
		return Report{}, ErrRevisionConflict
	}
	next := r.Clone()
	next.ProbeResources, next.CleanupStatus, next.UpdatedAt = append([]ProbeResource(nil), u.ProbeResources...), u.CleanupStatus, u.Now
	if u.Release {
		next.Owner, next.LeaseExpiresAt = "", nil
	} else {
		lease := u.LeaseExpiresAt
		next.LeaseExpiresAt = &lease
	}
	next.Revision++
	if err := r.validateProgress(next); err != nil {
		return Report{}, err
	}
	return next, nil
}

// Affects returns, in display order, the selected use cases among cases. A
// GENERAL scope selects none.
func (s Scope) Affects(cases ...string) []string {
	var out []string
	for _, selected := range s.UseCases {
		for _, c := range cases {
			if c == selected {
				out = append(out, selected)
				break
			}
		}
	}
	return out
}
