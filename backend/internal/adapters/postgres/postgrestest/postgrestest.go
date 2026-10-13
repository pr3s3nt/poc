// Package postgrestest provisions disposable option-A PostgreSQL databases for
// tests outside the postgres package: fresh roles, then the explicit owner
// setup. Runtime code under test connects with the Console or State login.
package postgrestest

import (
	"context"
	"testing"

	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/ports/persistence/persistencetest"
)

// Fresh returns a database initialized by the owner setup with the Console
// and State runtime roles granted. It skips when no test server is
// configured, and fails instead under REQUIRE_POSTGRES=1.
func Fresh(t *testing.T) persistencetest.PostgresDatabase {
	t.Helper()
	db := persistencetest.FreshPostgresRoles(t)
	if _, err := postgres.Setup(context.Background(), db.Owner, postgres.Roles{Console: db.ConsoleRole, State: db.StateRole}); err != nil {
		t.Fatalf("owner setup: %v", err)
	}
	return db
}

// OpenConsole opens the Console runtime store and closes it after the test.
func OpenConsole(t *testing.T, db persistencetest.PostgresDatabase) *postgres.Store {
	t.Helper()
	st, err := postgres.Open(context.Background(), db.Console)
	if err != nil {
		t.Fatalf("open console store: %v", err)
	}
	t.Cleanup(st.Close)
	return st
}
