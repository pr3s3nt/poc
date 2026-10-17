package postgres

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"orchestrator/internal/ports/persistence"
)

// A commit the server rejected rolled back; anything else (broken connection,
// cancelled context) may have committed and must be reported as uncertain.
func TestCommitErrorSeparatesKnownRollbackFromUnknownOutcome(t *testing.T) {
	for name, err := range map[string]error{
		"lost connection": io.ErrUnexpectedEOF, "cancelled": context.Canceled, "deadline": context.DeadlineExceeded,
	} {
		if got := commitError(err); !errors.Is(got, persistence.ErrCommitUncertain) {
			t.Errorf("%s: %v", name, got)
		}
	}
	if got := commitError(&pgconn.PgError{Code: "40001"}); !errors.Is(got, persistence.ErrVersionConflict) || errors.Is(got, persistence.ErrCommitUncertain) {
		t.Errorf("serialization failure: %v", got)
	}
	if got := commitError(&pgconn.PgError{Code: "23503", ConstraintName: "x"}); errors.Is(got, persistence.ErrCommitUncertain) {
		t.Errorf("deferred constraint failure is definite: %v", got)
	}
	if got := commitError(pgx.ErrTxCommitRollback); errors.Is(got, persistence.ErrCommitUncertain) {
		t.Errorf("rolled-back marker is definite: %v", got)
	}
}
