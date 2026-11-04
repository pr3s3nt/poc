package pending_test

import (
	"context"
	"errors"
	"testing"

	catalogapp "orchestrator/internal/application/catalogrelease"
	"orchestrator/internal/application/definitionversions"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/pending"
	"orchestrator/internal/domain/resource"
)

func TestDefinitionUpgradeCannotBypassDatabaseMigrationGuard(t *testing.T) {
	ctx := context.Background()
	app, workloads, svc, opts := fixture(t)
	defer app.Close()
	raw := scoreFor("api", "example.invalid/api:v1", false)
	raw["resources"] = map[string]any{"db": map[string]any{"type": "postgres", "id": "shared-db", "params": map[string]any{"database": "appdb", "username": "appuser"}}}
	raw["containers"].(map[string]any)["main"].(map[string]any)["variables"] = map[string]any{"PGHOST": "${resources.db.host}"}
	if _, err := workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "api", raw, 0); err != nil {
		t.Fatal(err)
	}
	p, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	if r, err := svc.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", p.Token); err != nil || r.Status != "SUCCEEDED" {
		t.Fatalf("initial deploy: %+v %v", r, err)
	}
	defs, pin, err := catalogapp.ResolveDefinitions(ctx, app.Store, opts.OrganizationKey, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	var changed resource.Definition
	for _, d := range defs {
		if d.Key == "postgres-internal-statefulset" {
			changed = d
		}
	}
	changed.DriverInputs = map[string]any{"values": map[string]any{"variables": map[string]any{"image": "postgres:17-alpine", "storage": "2Gi", "namespace": "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"}}}
	v, err := definitionversions.Record(ctx, app.Store, opts.OrganizationKey, changed, false)
	if err != nil {
		t.Fatal(err)
	}
	selected, _ := app.Store.GetDefinitionSelection(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	env, _ := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err := definitionversions.Select(ctx, app.Store, opts.OrganizationKey, opts.ApplicationKey, opts.EnvironmentKey, changed.Key, v.RevisionID, env.Version, selected.Revision, defs, pin, false); err != nil {
		t.Fatal(err)
	}
	calls := len(app.FakeExec.Calls)
	applies := len(app.FakeDeploy.Applied)
	if _, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey); !errors.Is(err, pending.ErrInvalid) {
		t.Fatal("database update bypassed Preview guard", err)
	}
	if _, err := app.Deployments.DeployWorkload(ctx, appsvc.DeployCommand{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, WorkloadID: "api", WorkloadBefore: raw, WorkloadAfter: raw, Actor: "test"}); !errors.Is(err, appsvc.ErrStalePlan) {
		t.Fatal("direct deploy bypassed Environment upgrade", err)
	}
	after, _ := app.Store.GetDefinitionSelection(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if !after.Pending() || after.AppliedRevision != selected.AppliedRevision || len(app.FakeExec.Calls) != calls || len(app.FakeDeploy.Applied) != applies {
		t.Fatal("blocked upgrade changed applied policy or caused effects")
	}
}
