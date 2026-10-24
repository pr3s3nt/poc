// Package bootstrap wires the orchestrator process: store, adapters, application
// services and the HTTP delivery layer. Tests reuse it so production wiring and
// test wiring cannot drift apart.
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"orchestrator/internal/adapters/tlsprobe"
	"orchestrator/internal/application/accountdiag"
	diagprobes "orchestrator/internal/ports/diagnostics"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"orchestrator/internal/adapters/awsaccess"
	"orchestrator/internal/adapters/configmemory"
	"orchestrator/internal/adapters/credentialmemory"
	"orchestrator/internal/adapters/fake"
	"orchestrator/internal/adapters/gitops"
	k8s "orchestrator/internal/adapters/kubernetes"
	pgstore "orchestrator/internal/adapters/postgres"
	"orchestrator/internal/adapters/renderbundles"
	"orchestrator/internal/adapters/scorek8s"
	"orchestrator/internal/adapters/secrets"
	"orchestrator/internal/adapters/statecipher"
	"orchestrator/internal/adapters/store"
	moduleregistry "orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/adapters/vault"
	appcreate "orchestrator/internal/application/application"
	"orchestrator/internal/application/authentication"
	appconfig "orchestrator/internal/application/configuration"
	"orchestrator/internal/application/connection"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/deploymentreview"
	"orchestrator/internal/application/envops"
	"orchestrator/internal/application/pending"
	"orchestrator/internal/application/preview"
	"orchestrator/internal/application/provisioning"
	"orchestrator/internal/application/resourcecleanup"
	"orchestrator/internal/application/runcleanup"
	"orchestrator/internal/application/secretstores"
	"orchestrator/internal/application/statebackend"
	workloadconfig "orchestrator/internal/application/workloadconfig"
	deliveryhttp "orchestrator/internal/delivery/http"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/platform/clock"
	configport "orchestrator/internal/ports/configuration"
	"orchestrator/internal/ports/credentials"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/renderregistry"
	"orchestrator/internal/ports/terraformstate"
	"orchestrator/internal/renderengine"
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
	DatabaseURL   string
	Adapters      AdapterMode
	KubectlPath   string
	TerraformPath string
	ScoreK8sPath  string
	// RequireScoreRenderer is the product mode (UC-05 BR-09, UC-06 BR-17):
	// every Workload needs exactly one compatible score-k8s Definition and an
	// installed bundle on both profiles; there is no native renderer
	// fallback. A missing binary makes every Workload plan fail before
	// resources instead of rendering natively.
	RequireScoreRenderer bool
	TerraformRoot        string

	// TerraformPluginCache keeps provider downloads shared between workspaces.
	TerraformPluginCache string
	// TerraformModuleCacheDir is the private, absolute cache of verified
	// module content, outside any Git work tree. Empty uses a private
	// temporary directory removed on Close. Configure a durable directory to
	// reuse direct Git modules across restarts; legacy policies require one.
	TerraformModuleCacheDir string
	// TerraformModulePolicyFile is legacy source configuration and optional
	// repository CA settings; direct HTTPS inspection needs no allowlist.
	TerraformModulePolicyFile string
	// TerraformRetainedReleasesFile lists trusted historical platform releases
	// (manifest plus absolute Terraform binary path) kept for cleanup of
	// resources created before a release upgrade.
	TerraformRetainedReleasesFile string
	// GitPath is the git CLI used to resolve and fetch module commits.
	GitPath string
	// RenderBundleCacheDir is the durable private cache of verified Git
	// render bundle content (ADR-021). Empty uses a private temporary
	// directory removed on Close; production configures a volume.
	RenderBundleCacheDir string
	// RenderWorkerPath overrides the template worker executable; empty
	// runs the product binary itself.
	RenderWorkerPath string
	// Region and Tags apply to every cloud resource of one verification run.
	Region             string
	Tags               map[string]string
	WorkDir            string
	Clock              clock.Clock
	VaultDelivery      string
	WorkloadDelivery   string
	GitOpsRepoDir      string
	GitOpsBranch       string
	FleetGitRepoName   string
	HarborRegistryHost string
	// DiagnosticProbeImage is the platform-owned image of Kubernetes execution
	// diagnostics; empty selects the pause image a node already holds.
	DiagnosticProbeImage string
	// DiagnosticProvidersOverride replaces the diagnostic probe providers and
	// DiagnosticConfigOverride their timings (tests only; production always
	// probes the real account).
	DiagnosticProvidersOverride map[string]diagprobes.Provider
	DiagnosticConfigOverride    *accountdiag.Config
	HarborDockerConfigFile      string
	HarborPullSecretName        string

	// ConnectionCredentialStore selects the UC-04 Connection credential
	// store: "" (none; upload registration fails closed with 503), "vault"
	// (durable KV v2, scoped token read from ConnectionVaultTokenFile) or
	// "memory" (explicit non-durable local/test store, only with fake
	// adapters and no persistent state).
	ConnectionCredentialStore string
	ConnectionVaultAddress    string
	ConnectionVaultTokenFile  string
	ConnectionVaultMount      string

	// OutputSecretStore selects the UC-08 output-secret store: "" or "memory"
	// (in-process, lost on restart; local/test only) or "vault" (durable KV v2
	// under orchestrator/outputs, a token scoped to that path read from
	// OutputSecretVaultTokenFile). The real acceptance profile requires vault.
	OutputSecretStore string
	// EphemeralOutputSecrets is the explicit, labelled test/fixture opt-in to
	// the in-process output-secret store together with REAL adapters and the
	// durable SQL Terraform state backend. Without it that combination is
	// refused: the state would outlive the references to its output secrets.
	EphemeralOutputSecrets     bool
	OutputSecretVaultAddress   string
	OutputSecretVaultTokenFile string
	OutputSecretVaultMount     string

	// TerraformInvocationTimeout and TerraformReadinessTimeout bound one
	// Terraform session and the readiness query of an applied resource; zero
	// keeps the executor defaults (45m and 5m). Acceptance runs set finite,
	// smaller values inside their overall deadline.
	TerraformInvocationTimeout time.Duration
	// DeployDeadline is the finite hold time of one whole Deploy (all sequential
	// Terraform nodes and the delivery). Zero keeps the default; it is bounded
	// by envops.MaxDeadline.
	DeployDeadline            time.Duration
	TerraformReadinessTimeout time.Duration

	// TerraformStateKeyFile enables the Terraform v2 durable state backend: an
	// owner-only file holding the 32-byte AES-256-GCM state key (hex or
	// base64), kept outside the database. It requires PostgreSQL; there is no
	// plaintext, keyless, JSON or in-memory state backend.
	TerraformStateKeyFile string
	// TerraformStateKeyID names that key in every encrypted state row.
	TerraformStateKeyID string
	// TerraformRecoveryDir is the private (0700, outside any Git work tree)
	// directory of encrypted recovery artifacts of a state the backend could
	// not store. Required when the Terraform v2 executor is enabled (real
	// adapters with the state backend).
	TerraformRecoveryDir string
	// TerraformStateDatabaseURL is the connection URL of the State runtime
	// login (option A): a login separate from the Console login of
	// DatabaseURL, to the same database and schema. The state backend needs
	// both it and the key; neither falls back to the Console or owner login.
	TerraformStateDatabaseURL string

	// PlatformVault, when set, is the explicit opt-in Compose bootstrap of the
	// bundled Vault as an ordinary verified store (UC-04 SS-07/08). It is
	// the only runtime path that seeds a store from process configuration.
	PlatformVault *PlatformVaultBootstrap

	// CleanupEngineOverride replaces the engine that runs an admitted resource
	// cleanup. It exists for tests and for the local browser runner, which run
	// the real admission, claim and persistence over a deterministic fake
	// provider; production wiring uses the Terraform executor.
	CleanupEngineOverride resourcecleanup.Engine

	// Overrides replace individual adapters. Tests use them to inject failures.
	// ClusterReadinessOverride replaces the cluster usability proof (test seam; refused by the real acceptance profile).
	ClusterReadinessOverride   execution.ClusterReadiness
	RegistryOverride           execution.ExecutorRegistry
	KubeconfigVerifierOverride connection.KubeconfigVerifier
	// AWSVerifierOverride replaces the AWS access-key verifier (tests and
	// local deterministic transports). Without it the real read-only
	// awsaccess verifier is used for Kubernetes/AWS adapters and AWS
	// registration is unavailable (fails closed) for fake adapters, so a
	// fixture key can never reach live AWS.
	AWSVerifierOverride           connection.AWSVerifier
	EC2RoleProviderOverride       connection.EC2RoleProvider
	ConnectionCredentialsOverride credentials.Store
	RendererOverride              execution.WorkloadRenderer
	DeployerOverride              execution.WorkloadDeployer
	// StoreRegistryOverride and SecretStoreVerifierOverride replace the
	// workload secret-store registry and the registration verifier (tests).
	StoreRegistryOverride       configport.Registry
	SecretStoreVerifierOverride configport.Verifier
}

