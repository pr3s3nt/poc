package postgres

import (
	"context"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"orchestrator/internal/ports/persistence/persistencetest"
)

// databases maps a Console URL to its disposable database, so a test that
// only holds the Console URL can still arrange faults as the administrator.
var databases sync.Map

// freshDatabase provisions fresh option-A roles and runs the owner setup.
// Runtime stores under test connect with the Console or State login only.
func freshDatabase(t *testing.T) persistencetest.PostgresDatabase {
	t.Helper()
	db := persistencetest.FreshPostgresRoles(t)
	if _, err := Setup(context.Background(), db.Owner, Roles{Console: db.ConsoleRole, State: db.StateRole}); err != nil {
		t.Fatalf("owner setup: %v", err)
	}
	databases.Store(db.Console, db)
	return db
}

// openFresh opens the Console store of a fresh database and returns it with
// the Console URL.
func openFresh(t *testing.T) (*Store, string) {
	t.Helper()
	db := freshDatabase(t)
	st, err := Open(context.Background(), db.Console)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	return st, db.Console
}

// adminExec runs fault-injection statements (test-only DDL) as the test
// server administrator on the database behind consoleURL. Runtime logins can
// never run DDL.
func adminExec(t *testing.T, consoleURL string, statements ...string) {
	t.Helper()
	v, ok := databases.Load(consoleURL)
	if !ok {
		t.Fatal("unknown test database")
	}
	sqlOn(t, v.(persistencetest.PostgresDatabase).Admin, statements...)
}

func sqlOn(t *testing.T, url string, statements ...string) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	for _, sql := range statements {
		if _, err := conn.Exec(ctx, sql); err != nil {
			t.Fatalf("%v\n%s", err, sql)
		}
	}
}
