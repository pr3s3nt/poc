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

// txKey identifies the transaction of one pool in a context. It is keyed by
// the pool, so a transaction of the Console connection is never reused by the
// State connection (or the reverse) when contexts are passed between them.
type txKey struct{ db *db }

// db is one PostgreSQL connection pool of one runtime login role.
type db struct{ pool *pgxpool.Pool }

// Store is the normalized PostgreSQL persistence adapter of the Console
// runtime role. It never reads Terraform state blobs or capability hashes;
// the database refuses them to its login (see grants.go).
type Store struct {
	db
	identity Identity
}

// Open connects the Console runtime role and validates, without any DDL,
// that the database holds the exact baseline created by the owner setup and
// that the login has exactly the Console grants. It never initializes an
// empty database and never accepts an owner, superuser or State login.
func Open(ctx context.Context, databaseURL string) (*Store, error) {
	d, identity, err := openRuntime(ctx, databaseURL, ConsoleRuntime)
	if err != nil {
		return nil, err
	}
	return &Store{db: d, identity: identity}, nil
}

// Identity is the validated installation the Console connection serves.
func (s *Store) Identity() Identity { return s.identity }

func connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("postgres: empty database URL")
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		// The parse error may echo the URL, which can carry a password.
		return nil, fmt.Errorf("postgres: the database URL cannot be parsed")
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: open pool: %w", err)
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	return pool, nil
}

func (d *db) Close() { d.pool.Close() }

func (d *db) q(ctx context.Context) querier {
	if tx, ok := ctx.Value(txKey{d}).(pgx.Tx); ok {
		return tx
	}
	return d.pool
}

func (d *db) Transact(ctx context.Context, fn func(context.Context) error) error {
	if _, nested := ctx.Value(txKey{d}).(pgx.Tx); nested {
		return fn(ctx)
	}
	tx, err := d.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("postgres: begin: %w", err)
	}
	defer tx.Rollback(ctx) // no-op after commit
	if err = fn(context.WithValue(ctx, txKey{d}, tx)); err != nil {
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
	if _, nested := ctx.Value(txKey{&s.db}).(pgx.Tx); nested {
		return fn(ctx, s)
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("postgres: begin read snapshot: %w", err)
	}
	// Nothing may be kept from a read, so the transaction always rolls back.
	defer tx.Rollback(ctx)
	return fn(context.WithValue(ctx, txKey{&s.db}, tx), s)
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
