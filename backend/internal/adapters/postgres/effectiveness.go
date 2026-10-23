package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"orchestrator/internal/domain/effectiveness"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

const baselineSelect = `SELECT b.id::text,o.organization_key,COALESCE(a.application_key,''),b.metric_id,b.version,b.retired,
  COALESCE(b.statistic,''),COALESCE(b.value_seconds,0),b.source,b.sample_size,b.measured_from,b.measured_to,b.effective_from,b.created_by::text,b.created_at
FROM effectiveness_baselines b
JOIN organizations o ON o.id=b.organization_id
LEFT JOIN applications a ON a.id=b.application_id`

func dateOf(t time.Time) effectiveness.Date { return effectiveness.Date(t.Format("2006-01-02")) }

func scanBaseline(row interface{ Scan(...any) error }) (effectiveness.Baseline, error) {
	var b effectiveness.Baseline
	var sample *int32
	var from, to *time.Time
	var effective time.Time
	if err := row.Scan(&b.ID, &b.OrganizationKey, &b.ApplicationKey, &b.Metric, &b.Version, &b.Retired, &b.Statistic, &b.ValueSeconds,
		&b.Source, &sample, &from, &to, &effective, &b.CreatedBy, &b.CreatedAt); err != nil {
		return b, err
	}
	if sample != nil {
		n := int(*sample)
		b.SampleSize = &n
	}
	if from != nil && to != nil {
		f, t := dateOf(*from), dateOf(*to)
		b.MeasuredFrom, b.MeasuredTo = &f, &t
	}
	b.EffectiveFrom = dateOf(effective)
	return b, nil
}

// ListEffectivenessBaselines returns the versions of one metric and scope,
// highest version first.
func (s *Store) ListEffectivenessBaselines(ctx context.Context, organizationKey, metric, applicationKey string) ([]effectiveness.Baseline, error) {
	rows, err := s.q(ctx).Query(ctx, baselineSelect+` WHERE o.organization_key=$1 AND b.metric_id=$2 AND (($3='' AND b.application_id IS NULL) OR a.application_key=$3) ORDER BY b.version DESC`, organizationKey, metric, applicationKey)
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()
	out := []effectiveness.Baseline{}
	for rows.Next() {
		b, err := scanBaseline(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func nullableDate(d *effectiveness.Date) any {
	if d == nil {
		return nil
	}
	return string(*d)
}

// AppendEffectivenessBaseline inserts version expectedVersion+1 of the scope
// in one statement that re-checks the current version; a concurrent insert of
// the same version fails the unique index and is reported as a conflict.
func (s *Store) AppendEffectivenessBaseline(ctx context.Context, b effectiveness.Baseline, expectedVersion int64) (effectiveness.Baseline, error) {
	if !validUUID(b.CreatedBy) {
		return effectiveness.Baseline{}, fmt.Errorf("%w: author", persistence.ErrNotFound)
	}
	var sample any
	if b.SampleSize != nil {
		sample = *b.SampleSize
	}
	var statistic, value any
	if !b.Retired {
		statistic, value = b.Statistic, b.ValueSeconds
	}
	id := ids.New()
	var stored effectiveness.Baseline
	err := s.Transact(ctx, func(ctx context.Context) error {
		tag, err := s.q(ctx).Exec(ctx, `INSERT INTO effectiveness_baselines(id,organization_id,application_id,metric_id,version,retired,statistic,value_seconds,source,sample_size,measured_from,measured_to,effective_from,created_by,created_at)
SELECT $1::uuid,o.id,a.id,$4,$5::bigint+1,$6,$7,$8,$9,$10,$11::date,$12::date,$13::date,u.id,$15
FROM organizations o
JOIN user_accounts u ON u.id=$14::uuid AND u.organization_id=o.id
LEFT JOIN applications a ON a.application_key=NULLIF($3,'') AND a.organization_id=o.id
WHERE o.organization_key=$2 AND (NULLIF($3,'') IS NULL OR a.id IS NOT NULL)
  AND COALESCE((SELECT max(x.version) FROM effectiveness_baselines x WHERE x.organization_id=o.id AND x.metric_id=$4 AND x.application_id IS NOT DISTINCT FROM a.id),0)=$5::bigint`,
			id, b.OrganizationKey, b.ApplicationKey, b.Metric, expectedVersion, b.Retired, statistic, value, b.Source, sample,
			nullableDate(b.MeasuredFrom), nullableDate(b.MeasuredTo), string(b.EffectiveFrom), b.CreatedBy, b.CreatedAt)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return persistence.ErrVersionConflict
			}
			return translate(err)
		}
		if tag.RowsAffected() == 0 {
			var exists bool
			if err := s.q(ctx).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM organizations o JOIN user_accounts u ON u.id=$3::uuid AND u.organization_id=o.id
WHERE o.organization_key=$1 AND (NULLIF($2,'') IS NULL OR EXISTS(SELECT 1 FROM applications a WHERE a.application_key=$2 AND a.organization_id=o.id)))`, b.OrganizationKey, b.ApplicationKey, b.CreatedBy).Scan(&exists); err != nil {
				return translate(err)
			}
			if exists {
				return persistence.ErrVersionConflict
			}
			return fmt.Errorf("%w: baseline scope", persistence.ErrNotFound)
		}
		stored, err = scanBaseline(s.q(ctx).QueryRow(ctx, baselineSelect+` WHERE b.id=$1::uuid`, id))
		return translate(err)
	})
	if err != nil {
		return effectiveness.Baseline{}, err
	}
	return stored, nil
}

// ListCleanupRuns returns the safe projection of every RESOURCE_CLEANUP
// operation of the Organization, oldest first. It reads no scope snapshot,
// confirmed selection, detail or failure text.
func (s *Store) ListCleanupRuns(ctx context.Context, organizationKey string) ([]persistence.CleanupRun, error) {
	rows, err := s.q(ctx).Query(ctx, `SELECT op.id::text,a.application_key,e.environment_key,COALESCE(op.root_resource_instance_id::text,''),
  COALESCE(op.authorization_mode,''),COALESCE(op.actor_ref,''),op.status,op.started_at,op.finished_at
FROM environment_operations op
JOIN environments e ON e.id=op.environment_id
JOIN applications a ON a.id=e.application_id
JOIN organizations o ON o.id=a.organization_id
WHERE op.kind='RESOURCE_CLEANUP' AND o.organization_key=$1
ORDER BY op.started_at,op.id`, organizationKey)
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()
	out := []persistence.CleanupRun{}
	for rows.Next() {
		var r persistence.CleanupRun
		if err := rows.Scan(&r.OperationID, &r.ApplicationKey, &r.EnvironmentKey, &r.RootID, &r.Mode, &r.ActorRef, &r.Status, &r.StartedAt, &r.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
