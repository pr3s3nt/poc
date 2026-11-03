package postgres

import (
	"context"
	"errors"
	"orchestrator/internal/domain/environment"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/persistence"
)

func TestVersionSixCapturesPopulatedFiveAndProtectsSnapshots(t *testing.T) {
	ctx := context.Background()
	owner, console, roles := legacyDatabase(t, versionFiveSQL)
	sqlOn(t, owner,
		`INSERT INTO environments(id,application_id,environment_key,name,environment_type,namespace_identity,version) VALUES('77777777-7777-4777-8777-777777777777','33333333-3333-4333-8333-333333333333','staging','Staging','staging','retained-staging',9)`,
		`INSERT INTO resource_types VALUES('88888888-8888-4888-8888-888888888888','22222222-2222-4222-8222-222222222222','example','{}','{}')`,
		`INSERT INTO resource_definitions(id,organization_id,definition_key,resource_type_id,definition_name,driver_type,driver_inputs,provision_rules,version) VALUES('99999999-9999-4999-8999-999999999999','22222222-2222-4222-8222-222222222222','retained-definition','88888888-8888-4888-8888-888888888888','Retained Definition','echo','{"values":{"variables":{"size":"small"}}}','{}',4)`,
		`INSERT INTO matching_criteria VALUES('aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa','99999999-9999-4999-8999-999999999999',0,'staging','retained-app',NULL,NULL,'default',19)`)
	before := retainedFingerprints(t, owner)
	if _, err := Setup(ctx, owner, roles); err != nil {
		t.Fatal(err)
	}
	st, err := Open(ctx, console)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	h, err := st.GetDefinitionHistory(ctx, "retained-org", "retained-definition")
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Versions) != 1 || h.ActiveID != h.Versions[0].ID || h.Versions[0].Definition.Version != 4 || h.Versions[0].Definition.Criteria[0].ApplicationID != "retained-app" {
		t.Fatalf("incorrect captured history: %+v", h)
	}
	selected, err := st.GetDefinitionSelection(ctx, "retained-app", "staging")
	if err != nil {
		t.Fatal(err)
	}
	if !selected.Frozen || selected.AppliedVerified || selected.Pending() || len(selected.Desired) != 1 || selected.Desired[0].RevisionID != h.ActiveID {
		t.Fatalf("invented deployment evidence or missing baseline: %+v", selected)
	}
	if _, err := Setup(ctx, owner, roles); err != nil {
		t.Fatal(err)
	}
	for table, hash := range before {
		if retainedFingerprints(t, owner)[table] != hash {
			t.Errorf("retained rows changed in %s", table)
		}
	}
	d := h.Versions[0].Definition
	d.Name = "Proposed"
	v := resource.NewDefinitionRevision(d, 2)
	h.Versions = append(h.Versions, v)
	h.Generation++
	if err := st.SaveDefinitionHistory(ctx, "retained-org", 1, h); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveDefinitionHistory(ctx, "retained-org", 1, h); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatal("stale write accepted", err)
	}
	h.Generation++
	h.Versions[0].Definition.Name = "tampered"
	if err := st.SaveDefinitionHistory(ctx, "retained-org", 2, h); !errors.Is(err, persistence.ErrImmutable) {
		t.Fatal("immutable write accepted", err)
	}

	selected.Revision = 1
	selected.Desired = []resource.Definition{v.Definition}
	if err := st.SelectDefinitionSelection(ctx, "other-org", 9, 0, selected); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatal("cross-organization selection accepted", err)
	}
	if err := st.SelectDefinitionSelection(ctx, "retained-org", 9, 0, selected); err != nil {
		t.Fatal(err)
	}
	if err := st.SelectDefinitionSelection(ctx, "retained-org", 9, 0, selected); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatal("stale selection accepted", err)
	}
	if err := st.ApplyDefinitionSelection(ctx, "retained-app", "staging", 1, selected.Desired, ""); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatal("unfenced activation accepted", err)
	}
	op, err := st.ClaimEnvironment(ctx, persistence.OperationClaim{ApplicationKey: "retained-app", EnvironmentKey: "staging", Owner: "versions-test", Kind: environment.OpDeploy, Deadline: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	next := selected
	next.Revision = 2
	if err := st.SelectDefinitionSelection(ctx, "retained-org", 10, 1, next); !errors.Is(err, persistence.ErrEnvironmentBusy) {
		t.Fatal("busy selection accepted", err)
	}
	lease := persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence}
	owned := persistence.WithOwner(ctx, lease)
	if err := st.ApplyDefinitionSelection(owned, "retained-app", "staging", 1, selected.Desired, ""); err != nil {
		t.Fatal(err)
	}
	applied, err := st.GetDefinitionSelection(ctx, "retained-app", "staging")
	if err != nil || !applied.AppliedVerified || applied.Pending() || applied.Applied[0].RevisionID != v.ID {
		t.Fatal("verified fenced activation failed", err)
	}
	if err := st.ReleaseOperation(ctx, lease, environment.OpSucceeded, ""); err != nil {
		t.Fatal(err)
	}
	if err := st.ApplyDefinitionSelection(owned, "retained-app", "staging", 1, selected.Desired, ""); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatal("released owner activated selection", err)
	}
	c := connectAs(t, console)
	if _, err := c.Exec(ctx, `UPDATE resource_definition_versions SET snapshot=jsonb_set(snapshot,'{name}','"tampered"')`); err == nil {
		t.Fatal("SQL bypassed immutable guard")
	}
	denied(t, c, "revision delete", `DELETE FROM resource_definition_versions`)
	if scalar[bool](t, owner, `SELECT has_table_privilege('`+roles.State+`','resource_definition_versions','SELECT')`) {
		t.Fatal("State runtime received definition access")
	}
}

func TestVersionSixRollbackPreservesFive(t *testing.T) {
	owner, _, roles := legacyDatabase(t, versionFiveSQL)
	before := retainedFingerprints(t, owner)
	sqlOn(t, owner, `GRANT DELETE ON schema_baseline TO `+pgx.Identifier{roles.Console}.Sanitize())
	if _, err := Setup(context.Background(), owner, roles); err == nil {
		t.Fatal("expected grant verification failure")
	}
	if scalar[string](t, owner, `SELECT baseline_sha256 FROM schema_baseline`) != digest(versionFiveSQL) || scalar[int](t, owner, `SELECT count(*) FROM schema_upgrade_history`) != 5 {
		t.Fatal("rollback changed source identity/history")
	}
	if scalar[bool](t, owner, `SELECT to_regclass('resource_definition_versions') IS NOT NULL`) {
		t.Fatal("migration DDL committed")
	}
	for table, hash := range before {
		if retainedFingerprints(t, owner)[table] != hash {
			t.Errorf("retained rows changed in %s", table)
		}
	}
}