// PlatformVaultBootstrap is the input of the Compose Vault bootstrap. TokenFile
// is read once at startup; it is bootstrap input only and never a runtime path.
type PlatformVaultBootstrap struct {
	Address, WorkloadAddress, TokenFile, Mount, AuthMount string
}

// App holds the built components.
type App struct {
	// Diagnostics is the Infrastructure Account diagnostics service.
	Diagnostics *accountdiag.Service
	// RenderRegistry is the Git render bundle registry (ADR-021); nil
	// without the score-k8s renderer.
	RenderRegistry *renderbundles.Registry
	// ConnectionCredentials is nil when no credential store is configured.
	ConnectionCredentials credentials.Store
	Store                 persistence.Store
	Secrets               execution.SecretStore
	Server                *deliveryhttp.Server
	Deployments           *appsvc.Service
	Queries               *appsvc.QueryService
	Previews              *preview.Service
	// ResourceCleanup is the scoped resource detail and confirmed deletion service.
	ResourceCleanup *resourcecleanup.Service
	FakeExec        *fake.ResourceExecutor
	FakeDeploy      *fake.WorkloadDeployer
	// Configurations and Pending are the UC-12 and UC-05/06 services.
	Configurations *appconfig.Service
	Pending        *pending.Service
	// Workloads is the UC-16 draft editor service.
	Workloads *workloadconfig.Service
	// Operations is the shared Environment claim manager.
	Operations *envops.Manager
	// StoreRegistry resolves workload secret stores.
	StoreRegistry configport.Registry
	// Modules is the verified Terraform module registry.
	Modules *moduleregistry.Service
	// RunCleanup is the independent safety net: reverse destroy of every
	// instance a run left behind through the original cleanup engine. Nil
	// unless the Terraform executor exists (real adapters with the state
	// backend).
	RunCleanup *runcleanup.Service
	// WorkloadDeployer is the real workload deployer (Remove is the product's
	// own workload removal).
	WorkloadDeployer execution.WorkloadDeployer
	// FakeCluster is the in-memory Kubernetes stand-in of fake mode.
	FakeCluster *fake.Cluster
	// TerraformStates and TerraformStateHandler are the internal Terraform
	// state backend; nil unless a state key and PostgreSQL are configured.
	// The handler is never mounted on the Console API server.
	TerraformStates       *statebackend.Service
	TerraformStateHandler *deliveryhttp.TerraformStateHandler

	closers   []func()
	closeOnce sync.Once
}

