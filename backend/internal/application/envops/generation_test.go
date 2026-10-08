package envops_test

import (
	"testing"

	"orchestrator/internal/application/envops"
	"orchestrator/internal/domain/resource"
)

func TestInGenerationZeroExcludesEveryLaterGenerationAndViceVersa(t *testing.T) {
	scopes := map[string]int64{
		"app.staging":         0,
		"app.staging.web":     0,
		"app.staging.g1":      0, // a generation-0 workload that is merely named g1
		"app.staging~g1":      1,
		"app.staging~g1.web":  1,
		"app.staging~g12":     12,
		"app.staging~g12.web": 12,
		"app.production":      -1,
		"app.production~g1":   -1,
		"app.staging2":        -1,
		"other.staging":       -1,
		"app.staging~g1extra": -1,
		"app.stagingg1":       -1,
	}
	for id, want := range scopes {
		for generation := int64(0); generation <= 12; generation++ {
			got := envops.InGeneration(resource.Scope{Type: resource.ScopeWorkload, ID: id}, "app", "staging", generation)
			if got != (want == generation) {
				t.Fatalf("scope %q in generation %d = %v, want %v", id, generation, got, want == generation)
			}
		}
	}
}
