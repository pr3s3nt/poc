// Package jobstore is the shared PostgreSQL access layer of the acceptance
// application. The backend and the worker both bind to the same shared database
// resource, so they use the same schema and the same queries.
package jobstore

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Status values of a job.
const (
	StatusPending = "PENDING"
	StatusDone    = "DONE"
)

// Values of the per-job write-through marker the worker records after it tries
// the Redis cache. A NULL marker means the worker has not reported an attempt.
const (
	CacheWriteOK     = "OK"
	CacheWriteFailed = "FAILED"
)

const (
	maxFieldLen    = 256
	maxPasswordLen = 1024
)

// Job is one unit of work submitted through the backend API.
type Job struct {
	ID          int64      `json:"id"`
	Payload     string     `json:"payload"`
	Status      string     `json:"status"`
	Result      string     `json:"result,omitempty"`
	ProcessedBy string     `json:"processedBy,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	ProcessedAt *time.Time `json:"processedAt,omitempty"`
}

// Config is the database connection configuration read from the environment.
// The orchestrator injects these variables from the postgres resource outputs.
type Config struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
}

// ConfigFromEnv reads PGHOST, PGPORT, PGDATABASE, PGUSER and PGPASSWORD. Values
// are bounded and errors never echo them.
func ConfigFromEnv() (Config, error) {
	port, err := strconv.Atoi(envOr("PGPORT", "5432"))
	if err != nil || port < 1 || port > 65535 {
		return Config{}, errors.New("jobstore: PGPORT must be a port number")
	}
	cfg := Config{
		Host:     os.Getenv("PGHOST"),
		Port:     port,
		Database: os.Getenv("PGDATABASE"),
		User:     os.Getenv("PGUSER"),
		Password: os.Getenv("PGPASSWORD"),
	}
	if cfg.Host == "" || cfg.Database == "" || cfg.User == "" {
		return Config{}, errors.New("jobstore: PGHOST, PGDATABASE and PGUSER are required")
	}
	for name, value := range map[string]string{"PGHOST": cfg.Host, "PGDATABASE": cfg.Database, "PGUSER": cfg.User} {
		if len(value) > maxFieldLen || strings.ContainsFunc(value, isControl) {
			return Config{}, fmt.Errorf("jobstore: %s is too long or contains control characters", name)
		}
	}
	if len(cfg.Password) > maxPasswordLen || strings.ContainsFunc(cfg.Password, isControl) {
		return Config{}, errors.New("jobstore: PGPASSWORD is too long or contains control characters")
	}
	return cfg, nil
}

func isControl(r rune) bool { return r < 0x20 || r == 0x7f }

// DSN renders the connection URL. Every component is escaped, so reserved
// characters in the user name, password or database name cannot change the
// meaning of the URL. It is never logged.
func (c Config) DSN() string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(c.User, c.Password),
		Host:     net.JoinHostPort(c.Host, strconv.Itoa(c.Port)),
		Path:     "/" + c.Database,
		RawQuery: "sslmode=prefer&connect_timeout=5",
	}
	return u.String()
}

// Store owns the connection pool.
type Store struct {
	pool *pgxpool.Pool
}

// Open connects with retries so a workload can start before PostgreSQL accepts
// connections.
func Open(ctx context.Context, cfg Config, attempts int) (*Store, error) {
	var lastErr error
	for i := 0; i < attempts; i++ {
		pool, err := pgxpool.New(ctx, cfg.DSN())
		if err == nil {
			pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err = pool.Ping(pingCtx)
			cancel()
			if err == nil {
				return &Store{pool: pool}, nil
			}
			pool.Close()
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return nil, fmt.Errorf("jobstore: database %s:%d is not reachable: %w", cfg.Host, cfg.Port, lastErr)
}

// Close releases the pool.
func (s *Store) Close() { s.pool.Close() }

// Ping reports database readiness.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// Migrate creates the jobs table when it is missing.
func (s *Store) Migrate(ctx context.Context) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("jobstore: migrate: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// The backend and the worker both migrate at start; serialize them.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7040901)`); err != nil {
		return fmt.Errorf("jobstore: migrate: %w", err)
	}
	if _, err := tx.Exec(ctx, `
CREATE TABLE IF NOT EXISTS jobs (
  id           bigserial PRIMARY KEY,
  payload      text NOT NULL,
  status       text NOT NULL DEFAULT 'PENDING',
  result       text,
  processed_by text,
  created_at   timestamptz NOT NULL DEFAULT now(),
  processed_at timestamptz
)`); err != nil {
		return fmt.Errorf("jobstore: migrate: %w", err)
	}
	if _, err := tx.Exec(ctx, `ALTER TABLE jobs ADD COLUMN IF NOT EXISTS cache_write text`); err != nil {
		return fmt.Errorf("jobstore: migrate: %w", err)
	}
	if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS acceptance_scheduled_runs (
run_id text NOT NULL, invocation text NOT NULL, value_sha256 text NOT NULL,
completed_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY (run_id, invocation))`); err != nil {
		return errors.New("jobstore: scheduled receipt migration failed")
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("jobstore: migrate: %w", err)
	}
	return nil
}

// Submit inserts a pending job.
func (s *Store) Submit(ctx context.Context, payload string) (Job, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO jobs (payload, status) VALUES ($1, 'PENDING') RETURNING id, payload, status, created_at`, payload)
	var job Job
	if err := row.Scan(&job.ID, &job.Payload, &job.Status, &job.CreatedAt); err != nil {
		return Job{}, fmt.Errorf("jobstore: submit: %w", err)
	}
	return job, nil
}

