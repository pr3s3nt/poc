package store

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"time"

	"orchestrator/internal/domain/diagnostics"
	diagport "orchestrator/internal/ports/diagnostics"
	"orchestrator/internal/ports/persistence"
)

var _ diagport.Repository = (*Store)(nil)

func (s *Store) reportOf(org, accountKey, id string) (diagnostics.Report, error) {
	r, ok := s.state.DiagnosticReports[id]
	if !ok || r.OrganizationKey != org || r.AccountKey != accountKey {
		return diagnostics.Report{}, fmt.Errorf("%w: diagnostic report", persistence.ErrNotFound)
	}
	return r, nil
}

// put stores a private deep copy and returns another, so neither the caller's
// value nor a returned one aliases persisted history.
func (s *Store) put(r diagnostics.Report) diagnostics.Report {
	s.state.DiagnosticReports[r.ID] = r.Clone()
	return r.Clone()
}

// CreateReport implements diagnostics.Repository.
func (s *Store) CreateReport(ctx context.Context, report diagnostics.Report) (diagnostics.Report, error) {
	defer s.lock(ctx)()
	conn, ok := s.state.Connections[catalogKey(report.OrganizationKey, report.AccountKey)]
	if !ok {
		return diagnostics.Report{}, fmt.Errorf("%w: account", persistence.ErrNotFound)
	}
	if author, ok := s.state.UserAccounts[report.CreatedBy]; !ok || author.OrganizationKey != report.OrganizationKey {
		return diagnostics.Report{}, fmt.Errorf("%w: author", persistence.ErrNotFound)
	}
	report = report.Clone()
	report.AccountID = conn.ID
	if err := report.Validate(); err != nil {
		return diagnostics.Report{}, err
	}
	if _, dup := s.state.DiagnosticReports[report.ID]; dup {
		return diagnostics.Report{}, persistence.ErrDuplicate
	}
	for _, other := range s.state.DiagnosticReports {
		if other.OrganizationKey == report.OrganizationKey && other.AccountKey == report.AccountKey && other.Status.Active() {
			return diagnostics.Report{}, diagnostics.ErrBusy
		}
	}
	return s.put(report), nil
}

// GetReport implements diagnostics.Repository.
func (s *Store) GetReport(ctx context.Context, org, accountKey, id string) (diagnostics.Report, error) {
	defer s.rlock(ctx)()
	r, err := s.reportOf(org, accountKey, id)
	if err != nil {
		return diagnostics.Report{}, err
	}
	return r.Clone(), nil
}

func encodeCursor(r diagnostics.Report) string {
	return base64.RawURLEncoding.EncodeToString([]byte(r.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + r.ID))
}

func decodeCursor(cursor string) (time.Time, string, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", false
	}
	at, id, ok := strings.Cut(string(raw), "|")
	if !ok || id == "" {
		return time.Time{}, "", false
	}
	t, err := time.Parse(time.RFC3339Nano, at)
	return t, id, err == nil
}

// ListReports implements diagnostics.Repository.
func (s *Store) ListReports(ctx context.Context, org, accountKey string, q diagport.ListQuery) (diagport.Page, error) {
	defer s.rlock(ctx)()
	if _, ok := s.state.Connections[catalogKey(org, accountKey)]; !ok {
		return diagport.Page{}, fmt.Errorf("%w: account", persistence.ErrNotFound)
	}
	var all []diagnostics.Report
	for _, r := range s.state.DiagnosticReports {
		if r.OrganizationKey == org && r.AccountKey == accountKey && (q.Purpose == "" || r.Purpose == q.Purpose) {
			all = append(all, r)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if !all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].CreatedAt.After(all[j].CreatedAt)
		}
		return all[i].ID > all[j].ID
	})
	if q.Cursor != "" {
		at, id, ok := decodeCursor(q.Cursor)
		if !ok {
			return diagport.Page{}, diagnostics.ErrInvalidScope
		}
		kept := all[:0:0]
		for _, r := range all {
			if r.CreatedAt.Before(at) || (r.CreatedAt.Equal(at) && r.ID < id) {
				kept = append(kept, r)
			}
		}
		all = kept
	}
	limit := q.Limit
	if limit < 1 || limit > 100 {
		limit = 20
	}
	page := diagport.Page{}
	if len(all) > limit {
		all = all[:limit]
		page.NextCursor = encodeCursor(all[len(all)-1])
	}
	for _, r := range all {
		page.Reports = append(page.Reports, r.Clone())
	}
	return page, nil
}

