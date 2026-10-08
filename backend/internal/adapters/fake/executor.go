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
	mu      sync.Mutex
	Calls   []execution.ProvisionRequest
	cluster *Cluster
}

// AttachCluster lets provisioned PostgreSQL resources appear in a fake Cluster.
func (e *ResourceExecutor) AttachCluster(c *Cluster) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cluster = c
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
				"kubeContext": contextInput(req.Inputs),
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
		e.mu.Lock()
		cluster := e.cluster
		e.mu.Unlock()
		if cluster != nil {
			cluster.EnsureDatabase(req.Target, id)
		}
		return execution.ProvisionResult{
			Outputs: map[string]any{
				"host":     host,
				"port":     float64(5432),
				"database": db,
				"username": "app",
				"password": "fake-password-" + req.Descriptor,
			},
			State: map[string]any{"driver": "fake", "descriptor": req.Descriptor, "kind": "StatefulSet", "name": id, "namespace": req.Target.Namespace},
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
	cluster *Cluster
	Applied []AppliedWorkload
	Ready   []execution.WorkloadRef
	Removed []string
}

// NewWorkloadDeployer returns an empty fake deployer.
func NewWorkloadDeployer() *WorkloadDeployer { return &WorkloadDeployer{} }

// AttachCluster lets applied Deployments appear in a fake Cluster.
func (d *WorkloadDeployer) AttachCluster(c *Cluster) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cluster = c
}

// Apply records the manifests.
func (d *WorkloadDeployer) Apply(_ context.Context, target execution.Target, manifests []execution.Manifest) error {
	d.mu.Lock()
	d.Applied = append(d.Applied, AppliedWorkload{Target: target, Manifests: manifests})
	cluster := d.cluster
	d.mu.Unlock()
	if cluster != nil {
		cluster.Observe(target, manifests)
	}
	return nil
}

// WaitReady records the refs and reports success.
func (d *WorkloadDeployer) WaitReady(_ context.Context, _ execution.Target, refs []execution.WorkloadRef) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Ready = append(d.Ready, refs...)
	return nil
}

func (d *WorkloadDeployer) Remove(_ context.Context, _ execution.Target, workloadID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Removed = append(d.Removed, workloadID)
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

// contextInput returns the Connection's kube context when the Definition passes
// it, so two logical Connections can name two fake clusters.
func contextInput(inputs map[string]any) string {
	if v := stringInput(inputs, "kubeContext"); v != "" {
		return v
	}
	return "fake-context"
}
