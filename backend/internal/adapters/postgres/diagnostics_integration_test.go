package postgres

import (
	"context"
	"github.com/jackc/pgx/v5"
	"strings"
	"testing"

	"orchestrator/internal/ports/persistence/persistencetest"
)

func TestPostgresDiagnosticsContractSurvivesReopen(t *testing.T) {
	first, url := openFresh(t)
	fixture := persistencetest.Diagnostics(t, first)
	first.Close()
	reopened, err := Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	persistencetest.AssertDiagnostics(t, reopened, fixture)
}

// The baseline itself refuses a terminal findings rewrite, a general scope
// carrying use-case fields and an account of another Organization, whatever
// the repository does.
func TestPostgresDiagnosticsConstraintsHoldBelowTheRepository(t *testing.T) {
	st, url := openFresh(t)
	fixture := persistencetest.Diagnostics(t, st)
	for name, statement := range map[string]string{
		"terminal findings": `UPDATE account_diagnostic_reports SET findings='[]'::jsonb WHERE id='` + fixture.Completed.ID + `'`,
		"scope rewrite":     `UPDATE account_diagnostic_reports SET account_version=99 WHERE id='` + fixture.Completed.ID + `'`,
		"scope document":    `UPDATE account_diagnostic_reports SET scope = scope || '{"useCases":["deploy"]}'::jsonb WHERE id='` + fixture.Completed.ID + `'`,
	} {
		if err := adminTryExec(t, url, statement); err == nil || !strings.Contains(err.Error(), "immutable") {
			t.Fatalf("%s was not refused: %v", name, err)
		}
	}
	if err := adminTryExec(t, url, `INSERT INTO account_diagnostic_reports(id,organization_id,account_id,created_by,account_version,purpose,mode,scope,status,revision,cleanup_status,created_at,updated_at)
SELECT gen_random_uuid(),o.id,c.id,u.id,1,'GENERAL','CONDITIONS','{"useCases":["deploy"]}'::jsonb,'COMPLETED',1,'NOT_REQUIRED',now(),now() FROM organizations o, connections c, user_accounts u
WHERE o.organization_key='diag-acme' AND c.connection_key='k8s-b' AND u.organization_id=o.id`); err == nil {
		t.Fatal("a GENERAL report with use cases was accepted")
	}
	err := adminTryExec(t, url, `INSERT INTO account_diagnostic_reports(id,organization_id,account_id,created_by,account_version,purpose,mode,scope,status,revision,cleanup_status,created_at,updated_at)
SELECT gen_random_uuid(),o.id,c.id,u.id,1,'GENERAL','CONDITIONS','{}'::jsonb,'COMPLETED',1,'NOT_REQUIRED',now(),now() FROM organizations o, connections c, user_accounts u
WHERE o.organization_key='diag-globex' AND c.connection_key='k8s-a' AND u.organization_id=o.id`)
	if err == nil {
		t.Fatal("a report of another Organization's account was accepted")
	}
	err = adminTryExec(t, url, `INSERT INTO account_diagnostic_reports(id,organization_id,account_id,created_by,account_version,purpose,mode,scope,status,revision,cleanup_status,created_at,updated_at)
SELECT gen_random_uuid(),o.id,c.id,u.id,1,'GENERAL','EXECUTION','{}'::jsonb,'COMPLETED',1,'NOT_REQUIRED',now(),now() FROM organizations o, connections c, user_accounts u
WHERE o.organization_key='diag-acme' AND c.connection_key='k8s-a' AND u.organization_id=o.id`)
	if err == nil {
		t.Fatal("a GENERAL execution report was accepted")
	}
}

// adminTryExec runs one statement as the test administrator and returns its
// error, so a test can prove that the database itself refuses it.
func adminTryExec(t *testing.T, consoleURL, statement string) error {
	t.Helper()
	v, ok := databases.Load(consoleURL)
	if !ok {
		t.Fatal("unknown test database")
	}
	conn, err := pgx.Connect(context.Background(), v.(persistencetest.PostgresDatabase).Admin)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	_, err = conn.Exec(context.Background(), statement)
	return err
}
