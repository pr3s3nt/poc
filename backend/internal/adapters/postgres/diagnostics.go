package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"orchestrator/internal/domain/diagnostics"
	diagport "orchestrator/internal/ports/diagnostics"
	"orchestrator/internal/ports/persistence"
)

var _ diagport.Repository = (*Store)(nil)

const reportSelect = `SELECT r.id::text,o.organization_key,c.connection_key,r.account_id::text,r.created_by::text,r.account_version,r.purpose,r.mode,r.scope,
  r.status,r.revision,COALESCE(r.owner,''),r.lease_expires_at,r.heartbeat_at,r.findings,r.completed_checks,r.total_checks,r.probe_resources,
  r.cleanup_status,r.created_at,r.updated_at,r.started_at,r.finished_at
FROM account_diagnostic_reports r JOIN organizations o ON o.id=r.organization_id JOIN connections c ON c.id=r.account_id`

func scanReport(row interface{ Scan(...any) error }) (diagnostics.Report, error) {
	var r diagnostics.Report
	var scope, findings, resources []byte
	var purpose, mode, status, cleanup string
	if err := row.Scan(&r.ID, &r.OrganizationKey, &r.AccountKey, &r.AccountID, &r.CreatedBy, &r.AccountVersion, &purpose, &mode, &scope,
		&status, &r.Revision, &r.Owner, &r.LeaseExpiresAt, &r.HeartbeatAt, &findings, &r.CompletedChecks, &r.TotalChecks, &resources,
		&cleanup, &r.CreatedAt, &r.UpdatedAt, &r.StartedAt, &r.FinishedAt); err != nil {
		return r, err
	}
	r.Purpose, r.Mode, r.Status, r.CleanupStatus = diagnostics.Purpose(purpose), diagnostics.Mode(mode), diagnostics.Status(status), diagnostics.CleanupStatus(cleanup)
	if err := json.Unmarshal(scope, &r.Scope); err != nil {
		return r, fmt.Errorf("postgres: diagnostic report scope: %w", diagnostics.ErrInvalidReport)
	}
	if err := json.Unmarshal(findings, &r.Findings); err != nil {
		return r, fmt.Errorf("postgres: diagnostic report findings: %w", diagnostics.ErrInvalidReport)
	}
	if err := json.Unmarshal(resources, &r.ProbeResources); err != nil {
		return r, fmt.Errorf("postgres: diagnostic report resources: %w", diagnostics.ErrInvalidReport)
	}
	return r, nil
}

func terminalStatus(s diagnostics.Status) bool { return s.Terminal() }

// CreateReport implements diagnostics.Repository.
func (s *Store) CreateReport(ctx context.Context, r diagnostics.Report) (diagnostics.Report, error) {
	if !validUUID(r.CreatedBy) || !validUUID(r.ID) {
		return diagnostics.Report{}, fmt.Errorf("%w: author", persistence.ErrNotFound)
	}
	if err := r.Validate(); err != nil {
		return diagnostics.Report{}, err
	}
	scope, err := json.Marshal(r.Scope)
	if err != nil {
		return diagnostics.Report{}, err
	}
	findings, _ := json.Marshal(nonNilFindings(r.Findings))
	resources, _ := json.Marshal(nonNilResources(r.ProbeResources))
	tag, err := s.q(ctx).Exec(ctx, `INSERT INTO account_diagnostic_reports(id,organization_id,account_id,created_by,account_version,purpose,mode,scope,status,revision,owner,lease_expires_at,heartbeat_at,findings,completed_checks,total_checks,probe_resources,cleanup_status,created_at,updated_at,started_at,finished_at)
SELECT $1::uuid,o.id,c.id,u.id,$5,$6,$7,$8,$9,$10,NULLIF($11,''),$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22
FROM organizations o
JOIN connections c ON c.organization_id=o.id AND c.connection_key=$3
JOIN user_accounts u ON u.id=$4::uuid AND u.organization_id=o.id
WHERE o.organization_key=$2`,
		r.ID, r.OrganizationKey, r.AccountKey, r.CreatedBy, r.AccountVersion, string(r.Purpose), string(r.Mode), scope, string(r.Status), r.Revision,
		r.Owner, r.LeaseExpiresAt, r.HeartbeatAt, findings, r.CompletedChecks, r.TotalChecks, resources, string(r.CleanupStatus), r.CreatedAt, r.UpdatedAt, r.StartedAt, r.FinishedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if pgErr.ConstraintName == "account_diagnostic_reports_active" {
				return diagnostics.Report{}, diagnostics.ErrBusy
			}
			return diagnostics.Report{}, persistence.ErrDuplicate
		}
		return diagnostics.Report{}, translate(err)
	}
	if tag.RowsAffected() == 0 {
		return diagnostics.Report{}, fmt.Errorf("%w: account", persistence.ErrNotFound)
	}
	return s.GetReport(ctx, r.OrganizationKey, r.AccountKey, r.ID)
}

