package deployment_test

import (
	"context"
	"errors"
	"testing"

	appsvc "orchestrator/internal/application/deployment"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/resource"
)

// postFixture: two workloads share one workload Definition whose environment-scoped companion is_dependent
// follows BOTH of them.
func postFixture(t *testing.T) *placedFixture {
	t.Helper()
	f := placedApp(t)
	def, err := loadDefinition(f, "workload-score")
	if err != nil {
		t.Fatal(err)
	}
	def.Version++
	def.Provision = map[string]resource.ProvisionRule{"k8s-namespace.default#environments.@app.@env-post": {IsDependent: true}}
	if err := f.app.Store.SaveResourceDefinition(context.Background(), f.opts.OrganizationKey, def); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *placedFixture) deployWorkload(id string, before, after map[string]any) error {
	if after != nil {
		after["metadata"] = map[string]any{"name": id}
	}
	if before != nil {
		before["metadata"] = map[string]any{"name": id}
	}
	_, err := f.app.Deployments.DeployWorkload(context.Background(), appsvc.DeployCommand{
		OrganizationKey: f.opts.OrganizationKey, ApplicationKey: f.appKey, EnvironmentKey: "staging", WorkloadID: id,
		ScoreBefore: before, ScoreAfter: after, Actor: "test", RunID: "run-post-" + id,
	})
	return err
}

func (f *placedFixture) postCalls() int {
	n := 0
	for _, call := range f.app.FakeExec.Calls {
		if len(call.Descriptor) >= 5 && call.Descriptor[len(call.Descriptor)-5:] == "-post" {
			n++
		}
	}
	return n
}

func TestPlacedDeploy_PostResourceNeedsEveryWorkloadItFollows(t *testing.T) {
	ctx := context.Background()
	// Control: with the first workload applied on the planned target of this generation the second unlocks it.
	f := postFixture(t)
	if err := f.deployWorkload("api", nil, f.frontend("example.invalid/api:1")); err != nil {
		t.Fatal(err)
	}
	if err := f.deployWorkload("web", nil, f.frontend("example.invalid/web:1")); err != nil {
		t.Fatalf("control: %v", err)
	}
	// It ran after the first workload (which alone it followed) and again after the second.
	if f.postCalls() != 2 {
		t.Fatalf("the post resource must follow each applied workload: %d", f.postCalls())
	}

	// A provider instance that is not READY, from an old generation, or on another account never unlocks it.
	var oldRows []domain.WorkloadInstance
	for name, mutate := range map[string]func(*domain.WorkloadInstance){
		"not ready": func(i *domain.WorkloadInstance) { i.Status = domain.InstanceFailed },
		// The READY row of an old generation exists, the current generation's row is not READY.
		"old generation": func(i *domain.WorkloadInstance) {
			old := *i
			i.Status = domain.InstanceFailed
			old.Generation = 7
			oldRows = append(oldRows, old)
		},
		"other account": func(i *domain.WorkloadInstance) {
			placement, _ := i.TargetRef["placement"].(map[string]any)
			placement["account"] = "lab2"
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := postFixture(t)
			if err := f.deployWorkload("api", nil, f.frontend("example.invalid/api:1")); err != nil {
				t.Fatal(err)
			}
			key := f.appKey + "/staging"
			instances, _ := f.app.Store.ListWorkloadInstancesFor(ctx, key, 0)
			for _, instance := range instances {
				if instance.WorkloadID == "api" {
					mutate(&instance)
					if err := f.app.Store.UpsertWorkloadInstance(ctx, instance); err != nil {
						t.Fatal(err)
					}
				}
			}
			for _, old := range oldRows {
				if err := f.app.Store.UpsertWorkloadInstance(ctx, old); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := f.app.Store.GetEnvironment(ctx, f.appKey, "staging")
			callsBefore := f.postCalls()
			err := f.deployWorkload("web", nil, f.frontend("example.invalid/web:1"))
			if !errors.Is(err, appsvc.ErrDependencyNotReady) {
				t.Fatalf("the second workload must fail safely, not succeed: %v", err)
			}
			after, _ := f.app.Store.GetEnvironment(ctx, f.appKey, "staging")
			if after.CurrentDeploymentSetID != before.CurrentDeploymentSetID {
				t.Fatal("a failed required stage must not commit the current Deployment Set")
			}
			if f.postCalls() != callsBefore {
				t.Fatal("the post resource must not run again")
			}
		})
	}
}
