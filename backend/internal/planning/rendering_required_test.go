package planning

import (
	"errors"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
)

// dropWorkloadDefinitions removes the seeded workload Definitions so a test supplies its own.
func dropWorkloadDefinitions(req *Request) {
	var kept []resource.Definition
	for _, def := range req.Catalog.Definitions {
		if def.ResourceTypeKey != TypeWorkload {
			kept = append(kept, def)
		}
	}
	req.Catalog.Definitions = kept
}

func renderDefinition(key, profile string, criteria ...resource.Criterion) resource.Definition {
	if len(criteria) == 0 {
		criteria = []resource.Criterion{{}}
	}
	variables := map[string]any{"render_bundle": "installed"}
	if profile == string(application.ProfileInternalK8s) {
		// Definition-placed Applications reference the typed namespace and cluster outputs (ADR-016).
		variables["namespace"] = "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"
		variables["cluster"] = "${resources['k8s-cluster.internal#environments.@app.@env'].outputs.cluster}"
	}
	return resource.Definition{Key: key, Name: key, Version: 1, ResourceTypeKey: "workload", DriverType: resource.DriverScoreK8s,
		DriverInputs: map[string]any{"values": map[string]any{"variables": variables}}, Criteria: criteria}
}

var testBundle = resource.RenderBundle{ID: "installed", Version: "0.15.0", BinaryDigest: "binary", Digest: "bundle"}

func rendererReason(t *testing.T, err error) string {
	t.Helper()
	var staged *StageError
	if !errors.As(err, &staged) {
		t.Fatalf("expected a staged planning error, got %v", err)
	}
	return staged.Reason
}

type planningIntent struct{ typ, class, intent, definition string }

// pinnedInspector adds the verified source pin to the stub contract, as the
// registry inspector does.
type pinnedInspector struct{ stubInspector }

func (p pinnedInspector) Contract(def resource.Definition) (ModuleContract, error) {
	contract, err := p.stubInspector.Contract(def)
	if err != nil {
		return contract, err
	}
	inputs, err := tfmodule.ParseDriverInputs(def.DriverValues())
	if err != nil {
		return contract, err
	}
	source := inputs.Source
	contract.Source, contract.BundleID = &source, "bundle-"+source.ModuleID
	return contract, nil
}