// Get reads one job by id.
func (s *Store) Get(ctx context.Context, id int64) (Job, bool, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, payload, status, coalesce(result, ''), coalesce(processed_by, ''), created_at, processed_at FROM jobs WHERE id = $1`, id)
	var job Job
	if err := row.Scan(&job.ID, &job.Payload, &job.Status, &job.Result, &job.ProcessedBy, &job.CreatedAt, &job.ProcessedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Job{}, false, nil
		}
		return Job{}, false, fmt.Errorf("jobstore: get: %w", err)
	}
	return job, true, nil
}

// List returns the most recent jobs.
func (s *Store) List(ctx context.Context, limit int) ([]Job, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, payload, status, coalesce(result, ''), coalesce(processed_by, ''), created_at, processed_at
		 FROM jobs ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("jobstore: list: %w", err)
	}
	defer rows.Close()
	jobs := []Job{}
	for rows.Next() {
		var job Job
		if err := rows.Scan(&job.ID, &job.Payload, &job.Status, &job.Result, &job.ProcessedBy, &job.CreatedAt, &job.ProcessedAt); err != nil {
			return nil, fmt.Errorf("jobstore: scan: %w", err)
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// ListSummaries returns the most recent jobs without the result columns of
// completed jobs, so a reader resolves each completed result through the cache
// first. Pending jobs are always read from PostgreSQL.
func (s *Store) ListSummaries(ctx context.Context, limit int) ([]Job, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, payload, status, created_at FROM jobs ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("jobstore: list: %w", err)
	}
	defer rows.Close()
	jobs := []Job{}
	for rows.Next() {
		var job Job
		if err := rows.Scan(&job.ID, &job.Payload, &job.Status, &job.CreatedAt); err != nil {
			return nil, fmt.Errorf("jobstore: scan: %w", err)
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// RecordCacheWrite stores the outcome of the worker's write-through attempt for
// a completed job. It never changes the job status or result.
func (s *Store) RecordCacheWrite(ctx context.Context, id int64, ok bool) error {
	marker := CacheWriteFailed
	if ok {
		marker = CacheWriteOK
	}
	if _, err := s.pool.Exec(ctx, `UPDATE jobs SET cache_write = $2 WHERE id = $1 AND status = 'DONE'`, id, marker); err != nil {
		return fmt.Errorf("jobstore: record cache write: %w", err)
	}
	return nil
}

// CacheWriteCounts counts the completed jobs by the worker's recorded
// write-through outcome. unverified are completed jobs with no marker (the
// worker crashed or could not record it): nothing proves their cache write.
func (s *Store) CacheWriteCounts(ctx context.Context) (okCount, failedCount, unverified int64, err error) {
	row := s.pool.QueryRow(ctx, `SELECT
		count(*) FILTER (WHERE cache_write = 'OK'),
		count(*) FILTER (WHERE cache_write = 'FAILED'),
		count(*) FILTER (WHERE cache_write IS NULL OR cache_write NOT IN ('OK', 'FAILED'))
		FROM jobs WHERE status = 'DONE'`)
	if err := row.Scan(&okCount, &failedCount, &unverified); err != nil {
		return 0, 0, 0, fmt.Errorf("jobstore: cache write counts: %w", err)
	}
	return okCount, failedCount, unverified, nil
}

// CountJobs returns the number of rows in the jobs table.
func (s *Store) CountJobs(ctx context.Context) (int64, error) {
	var n int64
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM jobs`).Scan(&n); err != nil {
		return 0, fmt.Errorf("jobstore: count: %w", err)
	}
	return n, nil
}

// ClaimAndProcess takes one pending job, writes its result and returns it.
func (s *Store) ClaimAndProcess(ctx context.Context, workerID string, transform func(string) string) (Job, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Job{}, false, fmt.Errorf("jobstore: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row := tx.QueryRow(ctx,
		`SELECT id, payload FROM jobs WHERE status = 'PENDING' ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED`)
	var id int64
	var payload string
	if err := row.Scan(&id, &payload); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Job{}, false, nil
		}
		return Job{}, false, fmt.Errorf("jobstore: claim: %w", err)
	}

	result := transform(payload)
	updated := tx.QueryRow(ctx,
		`UPDATE jobs SET status = 'DONE', result = $2, processed_by = $3, processed_at = now()
		 WHERE id = $1 RETURNING id, payload, status, result, processed_by, created_at, processed_at`,
		id, result, workerID)
	var job Job
	if err := updated.Scan(&job.ID, &job.Payload, &job.Status, &job.Result, &job.ProcessedBy, &job.CreatedAt, &job.ProcessedAt); err != nil {
		return Job{}, false, fmt.Errorf("jobstore: complete: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Job{}, false, fmt.Errorf("jobstore: commit: %w", err)
	}
	return job, true, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// EnvOr exposes the environment helper to the acceptance workloads.
func EnvOr(key, fallback string) string { return envOr(key, fallback) }
