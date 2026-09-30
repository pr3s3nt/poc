package provisioning_test

import (
	"context"
	"testing"

	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/seed"
)

func deployBackend(t *testing.T, profile application.ExecutionProfile) (*bootstrap.App, seed.Options, string, string) {
	t.Helper()
	seedOptions := seed.Defaults()
	seedOptions.Region = "us-east-1"
	seedOptions.AccountID = "000000000000"
	applicationKey := seedOptions.ApplicationKey
	if profile == application.ProfileAWSEKS {
		applicationKey = seedOptions.CloudApplicationKey
	}
	app, err := bootstrap.Build(context.Background(), bootstrap.Options{Seed: seedOptions, Adapters: bootstrap.AdapterFake})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	scores := seed.AcceptanceScores(seedOptions)
	result, err := app.Deployments.DeployWorkload(context.Background(), appsvc.DeployCommand{
		OrganizationKey: seedOptions.OrganizationKey,
		ApplicationKey:  applicationKey,
		EnvironmentKey:  seedOptions.EnvironmentKey,
		WorkloadID:      "backend",
		ScoreAfter:      scores["backend"],
		Actor:           "test",
		RunID:           "run-test",
	})
	if err != nil {
		t.Fatalf("deploy backend on %s: %v", profile, err)
	}
	return app, seedOptions, applicationKey, result.DeploymentID
}

func TestProvision_PostgresOutputContractEquivalentAcrossProfiles(t *testing.T) {
	ctx := context.Background()
	want := []string{"database", "host", "password", "port", "username"}

	for _, profile := range []application.ExecutionProfile{application.ProfileInternalK8s, application.ProfileAWSEKS} {
		app, seedOptions, applicationKey, deploymentID := deployBackend(t, profile)
		view, err := app.Queries.GetDeployment(ctx, appsvc.GetDeploymentQuery{OrganizationKey: seedOptions.OrganizationKey, ApplicationKey: applicationKey, EnvironmentKey: seedOptions.EnvironmentKey, DeploymentID: deploymentID})
		if err != nil {
			t.Fatalf("get deployment: %v", err)
		}
		found := false
		for _, res := range view.Resources {
			if res.ResourceType != "postgres" {
				continue
			}
			found = true
			for _, key := range want {
				if _, ok := res.Outputs[key]; !ok {
					t.Fatalf("%s postgres outputs are missing %q: %v", profile, key, res.Outputs)
				}
			}
		}
		if !found {
			t.Fatalf("%s deployment has no postgres resource", profile)
		}
	}
}

func TestProvision_ReusesApplicationScopedVPCAndEKS(t *testing.T) {
	ctx := context.Background()
	app, seedOptions, applicationKey, _ := deployBackend(t, application.ProfileAWSEKS)

	scores := seed.AcceptanceScores(seedOptions)
	if _, err := app.Deployments.DeployWorkload(ctx, appsvc.DeployCommand{
		OrganizationKey: seedOptions.OrganizationKey,
		ApplicationKey:  applicationKey,
		EnvironmentKey:  seedOptions.EnvironmentKey,
		WorkloadID:      "worker",
		ScoreAfter:      scores["worker"],
		Actor:           "test",
		RunID:           "run-test",
	}); err != nil {
		t.Fatalf("deploy worker: %v", err)
	}

	active, err := app.Store.ListActiveResources(ctx, seedOptions.OrganizationKey)
	if err != nil {
		t.Fatalf("list active resources: %v", err)
	}
	counts := map[string]int{}
	for _, a := range active {
		counts[a.Descriptor.Type]++
	}
	if counts["vpc"] != 1 || counts["k8s-cluster"] != 1 {
		t.Fatalf("an Application must keep one VPC and one EKS: %v", counts)
	}
	for _, a := range active {
		if a.Descriptor.Type == "vpc" && a.Version < 2 {
			t.Fatalf("the second deployment must reconcile the same VPC row, version %d", a.Version)
		}
	}
}
