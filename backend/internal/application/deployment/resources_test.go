package deployment_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/domain/environment"
)

// Expected container resources of the seeded acceptance workloads: backend
// declares everything, worker a partial set and frontend nothing.
var (
	declaredRequirements = map[string]string{
		"backend":  `{"requests":{"cpu":"50m","memory":"64Mi"},"limits":{"cpu":"500m","memory":"256Mi"}}`,
		"worker":   `{"requests":{"memory":"48Mi"},"limits":{"memory":"128Mi"}}`,
		"frontend": `null`,
	}
	renderedResources = map[string]map[string]any{
		"backend": {
			"requests": map[string]any{"cpu": "50m", "memory": "64Mi"},
			"limits":   map[string]any{"cpu": "500m", "memory": "256Mi"},
		},
		"worker": {
			"requests": map[string]any{"cpu": "10m", "memory": "48Mi"},
			"limits":   map[string]any{"memory": "128Mi"},
		},
		"frontend": {
			"requests": map[string]any{"cpu": "10m", "memory": "32Mi"},
		},
	}
)

func mainRequirements(t *testing.T, doc environment.Document, workload string) string {
	t.Helper()
	module, ok := doc.Modules[workload]
	if !ok {
		t.Fatalf("module %s is missing", workload)
	}
	b, err := json.Marshal(module.Spec.Containers["main"].Resources)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestDeployWorkload_RendersDeclaredContainerResources(t *testing.T) {
	ctx := context.Background()
	statePath := filepath.Join(t.TempDir(), "state.json")
	app, seedOptions := newApp(t, func(o *bootstrap.Options) { o.StatePath = statePath })
	results := deployAll(t, app, seedOptions)

	// Rendered manifests: declared values verbatim, defaults only for missing
	// request fields and limits only when declared.
	rendered := map[string]map[string]any{}
	for _, applied := range app.FakeDeploy.Applied {
		for _, m := range applied.Manifests {
			if m.Kind != "Deployment" {
				continue
			}
			containers := m.Object["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)["containers"].([]any)
			rendered[m.Name] = containers[0].(map[string]any)["resources"].(map[string]any)
		}
	}
	for workload, want := range renderedResources {
		if !reflect.DeepEqual(rendered[workload], want) {
			got, _ := json.Marshal(rendered[workload])
			exp, _ := json.Marshal(want)
			t.Fatalf("%s container resources:\nwant %s\ngot  %s", workload, exp, got)
		}
	}

	// Persisted Candidate Deployment Set and Delta Snapshot, read back from the
	// state snapshot file.
	reloaded, err := store.NewWithSnapshot(statePath)
	if err != nil {
		t.Fatalf("reload store: %v", err)
	}
	env, err := reloaded.GetEnvironment(ctx, seedOptions.ApplicationKey, seedOptions.EnvironmentKey)
	if err != nil {
		t.Fatalf("environment: %v", err)
	}
	current, err := reloaded.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
	if err != nil {
		t.Fatalf("current set: %v", err)
	}
	for workload, want := range declaredRequirements {
		if got := mainRequirements(t, current.Document, workload); got != want {
			t.Fatalf("%s persisted requirements:\nwant %s\ngot  %s", workload, want, got)
		}
	}
	for _, r := range results {
		record, err := reloaded.GetDeployment(ctx, r.DeploymentID)
		if err != nil {
			t.Fatalf("deployment: %v", err)
		}
		snapshot, err := reloaded.GetDeltaSnapshot(ctx, record.DeltaSnapshotID)
		if err != nil {
			t.Fatalf("snapshot: %v", err)
		}
		added := environment.Document{Modules: snapshot.Document.Modules.Add}
		if got := mainRequirements(t, added, r.WorkloadID); got != declaredRequirements[r.WorkloadID] {
			t.Fatalf("%s delta requirements:\nwant %s\ngot  %s", r.WorkloadID, declaredRequirements[r.WorkloadID], got)
		}
	}
}