func nonNilFindings(f []diagnostics.Finding) []diagnostics.Finding {
	if f == nil {
		return []diagnostics.Finding{}
	}
	return f
}

func nonNilResources(f []diagnostics.ProbeResource) []diagnostics.ProbeResource {
	if f == nil {
		return []diagnostics.ProbeResource{}
	}
	return f
}

// GetReport implements diagnostics.Repository.
func (s *Store) GetReport(ctx context.Context, org, accountKey, id string) (diagnostics.Report, error) {
	if !validUUID(id) {
		return diagnostics.Report{}, fmt.Errorf("%w: diagnostic report", persistence.ErrNotFound)
	}
	r, err := scanReport(s.q(ctx).QueryRow(ctx, reportSelect+` WHERE o.organization_key=$1 AND c.connection_key=$2 AND r.id=$3::uuid`, org, accountKey, id))
	if err != nil {
		return diagnostics.Report{}, translate(err)
	}
	return r, nil
}

func encodeCursor(r diagnostics.Report) string {
	return base64.RawURLEncoding.EncodeToString([]byte(r.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + r.ID))
}

// ListReports implements diagnostics.Repository.
func (s *Store) ListReports(ctx context.Context, org, accountKey string, q diagport.ListQuery) (diagport.Page, error) {
	limit := q.Limit
	if limit < 1 || limit > 100 {
		limit = 20
	}
	var exists bool
	if err := s.q(ctx).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM connections c JOIN organizations o ON o.id=c.organization_id WHERE o.organization_key=$1 AND c.connection_key=$2)`, org, accountKey).Scan(&exists); err != nil {
		return diagport.Page{}, translate(err)
	}
	if !exists {
		return diagport.Page{}, fmt.Errorf("%w: account", persistence.ErrNotFound)
	}
	args := []any{org, accountKey, string(q.Purpose), limit + 1}
	where := ` WHERE o.organization_key=$1 AND c.connection_key=$2 AND ($3='' OR r.purpose=$3)`
	if q.Cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(q.Cursor)
		at, id, ok := strings.Cut(string(raw), "|")
		when, perr := time.Parse(time.RFC3339Nano, at)
		if err != nil || !ok || perr != nil || !validUUID(id) {
			return diagport.Page{}, diagnostics.ErrInvalidScope
		}
		args = append(args, when, id)
		where += ` AND (r.created_at,r.id) < ($5,$6::uuid)`
	}
	rows, err := s.q(ctx).Query(ctx, reportSelect+where+` ORDER BY r.created_at DESC, r.id DESC LIMIT $4`, args...)
	if err != nil {
		return diagport.Page{}, translate(err)
	}
	defer rows.Close()
	var page diagport.Page
	for rows.Next() {
		r, err := scanReport(rows)
		if err != nil {
			return diagport.Page{}, err
		}
		page.Reports = append(page.Reports, r)
	}
	if err := rows.Err(); err != nil {
		return diagport.Page{}, translate(err)
	}
	if len(page.Reports) > limit {
		page.Reports = page.Reports[:limit]
		page.NextCursor = encodeCursor(page.Reports[limit-1])
	}
	return page, nil
}

// LatestReports implements diagnostics.Repository.
func (s *Store) LatestReports(ctx context.Context, org string) (map[string]diagnostics.Report, error) {
	rows, err := s.q(ctx).Query(ctx, reportSelect+` WHERE o.organization_key=$1 AND r.id=(SELECT x.id FROM account_diagnostic_reports x WHERE x.account_id=r.account_id ORDER BY x.created_at DESC, x.id DESC LIMIT 1)`, org)
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()
	out := map[string]diagnostics.Report{}
	for rows.Next() {
		r, err := scanReport(rows)
		if err != nil {
			return nil, err
		}
		out[r.AccountKey] = r
	}
	return out, translate(rows.Err())
}

// lockedReport reads one report FOR UPDATE inside the caller's transaction.
func (s *Store) lockedReport(ctx context.Context, org, accountKey, id string) (diagnostics.Report, error) {
	if !validUUID(id) {
		return diagnostics.Report{}, fmt.Errorf("%w: diagnostic report", persistence.ErrNotFound)
	}
	r, err := scanReport(s.q(ctx).QueryRow(ctx, reportSelect+` WHERE o.organization_key=$1 AND c.connection_key=$2 AND r.id=$3::uuid FOR UPDATE OF r`, org, accountKey, id))
	if err != nil {
		return diagnostics.Report{}, translate(err)
	}
	return r, nil
}

// writeReport persists the mutable columns of next when the stored revision is
// still prev's. Identity and scope columns are never written here.
func (s *Store) writeReport(ctx context.Context, prev, next diagnostics.Report) error {
	findings, err := json.Marshal(nonNilFindings(next.Findings))
	if err != nil {
		return err
	}
	resources, err := json.Marshal(nonNilResources(next.ProbeResources))
	if err != nil {
		return err
	}
	tag, err := s.q(ctx).Exec(ctx, `UPDATE account_diagnostic_reports SET status=$3,owner=NULLIF($4,''),lease_expires_at=$5,heartbeat_at=$6,findings=$7,completed_checks=$8,total_checks=$9,
  probe_resources=$10,cleanup_status=$11,updated_at=$12,started_at=$13,finished_at=$14,revision=$15
