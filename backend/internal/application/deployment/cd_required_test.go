package deployment_test

import (
	"context"
	"errors"
	"testing"

	"orchestrator/internal/adapters/terraform/registry/registrytest"
	deliveryapp "orchestrator/internal/application/delivery"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/planning"
	"orchestrator/internal/platform/clock"
	"orchestrator/internal/ports/execution"
)

func TestRealDeploymentRequiresCDBeforeProvisioningOrDirectApply(t *testing.T) {
	app, opts := newApp(t)
	// Nil provisioning and rendering collaborators are deliberate: an attempt to
	// use either before rejecting the missing CD binding would panic this test.
	svc := appsvc.NewService(app.Store, planning.NewService(bootstrap.FakeTemplateSources()), nil, nil, &rejectDirectApply{}, registrytest.Inspector(t, app.Store), clock.System{})
	_, err := svc.DeployWorkload(context.Background(), appsvc.DeployCommand{
		OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, WorkloadID: "frontend", Actor: "test",
		WorkloadAfter: map[string]any{"apiVersion": "orchestrator.dev/workload/v1", "metadata": map[string]any{"name": "frontend"}, "containers": map[string]any{"main": map[string]any{"image": "frontend:v1"}}},
	})
	if !errors.Is(err, deliveryapp.ErrCDRequired) {
		t.Fatalf("missing CD did not fail before infrastructure/workload effects: %v", err)
	}
}

// Real deployment must never invoke this collaborator as a fallback.
var _ execution.WorkloadDeployer = (*rejectDirectApply)(nil)

type rejectDirectApply struct{}

func (*rejectDirectApply) Apply(context.Context, execution.Target, []execution.Manifest) error {
	panic("direct application apply is forbidden")
}
func (*rejectDirectApply) WaitReady(context.Context, execution.Target, []execution.WorkloadRef) error {
	panic("direct application readiness is forbidden")
}
func (*rejectDirectApply) Remove(context.Context, execution.Target, string) error {
	panic("new CD inventories must remove through their provider")
}
