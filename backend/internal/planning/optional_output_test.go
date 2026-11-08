package planning

import (
	"strings"
	"testing"

	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/planning/workloadspec"
	"orchestrator/internal/seed"
)

// UC-02 EXT-05 (issue #52): a workload may bind an optional Type output only
// when the selected Definition provides it; otherwise Preview stops before any
// execution and names the output.
func TestPlan_OptionalOutputMustBeProvidedByTheSelectedDefinition(t *testing.T) {
	request := func(mapURI bool) Request {
		req := testRequest(t, "internal-k8s", "backend")
		backend := seed.AcceptanceWorkloads(seedOptions())["backend"]
		backend["containers"].(map[string]any)["main"].(map[string]any)["variables"].(map[string]any)["DATABASE_URL"] = "${resources.db.uri}"
		doc, err := workloadspec.FromMap(backend)
		if err != nil {
			t.Fatal(err)
		}
		req.After = doc
		def := kubernetesModuleDefinition(map[string]any{"target": tfmodule.TargetResource, "resource_ref": "k8s-cluster.internal#environments.@app.@env"})
		if mapURI {
			def.DriverInputs["values"].(map[string]any)["output_mapping"].(map[string]any)["uri"] = "uri"
		}
		req.Catalog.Definitions = append(req.Catalog.Definitions, def)
		return req
	}
	plan, err := NewService(testBundles()).Plan(request(true))
	if err != nil {
		t.Fatalf("a mapped optional output must plan: %v", err)
	}
	if contractFor(t, plan, "postgres-k8s-module").Module != "pgk8s" {
		t.Fatal("the Terraform Definition was not selected")
	}
	_, err = NewService(testBundles()).Plan(request(false))
	if err == nil || !strings.Contains(err.Error(), `binds output "uri"`) {
		t.Fatalf("an unmapped optional output must stop planning, got %v", err)
	}
}
