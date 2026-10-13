package provisioning

import (
	"errors"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/planning"
)

func pinPlan(provider string) *planning.Plan {
	return &planning.Plan{Graph: planning.Graph{Edges: []planning.Edge{{Consumer: "redis.default#modules.x", Provider: provider, Reason: planning.ReasonProviderBinding}}}}
}

func pinContext(profile application.ExecutionProfile, connection, region string, generation int64) planning.Context {
	return planning.Context{Env: environment.Environment{Profile: profile, ConnectionKey: connection, Region: region, TargetGeneration: generation}}
}

// Pins that differ from the Environment target (another connection, region,
// profile or generation), or name a cluster that is not a provider edge of the
// node, are refused with ErrProviderPin before any effect.
func TestCheckPinsAgainstPlanRejectsStaleAndForeignTargets(t *testing.T) {
	cluster := "k8s-cluster.eks#environments.app.env"
	good := tfmodule.ProviderPin{Name: "kubernetes", Kind: tfmodule.BindingKubernetes, Target: tfmodule.TargetEnvironment, Provider: cluster, Profile: "aws-eks", Generation: 2}
	aws := tfmodule.ProviderPin{Name: "aws", Kind: tfmodule.BindingAWS, Target: tfmodule.TargetEnvironment, ConnectionKey: "acct", ConnectionKind: "AWS", Region: "us-east-1", Profile: "aws-eks", Generation: 2}
	ctx := pinContext(application.ProfileAWSEKS, "acct", "us-east-1", 2)
	plan := pinPlan(cluster)
	if err := checkPinsAgainstPlan(plan, "redis.default#modules.x", []tfmodule.ProviderPin{good, aws}, ctx); err != nil {
		t.Fatalf("matching pins: %v", err)
	}
	stale := map[string]planning.Context{
		"connection": pinContext(application.ProfileAWSEKS, "other-account", "us-east-1", 2),
		"region":     pinContext(application.ProfileAWSEKS, "acct", "eu-west-1", 2),
		"profile":    pinContext(application.ProfileInternalK8s, "acct", "us-east-1", 2),
		"generation": pinContext(application.ProfileAWSEKS, "acct", "us-east-1", 3),
	}
	for name, c := range stale {
		if err := checkPinsAgainstPlan(plan, "redis.default#modules.x", []tfmodule.ProviderPin{good, aws}, c); !errors.Is(err, ErrProviderPin) {
			t.Errorf("%s drift accepted: %v", name, err)
		}
	}
	foreign := good
	foreign.Provider = "k8s-cluster.eks#environments.other.env"
	if err := checkPinsAgainstPlan(plan, "redis.default#modules.x", []tfmodule.ProviderPin{foreign}, ctx); !errors.Is(err, ErrProviderPin) {
		t.Errorf("a cluster without a graph edge was accepted: %v", err)
	}
	none := good
	none.Provider = ""
	if err := checkPinsAgainstPlan(plan, "redis.default#modules.x", []tfmodule.ProviderPin{none}, ctx); !errors.Is(err, ErrProviderPin) {
		t.Errorf("a Kubernetes pin without a cluster was accepted: %v", err)
	}
}
