package postgres

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

const (
	privateModule   = "verify-PRIVATE-MODULE-HANDLE"
	privateTarget   = "vault://secret/PRIVATE-TARGET-CREDENTIAL"
	privateRecovery = "recovery/PRIVATE-PATH"
)

// ADR-014 / schema `terraform_resource_attempts.deployment_id`: the Console
// reads a safe stage/attempt/status projection of the attempts linked to one
// Deployment, through a login that cannot read the private columns at all.
func TestConsoleReadsSafeAttemptProgressOnly(t *testing.T) {
	ctx := context.Background()
	f := newStateFixture(t)
	env, err := f.console.GetEnvironment(ctx, "app", "staging")
	if err != nil {
		t.Fatal(err)
	}
	record := deployment.Deployment{ID: ids.New(), EnvironmentID: env.ID, OrganizationKey: "acme", ApplicationKey: "app", EnvironmentKey: "staging", ExecutionProfile: "internal-k8s", Action: deployment.ActionDeploy, WorkloadID: "api", ActorRef: "test", Status: deployment.StatusPlanning, BaseEnvironmentVersion: env.Version, StartedAt: time.Now().UTC()}
	if err := f.console.SaveDeployment(ctx, record); err != nil {
		t.Fatal(err)
	}
	owner := persistence.WithOwner(ctx, persistence.Owner{OperationID: f.op.ID, Owner: f.op.Owner, Fence: f.op.Fence})
	attempt, err := f.state.BeginAttempt(owner, tfresource.Attempt{
		InstanceID: f.inst.ID, DeploymentID: record.ID, RunID: "run-1", Action: tfresource.ActionUpdate, Ownership: tfresource.Preexisting, Stage: "INTENT",
		ModuleSnapshot: map[string]any{"verifyHandle": privateModule, "variables": map[string]any{"password": privateModule}},
		TargetSnapshot: map[string]any{"secretRef": privateTarget}, Deadline: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.state.UpdateAttempt(owner, attempt.ID, tfresource.AttemptProgress{Stage: "APPLYING", Status: tfresource.AttemptRunning, ObservedPhysicalIDs: []string{"aws_instance.x=i-PRIVATE"}, RecoveryHandle: privateRecovery}); err != nil {
		t.Fatal(err)
	}

	check := func(label string, store *Store) {
		t.Helper()
		got, err := store.ListDeploymentAttempts(ctx, record.ID)
		if err != nil || len(got) != 1 {
			t.Fatalf("%s: %+v %v", label, got, err)
		}
		a := got[0]
		if a.Descriptor != f.inst.Identity.Descriptor || a.ScopeType != "SHARED" || a.Number != 1 || a.Action != "UPDATE" || a.Ownership != "PREEXISTING" ||
			a.Stage != "APPLYING" || a.Status != "RUNNING" || a.ModuleID != "redis" || a.ModuleRevision != "0123456789abcdef0123456789abcdef01234567" || a.StartedAt.IsZero() {
			t.Fatalf("%s: projection %+v", label, a)
		}
		raw, _ := json.Marshal(got)
		for _, secret := range []string{privateModule, privateTarget, privateRecovery, "PRIVATE", "i-PRIVATE", f.inst.StateKey, "k8s-4f"} {
			if strings.Contains(string(raw), secret) {
				t.Fatalf("%s: projection exposes %q: %s", label, secret, raw)
			}
		}
	}
	check("live", f.console)

	// Reopen: a new Console session over the same database sees the same rows.
	reopened, err := Open(ctx, f.db.Console)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	check("reopened", reopened)

	// A Deployment without linked attempts, and an unknown or malformed ID, have none.
	other := record
	other.ID = ids.New()
	if err := f.console.SaveDeployment(ctx, other); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{other.ID, ids.New(), "not-a-uuid", ""} {
		if got, err := f.console.ListDeploymentAttempts(ctx, id); err != nil || len(got) != 0 {
			t.Fatalf("deployment %q: %+v %v", id, got, err)
		}
	}

	// The Console login itself cannot read a private column, whatever the statement.
	console := connectAs(t, f.db.Console)
	for label, sql := range map[string]string{
		"module snapshot":  `SELECT module_snapshot FROM terraform_resource_attempts`,
		"target snapshot":  `SELECT target_snapshot FROM terraform_resource_attempts`,
		"recovery handle":  `SELECT recovery_handle FROM terraform_resource_attempts`,
		"physical ids":     `SELECT observed_physical_ids FROM terraform_resource_attempts`,
		"state serial":     `SELECT state_serial FROM terraform_resource_attempts`,
		"attempt row":      `SELECT * FROM terraform_resource_attempts`,
		"attempt via join": `SELECT a.module_snapshot FROM terraform_resource_instances i JOIN terraform_resource_attempts a ON a.resource_instance_id=i.id`,
		"attempt to json":  `SELECT to_jsonb(a) FROM terraform_resource_attempts a`,
		"instance target":  `SELECT target_ref FROM terraform_resource_instances`,
		"instance intent":  `SELECT physical_intent FROM terraform_resource_instances`,
		"instance row":     `SELECT * FROM terraform_resource_instances`,
		"instance to json": `SELECT to_jsonb(i) FROM terraform_resource_instances i`,
		"attempt write":    `UPDATE terraform_resource_attempts SET stage='X'`,
		"attempt delete":   `DELETE FROM terraform_resource_attempts`,
		"attempt insert":   `INSERT INTO terraform_resource_attempts(id) VALUES(gen_random_uuid())`,
	} {
		denied(t, console, label, sql)
	}
	allowed(t, console, "safe attempt columns", `SELECT id,resource_instance_id,deployment_id,attempt_number,action,ownership,stage,status,safe_error_code,created_at,updated_at FROM terraform_resource_attempts`)
	allowed(t, console, "safe instance columns", `SELECT id,descriptor,scope_type,module_bundle_id,status FROM terraform_resource_instances`)
}

// Persisted attempt strings are free text to PostgreSQL: a hostile value written
// behind the service's back must still not leave the Console API.
func TestDeploymentViewAllowlistsHostilePersistedAttemptText(t *testing.T) {
	ctx := context.Background()
	f := newStateFixture(t)
	env, err := f.console.GetEnvironment(ctx, "app", "staging")
	if err != nil {
		t.Fatal(err)
	}
	record := deployment.Deployment{ID: ids.New(), EnvironmentID: env.ID, OrganizationKey: "acme", ApplicationKey: "app", EnvironmentKey: "staging", ExecutionProfile: "internal-k8s", Action: deployment.ActionDeploy, WorkloadID: "api", ActorRef: "test", Status: deployment.StatusPlanning, BaseEnvironmentVersion: env.Version, StartedAt: time.Now().UTC()}
	if err := f.console.SaveDeployment(ctx, record); err != nil {
		t.Fatal(err)
	}
	owner := persistence.WithOwner(ctx, persistence.Owner{OperationID: f.op.ID, Owner: f.op.Owner, Fence: f.op.Fence})
	if _, err := f.state.BeginAttempt(owner, tfresource.Attempt{InstanceID: f.inst.ID, DeploymentID: record.ID, RunID: "run-1", Action: tfresource.ActionUpdate, Ownership: tfresource.Preexisting, Stage: "INTENT", ModuleSnapshot: map[string]any{"a": "b"}, TargetSnapshot: map[string]any{"c": "d"}, Deadline: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	sqlOn(t, f.db.Admin, `UPDATE terraform_resource_attempts SET stage='AKIAPRIVATE /var/lib/private', safe_error_code='AUTH vault://PRIVATE-TOKEN', status='RUNNING'`)
	view, err := appsvc.NewQueryService(f.console).GetDeployment(ctx, appsvc.GetDeploymentQuery{OrganizationKey: "acme", ApplicationKey: "app", EnvironmentKey: "staging", DeploymentID: record.ID})
	if err != nil || len(view.ResourceProgress) != 1 {
		t.Fatalf("%+v %v", view, err)
	}
	raw, _ := json.Marshal(view.ResourceProgress)
	for _, secret := range []string{"AKIA", "PRIVATE", "/var/", "vault://"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("hostile persisted text leaks %q: %s", secret, raw)
		}
	}
	if p := view.ResourceProgress[0]; p.Stage != "UNKNOWN" || p.SafeErrorCode != "UNKNOWN_ERROR" || p.Status != "RUNNING" {
		t.Fatalf("projection %+v", p)
	}
}