// Close releases the database pools of the App (the Console store and the
// State runtime connection of the Terraform state backend). It is idempotent.
func (a *App) Close() {
	a.closeOnce.Do(func() { closeAll(a.closers) })
}

func closeAll(closers []func()) {
	for i := len(closers) - 1; i >= 0; i-- {
		closers[i]()
	}
}

// Build wires every component and seeds the catalog. When Build fails, every
// database pool it opened is closed before it returns; on success the caller
// releases them with App.Close.
func Build(ctx context.Context, opts Options) (*App, error) {
	var closers []func()
	app, err := build(ctx, opts, &closers)
	if err != nil {
		closeAll(closers)
		return nil, err
	}
	app.closers = closers
	return app, nil
}

func build(ctx context.Context, opts Options, closers *[]func()) (*App, error) {
	if err := validateRealProfile(opts); err != nil {
		return nil, err
	}
	if opts.WorkloadDelivery == "fleet-gitrepo" && opts.Adapters != AdapterKubernetes {
		return nil, fmt.Errorf("bootstrap: Fleet GitRepo delivery requires Kubernetes adapters")
	}
	var (
		st  persistence.Store
		err error
	)
	if opts.DatabaseURL != "" && opts.StatePath != "" {
		return nil, fmt.Errorf("bootstrap: configure only one of PostgreSQL or JSON state")
	}
	if opts.DatabaseURL != "" {
		pg, openErr := pgstore.Open(ctx, opts.DatabaseURL)
		if openErr != nil {
			return nil, openErr
		}
		*closers = append(*closers, pg.Close)
		st = pg
	} else if opts.StatePath != "" {
		st, err = store.NewWithSnapshot(opts.StatePath)
		if err != nil {
			return nil, err
		}
	} else {
		st = store.New()
	}
	if err != nil {
		return nil, err
	}

	modules, err := moduleRegistry(ctx, &opts, st, closers)
	if err != nil {
		return nil, err
	}

	if err := seed.Apply(ctx, st, opts.Seed); err != nil {
		return nil, err
	}

	terraformRuntime, err := terraformStateBackend(ctx, opts, st, closers)
	if err != nil {
		return nil, err
	}
	var terraformStates *statebackend.Service
	if terraformRuntime != nil {
		terraformStates = terraformRuntime.svc
	}

	connectionCredentials, err := connectionCredentialStore(opts)
	if err != nil {
		return nil, err
	}
	credentialResolver := connection.NewCredentialResolver(st, connectionCredentials)
	ec2RoleProvider := opts.EC2RoleProviderOverride
	if ec2RoleProvider == nil && (opts.Adapters == AdapterKubernetes || opts.Adapters == AdapterAWS) {
		ec2RoleProvider = awsaccess.NewEC2RoleProvider()
	}
	credentialResolver.SetEC2RoleProvider(ec2RoleProvider)
	kubeconfigSource := kubeconfigSourceFor(opts.Adapters, credentialResolver, nil)

	secretStore, err := outputSecretStore(opts, terraformRuntime != nil)
	if err != nil {
		return nil, err
	}
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
		// fakeRenderer: the fake adapters render with the native renderer behind the
		// deterministic fake bundle, so planning stays Definition-placed (ADR-016).
		fakeRenderer bool
	)
	switch opts.Adapters {
	case "", AdapterFake:
		fakeExec = fake.NewResourceExecutor()
		fakeDep = fake.NewWorkloadDeployer()
		registry = fake.Registry{Executor: fakeExec}
		renderer = kubernetesRenderer()
		deployer = fakeDep
		fakeRenderer = true
	case AdapterKubernetes, AdapterAWS:
		registry, renderer, deployer, err = realAdapters(opts, credentialResolver, kubeconfigSource, terraformRuntime, secretStore, modules, closers)
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

	var bundles map[string]resource.RenderBundle
	useFakeRenderer := false
	if fakeRenderer && opts.RendererOverride == nil && opts.ScoreK8sPath == "" && !opts.RequireScoreRenderer {
		fake := fakeBundleRenderer{native: renderer}
		bundles, renderer = fake.Bundles(), fake
		opts.RequireScoreRenderer = true
		useFakeRenderer = true
	}
	switch {
	case useFakeRenderer:
	case opts.RequireScoreRenderer && opts.ScoreK8sPath != "":
		scoreRenderer, err := scorek8s.New(ctx, opts.ScoreK8sPath, nil)
		if err != nil {
			return nil, err
		}
		bundles = scoreRenderer.Bundles()
		renderer = scoreRenderer
	case opts.RequireScoreRenderer:
		renderer = unavailableRenderer{}
	case opts.ScoreK8sPath != "":
		scoreRenderer, err := scorek8s.New(ctx, opts.ScoreK8sPath, renderer)
		if err != nil {
			return nil, err
		}
		bundles = scoreRenderer.Bundles()
		renderer = scoreRenderer
	}
	var renderRegistry *renderbundles.Registry
	if score, ok := renderer.(*scorek8s.Renderer); ok {
		if renderRegistry, err = gitRenderRegistry(opts, st, score, closers); err != nil {
			return nil, err
		}
		score.SetGitBundles(renderRegistry, renderengine.Executor{Command: opts.RenderWorkerPath})
	}
	planner := planning.NewService(bundles)
	if opts.RequireScoreRenderer {
		planner = planning.NewRequiredRendererService(bundles)
	}
	prov := provisioning.NewService(st, registry, secretStore, c)
	prov.SetModuleVerifier(modules)
	if opts.ClusterReadinessOverride != nil {
		prov.SetClusterReadiness(opts.ClusterReadinessOverride, clusterReadyTimeout)
	} else if opts.Adapters == AdapterKubernetes || opts.Adapters == AdapterAWS {
		// ADR-017: a cluster (existing or created in this execution) must authenticate, authorise, be reachable
		// from the executor network and have usable Ready node capacity before any consumer runs.
		prov.SetClusterReadiness(&k8s.ClusterReady{KubectlPath: opts.KubectlPath, Credentials: kubeconfigSource}, clusterReadyTimeout)
	}
	inspector := planning.RegistryInspector{Metadata: modules}
	reviewer := deploymentreview.Assembler{Sources: modules, Renderer: renderer}
	deployments := appsvc.NewService(st, planner, prov, renderer, deployer, inspector, c)
	deployments.SetReviewer(reviewer)
	if opts.Adapters == AdapterKubernetes {
		if opts.WorkloadDelivery == "fleet-gitrepo" {
			fleet, ok := deployer.(*gitops.Deployer)
			if !ok {
				return nil, fmt.Errorf("bootstrap: Fleet route delivery requires GitOps deployer")
			}
			deployments.SetPublicRouteManager(fleet, opts.Seed.BaseDomain)
		} else {
			deployments.SetPublicRouteManager(&k8s.PublicRoutes{KubectlPath: opts.KubectlPath, Credentials: kubeconfigSource}, opts.Seed.BaseDomain)
		}
	}
	if opts.WorkloadDelivery == "fleet-gitrepo" {
		deployments.SetImagePullSecret(opts.HarborPullSecretName)
	}
	queries := appsvc.NewQueryService(st)
	var rejectedAccountIDs []string
	if !seed.AllowsFixedTestAccounts(opts.Seed.Profile) {
		if rejectedAccountIDs, err = seed.FixedTestAccountIDsIn(ctx, st); err != nil {
			return nil, fmt.Errorf("bootstrap: find fixed test accounts: %w", err)
		}
	}
	auth := authentication.NewService(st, rejectedAccountIDs...)
	applications := appcreate.NewService(st)
	operations := envops.NewManager(st)
	if opts.DeployDeadline != 0 {
		if err := operations.SetDeadline(environment.OpDeploy, opts.DeployDeadline); err != nil {
			return nil, err
		}
	}
	var storeRegistry configport.Registry
	var storeVerifier configport.Verifier
	switch {
	case opts.StoreRegistryOverride != nil:
		storeRegistry = opts.StoreRegistryOverride
	case opts.PlatformVault != nil || connectionCredentials != nil && opts.Adapters != "" && opts.Adapters != AdapterFake:
		storeRegistry = vault.NewRegistry(st, connectionCredentials)
	case opts.Adapters == "" || opts.Adapters == AdapterFake:
		storeRegistry = configmemory.NewRegistry(st)
	default:
		storeRegistry = configmemory.Unavailable{}
	}
	switch {
	case opts.SecretStoreVerifierOverride != nil:
		storeVerifier = opts.SecretStoreVerifierOverride
	case opts.PlatformVault != nil:
		// An explicit bootstrap must reach the real Vault in every adapter mode.
		storeVerifier = vault.Verifier{}
	case opts.Adapters == "" || opts.Adapters == AdapterFake:
		storeVerifier = configmemory.Verifier{}
	default:
		storeVerifier = vault.Verifier{}
	}
	configurations := appconfig.NewService(st, storeRegistry, operations)
	secretStoreService := secretstores.NewService(st, storeVerifier, connectionCredentials)
	if opts.PlatformVault != nil {
		if err := seedPlatformVault(ctx, st, secretStoreService, opts); err != nil {
			return nil, err
		}
	}
	deployments.SetStoreRegistry(storeRegistry)
	deployments.SetOperations(operations)
	vaultDelivery := opts.VaultDelivery
	if vaultDelivery == "auto" && opts.Adapters == AdapterKubernetes {
		vaultDelivery = "vso"
	} else if vaultDelivery == "auto" {
		vaultDelivery = "agent"
	}
	if vaultDelivery == "vso" {
		if opts.Adapters != AdapterKubernetes {
			return nil, fmt.Errorf("bootstrap: VSO delivery requires Kubernetes adapters")
		}
		deployments.SetConfigSecretSynchronizer(&k8s.VSOSynchronizer{KubectlPath: opts.KubectlPath, Credentials: credentialResolver})
	} else if vaultDelivery != "" && vaultDelivery != "agent" {
		return nil, fmt.Errorf("bootstrap: unknown Vault delivery %q", vaultDelivery)
	}
	workloads := workloadconfig.NewService(st)
	pendingChanges := pending.NewService(st, planner, workloads, inspector)
	pendingChanges.SetReviewer(reviewer)
	if opts.WorkloadDelivery == "fleet-gitrepo" {
		pendingChanges.SetImageRegistryHost(opts.HarborRegistryHost)
	}
	pendingChanges.SetDeployer(deployments)
	pendingChanges.SetOperations(operations)
	previews := preview.NewService(st, planner, inspector)
	previews.SetReviewer(reviewer)
	kubeconfigVerifier := opts.KubeconfigVerifierOverride
	if kubeconfigVerifier == nil {
		kubeconfigVerifier = k8s.ConnectionVerifier{KubectlPath: opts.KubectlPath}
	}

	awsVerifier := opts.AWSVerifierOverride
	if awsVerifier == nil && (opts.Adapters == AdapterKubernetes || opts.Adapters == AdapterAWS) {
		awsVerifier = awsaccess.Verifier{Client: awsaccess.New()}
	}

	var fakeCluster *fake.Cluster
	switch opts.Adapters {
	case "", AdapterFake:
		fakeCluster = fake.NewCluster()
		fakeDep.AttachCluster(fakeCluster)
		fakeExec.AttachCluster(fakeCluster)
		deployments.SetPublicRouteManager(fakeCluster, opts.Seed.BaseDomain)
	}

	resourceCleanup := resourcecleanup.New(st, operations)
	var safetyNet *runcleanup.Service
	if terraformRuntime != nil && terraformRuntime.executor != nil {
		if claims, ok := st.(terraformstate.CleanupRepository); ok {
			safetyNet = runcleanup.New(st, terraformRuntime.repo, terraformRuntime.executor, claims, operations)
		}
	}
	if terraformRuntime != nil {
		resourceCleanup.SetState(terraformRuntime.repo)
		if terraformRuntime.executor != nil {
			resourceCleanup.SetEngine(terraformRuntime.executor)
			deployments.SetInstanceMarker(terraformRuntime.executor)
		}
	}
	if opts.CleanupEngineOverride != nil {
		resourceCleanup.SetEngine(opts.CleanupEngineOverride)
	}

	var renderPort renderregistry.Registry
	if renderRegistry != nil {
		renderPort = renderRegistry
	}
	diagnostics, err := buildDiagnostics(opts, st, connectionCredentials, ec2RoleProvider, vaultDelivery, pendingChanges, closers)
	if err != nil {
		return nil, err
	}
	server := deliveryhttp.NewServer(deliveryhttp.Config{
		Diagnostics:           diagnostics,
		RenderBundles:         bundles,
		RenderRegistry:        renderPort,
		Deployments:           deployments,
		Queries:               queries,
		Authentication:        auth,
		Applications:          applications,
		KubeconfigVerifier:    kubeconfigVerifier,
		AWSVerifier:           awsVerifier,
		EC2RoleProvider:       ec2RoleProvider,
		ConnectionCredentials: connectionCredentials,
		Configurations:        configurations,
		SecretStores:          secretStoreService,
		Operations:            operations,
		Workloads:             workloads,
		Pending:               pendingChanges,
		Previews:              previews,
		ResourceCleanup:       resourceCleanup,
		Store:                 st,
		ModuleRegistry:        modules,
		SeedOptions:           opts.Seed,
		UIDir:                 opts.UIDir,
	})

	return &App{
		Diagnostics:           diagnostics,
		RenderRegistry:        renderRegistry,
		ConnectionCredentials: connectionCredentials,
		Store:                 st,
		Secrets:               secretStore,
		Server:                server,
		Deployments:           deployments,
		Queries:               queries,
		Previews:              previews,
		ResourceCleanup:       resourceCleanup,
		FakeExec:              fakeExec,
		FakeDeploy:            fakeDep,
		Operations:            operations,
		Configurations:        configurations,
		Pending:               pendingChanges,
		Workloads:             workloads,
		StoreRegistry:         storeRegistry,
		FakeCluster:           fakeCluster,
		Modules:               modules,
		RunCleanup:            safetyNet,
		WorkloadDeployer:      deployer,
		TerraformStates:       terraformStates,
		TerraformStateHandler: stateHandler(terraformStates),
	}, nil
}

