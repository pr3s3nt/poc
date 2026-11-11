package deployment_test

import (
	"context"
	appsvc "orchestrator/internal/application/deployment"
	"testing"
)

func TestDeploymentPreparationSummariesPersistThroughQuery(t *testing.T) {
	app, options := newApp(t)
	results := deployAll(t, app, options)
	for _, result := range results {
		view, err := app.Queries.GetDeployment(context.Background(), appsvc.GetDeploymentQuery{OrganizationKey: options.OrganizationKey, ApplicationKey: options.ApplicationKey, EnvironmentKey: options.EnvironmentKey, DeploymentID: result.DeploymentID})
		if err != nil {
			t.Fatal(err)
		}
		if view.Progress == nil {
			t.Fatal("missing execution record")
		}
		secrets, rendered := false, false
		for _, step := range view.Progress.Steps {
			if step.DeploymentID != result.DeploymentID {
				continue
			}
			if step.Kind == "SECRETS" {
				if step.Status != "SUCCEEDED" || step.SecretBindings == nil {
					t.Fatalf("missing preparation result: %+v", step)
				}
				secrets = true
			}
			if step.Kind == "RENDER" {
				if step.Status != "SUCCEEDED" || step.Rendered == nil || step.Rendered.Objects == 0 || len(step.Rendered.Kinds) == 0 {
					t.Fatalf("missing render result: %+v", step)
				}
				rendered = true
			}
		}
		if !secrets || !rendered {
			t.Fatalf("workload %s missing preparation steps", result.WorkloadID)
		}
	}
}
