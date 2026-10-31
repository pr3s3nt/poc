package pending

import (
	"errors"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"testing"
)

func TestCatalogCannotImplicitlyUpgradeExistingDatabaseOrStorage(t *testing.T) {
	for _, typ := range []string{"postgres", "mysql", "redis", "volume"} {
		t.Run(typ, func(t *testing.T) {
			node := planning.Node{Descriptor: typ + ".default#data", ResourceType: typ, Kind: planning.NodeResource, Scope: resource.Scope{Type: resource.ScopeEnvironment, ID: "app.staging"}}
			// Existing identity with no provable baseline is UNVERIFIED, never safe.
			plan := &planning.Plan{Graph: planning.Graph{Nodes: []planning.Node{node}}, Matches: map[string]planning.Match{node.Descriptor: {DriverType: resource.DriverTerraform}}, Classification: planning.Classification{Existing: []string{node.Descriptor}}}
			types := map[string]resource.Type{"volume": {Outputs: []resource.OutputField{{Name: "claimName", Type: "string"}, {Name: "namespace", Type: "string"}}}}
			if err := validateCatalogDataUpdates(plan, func(planning.Node) *planning.Plan { return nil }, types); !errors.Is(err, ErrInvalid) {
				t.Fatal("unverified data update admitted", err)
			}
			plan.Terraform = []planning.TerraformContract{{Descriptor: node.Descriptor, Fingerprint: "sha256:verified"}}
			plan.IntentEvidence = map[string]string{node.Descriptor: "unchanged-definition"}
			if err := validateCatalogDataUpdates(plan, func(planning.Node) *planning.Plan { return plan }, types); err != nil {
				t.Fatal("provably unchanged data component blocked", err)
			}
			plan.Classification = planning.Classification{New: []string{node.Descriptor}}
			if err := validateCatalogDataUpdates(plan, func(planning.Node) *planning.Plan { return nil }, types); err != nil {
				t.Fatal("new resource incorrectly blocked", err)
			}
		})
	}
}