// gitRenderRegistry opens the verified Git render bundle registry (ADR-021):
// the durable content cache and the hardened Git transport. Inspection needs
// a store with the render bundle repository; rendering needs only the cache.
func gitRenderRegistry(opts Options, st persistence.Store, score *scorek8s.Renderer, closers *[]func()) (*renderbundles.Registry, error) {
	dir := opts.RenderBundleCacheDir
	if dir == "" {
		temp, err := os.MkdirTemp("", "orchestrator-render-bundles-")
		if err != nil {
			return nil, fmt.Errorf("bootstrap: create the private render bundle cache: %w", err)
		}
		*closers = append(*closers, func() { _ = os.RemoveAll(temp) })
		dir = temp
	}
	cache, err := renderbundles.OpenCache(dir)
	if err != nil {
		return nil, err
	}
	git, err := moduleregistry.NewGitCLI(opts.GitPath, cache.TempDir())
	if err != nil {
		return nil, err
	}
	registry := &renderbundles.Registry{Cache: cache, Git: git, Runtime: score}
	if repo, ok := st.(persistence.RenderBundleRepository); ok {
		registry.Store = repo
	}
	return registry, nil
}

// moduleRegistry opens the verified Terraform module registry and prepares
// the packaged release modules. In AWS mode the Terraform binary is resolved
// once; the executor runs exactly the path whose digest execution
// verification checks.
func moduleRegistry(ctx context.Context, opts *Options, st persistence.Store, closers *[]func()) (*moduleregistry.Service, error) {
	cacheDir := opts.TerraformModuleCacheDir
	if cacheDir == "" {
		if opts.TerraformModulePolicyFile != "" {
			return nil, fmt.Errorf("bootstrap: allowlisted Git module sources need a durable -terraform-module-cache-dir")
		}
		dir, err := os.MkdirTemp("", "orchestrator-modules-")
		if err != nil {
			return nil, fmt.Errorf("bootstrap: create the private module cache: %w", err)
		}
		*closers = append(*closers, func() { _ = os.RemoveAll(dir) })
		cacheDir = dir
	}
	binary := ""
	// Both real adapter modes resolve the pinned Terraform binary: AWS and
	// the in-cluster (Kubernetes) mode run registered modules. Without the
	// v2 state backend the Kubernetes mode runs no Terraform, so a missing
	// binary only fails startup when Terraform can actually execute.
	if opts.Adapters == AdapterAWS || opts.Adapters == AdapterKubernetes {
		path := opts.TerraformPath
		if path == "" {
			path = "terraform"
		}
		terraformRequired := opts.Adapters == AdapterAWS || opts.TerraformStateKeyFile != ""
		resolved, err := exec.LookPath(path)
		switch {
		case err != nil && terraformRequired:
			return nil, fmt.Errorf("bootstrap: the Terraform binary was not found")
		case err == nil:
			if resolved, err = filepath.Abs(resolved); err != nil {
				return nil, fmt.Errorf("bootstrap: resolve the Terraform binary: %w", err)
			}
			opts.TerraformPath, binary = resolved, resolved
		}
	}
	return moduleregistry.Open(ctx, moduleregistry.Options{
		PolicyFile: opts.TerraformModulePolicyFile, CacheDir: cacheDir, GitBinary: opts.GitPath,
		TerraformBinary: binary, RetainedReleasesFile: opts.TerraformRetainedReleasesFile, Store: st,
	})
}

