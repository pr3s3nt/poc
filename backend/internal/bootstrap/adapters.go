package bootstrap

import (
	"fmt"

	"orchestrator/internal/adapters/gitops"
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
func realAdapters(opts Options, credentials execution.KubeconfigSource) (execution.ExecutorRegistry, execution.WorkloadRenderer, execution.WorkloadDeployer, error) {
	kubernetesExecutor := k8s.NewExecutor(opts.KubectlPath)
	kubernetesExecutor.Credentials = credentials
	existingCluster := k8s.NewExistingClusterAdapter(opts.KubectlPath)
	existingCluster.Credentials = credentials
	executors := map[resource.DriverType]execution.ResourceExecutor{
		resource.DriverKubernetes:      kubernetesExecutor,
		resource.DriverExistingCluster: existingCluster,
	}
	if opts.Adapters == AdapterAWS {
		terraformExecutor, err := newTerraformExecutor(opts)
		if err != nil {
			return nil, nil, nil, err
		}
		executors[resource.DriverTerraform] = terraformExecutor
	}
	direct := k8s.NewDeployer(opts.KubectlPath)
	direct.Credentials = credentials
	deployer := execution.WorkloadDeployer(direct)
	if opts.WorkloadDelivery == "fleet-gitrepo" {
		if opts.Adapters != AdapterKubernetes {
			return nil, nil, nil, fmt.Errorf("bootstrap: Fleet GitRepo delivery is only supported on internal Kubernetes")
		}
		fleetDeployer, err := gitops.New(gitops.Options{
			RepoDir: opts.GitOpsRepoDir, Branch: opts.GitOpsBranch,
			GitRepoName: opts.FleetGitRepoName, KubeContext: opts.Seed.KubeContext,
			KubectlPath: opts.KubectlPath, RegistryHost: opts.HarborRegistryHost,
			DockerConfigFile: opts.HarborDockerConfigFile, PullSecretName: opts.HarborPullSecretName,
		})
		if err != nil {
			return nil, nil, nil, err
		}
		deployer = fleetDeployer
	} else if opts.WorkloadDelivery != "" && opts.WorkloadDelivery != "direct" {
		return nil, nil, nil, fmt.Errorf("bootstrap: unknown workload delivery %q", opts.WorkloadDelivery)
	}
	return registry{executors: executors}, k8s.NewRenderer(), deployer, nil
}
