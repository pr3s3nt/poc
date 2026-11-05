package deployment_test

import (
	"context"
	"encoding/json"
	"orchestrator/internal/application/envops"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"testing"
)

// Existing fake execution/delivery, real planning, binding, persisted applied
// snapshots and consumer refresh; this is not cloud acceptance.
func TestRepairRefreshesAppliedWorkloadConsumersWithoutChangingDesiredOrHistory(t *testing.T) {
	ctx := context.Background()
	app, options := newApp(t)
	results := deployAll(t, app, options)
	before, err := app.Store.GetEnvironment(ctx, options.ApplicationKey, options.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	active, err := app.Store.ListActiveResources(ctx, options.OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	var database resource.ActiveResource
	for _, a := range active {
		if a.Descriptor.Type == "postgres" {
			database = a
			break
		}
	}
	if database.ID == "" {
		t.Fatal("database fixture missing")
	}
	coordinator := app.Deployments.RepairConsumerCoordinator(nil)
	refs, err := coordinator.PreviewRepairConsumers(ctx, options.OrganizationKey, options.ApplicationKey, options.EnvironmentKey, []string{database.Descriptor.String()})
	if err != nil {
		t.Fatal(err)
	}
	workloadRefs := refs[:0]
	for _, ref := range refs {
		if ref.WorkloadID != "" {
			workloadRefs = append(workloadRefs, ref)
		}
	}
	refs = workloadRefs
	if len(refs) == 0 {
		t.Fatal("dependent workloads not discovered")
	}
	database.Outputs["host"] = "repaired-database.internal"
	if _, err = app.Store.UpsertActiveResource(ctx, database); err != nil {
		t.Fatal(err)
	}
	rawPlan, e := app.Store.GetPlan(ctx, refs[0].DeploymentID)
	if e != nil {
		t.Fatal(e)
	}
	body, _ := json.Marshal(rawPlan)
	var frozen planning.Plan
	_ = json.Unmarshal(body, &frozen)
	restored, e := coordinator.Bindings.LoadResult(ctx, options.OrganizationKey, &frozen)
	if e != nil || restored.Outputs[database.Descriptor.String()]["host"] != "repaired-database.internal" {
		t.Fatal("provider output not restored for repair", e)
	}
	// Pending/current catalog changes are not the configuration used by Repair.
	defs, _ := app.Store.ListResourceDefinitions(ctx, options.OrganizationKey)
	mutated := 0
	for _, def := range defs {
		if def.ResourceTypeKey == "workload" {
			def.DriverInputs = map[string]any{"values": map[string]any{"unrelated_draft": "must-not-be-used"}}
			if _, err := app.Store.UpdateResourceDefinition(ctx, options.OrganizationKey, def.Version, def); err != nil {
				t.Fatal(err)
			}
			mutated++
		}
	}
	if mutated == 0 {
		t.Fatal("no current workload Definition was changed by the fixture")
	}
	lease, err := app.Operations.Begin(ctx, envops.Claim{ApplicationKey: options.ApplicationKey, EnvironmentKey: options.EnvironmentKey, Kind: environment.OpDriftRepair})
	if err != nil {
		t.Fatal(err)
	}
	if err = coordinator.RefreshRepairConsumers(lease.Ctx, options.OrganizationKey, options.ApplicationKey, options.EnvironmentKey, "test", refs); err != nil {
		_ = lease.End(environment.OpFailed, "")
		t.Fatal(err)
	}
	if err = lease.End(environment.OpSucceeded, ""); err != nil {
		t.Fatal(err)
	}
	after, _ := app.Store.GetEnvironment(ctx, options.ApplicationKey, options.EnvironmentKey)
	if after.CurrentDeploymentSetID != before.CurrentDeploymentSetID || after.Version != before.Version {
		t.Fatal("repair changed Desired State")
	}
	for _, result := range results {
		history, err := app.Store.GetDeployment(ctx, result.DeploymentID)
		if err != nil || string(history.Status) != "SUCCEEDED" {
			t.Fatal("historical deployment changed", err)
		}
	}

	found := false
	for _, applied := range app.FakeDeploy.Applied {
		for _, m := range applied.Manifests {
			if m.Kind == "Deployment" {
				spec := m.Object["spec"].(map[string]any)
				template := spec["template"].(map[string]any)
				pod := template["spec"].(map[string]any)
				for _, container := range pod["containers"].([]any) {
					envs, _ := container.(map[string]any)["env"].([]any)
					for _, env := range envs {
						entry := env.(map[string]any)
						if entry["name"] == "PGHOST" && entry["value"] == "repaired-database.internal" {
							found = true
						}
					}
				}
			}
		}
	}
	if !found {
		t.Fatal("new provider output was not propagated into rendered workload manifests")
	}
}