// terraformStateBackend builds the durable Terraform state backend on its
// own State runtime connection. A key without the State connection (or the
// reverse), a non-PostgreSQL store, a State connection to another database,
// schema or installation, or an unreadable/invalid key fails startup.
func terraformStateBackend(ctx context.Context, opts Options, st persistence.Store, closers *[]func()) (*terraformRuntime, error) {
	if opts.TerraformStateKeyFile == "" && opts.TerraformStateDatabaseURL == "" {
		return nil, nil
	}
	console, ok := st.(*pgstore.Store)
	if !ok || opts.DatabaseURL == "" {
		return nil, fmt.Errorf("bootstrap: the Terraform state backend requires PostgreSQL")
	}
	switch {
	case opts.TerraformStateKeyFile == "":
		return nil, fmt.Errorf("bootstrap: the Terraform state backend needs a state key file")
	case opts.TerraformStateDatabaseURL == "":
		return nil, fmt.Errorf("bootstrap: the Terraform state backend needs its own State runtime database connection; it never uses the Console or owner login")
	case opts.TerraformStateDatabaseURL == opts.DatabaseURL:
		return nil, fmt.Errorf("bootstrap: the State runtime connection must use a login separate from the Console connection")
	case opts.TerraformStateKeyID == "":
		return nil, fmt.Errorf("bootstrap: the Terraform state backend needs a state key id")
	}
	cipher, err := statecipher.LoadKeyFile(opts.TerraformStateKeyID, opts.TerraformStateKeyFile)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: %w", err)
	}
	repo, err := pgstore.OpenState(ctx, opts.TerraformStateDatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: State runtime connection: %w", err)
	}
	*closers = append(*closers, repo.Close)
	if repo.Identity() != console.Identity() {
		return nil, fmt.Errorf("bootstrap: the State and Console connections serve different databases, schemas or installations")
	}
	service, err := statebackend.New(repo, cipher)
	if err != nil {
		return nil, err
	}
	return &terraformRuntime{svc: service, repo: repo, cipher: cipher}, nil
}

