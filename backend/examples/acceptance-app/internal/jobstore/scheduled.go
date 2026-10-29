package jobstore

import (
	"context"
	"errors"
	"regexp"
	"time"
)

var scheduleID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
var digest = regexp.MustCompile(`^[a-f0-9]{64}$`)
var ErrReceipt = errors.New("invalid or conflicting scheduled receipt")

func ValidScheduleID(value string) bool { return scheduleID.MatchString(value) }

type ScheduledRun struct {
	RunID       string    `json:"runId"`
	Invocation  string    `json:"invocation"`
	ValueSHA256 string    `json:"valueSha256"`
	CompletedAt time.Time `json:"completedAt"`
}

func (s *Store) RecordScheduledRun(ctx context.Context, run, invocation, hash string) error {
	if !ValidScheduleID(run) || !ValidScheduleID(invocation) || !digest.MatchString(hash) {
		return ErrReceipt
	}
	tag, err := s.pool.Exec(ctx, `INSERT INTO acceptance_scheduled_runs (run_id, invocation, value_sha256) VALUES ($1,$2,$3)
ON CONFLICT (run_id, invocation) DO UPDATE SET value_sha256=acceptance_scheduled_runs.value_sha256
WHERE acceptance_scheduled_runs.value_sha256=EXCLUDED.value_sha256`, run, invocation, hash)
	if err != nil {
		return errors.New("could not record scheduled receipt")
	}
	if tag.RowsAffected() != 1 {
		return ErrReceipt
	}
	return nil
}
func (s *Store) ScheduledRuns(ctx context.Context, run string) ([]ScheduledRun, error) {
	if !ValidScheduleID(run) {
		return nil, ErrReceipt
	}
	rows, err := s.pool.Query(ctx, `SELECT run_id,invocation,value_sha256,completed_at FROM acceptance_scheduled_runs WHERE run_id=$1 ORDER BY completed_at DESC,invocation LIMIT 20`, run)
	if err != nil {
		return nil, errors.New("scheduled receipts unavailable")
	}
	defer rows.Close()
	out := []ScheduledRun{}
	for rows.Next() {
		var row ScheduledRun
		if err := rows.Scan(&row.RunID, &row.Invocation, &row.ValueSHA256, &row.CompletedAt); err != nil {
			return nil, errors.New("scheduled receipts unavailable")
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
