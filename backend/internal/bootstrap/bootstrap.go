// Package bootstrap wires the orchestrator process: store, adapters, application
// services and the HTTP delivery layer. Tests reuse it so production wiring and
// test wiring cannot drift apart.
package bootstrap

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"orchestrator/internal/adapters/configmemory"
	"orchestrator/internal/adapters/credentialmemory"
	"orchestrator/internal/adapters/fake"
	"orchestrator/internal/adapters/gitops"
	k8s "orchestrator/internal/adapters/kubernetes"
	pgstore "orchestrator/internal/adapters/postgres"
	"orchestrator/internal/adapters/scorek8s"
	"orchestrator/internal/adapters/secrets"
	"orchestrator/internal/adapters/statecipher"
	"orchestrator/internal/adapters/store"
	tf "orchestrator/internal/adapters/terraform"
	"orchestrator/internal/adapters/vault"
	appcreate "orchestrator/internal/application/application"
	"orchestrator/internal/application/authentication"
	appconfig "orchestrator/internal/application/configuration"
	"orchestrator/internal/application/connection"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/envops"
	"orchestrator/internal/application/pending"
	"orchestrator/internal/application/preview"
	"orchestrator/internal/application/provisioning"
	"orchestrator/internal/application/secretstores"
	"orchestrator/internal/application/statebackend"
	"orchestrator/internal/application/transition"
	workloadconfig "orchestrator/internal/application/workloadconfig"
	deliveryhttp "orchestrator/internal/delivery/http"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/platform/clock"
	configport "orchestrator/internal/ports/configuration"
	"orchestrator/internal/ports/credentials"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
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
	TerraformRoot string

	// TerraformPluginCache keeps provider downloads shared between workspaces.
	TerraformPluginCache string
	// Region and Tags apply to every cloud resource of one verification run.
	Region                 string
	Tags                   map[string]string
	WorkDir                string
	Clock                  clock.Clock
	VaultDelivery          string
	WorkloadDelivery       string
	GitOpsRepoDir          string
	GitOpsBranch           string
	FleetGitRepoName       string
	HarborRegistryHost     string
	HarborDockerConfigFile string
	HarborPullSecretName   string

	// ConnectionCredentialStore selects the UC-04 Connection credential
	// store: "" (none; upload registration fails closed with 503), "vault"
	// (durable KV v2, scoped token read from ConnectionVaultTokenFile) or
	// "memory" (explicit non-durable local/test store, only with fake
	// adapters and no persistent state).
	ConnectionCredentialStore string
	ConnectionVaultAddress    string
	ConnectionVaultTokenFile  string
	ConnectionVaultMount      string

	// TerraformStateKeyFile enables the Terraform v2 durable state backend: an
	// owner-only file holding the 32-byte AES-256-GCM state key (hex or
	// base64), kept outside the database. It requires PostgreSQL; there is no
	// plaintext, keyless, JSON or in-memory state backend.
	TerraformStateKeyFile string
	// TerraformStateKeyID names that key in every encrypted state row.
	TerraformStateKeyID string
	// TerraformStateDatabaseURL is the connection URL of the State runtime
	// login (option A): a login separate from the Console login of
	// DatabaseURL, to the same database and schema. The state backend needs
	// both it and the key; neither falls back to the Console or owner login.
	TerraformStateDatabaseURL string

	// PlatformVault, when set, is the explicit opt-in Compose bootstrap of the
	// bundled Vault as an ordinary verified store (UC-04 SS-07/08). It is
	// the only runtime path that seeds a store from process configuration.
	PlatformVault *PlatformVaultBootstrap

	// Overrides replace individual adapters. Tests use them to inject failures.
	RegistryOverride              execution.ExecutorRegistry
	KubeconfigVerifierOverride    connection.KubeconfigVerifier
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
	// ConnectionCredentials is nil when no credential store is configured.
	ConnectionCredentials credentials.Store
	Store                 persistence.Store
	Secrets               *secrets.Memory
	Server                *deliveryhttp.Server
	Deployments           *appsvc.Service
	Queries               *appsvc.QueryService
	Previews              *preview.Service
	FakeExec              *fake.ResourceExecutor
	FakeDeploy            *fake.WorkloadDeployer
	// Configurations and Pending are the UC-12 and UC-05/06 services.
	Configurations *appconfig.Service
	Pending        *pending.Service
	// Workloads is the UC-16 draft editor service.
	Workloads *workloadconfig.Service
	// Operations is the shared Environment claim manager.
	Operations *envops.Manager
	// StoreRegistry resolves workload secret stores.
	StoreRegistry configport.Registry
	// Transitions runs Environment target transitions.
	Transitions *transition.Service
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

	if err := seed.Apply(ctx, st, opts.Seed); err != nil {
		return nil, err
	}

	terraformStates, err := terraformStateBackend(ctx, opts, st, closers)
	if err != nil {
		return nil, err
	}

	connectionCredentials, err := connectionCredentialStore(opts)
	if err != nil {
		return nil, err
	}
	credentialResolver := connection.NewCredentialResolver(st, connectionCredentials)

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
		registry, renderer, deployer, err = realAdapters(opts, credentialResolver)
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
	if opts.ScoreK8sPath != "" {
		scoreRenderer, err := scorek8s.New(ctx, opts.ScoreK8sPath, renderer)
		if err != nil {
			return nil, err
		}
		bundles = scoreRenderer.Bundles()
		renderer = scoreRenderer
	}
	planner := planning.NewService(bundles)
	prov := provisioning.NewService(st, registry, secretStore, c)
	deployments := appsvc.NewService(st, planner, prov, renderer, deployer, tf.NewInspector(), c)
	if opts.Adapters == AdapterKubernetes {
		if opts.WorkloadDelivery == "fleet-gitrepo" {
			fleet, ok := deployer.(*gitops.Deployer)
			if !ok {
				return nil, fmt.Errorf("bootstrap: Fleet route delivery requires GitOps deployer")
			}
			deployments.SetPublicRouteManager(fleet, opts.Seed.BaseDomain)
		} else {
			deployments.SetPublicRouteManager(&k8s.PublicRoutes{KubectlPath: opts.KubectlPath, Credentials: credentialResolver}, opts.Seed.BaseDomain)
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
	pendingChanges := pending.NewService(st, planner, workloads, tf.NewInspector())
	if opts.WorkloadDelivery == "fleet-gitrepo" {
		pendingChanges.SetImageRegistryHost(opts.HarborRegistryHost)
	}
	pendingChanges.SetDeployer(deployments)
	pendingChanges.SetOperations(operations)
	previews := preview.NewService(st, planner, tf.NewInspector())
	kubeconfigVerifier := opts.KubeconfigVerifierOverride
	if kubeconfigVerifier == nil {
		kubeconfigVerifier = k8s.ConnectionVerifier{KubectlPath: opts.KubectlPath}
	}

	transitions := transition.New(st, planner, tf.NewInspector(), deployments, operations)
	transitions.SetStoreRegistry(storeRegistry)
	transitions.SetWorkloads(workloads)
	transitions.SetDirectDelivery(opts.WorkloadDelivery != "fleet-gitrepo")
	var fakeCluster *fake.Cluster
	switch opts.Adapters {
	case "", AdapterFake:
		fakeCluster = fake.NewCluster()
		fakeDep.AttachCluster(fakeCluster)
		fakeExec.AttachCluster(fakeCluster)
		deployments.SetPublicRouteManager(fakeCluster, opts.Seed.BaseDomain)
		transitions.SetCluster(fakeCluster, fakeCluster, fakeCluster, fakeCluster)
		transitions.SetProbe(fakeCluster)
	case AdapterKubernetes:
		kubeTransition := &k8s.Transition{KubectlPath: opts.KubectlPath, Credentials: credentialResolver}
		transitions.SetCluster(kubeTransition, kubeTransition, kubeTransition, kubeTransition)
		transitions.SetProbe(kubeTransition)
	}

	server := deliveryhttp.NewServer(deliveryhttp.Config{
		Transitions:           transitions,
		RenderBundles:         bundles,
		Deployments:           deployments,
		Queries:               queries,
		Authentication:        auth,
		Applications:          applications,
		KubeconfigVerifier:    kubeconfigVerifier,
		ConnectionCredentials: connectionCredentials,
		Configurations:        configurations,
		SecretStores:          secretStoreService,
		Operations:            operations,
		Workloads:             workloads,
		Pending:               pendingChanges,
		Previews:              previews,
		Store:                 st,
		SeedOptions:           opts.Seed,
		UIDir:                 opts.UIDir,
	})

	return &App{
		ConnectionCredentials: connectionCredentials,
		Store:                 st,
		Secrets:               secretStore,
		Server:                server,
		Deployments:           deployments,
		Queries:               queries,
		Previews:              previews,
		FakeExec:              fakeExec,
		FakeDeploy:            fakeDep,
		Operations:            operations,
		Configurations:        configurations,
		Pending:               pendingChanges,
		Workloads:             workloads,
		StoreRegistry:         storeRegistry,
		Transitions:           transitions,
		FakeCluster:           fakeCluster,
		TerraformStates:       terraformStates,
		TerraformStateHandler: stateHandler(terraformStates),
	}, nil
}

// terraformStateBackend builds the durable Terraform state backend on its
// own State runtime connection. A key without the State connection (or the
// reverse), a non-PostgreSQL store, a State connection to another database,
// schema or installation, or an unreadable/invalid key fails startup.
func terraformStateBackend(ctx context.Context, opts Options, st persistence.Store, closers *[]func()) (*statebackend.Service, error) {
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
	return statebackend.New(repo, cipher)
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