func stateHandler(states *statebackend.Service) *deliveryhttp.TerraformStateHandler {
	if states == nil {
		return nil
	}
	return deliveryhttp.NewTerraformStateHandler(states)
}

// connectionCredentialStore builds the UC-04 credential store. The memory store
// is never selected silently: it must be requested and is refused for real
// adapters or persistent state, where a READY Connection would outlive its
// credential. With no store, upload registration fails closed.
func connectionCredentialStore(opts Options) (credentials.Store, error) {
	if opts.ConnectionCredentialsOverride != nil {
		return opts.ConnectionCredentialsOverride, nil
	}
	switch opts.ConnectionCredentialStore {
	case "", "none":
		return nil, nil
	case "memory":
		if (opts.Adapters != "" && opts.Adapters != AdapterFake) || opts.DatabaseURL != "" || opts.StatePath != "" {
			return nil, fmt.Errorf("bootstrap: the memory connection credential store is only for fake adapters without persistent state")
		}
		return credentialmemory.New(), nil
	case "vault":
		if opts.ConnectionVaultAddress == "" || opts.ConnectionVaultTokenFile == "" {
			return nil, fmt.Errorf("bootstrap: the Vault connection credential store needs an address and a token file")
		}
		token, err := os.ReadFile(opts.ConnectionVaultTokenFile)
		if err != nil {
			return nil, fmt.Errorf("bootstrap: read connection credential token file")
		}
		mount := opts.ConnectionVaultMount
		if mount == "" {
			mount = "kv"
		}
		return vault.NewConnectionCredentials(opts.ConnectionVaultAddress, strings.TrimSpace(string(token)), mount, nil)
	}
	return nil, fmt.Errorf("bootstrap: unknown connection credential store %q", opts.ConnectionCredentialStore)
}

