package v03run

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

// The receipt readers run against a real PostgreSQL when ORCHESTRATOR_POSTGRES_TEST_URL names a
// disposable server (REQUIRE_POSTGRES=1 makes its absence a failure). The tables mirror the
// product baseline columns the Console login may read.
func TestReceiptReadersReadTheProductRecordsAndFailOnAnError(t *testing.T) {
	admin := os.Getenv("ORCHESTRATOR_POSTGRES_TEST_URL")
	if admin == "" {
		if os.Getenv("REQUIRE_POSTGRES") == "1" {
			t.Fatal("ORCHESTRATOR_POSTGRES_TEST_URL is required")
		}
		t.Skip("no PostgreSQL test URL")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	schema := "v03_receipts_" + randomSuffix()
	if _, err := conn.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`) }()
	if _, err := conn.Exec(ctx, `SET search_path TO `+schema); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE terraform_resource_instances (descriptor text, state_key text, created_by_run_id text)`,
		`CREATE TABLE organizations (id int, organization_key text)`,
		`CREATE TABLE connections (organization_id int, connection_key text, secret_ref text)`,
		`INSERT INTO terraform_resource_instances VALUES
		  ('k8s-namespace.default#environments.app.dev', repeat('a',64), 'tfrun-1'),
		  ('k8s-namespace.default#environments.app.dev-aux', repeat('b',64), 'tfrun-2'),
		  ('postgres.default#shared.db', repeat('c',64), 'tfrun-3')`,
		`CREATE TABLE workload_instances (workload_id text, status text, target_ref jsonb)`,
		`INSERT INTO workload_instances VALUES ('frontend','READY','{"connection":"lab-1","cluster":"c","namespace":"ns","placement":{"account":"lab-1","accountVersion":2,"clusterDefinition":"cl","clusterDefinitionVersion":3,"cluster":"k8s-cluster.internal#environments.a.b"}}'), ('bad','READY','{}')`,
		`INSERT INTO organizations VALUES (1,'acme')`,
		`INSERT INTO connections VALUES (1,'lab-1','kv2://secret/orchestrator/connections/acme/lab-1/credentials/11111111-1111-4111-8111-111111111111')`,
	} {
		if _, err := conn.Exec(ctx, statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	receipts, err := ReadNamespaceReceipts(ctx, conn)
	if err != nil || len(receipts) != 2 || receipts[0].Instance != "aaaaaaaaaaaaaaaa" || receipts[0].TerraformRun != "tfrun-1" || receipts[1].Instance != "bbbbbbbbbbbbbbbb" {
		t.Fatalf("receipts = %+v err=%v", receipts, err)
	}
	refs, err := ReadConnectionRefs(ctx, conn)
	if err != nil || len(refs) != 1 || refs[0].Key != "lab-1" || refs[0].Organization != "acme" {
		t.Fatalf("refs = %+v err=%v", refs, err)
	}
	if _, err := ReadAppliedTargets(ctx, conn); err == nil {
		t.Fatal("a workload instance without a pinned placement must be an error")
	}
	if _, err := conn.Exec(ctx, `DELETE FROM workload_instances WHERE workload_id='bad'`); err != nil {
		t.Fatal(err)
	}
	targets, err := ReadAppliedTargets(ctx, conn)
	if err != nil || len(targets) != 1 || targets[0].Account != "lab-1" || targets[0].AccountVersion != 2 || targets[0].ClusterDefinitionVersion != 3 || targets[0].Namespace != "ns" {
		t.Fatalf("targets = %+v err=%v", targets, err)
	}
	// A broken schema is an error, never an empty list.
	if _, err := conn.Exec(ctx, `DROP TABLE terraform_resource_instances`); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadNamespaceReceipts(ctx, conn); err == nil || got != nil {
		t.Fatalf("a failed read must be an error: %v %v", got, err)
	}
}