WHERE id=$1::uuid AND revision=$2`,
		next.ID, prev.Revision, string(next.Status), next.Owner, next.LeaseExpiresAt, next.HeartbeatAt, findings, next.CompletedChecks, next.TotalChecks,
		resources, string(next.CleanupStatus), next.UpdatedAt, next.StartedAt, next.FinishedAt, next.Revision)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23514" {
			return diagnostics.ErrInvalidReport
		}
		return translate(err)
	}
	if tag.RowsAffected() == 0 {
		return diagnostics.ErrRevisionConflict
	}
	return nil
}

// mutate runs one locked read-apply-write cycle. The pure state machine in the
// domain decides; this adapter only locks, loads and stores, so memory and
// PostgreSQL cannot disagree about a transition. No remote call is made here.
func (s *Store) mutate(ctx context.Context, org, accountKey, id string, apply func(diagnostics.Report) (diagnostics.Report, bool, error)) (diagnostics.Report, error) {
	var out diagnostics.Report
	err := s.Transact(ctx, func(ctx context.Context) error {
		cur, err := s.lockedReport(ctx, org, accountKey, id)
		if err != nil {
			return err
		}
		next, changed, err := apply(cur)
		if err != nil {
			return err
		}
		out = next
		if !changed {
			return nil
		}
		return s.writeReport(ctx, cur, next)
	})
	return out, err
}

// ClaimRun implements diagnostics.Repository.
func (s *Store) ClaimRun(ctx context.Context, org, accountKey, id, owner string, expectedRevision int64, lease, now time.Time) (diagnostics.Report, error) {
	return s.mutate(ctx, org, accountKey, id, func(r diagnostics.Report) (diagnostics.Report, bool, error) {
		next, err := r.ApplyClaim(owner, expectedRevision, lease, now)
		return next, err == nil, err
	})
}

// UpdateRun implements diagnostics.Repository.
func (s *Store) UpdateRun(ctx context.Context, u diagnostics.RunUpdate) (diagnostics.Report, error) {
	return s.mutate(ctx, u.OrganizationKey, u.AccountKey, u.ID, func(r diagnostics.Report) (diagnostics.Report, bool, error) {
		next, err := r.ApplyUpdate(u)
		return next, err == nil, err
	})
}

// RequestCancel implements diagnostics.Repository.
func (s *Store) RequestCancel(ctx context.Context, org, accountKey, id string, now time.Time) (diagnostics.Report, error) {
	return s.mutate(ctx, org, accountKey, id, func(r diagnostics.Report) (diagnostics.Report, bool, error) {
		next, changed := r.ApplyCancel(now)
		return next, changed, nil
	})
}

// UpdateCleanup implements diagnostics.Repository.
func (s *Store) UpdateCleanup(ctx context.Context, u diagnostics.CleanupUpdate) (diagnostics.Report, error) {
	return s.mutate(ctx, u.OrganizationKey, u.AccountKey, u.ID, func(r diagnostics.Report) (diagnostics.Report, bool, error) {
		next, err := r.ApplyCleanupUpdate(u)
		return next, err == nil, err
	})
}

// ExpireLeases implements diagnostics.Repository.
func (s *Store) ExpireLeases(ctx context.Context, now time.Time) (int, error) {
	count := 0
	err := s.Transact(ctx, func(ctx context.Context) error {
		count = 0
		rows, err := s.q(ctx).Query(ctx, reportSelect+` WHERE r.status IN ('QUEUED','RUNNING','CANCELLING') AND r.lease_expires_at <= $1 FOR UPDATE OF r SKIP LOCKED`, now)
		if err != nil {
			return translate(err)
		}
		var due []diagnostics.Report
		for rows.Next() {
			r, err := scanReport(rows)
			if err != nil {
				rows.Close()
				return err
			}
			due = append(due, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return translate(err)
		}
		for _, r := range due {
			if next, changed := r.ApplyExpire(now); changed {
				if err := s.writeReport(ctx, r, next); err != nil {
					return err
				}
				count++
			}
		}
		return nil
	})
	return count, err
}

// ClaimCleanup implements diagnostics.Repository.
func (s *Store) ClaimCleanup(ctx context.Context, owner string, lease, now time.Time) (diagnostics.Report, bool, error) {
	var claimed diagnostics.Report
	var ok bool
	err := s.Transact(ctx, func(ctx context.Context) error {
		ok = false
		rows, err := s.q(ctx).Query(ctx, reportSelect+` WHERE r.status IN ('COMPLETED','CANCELLED','INTERRUPTED') AND r.mode='EXECUTION' AND jsonb_array_length(r.probe_resources)>0
  AND r.cleanup_status IN ('PENDING','UNKNOWN') AND (r.owner IS NULL OR r.lease_expires_at <= $1) ORDER BY r.created_at LIMIT 8 FOR UPDATE OF r SKIP LOCKED`, now)
		if err != nil {
			return translate(err)
		}
		var candidates []diagnostics.Report
		for rows.Next() {
			r, err := scanReport(rows)
			if err != nil {
				rows.Close()
				return err
			}
			candidates = append(candidates, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return translate(err)
		}
		for _, r := range candidates {
			if next, changed := r.ApplyCleanupClaim(owner, lease, now); changed {
				if err := s.writeReport(ctx, r, next); err != nil {
					return err
				}
				claimed, ok = next, true
				return nil
			}
		}
		return nil
	})
	return claimed, ok, err
}