// unavailableRenderer is the required-renderer mode without a score-k8s
// binary: planning already fails for every Workload; this refuses any render
// that would otherwise bypass it.
type unavailableRenderer struct{}

func (unavailableRenderer) Render(context.Context, execution.RenderRequest) ([]execution.Manifest, error) {
	return nil, errors.New("bootstrap: the score-k8s renderer is not installed")
}

// outputSecretStore builds the UC-08 output-secret store. The in-process
// memory store loses every reference on restart, so the real acceptance
// profile refuses it.
func outputSecretStore(opts Options, durableState bool) (execution.SecretStore, error) {
	switch opts.OutputSecretStore {
	case "", "memory":
		if opts.Seed.Profile == seed.ProfileAcceptanceV2 {
			return nil, fmt.Errorf("bootstrap: the real acceptance profile needs the durable Vault output-secret store")
		}
		// Real adapters with the durable SQL Terraform state backend persist outputs and password references: the in-process store would lose
		// them on a restart while the state survives. Only an explicitly labelled fixture may keep it.
		if durableState && (opts.Adapters == AdapterKubernetes || opts.Adapters == AdapterAWS) && !opts.EphemeralOutputSecrets {
			return nil, fmt.Errorf("bootstrap: real adapters with the durable Terraform state backend need the durable Vault output-secret store")
		}
		return secrets.NewMemory(), nil
	case "vault":
		if opts.OutputSecretVaultAddress == "" || opts.OutputSecretVaultTokenFile == "" {
			return nil, fmt.Errorf("bootstrap: the Vault output-secret store needs an address and a token file")
		}
		token, err := ReadCredentialFile("output secret store token", opts.OutputSecretVaultTokenFile)
		if err != nil {
			return nil, err
		}
		mount := opts.OutputSecretVaultMount
		if mount == "" {
			mount = "kv"
		}
		return vault.NewOutputSecrets(opts.OutputSecretVaultAddress, token, mount, nil)
	}
	return nil, fmt.Errorf("bootstrap: unknown output secret store %q", opts.OutputSecretStore)
}

