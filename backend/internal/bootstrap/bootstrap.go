// Package bootstrap wires the orchestrator process: store, adapters, application
// services and the HTTP delivery layer. Tests reuse it so production wiring and
// test wiring cannot drift apart.
package bootstrap

import (
	"context"
	"fmt"
	"os"
	"strings"

	"orchestrator/internal/adapters/configmemory"
	"orchestrator/internal/adapters/fake"
	"orchestrator/internal/adapters/gitops"
	k8s "orchestrator/internal/adapters/kubernetes"
	"orchestrator/internal/adapters/secrets"
	"orchestrator/internal/adapters/store"
	tf "orchestrator/internal/adapters/terraform"
	"orchestrator/internal/adapters/vault"
	appcreate "orchestrator/internal/application/application"
	"orchestrator/internal/application/authentication"
	appconfig "orchestrator/internal/application/configuration"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/pending"
	"orchestrator/internal/application/provisioning"
	workloadconfig "orchestrator/internal/application/workloadconfig"
	deliveryhttp "orchestrator/internal/delivery/http"
	"orchestrator/internal/planning"
	"orchestrator/internal/platform/clock"
	configport "orchestrator/internal/ports/configuration"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/seed"
)

// AdapterMode selects the executor adapters used by UC-08 and UC-06.
type AdapterMode string

// Adapter modes.
const (
	// AdapterFake keeps everything in process: no cluster and no cloud account.
	AdapterFake AdapterMode = "fake"
	// AdapterKubernetes provisions real Kubernetes objects through kubectl.
	AdapterKubernetes AdapterMode = "kubernetes"
	// AdapterAWS adds Terraform-managed VPC, EKS and Aurora on top of Kubernetes.
	AdapterAWS AdapterMode = "aws"
)

// Options configures one orchestrator instance.
type Options struct {
	Seed          seed.Options
	UIDir         string
	StatePath     string
	Adapters      AdapterMode
	KubectlPath   string
	TerraformPath string
	TerraformRoot string

	// TerraformPluginCache keeps provider downloads shared between workspaces.
	TerraformPluginCache string
	// Region and Tags apply to every cloud resource of one verification run.
	Region                 string
	Tags                   map[string]string
	WorkDir                string
	Clock                  clock.Clock
	VaultAddress           string
	VaultTokenFile         string
	VaultAgentAddress      string
	VaultDelivery          string
	WorkloadDelivery       string
	GitOpsRepoDir          string
	GitOpsBranch           string
	FleetGitRepoName       string
	HarborRegistryHost     string
	HarborDockerConfigFile string
	HarborPullSecretName   string

	// Overrides replace individual adapters. Tests use them to inject failures.
	RegistryOverride              execution.ExecutorRegistry
	RendererOverride              execution.WorkloadRenderer
	DeployerOverride              execution.WorkloadDeployer
	ConfigurationProviderOverride configport.Provider
}

// App holds the built components.
type App struct {
	Store       *store.Store
	Secrets     *secrets.Memory
	Server      *deliveryhttp.Server
	Deployments *appsvc.Service
	Queries     *appsvc.QueryService
	FakeExec    *fake.ResourceExecutor
	FakeDeploy  *fake.WorkloadDeployer
}

