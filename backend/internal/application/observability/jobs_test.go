package observability

import (
	"context"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/ports/execution"
	obs "orchestrator/internal/ports/observability"
	"orchestrator/internal/ports/persistence/persistencetest"
)

type jobReader struct {
	receivedReader
	inspected int
	ref       execution.ObjectRef
}

func (r *jobReader) InspectJobs(_ context.Context, source obs.Source, ref execution.ObjectRef) obs.JobHistory {
	r.inspected++
	r.source = source
	r.ref = ref
	return obs.JobHistory{Code: "OK", Jobs: []obs.Job{}}
}
func TestJobsUseOnlyAppliedScopedInventory(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	f := persistencetest.Telemetry(t, st)
	dep := deployment.Deployment{ID: "job-deployment", OrganizationKey: f.Scope.OrganizationKey, ApplicationKey: f.Scope.ApplicationKey, EnvironmentKey: f.Scope.EnvironmentKey, WorkloadID: "backup"}
	if err := st.SaveDeployment(ctx, dep); err != nil {
		t.Fatal(err)
	}
	instance := deployment.WorkloadInstance{ID: "job-instance", EnvironmentKey: f.Scope.ApplicationKey + "/" + f.Scope.EnvironmentKey, WorkloadID: "backup", LastDeploymentID: dep.ID, Status: deployment.InstanceReady, TargetRef: map[string]any{"organization": f.Scope.OrganizationKey, "connection": "job-account", "namespace": "app-staging", "context": "selected-context", "objects": []any{map[string]any{"apiVersion": "batch/v1", "kind": "CronJob", "name": "backup", "owners": []any{dep.ID}}}}}
	if err := st.UpsertWorkloadInstance(ctx, instance); err != nil {
		t.Fatal(err)
	}
	reader := &jobReader{}
	service := New(st, reader)
	if _, err := service.Jobs(ctx, f.Scope, "backup"); err != nil {
		t.Fatal(err)
	}
	if reader.inspected != 1 || reader.source.Target.Connection != "job-account" || reader.ref.Owners[0] != dep.ID {
		t.Fatal("did not use applied target/inventory")
	}
	foreign := f.Scope
	foreign.OrganizationKey = "foreign"
	if _, err := service.Jobs(ctx, foreign, "backup"); err == nil {
		t.Fatal("foreign scope accepted")
	}
	if reader.inspected != 1 {
		t.Fatal("foreign scope reached cluster reader")
	}
	instance.Status = deployment.InstanceRemoved
	if err := st.UpsertWorkloadInstance(ctx, instance); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Jobs(ctx, f.Scope, "backup"); err == nil {
		t.Fatal("removed workload queried")
	}
	if reader.inspected != 1 {
		t.Fatal("removed workload reached reader")
	}
}