// FakeRenderBundleID is the render bundle the fake adapters install. It carries the
// ID of the real score-k8s bundle so Definitions authored for either run unchanged.
const FakeRenderBundleID = "score-k8s-internal-v1"

// clusterReadyTimeout bounds the usability proof of one completed cluster (ADR-017).
const clusterReadyTimeout = 10 * time.Minute

// fakeBundleRenderer is the fake adapters' renderer: the native renderer behind a
// deterministic, clearly fake bundle. It exists only for AdapterFake, which touches
// no cluster and no cloud account.
type fakeBundleRenderer struct{ native execution.WorkloadRenderer }

// NewFakeBundleRenderer is the fake adapters' renderer for tests that wire the Deployment service by hand.
func NewFakeBundleRenderer(native execution.WorkloadRenderer) execution.WorkloadRenderer {
	return fakeBundleRenderer{native: native}
}

func (fakeBundleRenderer) Bundles() map[string]resource.RenderBundle { return FakeRenderBundles() }

// FakeRenderBundles is the bundle snapshot the fake adapters install; tests that build
// their own planner use it so plans pin the same bundle as the application's.
func FakeRenderBundles() map[string]resource.RenderBundle {
	return map[string]resource.RenderBundle{FakeRenderBundleID: {ID: FakeRenderBundleID, Version: "fake", BinaryDigest: "fake-binary", Digest: "fake-bundle"}}
}

func (r fakeBundleRenderer) Render(ctx context.Context, req execution.RenderRequest) ([]execution.Manifest, error) {
	return r.native.Render(ctx, req)
}

func (fakeBundleRenderer) ValidateSelection(_ context.Context, selected resource.RenderingSelection) error {
	if selected.Bundle.ID != FakeRenderBundleID || selected.DriverType != resource.DriverScoreK8s {
		return errors.New("bootstrap: the fake renderer only serves its own bundle")
	}
	return nil
}

// buildDiagnostics wires the account diagnostics service. The probes use only
// the account's saved scoped credential; a store without the diagnostic
// repository leaves the feature unavailable (the routes answer 503).
func buildDiagnostics(opts Options, st persistence.Store, creds credentials.Store, ec2 connection.EC2RoleProvider, secretDelivery string, planner accountdiag.Planner, closers *[]func()) (*accountdiag.Service, error) {
	repo, ok := st.(diagprobes.Repository)
	if !ok {
		return nil, nil
	}
	providers := opts.DiagnosticProvidersOverride
	if providers == nil {
		providers = map[string]diagprobes.Provider{
			"INTERNAL": k8s.Diagnostics{KubectlPath: opts.KubectlPath, Credentials: creds, ProbeImage: opts.DiagnosticProbeImage},
			"AWS":      awsaccess.Diagnostics{Client: awsaccess.New(), Credentials: creds, EC2: ec2},
		}
	}
	cfg := accountdiag.Config{}
	if opts.DiagnosticConfigOverride != nil {
		cfg = *opts.DiagnosticConfigOverride
	}
	cfg.SecretDelivery, cfg.BaseDomain = secretDelivery, opts.Seed.BaseDomain
	svc := accountdiag.New(repo, connection.NewService(st), st, providers, cfg)
	svc.SetPlanner(planner)
	svc.SetInspectors(vault.StoreInspector{Stores: st, Credentials: creds}, tlsprobe.Routes{})
	sweepCtx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.Run(sweepCtx)
	}()
	*closers = append(*closers, func() {
		stop()
		<-done
		svc.Close()
	})
	return svc, nil
}
