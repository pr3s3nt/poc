// Package diagnostics defines the persistence port of Infrastructure Account
// diagnostic reports (ADR-022). It is separate from persistence.Store: a
// diagnostic write never changes Connection status or version.
package diagnostics

import (
	"context"
	"time"

	domain "orchestrator/internal/domain/diagnostics"
)

// ListQuery selects one page of an account's history, newest first.
type ListQuery struct {
	Purpose domain.Purpose // empty: every purpose
	Limit   int            // 1..100
	Cursor  string         // opaque value from a previous page
}

// Page is one history page.
type Page struct {
	Reports    []domain.Report
	NextCursor string
}

// Repository is the durable report store. Every query is Organization and
// account scoped; a foreign or unknown key is persistence.ErrNotFound.
type Repository interface {
	// CreateReport inserts a QUEUED report for an account of the
	// Organization. The report carries its owner token and lease. ErrBusy
	// when the account already has an active run; persistence.ErrNotFound for
	// an unknown account or actor of another Organization.
	CreateReport(ctx context.Context, report domain.Report) (domain.Report, error)
	GetReport(ctx context.Context, org, accountKey, id string) (domain.Report, error)
	ListReports(ctx context.Context, org, accountKey string, q ListQuery) (Page, error)
	// LatestReports returns the newest report of each account of the
	// Organization, keyed by account key.
	LatestReports(ctx context.Context, org string) (map[string]domain.Report, error)
	// ClaimRun moves a QUEUED report owned by owner to RUNNING when its
	// revision and lease still match (domain.ReportClaim rules).
	ClaimRun(ctx context.Context, org, accountKey, id, owner string, expectedRevision int64, lease, now time.Time) (domain.Report, error)
	// UpdateRun applies a worker write. domain.ErrLeaseLost when the owner no
	// longer holds a live lease or the report is terminal;
	// domain.ErrRevisionConflict when the report changed since it was read.
	UpdateRun(ctx context.Context, u domain.RunUpdate) (domain.Report, error)
	// RequestCancel records a cancellation. A QUEUED report becomes
	// CANCELLED; RUNNING becomes CANCELLING; any other state is returned
	// unchanged (idempotent).
	RequestCancel(ctx context.Context, org, accountKey, id string, now time.Time) (domain.Report, error)
	// ExpireLeases turns active reports whose lease ended into INTERRUPTED,
	// keeping findings, and marks unresolved probe resources UNKNOWN cleanup.
	ExpireLeases(ctx context.Context, now time.Time) (int, error)
	// ClaimCleanup claims a terminal EXECUTION report with unresolved cleanup
	// for recovery when nobody holds a live recovery lease.
	ClaimCleanup(ctx context.Context, owner string, lease, now time.Time) (domain.Report, bool, error)
	UpdateCleanup(ctx context.Context, u domain.CleanupUpdate) (domain.Report, error)
}
