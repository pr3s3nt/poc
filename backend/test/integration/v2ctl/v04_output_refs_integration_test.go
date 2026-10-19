package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/ports/persistence/persistencetest"
	"orchestrator/test/integration/v04run"
)

// Runs the production query against a fresh option-A baseline, verifies the
// Console remains denied, then exercises exact run ownership and attempt history
// through the separate private owner login. This regression requires real PG.
func TestV04OutputRefsInventoryRealPostgresRolesHistoryAndEmpty(t *testing.T) {
	ctx := context.Background()
	db := persistencetest.FreshPostgresRoles(t)
	if _, err := postgres.Setup(ctx, db.Owner, postgres.Roles{Console: db.ConsoleRole, State: db.StateRole}); err != nil {
		t.Fatalf("owner setup: %v", err)
	}
	owner, err := pgx.Connect(ctx, db.Owner)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(ctx)
	console, err := pgx.Connect(ctx, db.Console)
	if err != nil {
		t.Fatal(err)
	}
	defer console.Close(ctx)
	state, err := pgx.Connect(ctx, db.State)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close(ctx)

	rows, err := console.Query(ctx, `SELECT physical_intent FROM terraform_resource_instances`)
	if err == nil {
		for rows.Next() {
		}
		err = rows.Err()
		rows.Close()
	}
	var pgErr *pgconn.PgError
	if err == nil || !asPgError(err, &pgErr) || pgErr.Code != "42501" {
		t.Fatalf("Console physical_intent read must stay denied (42501); got %v", err)
	}
	stateRows, err := state.Query(ctx, `SELECT physical_intent FROM terraform_resource_instances`)
	if err != nil {
		t.Fatalf("State owner handle must retain its existing narrow-purpose access: %v", err)
	}
	stateRows.Close()
	// Reopen's safe full-instance inventory must retain the Console's narrow
	// grants while joining the verified module identity on the real baseline.
	if got, err := v04run.ReadInstances(ctx, console); err != nil || len(got) != 0 {
		t.Fatalf("Console safe instance/module inventory: count=%d err=%v", len(got), err)
	}

	// The exact production SQL must prepare and return a complete empty result
	// on the real baseline when run as the private owner, before any provider writes.
	emptyRows, err := owner.Query(ctx, v04OutputRefQuery, "run-with-no-output-write")
	if err != nil {
		t.Fatalf("owner inventory query on production baseline: %v", err)
	}
	if emptyRows.Next() {
		t.Fatal("fresh baseline unexpectedly contains a run-owned intent")
	}
	if err := emptyRows.Err(); err != nil {
		t.Fatal(err)
	}
	emptyRows.Close()

	schema := fmt.Sprintf("v04_output_refs_%d", time.Now().UnixNano())
	if _, err := owner.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, "SET search_path TO "+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE terraform_resource_instances(id text, active_resource_id text, descriptor text, physical_intent jsonb, created_by_run_id text)`,
		`ALTER TABLE terraform_resource_instances ADD COLUMN state_key text DEFAULT repeat('a',64), ADD COLUMN status text DEFAULT 'DESTROYED', ADD COLUMN scope_type text DEFAULT 'SHARED', ADD COLUMN scope_id text DEFAULT 'fixture', ADD COLUMN module_bundle_id text DEFAULT 'verified-module'`,
		`CREATE TABLE terraform_module_bundles(id text, module_id text)`,
		`INSERT INTO terraform_module_bundles VALUES ('verified-module','postgres-k8s')`,
		`CREATE TABLE active_resources(id text, outputs jsonb)`,
		`CREATE TABLE terraform_resource_attempts(resource_instance_id text, run_id text, deployment_id text)`,
		`CREATE TABLE deployment_resources(deployment_id text, node_descriptor text, active_resource_id text, output_snapshot jsonb, PRIMARY KEY(deployment_id,node_descriptor))`,
		`INSERT INTO terraform_resource_instances(id, active_resource_id, descriptor, physical_intent, created_by_run_id) VALUES
		 ('owned-g1',NULL,'db.shared#resource','{"tags":{"run-id":"operator-run"},"secretRefs":{"intent":"secret://vault/v1/orchestrator/outputs/intent"}}','pending-pin-hash-one'),
		 ('new-owned',NULL,'cache.shared#resource','{"tags":{"run-id":"operator-run"},"secretRefs":{}}','pending-new'),
		 ('owned-g2','current-active','db.shared#resource','{"tags":{"run-id":"operator-run"},"secretRefs":{}}','pending-pin-hash-two'),
		 ('foreign','foreign-active','db.foreign#resource','{"tags":{"run-id":"foreign-run"},"secretRefs":{"intent":"secret://vault/v1/orchestrator/outputs/foreign-intent"}}','operator-run')`,
		`INSERT INTO active_resources VALUES
		 ('new-active','{"secretRef":"secret://vault/v1/orchestrator/outputs/new-instance-active"}'),
		 ('current-active','{"password":{"secretRef":"secret://vault/v1/orchestrator/outputs/current-output"}}'),
		 ('foreign-active','{"password":{"secretRef":"secret://vault/v1/orchestrator/outputs/foreign-active"}}')`,
		`INSERT INTO terraform_resource_attempts VALUES
		 ('new-owned','pending-new','created-deployment'),
		 ('owned-g1','pending-request-one','same-deployment'),
		 ('owned-g2','pending-request-two','new-deployment'),
		 ('foreign','pending-foreign','same-deployment')`,
		`INSERT INTO deployment_resources VALUES
		 ('created-deployment','cache.shared#resource','new-active','{"secretRef":"secret://vault/v1/orchestrator/outputs/new-instance-snapshot"}'),
		 ('reused-no-attempt','cache.shared#resource','new-active','{"secretRef":"secret://vault/v1/orchestrator/outputs/reuse-without-attempt"}'),
		 ('foreign-same-descriptor','cache.shared#resource','foreign-active','{"secretRef":"secret://vault/v1/orchestrator/outputs/foreign-same-descriptor"}'),
		 ('same-deployment','db.shared#resource',NULL,'{"nested":[{"secretRef":"secret://vault/v1/orchestrator/outputs/old-generation-history"}]}'),
		 ('same-deployment','db.foreign#resource','foreign-active','{"secretRef":"secret://vault/v1/orchestrator/outputs/foreign-history"}'),
		 ('new-deployment','db.shared#resource','current-active','{"secretRef":"secret://vault/v1/orchestrator/outputs/new-generation-history"}')`,
	} {
		if _, err := owner.Exec(ctx, q); err != nil {
			t.Fatalf("fixture SQL: %v", err)
		}
	}
	instances, err := v04run.ReadInstances(ctx, owner)
	if err != nil || len(instances) != 4 {
		t.Fatalf("full instance/module inventory: count=%d err=%v", len(instances), err)
	}
	for _, in := range instances {
		if in.Module != "postgres-k8s" || in.Status != "DESTROYED" || in.StateKey != "aaaaaaaaaaaa" || in.Scope != "SHARED" || in.ID == "" || in.CreatedByRun == "" {
			t.Fatalf("safe instance/module identity incomplete: %+v", in)
		}
	}

	got := map[string]bool{}
	result, err := owner.Query(ctx, v04OutputRefQuery, "operator-run")
	if err != nil {
		t.Fatal(err)
	}
	for result.Next() {
		var doc map[string]any
		if err := result.Scan(&doc); err != nil {
			result.Close()
			t.Fatal(err)
		}
		collectOutputRefs(doc, got)
	}
	if err := result.Err(); err != nil {
		result.Close()
		t.Fatal(err)
	}
	result.Close()
	for _, id := range []string{"intent", "old-generation-history", "new-generation-history", "current-output", "new-instance-active", "new-instance-snapshot", "reuse-without-attempt"} {
		if !got["secret://vault/v1/orchestrator/outputs/"+id] {
			t.Errorf("missing owned output-ref class %q", id)
		}
	}
	for ref := range got {
		if ref == "secret://vault/v1/orchestrator/outputs/foreign-intent" || ref == "secret://vault/v1/orchestrator/outputs/foreign-active" || ref == "secret://vault/v1/orchestrator/outputs/foreign-history" {
			t.Errorf("foreign run leaked into inventory: %s", ref)
		}
	}
	if len(got) != 7 {
		t.Fatalf("inventory size %d, want exactly seven owned refs", len(got))
	}
}

func asPgError(err error, out **pgconn.PgError) bool {
	return errors.As(err, out)
}

func TestV04OutputCleanAcceptsVerifiedEmptyInventory(t *testing.T) {
	file := t.TempDir() + "/refs.json"
	if err := os.WriteFile(file, []byte("[]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if code := v04OutputClean([]string{"-refs-file", file, "-token-file", "/missing/private/token"}); code != 0 {
		t.Fatalf("verified empty inventory should clean as a no-op, exit=%d", code)
	}
}

func TestV04OutputCleanRejectsMalformedOrNullInventory(t *testing.T) {
	for _, body := range []string{"null", "{}", "[", ""} {
		t.Run(fmt.Sprintf("bytes-%d-%s", len(body), body), func(t *testing.T) {
			file := t.TempDir() + "/refs.json"
			if err := os.WriteFile(file, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			if code := v04OutputClean([]string{"-refs-file", file, "-token-file", "/missing/private/token"}); code == 0 {
				t.Fatal("malformed/unproven inventory cannot mean an empty cleanup")
			}
		})
	}
}
