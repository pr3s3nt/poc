// Package postgres implements the persistence ports with normalized PostgreSQL tables.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"orchestrator/internal/ports/persistence"
)

type querier interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type txKey struct{}

// Store is a normalized PostgreSQL persistence adapter.
type Store struct{ pool *pgxpool.Pool }

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("postgres: empty database URL")
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse database URL: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: open pool: %w", err)
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	if err = initialize(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) q(ctx context.Context) querier {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return s.pool
}

func (s *Store) Transact(ctx context.Context, fn func(context.Context) error) error {
	if _, nested := ctx.Value(txKey{}).(pgx.Tx); nested {
		return fn(ctx)
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("postgres: begin: %w", err)
	}
	defer tx.Rollback(ctx) // no-op after commit
	if err = fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return translate(err)
	}
	return nil
}

// ReadSnapshot runs fn in one REPEATABLE READ READ ONLY transaction, so every
// query sees the same committed snapshot and no GET can persist a change.
// Inside Transact, fn reads the enclosing transaction.
func (s *Store) ReadSnapshot(ctx context.Context, fn func(context.Context, persistence.Store) error) error {
	if _, nested := ctx.Value(txKey{}).(pgx.Tx); nested {
		return fn(ctx, s)
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("postgres: begin read snapshot: %w", err)
	}
	// Nothing may be kept from a read, so the transaction always rolls back.
	defer tx.Rollback(ctx)
	return fn(context.WithValue(ctx, txKey{}, tx), s)
}

func translate(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return persistence.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf("%w: %s", persistence.ErrImmutable, pgErr.ConstraintName)
		case "23503":
			return fmt.Errorf("%w: %s", persistence.ErrNotFound, pgErr.ConstraintName)
		case "40001":
			return persistence.ErrVersionConflict
		}
	}
	return err
}

func jsonBytes(v any) ([]byte, error) { return marshalJSON(v) }

var _ persistence.Store = (*Store)(nil)
