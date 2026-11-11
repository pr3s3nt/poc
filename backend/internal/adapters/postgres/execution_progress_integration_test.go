package postgres

import (
	"context"
	"orchestrator/internal/application/envops"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/progress"
	"orchestrator/internal/platform/ids"
	"testing"
)

func TestExecutionProgressSurvivesReopenWithRuntimeRole(t *testing.T) {
	ctx := context.Background()
	db := freshDatabase(t)
	console, err := Open(ctx, db.Console)
	if err != nil {
		t.Fatal(err)
	}
	defer console.Close()
	org := application.Organization{ID: ids.New(), Key: "progress", Name: "Progress"}
	if err := console.SaveOrganization(ctx, org); err != nil {
		t.Fatal(err)
	}
	app := application.Application{ID: ids.New(), Key: "progress", OrganizationKey: org.Key, Name: "Progress", Version: 1}
	if err := console.SaveApplication(ctx, app); err != nil {
		t.Fatal(err)
	}
	env := environment.Environment{ID: ids.New(), ApplicationID: app.ID, ApplicationKey: app.Key, Key: "staging", Name: "staging", Type: "staging", Version: 1}
	if err := console.SaveEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	lease, err := envops.NewManager(console).Begin(ctx, envops.Claim{ApplicationKey: app.Key, EnvironmentKey: env.Key, Kind: environment.OpDeploy})
	if err != nil {
		t.Fatal(err)
	}
	progress.Report(lease.Ctx, progress.Step{ID: "wait", Kind: "READINESS", Status: "RUNNING", Pods: []progress.Pod{{Name: "api-1", Reason: "ImagePullBackOff", Image: "ghcr.io/acme/api:missing"}}})
	if err := lease.Stage("DEPLOY", map[string]any{"checkpoint": true}); err != nil {
		t.Fatal(err)
	}
	if err := lease.End(environment.OpFailed, "not ready"); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, db.Console)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	op, err := reopened.GetOperation(ctx, lease.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	r := progress.Read(op.Detail["progress"])
	if op.Status != environment.OpFailed || len(r.Steps) != 3 || r.Steps[1].Status != "FAILED" || r.Steps[1].FinishedAt == nil || r.Steps[1].Pods[0].Reason != "ImagePullBackOff" {
		t.Fatalf("lost durable progress: %+v", r)
	}
}
