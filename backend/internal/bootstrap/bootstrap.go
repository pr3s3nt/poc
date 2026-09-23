// Package bootstrap wires the orchestrator process: store, adapters, application
// services and the HTTP delivery layer. Tests reuse it so production wiring and
// test wiring cannot drift apart.
package bootstrap

import (
	"context"
	"fmt"

	"orchestrator/internal/adapters/fake"
	"orchestrator/internal/adapters/secrets"
	"orchestrator/internal/adapters/store"
	tf "orchestrator/internal/adapters/terraform"
	appcreate "orchestrator/internal/application/application"
	"orchestrator/internal/application/authentication"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/provisioning"
	deliveryhttp "orchestrator/internal/delivery/http"
	"orchestrator/internal/planning"
	"orchestrator/internal/platform/clock"
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
	Region  string
	Tags    map[string]string
	WorkDir string
	Clock   clock.Clock

	// Overrides replace individual adapters. Tests use them to inject failures.
	RegistryOverride execution.ExecutorRegistry
	RendererOverride execution.WorkloadRenderer
	DeployerOverride execution.WorkloadDeployer
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
	queries := appsvc.NewQueryService(st)
	auth := authentication.NewService(st)
	applications := appcreate.NewService(st)

	server := deliveryhttp.NewServer(deliveryhttp.Config{
		Deployments:    deployments,
		Queries:        queries,
		Authentication: auth,
		Applications:   applications,
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