// LatestReports implements diagnostics.Repository.
func (s *Store) LatestReports(ctx context.Context, org string) (map[string]diagnostics.Report, error) {
	defer s.rlock(ctx)()
	out := map[string]diagnostics.Report{}
	for _, r := range s.state.DiagnosticReports {
		if r.OrganizationKey != org {
			continue
		}
		if cur, ok := out[r.AccountKey]; !ok || r.CreatedAt.After(cur.CreatedAt) || (r.CreatedAt.Equal(cur.CreatedAt) && r.ID > cur.ID) {
			out[r.AccountKey] = r
		}
	}
	for k, r := range out {
		out[k] = r.Clone()
	}
	return out, nil
}

// ClaimRun implements diagnostics.Repository.
func (s *Store) ClaimRun(ctx context.Context, org, accountKey, id, owner string, expectedRevision int64, lease, now time.Time) (diagnostics.Report, error) {
	defer s.lock(ctx)()
	r, err := s.reportOf(org, accountKey, id)
	if err != nil {
		return diagnostics.Report{}, err
	}
	next, err := r.ApplyClaim(owner, expectedRevision, lease, now)
	if err != nil {
		return diagnostics.Report{}, err
	}
	return s.put(next), nil
}

// UpdateRun implements diagnostics.Repository.
func (s *Store) UpdateRun(ctx context.Context, u diagnostics.RunUpdate) (diagnostics.Report, error) {
	defer s.lock(ctx)()
	r, err := s.reportOf(u.OrganizationKey, u.AccountKey, u.ID)
	if err != nil {
		return diagnostics.Report{}, err
	}
	next, err := r.ApplyUpdate(u)
	if err != nil {
		return diagnostics.Report{}, err
	}
	return s.put(next), nil
}

// RequestCancel implements diagnostics.Repository.
func (s *Store) RequestCancel(ctx context.Context, org, accountKey, id string, now time.Time) (diagnostics.Report, error) {
	defer s.lock(ctx)()
	r, err := s.reportOf(org, accountKey, id)
	if err != nil {
		return diagnostics.Report{}, err
	}
	next, changed := r.ApplyCancel(now)
	if !changed {
		return r.Clone(), nil
	}
	return s.put(next), nil
}

// ExpireLeases implements diagnostics.Repository.
func (s *Store) ExpireLeases(ctx context.Context, now time.Time) (int, error) {
	// A sweep that finds nothing must not rewrite the snapshot file.
	if !s.anyReport(ctx, func(r diagnostics.Report) bool { _, changed := r.ApplyExpire(now); return changed }) {
		return 0, nil
	}
	defer s.lock(ctx)()
	count := 0
	for _, r := range s.state.DiagnosticReports {
		if next, changed := r.ApplyExpire(now); changed {
			s.put(next)
			count++
		}
	}
	return count, nil
}

// ClaimCleanup implements diagnostics.Repository.
func (s *Store) ClaimCleanup(ctx context.Context, owner string, lease, now time.Time) (diagnostics.Report, bool, error) {
	if !s.anyReport(ctx, func(r diagnostics.Report) bool { return r.CleanupEligible(now) }) {
		return diagnostics.Report{}, false, nil
	}
	defer s.lock(ctx)()
	var pick *diagnostics.Report
	for _, r := range s.state.DiagnosticReports {
		r := r
		if r.CleanupEligible(now) && (pick == nil || r.CreatedAt.Before(pick.CreatedAt)) {
			pick = &r
		}
	}
	if pick == nil {
		return diagnostics.Report{}, false, nil
	}
	next, _ := pick.ApplyCleanupClaim(owner, lease, now)
	return s.put(next), true, nil
}

// UpdateCleanup implements diagnostics.Repository.
func (s *Store) UpdateCleanup(ctx context.Context, u diagnostics.CleanupUpdate) (diagnostics.Report, error) {
	defer s.lock(ctx)()
	r, err := s.reportOf(u.OrganizationKey, u.AccountKey, u.ID)
	if err != nil {
		return diagnostics.Report{}, err
	}
	next, err := r.ApplyCleanupUpdate(u)
	if err != nil {
		return diagnostics.Report{}, err
	}
	return s.put(next), nil
}

// anyReport reports, under a read lock that persists nothing, whether some
// report satisfies match.
func (s *Store) anyReport(ctx context.Context, match func(diagnostics.Report) bool) bool {
	defer s.rlock(ctx)()
	for _, r := range s.state.DiagnosticReports {
		if match(r) {
			return true
		}
	}
	return false
}
