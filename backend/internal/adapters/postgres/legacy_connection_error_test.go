package postgres

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// Migration 8 rejects legacy Kubernetes rows with a safe reset instruction;
// other failures stay generic and keep their cause.
func TestLegacyConnectionDataClassification(t *testing.T) {
	violation := fmt.Errorf("exec: %w", &pgconn.PgError{Code: "23514", ConstraintName: "connections_authentication_type_check", Message: `check constraint "connections_authentication_type_check" of relation "connections" is violated by some row`, Detail: "secret-host-detail"})
	if !isLegacyConnectionData(violation) {
		t.Fatal("constraint violation not classified")
	}
	for name, err := range map[string]error{
		"other constraint": &pgconn.PgError{Code: "23514", ConstraintName: "environments_binding_consistent"},
		"other code":       &pgconn.PgError{Code: "42P01", ConstraintName: "connections_authentication_type_check"},
		"plain":            errors.New("boom"),
	} {
		if isLegacyConnectionData(err) {
			t.Errorf("%s misclassified", name)
		}
	}
	text := ErrLegacyConnectionData.Error()
	for _, leak := range []string{"23514", "constraint", "HOST_CONTEXT", "secret-host-detail", "SELECT"} {
		if strings.Contains(text, leak) {
			t.Errorf("error leaks %q: %s", leak, text)
		}
	}
	if !strings.Contains(text, "reset the development database") {
		t.Errorf("no reset guidance: %s", text)
	}
}
