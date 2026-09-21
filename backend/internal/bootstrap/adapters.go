package bootstrap

import (
	"fmt"

	k8s "orchestrator/internal/adapters/kubernetes"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/execution"
)

func kubernetesRenderer() execution.WorkloadRenderer { return k8s.NewRenderer() }

// registry maps a Driver Type onto its executor adapter (UC-08 MS-04).
type registry struct {
	executors map[resource.DriverType]execution.ResourceExecutor
}

// Resolve returns the executor registered for a driver type.
func (r registry) Resolve(driver resource.DriverType) (execution.ResourceExecutor, error) {
	executor, ok := r.executors[driver]
	if !ok {
		return nil, fmt.Errorf("bootstrap: no executor registered for driver %q", driver)
	}
	return executor, nil
}

// realAdapters wires the cluster and cloud executors used outside the walking
// skeleton. Kubernetes mode covers internal-k8s; AWS mode adds Terraform.
func realAdapters(opts Options) (execution.ExecutorRegistry, execution.WorkloadRenderer, execution.WorkloadDeployer, error) {
	executors := map[resource.DriverType]execution.ResourceExecutor{
		resource.DriverKubernetes:      k8s.NewExecutor(opts.KubectlPath),
		resource.DriverExistingCluster: k8s.NewExistingClusterAdapter(opts.KubectlPath),
	}
	if opts.Adapters == AdapterAWS {
		terraformExecutor, err := newTerraformExecutor(opts)
		if err != nil {
			return nil, nil, nil, err
		}
		executors[resource.DriverTerraform] = terraformExecutor
	}
	return registry{executors: executors}, k8s.NewRenderer(), k8s.NewDeployer(opts.KubectlPath), nil
}