// Build wires every component and seeds the catalog.
func Build(ctx context.Context, opts Options) (*App, error) {
	if opts.WorkloadDelivery == "fleet-gitrepo" && opts.Adapters != AdapterKubernetes {
		return nil, fmt.Errorf("bootstrap: Fleet GitRepo delivery requires Kubernetes adapters")
	}
	var (
		st  *store.Store
		err error
	)
	if opts.StatePath != "" {
		st, err = store.NewWithSnapshot(opts.StatePath)
		if err != nil {
			return nil, err
		}
	} else {
		st = store.New()
	}

	if err := seed.Apply(ctx, st, opts.Seed); err != nil {
		return nil, err
	}

	secretStore := secrets.NewMemory()
	c := opts.Clock
	if c == nil {
		c = clock.System{}
	}

	var (
		registry execution.ExecutorRegistry
		renderer execution.WorkloadRenderer
		deployer execution.WorkloadDeployer
		fakeExec *fake.ResourceExecutor
		fakeDep  *fake.WorkloadDeployer
	)
	switch opts.Adapters {
	case "", AdapterFake:
		fakeExec = fake.NewResourceExecutor()
		fakeDep = fake.NewWorkloadDeployer()
		registry = fake.Registry{Executor: fakeExec}
		renderer = kubernetesRenderer()
		deployer = fakeDep
	case AdapterKubernetes, AdapterAWS:
		registry, renderer, deployer, err = realAdapters(opts)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("bootstrap: unknown adapter mode %q", opts.Adapters)
	}

	if opts.RegistryOverride != nil {
		registry = opts.RegistryOverride
	}
	if opts.RendererOverride != nil {
		renderer = opts.RendererOverride
	}
	if opts.DeployerOverride != nil {
		deployer = opts.DeployerOverride
	}

	prov := provisioning.NewService(st, registry, secretStore, c)
	deployments := appsvc.NewService(st, planning.NewService(), prov, renderer, deployer, tf.NewInspector(), c)
	if opts.Adapters == AdapterKubernetes {
		if opts.WorkloadDelivery == "fleet-gitrepo" {
			fleet, ok := deployer.(*gitops.Deployer)
			if !ok {
				return nil, fmt.Errorf("bootstrap: Fleet route delivery requires GitOps deployer")
			}
			deployments.SetPublicRouteManager(fleet, opts.Seed.BaseDomain)
		} else {
			deployments.SetPublicRouteManager(&k8s.PublicRoutes{KubectlPath: opts.KubectlPath}, opts.Seed.BaseDomain)
		}
	}
	if opts.WorkloadDelivery == "fleet-gitrepo" {
		deployments.SetImagePullSecret(opts.HarborPullSecretName)
	}
	queries := appsvc.NewQueryService(st)
	auth := authentication.NewService(st)
	applications := appcreate.NewService(st)
	var configProvider configport.Provider
	if opts.ConfigurationProviderOverride != nil {
		configProvider = opts.ConfigurationProviderOverride
	} else if opts.VaultAddress != "" && opts.VaultTokenFile != "" {
		tokenBytes, err := os.ReadFile(opts.VaultTokenFile)
		if err != nil {
			return nil, fmt.Errorf("bootstrap: read Vault token file: %w", err)
		}
		provider, err := vault.New(opts.VaultAddress, strings.TrimSpace(string(tokenBytes)), "kv", nil)
		if err != nil {
			return nil, err
		}
		if opts.VaultAgentAddress != "" {
			if err := provider.SetAgentAddress(opts.VaultAgentAddress); err != nil {
				return nil, err
			}
		}
		configProvider = provider
	} else if opts.Adapters == "" || opts.Adapters == AdapterFake {
		configProvider = configmemory.New()
	} else {
		configProvider = configmemory.Unavailable{}
	}
	configurations := appconfig.NewService(st, configProvider)
	deployments.SetConfigurationProvider(configProvider)
	vaultDelivery := opts.VaultDelivery
	if vaultDelivery == "auto" && opts.Adapters == AdapterKubernetes && opts.VaultAddress != "" {
		vaultDelivery = "vso"
	} else if vaultDelivery == "auto" {
		vaultDelivery = "agent"
	}
	if vaultDelivery == "vso" {
		if opts.Adapters != AdapterKubernetes || opts.VaultAddress == "" || opts.VaultTokenFile == "" || opts.VaultAgentAddress == "" {
			return nil, fmt.Errorf("bootstrap: VSO delivery requires Kubernetes adapters and configured Vault API/in-cluster address")
		}
		deployments.SetConfigSecretSynchronizer(&k8s.VSOSynchronizer{KubectlPath: opts.KubectlPath})
	} else if vaultDelivery != "" && vaultDelivery != "agent" {
		return nil, fmt.Errorf("bootstrap: unknown Vault delivery %q", vaultDelivery)
	}
	workloads := workloadconfig.NewService(st)
	pendingChanges := pending.NewService(st, planning.NewService(), workloads, tf.NewInspector())
	if opts.WorkloadDelivery == "fleet-gitrepo" {
		pendingChanges.SetImageRegistryHost(opts.HarborRegistryHost)
	}
	pendingChanges.SetDeployer(deployments)

	server := deliveryhttp.NewServer(deliveryhttp.Config{
		Deployments:    deployments,
		Queries:        queries,
		Authentication: auth,
		Applications:   applications,
		Configurations: configurations,
		Workloads:      workloads,
		Pending:        pendingChanges,
		Store:          st,
		SeedOptions:    opts.Seed,
		UIDir:          opts.UIDir,
	})

	return &App{
		Store:       st,
		Secrets:     secretStore,
		Server:      server,
		Deployments: deployments,
		Queries:     queries,
		FakeExec:    fakeExec,
		FakeDeploy:  fakeDep,
	}, nil
}
