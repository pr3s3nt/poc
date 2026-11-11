package deployment_test

import (
	"context"
	"orchestrator/internal/adapters/store"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/envops"
	"orchestrator/internal/domain/application"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/progress"
	"orchestrator/internal/platform/ids"
	"testing"
	"time"
)

func TestProgressHistoryUsesExactOperationBeforeAPlanExists(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	app := application.Application{ID: ids.New(), Key: "app", OrganizationKey: "acme", Name: "App", Version: 1}
	if err := st.SaveApplication(ctx, app); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveEnvironment(ctx, environment.Environment{ID: ids.New(), ApplicationID: app.ID, ApplicationKey: app.Key, Key: "staging", Version: 1}); err != nil {
		t.Fatal(err)
	}
	m := envops.NewManager(st)
	requestID := ids.New()
	lease, err := m.Begin(ctx, envops.Claim{ApplicationKey: app.Key, EnvironmentKey: "staging", Kind: environment.OpDeploy, ReviewRequestID: requestID})
	if err != nil {
		t.Fatal(err)
	}
	record := domain.Deployment{ID: ids.New(), ReviewRequestID: requestID, OrganizationKey: app.OrganizationKey, ApplicationKey: app.Key, EnvironmentKey: "staging", Status: domain.StatusFailed, StartedAt: time.Now()}
	if err := st.SaveDeployment(lease.Ctx, record); err != nil {
		t.Fatal(err)
	}
	progress.Report(lease.Ctx, progress.Step{ID: "old", Kind: "PLAN", Status: "FAILED", Reason: "UNKNOWN"})
	if err := lease.End(environment.OpFailed, "failed"); err != nil {
		t.Fatal(err)
	}
	next, err := m.Begin(ctx, envops.Claim{ApplicationKey: app.Key, EnvironmentKey: "staging", Kind: environment.OpDeploy, ReviewRequestID: ids.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer next.End(environment.OpFailed, "")
	progress.Report(next.Ctx, progress.Step{ID: "new", Kind: "READINESS", Status: "RUNNING", Pods: []progress.Pod{{Name: "other-pod", Reason: "CrashLoopBackOff"}}})
	view, err := appsvc.NewQueryService(st).GetDeployment(ctx, appsvc.GetDeploymentQuery{OrganizationKey: app.OrganizationKey, ApplicationKey: app.Key, EnvironmentKey: "staging", DeploymentID: record.ID})
	if err != nil {
		t.Fatal(err)
	}
	if view.Progress == nil || view.OperationStatus != "FAILED" {
		t.Fatalf("missing pre-plan history: %+v", view)
	}
	found := false
	for _, step := range view.Progress.Steps {
		if step.ID == "old" {
			found = true
		}
		if step.ID == "new" {
			t.Fatal("history leaked a later operation")
		}
	}
	if !found {
		t.Fatal("history lost the original operation")
	}
}
