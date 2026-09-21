// Package fake provides deterministic executor and deployer adapters for the
// local walking skeleton. They touch no cluster and no cloud account.
package fake

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/execution"
)

// ResourceExecutor returns deterministic outputs for every supported resource type.
type ResourceExecutor struct {
	mu    sync.Mutex
	Calls []execution.ProvisionRequest
}

// NewResourceExecutor returns an empty fake executor.
func NewResourceExecutor() *ResourceExecutor { return &ResourceExecutor{} }

// Provision records the call and synthesises contract-compliant outputs.
func (e *ResourceExecutor) Provision(_ context.Context, req execution.ProvisionRequest) (execution.ProvisionResult, error) {
	e.mu.Lock()
	e.Calls = append(e.Calls, req)
	e.mu.Unlock()

	id := stringInput(req.Inputs, "name")
	if id == "" {
		id = req.Descriptor
	}
	switch req.ResourceType {
	case "vpc":
		return execution.ProvisionResult{
			Outputs: map[string]any{
				"id":        "vpc-fake-" + req.ApplicationKey,
				"cidr":      "10.42.0.0/16",
				"subnetIds": "subnet-fake-a,subnet-fake-b",
			},
			State: map[string]any{"driver": "fake", "descriptor": req.Descriptor},
		}, nil
	case "k8s-cluster":
		return execution.ProvisionResult{
			Outputs: map[string]any{
				"name":        "fake-" + req.ApplicationKey,
				"endpoint":    "https://fake-cluster.invalid",
				"kubeContext": "fake-context",
			},
			State: map[string]any{"driver": "fake", "descriptor": req.Descriptor},
		}, nil
	case "k8s-namespace":
		return execution.ProvisionResult{
			Outputs: map[string]any{"name": id},
			State:   map[string]any{"driver": "fake", "descriptor": req.Descriptor},
		}, nil
	case "postgres":
		db := stringInput(req.Inputs, "database")
		if db == "" {
			db = "app"
		}
		host := fmt.Sprintf("%s.%s.svc.cluster.local", id, req.Target.Namespace)
		return execution.ProvisionResult{
			Outputs: map[string]any{
				"host":     host,
				"port":     float64(5432),
				"database": db,
				"username": "app",
				"password": "fake-password-" + req.Descriptor,
			},
			State: map[string]any{"driver": "fake", "descriptor": req.Descriptor},
		}, nil
	default:
		return execution.ProvisionResult{}, fmt.Errorf("fake: no output template for resource type %q", req.ResourceType)
	}
}

// Descriptors lists the descriptors this executor was called with, in call order.
func (e *ResourceExecutor) Descriptors() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]string, 0, len(e.Calls))
	for _, c := range e.Calls {
		out = append(out, c.Descriptor)
	}
	return out
}

func stringInput(inputs map[string]any, key string) string {
	if v, ok := inputs[key].(string); ok {
		return v
	}
	return ""
}

// Registry resolves every driver type to the same fake executor.
type Registry struct {
	Executor *ResourceExecutor
}

// Resolve returns the fake executor for any driver type.
func (r Registry) Resolve(resource.DriverType) (execution.ResourceExecutor, error) {
	return r.Executor, nil
}

// AppliedWorkload records one fake apply call.
type AppliedWorkload struct {
	Target    execution.Target
	Manifests []execution.Manifest
}

// WorkloadDeployer records manifests instead of contacting a cluster.
type WorkloadDeployer struct {
	mu      sync.Mutex
	Applied []AppliedWorkload
	Ready   []execution.WorkloadRef
}

// NewWorkloadDeployer returns an empty fake deployer.
func NewWorkloadDeployer() *WorkloadDeployer { return &WorkloadDeployer{} }

// Apply records the manifests.
func (d *WorkloadDeployer) Apply(_ context.Context, target execution.Target, manifests []execution.Manifest) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Applied = append(d.Applied, AppliedWorkload{Target: target, Manifests: manifests})
	return nil
}

// WaitReady records the refs and reports success.
func (d *WorkloadDeployer) WaitReady(_ context.Context, _ execution.Target, refs []execution.WorkloadRef) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Ready = append(d.Ready, refs...)
	return nil
}

// AppliedNames lists every applied object as kind/name, sorted.
func (d *WorkloadDeployer) AppliedNames() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []string
	for _, a := range d.Applied {
		for _, m := range a.Manifests {
			out = append(out, m.Kind+"/"+m.Name)
		}
	}
	sort.Strings(out)
	return out
}

var (
	_ execution.ResourceExecutor = (*ResourceExecutor)(nil)
	_ execution.ExecutorRegistry = Registry{}
	_ execution.WorkloadDeployer = (*WorkloadDeployer)(nil)
)
