package deployment

import (
	"testing"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/execution"
)

// A Definition-placed Environment carries no deployment Connection or profile (ADR-016, ADR-017): the persisted workload
// target is the only source of the scoped account, the provider and the region.
func TestDefinitionPlacedEKSWorkloadTargetPersistsOnlyOpaqueIdentity(t *testing.T) {
	original := execution.Target{Kind: "kubernetes", Provider: "eks", Region: "us-east-1", ClusterName: "orch-eks", Namespace: "team-a", Organization: "acme", Connection: "aws-prod"}
	ref := targetRef(original)
	for _, forbidden := range []string{"token", "kubeconfig", "endpoint", "certificate-authority-data", "accessKeyId", "secretAccessKey"} {
		if _, exists := ref[forbidden]; exists {
			t.Fatalf("target reference persisted forbidden field %q", forbidden)
		}
	}
	var restored execution.Target
	if err := restoreEnvironmentTarget(&restored, ref, "acme", environment.Environment{}); err != nil {
		t.Fatal(err)
	}
	if restored.Provider != "eks" || restored.Region != "us-east-1" || restored.Connection != "aws-prod" || restored.ClusterName != original.ClusterName || restored.Organization != "acme" {
		t.Fatalf("opaque EKS target did not survive restore: %+v", restored)
	}
}

func TestDefinitionPlacedRestoreRefusesATargetWithoutAScopedAccountOrOfAnotherOrganization(t *testing.T) {
	ref := targetRef(execution.Target{Kind: "kubernetes", Provider: "eks", Region: "us-east-1", ClusterName: "orch-eks", Namespace: "team-a", Organization: "acme", Connection: "aws-prod"})
	for name, mutate := range map[string]func(map[string]any){
		"no scoped account":    func(copy map[string]any) { delete(copy, "connection") },
		"another organization": func(copy map[string]any) { copy["organization"] = "globex" },
	} {
		t.Run(name, func(t *testing.T) {
			copy := map[string]any{}
			for k, v := range ref {
				copy[k] = v
			}
			mutate(copy)
			var got execution.Target
			if err := restoreEnvironmentTarget(&got, copy, "acme", environment.Environment{}); err == nil {
				t.Fatal("a target without a scoped account of this Organization was accepted")
			}
		})
	}
}
