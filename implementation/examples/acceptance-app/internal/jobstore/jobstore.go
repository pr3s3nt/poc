// Package jobstore is the shared PostgreSQL access layer of the acceptance
// application. The backend and the worker both bind to the same shared database
// resource, so they use the same schema and the same queries.
package jobstore

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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

// ConfigFromEnv reads PGHOST, PGPORT, PGDATABASE, PGUSER and PGPASSWORD.
func ConfigFromEnv() (Config, error) {
	port, err := strconv.Atoi(envOr("PGPORT", "5432"))
	if err != nil {
		return Config{}, fmt.Errorf("jobstore: invalid PGPORT: %w", err)
	}
	cfg := Config{
		Host:     os.Getenv("PGHOST"),
		Port:     port,
		Database: os.Getenv("PGDATABASE"),
		User:     os.Getenv("PGUSER"),
		Password: os.Getenv("PGPASSWORD"),
	}
	if cfg.Host == "" || cfg.Database == "" || cfg.User == "" {
		return Config{}, fmt.Errorf("jobstore: PGHOST, PGDATABASE and PGUSER are required")
	}
	return cfg, nil
}

// DSN renders the connection string. It is never logged.
func (c Config) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=prefer&connect_timeout=5",
		c.User, c.Password, c.Host, c.Port, c.Database)
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
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS jobs (
  id           bigserial PRIMARY KEY,
  payload      text NOT NULL,
  status       text NOT NULL DEFAULT 'PENDING',
  result       text,
  processed_by text,
  created_at   timestamptz NOT NULL DEFAULT now(),
  processed_at timestamptz
)`)
	if err != nil {
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
		if err.Error() == "no rows in result set" {
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
		if err.Error() == "no rows in result set" {
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
