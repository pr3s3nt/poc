package tfexec_test

import (
	"context"
	"testing"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfresource"
)

func TestSafetyNetDestroysAWSResourcesInDependencyAndLayerOrder(t *testing.T) {
	h := newHarness(t)
	vpc := h.createInstance("run-1", "vpc.default#environments.app.staging", scopeOf("vpc"), nil)
	eks := h.createInstance("run-1", "k8s-cluster.eks#environments.app.staging", scopeOf("eks"), []string{vpc.InstanceID})
	ns := h.createInstance("run-1", "k8s-namespace.default#environments.app.staging", scopeOf("namespace"), []string{eks.InstanceID})
	aurora := h.createInstance("run-1", "postgres.cloud-managed#shared.acceptance-db", resource.Scope{Type: resource.ScopeEnvironment, ID: "staging"}, []string{vpc.InstanceID})
	cache := h.createInstance("run-1", "redis.cloud-managed#shared.acceptance-cache", resource.Scope{Type: resource.ScopeEnvironment, ID: "staging"}, []string{vpc.InstanceID})

	rep, err := h.safetyNet().Run(context.Background(), h.target())
	if err != nil || rep.Outcome != "CLEAN" || len(rep.Runs) != 1 || len(rep.Runs[0].Instances) != 5 {
		t.Fatalf("cleanup report %+v, error %v", rep, err)
	}
	positions := map[string]int{}
	for i, result := range rep.Runs[0].Instances {
		positions[result.InstanceID] = i
		if !result.Destroyed {
			t.Fatalf("resource %s was not destroyed: %+v", result.InstanceID, result)
		}
	}
	for _, id := range []string{aurora.InstanceID, cache.InstanceID} {
		if positions[id] >= positions[ns.InstanceID] {
			t.Fatalf("application resource %s was destroyed after namespace; positions=%v", id, positions)
		}
	}
	if !(positions[ns.InstanceID] < positions[eks.InstanceID] && positions[eks.InstanceID] < positions[vpc.InstanceID]) {
		t.Fatalf("namespace/EKS/VPC dependency order was not preserved: %v", positions)
	}
	for _, id := range []string{aurora.InstanceID, cache.InstanceID, ns.InstanceID, eks.InstanceID, vpc.InstanceID} {
		if h.status(id) != tfresource.InstanceDestroyed {
			t.Fatalf("resource %s is %s, not DESTROYED", id, h.status(id))
		}
	}
}
