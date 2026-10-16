package deployment

import (
	"testing"

	appdomain "orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/execution"
)

func TestRestoreEnvironmentTargetRejectsOtherConnection(t *testing.T) {
	ref := map[string]any{"organization": "acme", "connection": "other", "context": "c", "cluster": "k"}
	var target execution.Target
	env := environment.Environment{ConnectionKey: "lab", Profile: appdomain.ProfileInternalK8s}
	if err := restoreEnvironmentTarget(&target, ref, "acme", env); err == nil {
		t.Fatal("stored target on another connection was reused")
	}
	env.ConnectionKey = "other"
	if err := restoreEnvironmentTarget(&target, ref, "acme", env); err != nil {
		t.Fatal(err)
	}
	legacy := map[string]any{"context": "c", "cluster": "k"}
	target = execution.Target{}
	env.ConnectionKey = "lab"
	if err := restoreEnvironmentTarget(&target, legacy, "acme", env); err != nil {
		t.Fatalf("legacy host-context target: %v", err)
	}
}

func TestEKSWorkloadTargetPersistsOnlyOpaqueIdentityAndRequiresPinnedEnvironment(t *testing.T) {
	original := execution.Target{Kind: "kubernetes", Provider: "eks", Region: "us-east-1", ClusterName: "orch-eks", Namespace: "team-a", Organization: "acme", Connection: "aws-prod"}
	ref := targetRef(original)
	for _, forbidden := range []string{"token", "kubeconfig", "endpoint", "certificate-authority-data", "accessKeyId", "secretAccessKey"} {
		if _, exists := ref[forbidden]; exists {
			t.Fatalf("target reference persisted forbidden field %q", forbidden)
		}
	}
	var restored execution.Target
	env := environment.Environment{ConnectionKey: "aws-prod", Profile: appdomain.ProfileAWSEKS, Region: "us-east-1"}
	if err := restoreEnvironmentTarget(&restored, ref, "acme", env); err != nil {
		t.Fatal(err)
	}
	if restored.Provider != "eks" || restored.Region != env.Region || restored.Connection != env.ConnectionKey || restored.ClusterName != original.ClusterName || restored.Organization != "acme" {
		t.Fatalf("opaque EKS target did not survive restore: %+v", restored)
	}
	for name, mutate := range map[string]func(map[string]any){
		"wrong environment region": func(copy map[string]any) { copy["region"] = "us-west-2" },
		"missing provider":         func(copy map[string]any) { delete(copy, "provider"); delete(copy, "region") },
		"unsupported provider":     func(copy map[string]any) { copy["provider"] = "aws" },
	} {
		t.Run(name, func(t *testing.T) {
			copy := map[string]any{}
			for k, v := range ref {
				copy[k] = v
			}
			mutate(copy)
			var got execution.Target
			if err := restoreEnvironmentTarget(&got, copy, "acme", env); err == nil {
				t.Fatal("mismatched persisted target was accepted")
			}
		})
	}
}
