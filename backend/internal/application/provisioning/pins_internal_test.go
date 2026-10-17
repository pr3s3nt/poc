package provisioning

import (
	"errors"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
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
	aws := tfmodule.ProviderPin{Name: "aws", Kind: tfmodule.BindingAWS, Target: tfmodule.TargetAccount, ConnectionKey: "acct", ConnectionKind: "AWS", AccountVersion: 5, Region: "us-east-1", Profile: "aws-eks", Generation: 2}
	ctx := pinContext(application.ProfileAWSEKS, "env-account", "eu-west-1", 2)
	plan := pinPlan(cluster)
	def := resource.Definition{Key: "d", DriverType: resource.DriverTerraform, DriverAccount: "acct", DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"region": "us-east-1"}}}}
	account := application.Connection{Key: "acct", Kind: application.ConnectionAWS, Version: 5}
	// The Environment account and region are irrelevant to an AWS pin.
	if err := checkPinsAgainstPlan(plan, "redis.default#modules.x", []tfmodule.ProviderPin{good, aws}, def, account, ctx); err != nil {
		t.Fatalf("matching pins: %v", err)
	}
	stale := map[string]func() (resource.Definition, application.Connection, planning.Context){
		"definition account": func() (resource.Definition, application.Connection, planning.Context) {
			d := def
			d.DriverAccount = "other"
			return d, account, ctx
		},
		"account identity": func() (resource.Definition, application.Connection, planning.Context) {
			a := account
			a.Key = "other"
			return def, a, ctx
		},
		"account version": func() (resource.Definition, application.Connection, planning.Context) {
			a := account
			a.Version = 6
			return def, a, ctx
		},
		"definition region": func() (resource.Definition, application.Connection, planning.Context) {
			d := def
			d.DriverInputs = map[string]any{"values": map[string]any{"variables": map[string]any{"region": "eu-west-1"}}}
			return d, account, ctx
		},
		"profile": func() (resource.Definition, application.Connection, planning.Context) {
			return def, account, pinContext(application.ProfileInternalK8s, "env-account", "eu-west-1", 2)
		},
		"generation": func() (resource.Definition, application.Connection, planning.Context) {
			return def, account, pinContext(application.ProfileAWSEKS, "env-account", "eu-west-1", 3)
		},
	}
	for name, build := range stale {
		d, a, c := build()
		if err := checkPinsAgainstPlan(plan, "redis.default#modules.x", []tfmodule.ProviderPin{good, aws}, d, a, c); !errors.Is(err, ErrProviderPin) {
			t.Errorf("%s drift accepted: %v", name, err)
		}
	}
	foreign := good
	foreign.Provider = "k8s-cluster.eks#environments.other.env"
	if err := checkPinsAgainstPlan(plan, "redis.default#modules.x", []tfmodule.ProviderPin{foreign}, def, account, ctx); !errors.Is(err, ErrProviderPin) {
		t.Errorf("a cluster without a graph edge was accepted: %v", err)
	}
	none := good
	none.Provider = ""
	if err := checkPinsAgainstPlan(plan, "redis.default#modules.x", []tfmodule.ProviderPin{none}, def, account, ctx); !errors.Is(err, ErrProviderPin) {
		t.Errorf("a Kubernetes pin without a cluster was accepted: %v", err)
	}
}
