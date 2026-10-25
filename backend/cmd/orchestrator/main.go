// Command orchestrator runs the orchestrator API and serves the Web Console bundle.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/renderengine"
	"orchestrator/internal/seed"
)

func main() {
	// A template worker child of this binary runs author templates (ADR-021).
	renderengine.RunWorkerIfRequested()
	opts := seed.Defaults()
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	addrFile := flag.String("addr-file", "", "optional file that receives the resolved listen address")
	uiDir := flag.String("ui-dir", "../frontend/dist", "directory holding the Web Console production bundle")
	statePath := flag.String("state", "", "optional path of the JSON state snapshot")
	databaseURLFile := flag.String("database-url-file", os.Getenv("ORCHESTRATOR_DATABASE_URL_FILE"), "path to a file containing the PostgreSQL connection URL")
	adapters := flag.String("adapters", "fake", "executor adapters: fake, kubernetes or aws")
	profile := flag.String("profile", opts.Profile, "seed profile: local, test, production or acceptance-v2 (isolated real acceptance: fixture accounts, the exact -namespace identity and no catalog)")
	seedCatalog := flag.String("seed-catalog", "", "seed catalog: empty (legacy), terraform-k8s (verified Terraform namespace/PostgreSQL/Redis) or none (acceptance-v2 only; the catalog is authored through the Console)")
	namespace := flag.String("namespace", opts.NamespaceIdentity, "seeded Environment namespace identity for local/test fixtures; product targets come from Resource Definitions and resource references")
	applicationKey := flag.String("application-key", opts.ApplicationKey, "key of the seeded fixture Application (acceptance-v2 uses a unique key per run so Vault paths never collide with other runs)")
	kubeContext := flag.String("kube-context", opts.KubeContext, "kubectl context of the registered cluster")
	cluster := flag.String("cluster", opts.ClusterName, "registered cluster name")
	region := flag.String("region", "", "seeded AWS Driver Account region hint for local/test fixtures; product targets use each AWS Definition's literal region")
	accountID := flag.String("account-id", "", "seeded AWS Driver Account identity for local/test fixtures; product targets use the selected Definition account")
	runID := flag.String("run-id", "", "verification run id, used in cloud resource names and tags")
	owner := flag.String("owner", "", "owner tag for cloud resources")
	ttl := flag.Duration("ttl", 2*time.Hour, "expires-at tag horizon for cloud resources")
	frontendImage := flag.String("frontend-image", opts.Images.Frontend, "acceptance frontend image")
	backendImage := flag.String("backend-image", opts.Images.Backend, "acceptance backend image")
	workerImage := flag.String("worker-image", opts.Images.Worker, "acceptance worker image")
	terraformRoot := flag.String("terraform-root", "", "optional parent directory of the private per-process Terraform work directory (provider credentials, readiness kubeconfig); default is the system temporary directory")
	terraformRecoveryDir := flag.String("terraform-recovery-dir", os.Getenv("ORCHESTRATOR_TERRAFORM_RECOVERY_DIR"), "private absolute directory (0700, outside any Git work tree) for encrypted recovery artifacts of a Terraform state the backend could not store; required with real adapters and the Terraform state backend")
	terraformCache := flag.String("terraform-plugin-cache", os.Getenv("TF_PLUGIN_CACHE_DIR"), "Terraform provider cache directory")
	kubectlPath := flag.String("kubectl", "kubectl", "kubectl binary")
	scorePath := flag.String("score-k8s", "", "path to the pinned score-k8s 0.15.0 renderer; required for Workload deployment (without it every Workload plan fails before resources, there is no native fallback)")
	terraformPath := flag.String("terraform", "terraform", "terraform binary")
	moduleCacheDir := flag.String("terraform-module-cache-dir", os.Getenv("ORCHESTRATOR_TERRAFORM_MODULE_CACHE_DIR"), "durable private absolute directory (0700, outside any Git work tree) caching verified Terraform modules; configure for Git module reuse across restarts")
	modulePolicyFile := flag.String("terraform-module-policy-file", os.Getenv("ORCHESTRATOR_TERRAFORM_MODULE_POLICY_FILE"), "legacy HTTPS Git module policy and optional repository CA configuration (JSON); direct Git inspection does not require an allowlist")
	retainedReleases := flag.String("terraform-retained-releases-file", os.Getenv("ORCHESTRATOR_TERRAFORM_RETAINED_RELEASES_FILE"), "JSON list of retained trusted historical platform releases ({release, terraformBinary}) so cleanup can run resources created before a release upgrade; empty retains none")
	gitPath := flag.String("git", "git", "git binary used to resolve and fetch Terraform module and render bundle commits")
	renderBundleCacheDir := flag.String("render-bundle-cache-dir", os.Getenv("ORCHESTRATOR_RENDER_BUNDLE_CACHE_DIR"), "durable private absolute directory (0700, outside any Git work tree) caching verified Git render bundles; empty uses a temporary cache lost on exit")
	vaultDelivery := flag.String("vault-delivery", "auto", "UC-12 Kubernetes delivery: auto, vso or agent")
	workloadDelivery := flag.String("workload-delivery", "direct", "workload delivery: direct or fleet-gitrepo (kind only)")
	gitopsRepoDir := flag.String("gitops-repo-dir", "", "writable local clone of the Fleet GitOps repository")
	gitopsBranch := flag.String("gitops-branch", "main", "Fleet GitOps branch")
	fleetGitRepoName := flag.String("fleet-gitrepo-name", "poc-workloads", "Fleet GitRepo resource name in fleet-local")
	diagnosticProbeImage := flag.String("diagnostic-probe-image", "", "platform-owned image of Kubernetes execution diagnostics (default: the pause image a node already holds)")
	harborRegistryHost := flag.String("harbor-registry", "", "Harbor registry host:port used in workload image references")
	harborDockerConfigFile := flag.String("harbor-dockerconfig-file", "", "owner-only Docker config JSON for namespace imagePullSecret")
	harborPullSecretName := flag.String("harbor-pull-secret", "harbor-pull", "namespace-local imagePullSecret name")
	publicBaseDomain := flag.String("public-base-domain", envOr("ORCHESTRATOR_PUBLIC_BASE_DOMAIN", opts.BaseDomain), "base domain of Environment public hosts (<subdomain>.<base>, staging: staging.<subdomain>.<base>)")
	publicRouteKind := flag.String("public-route-kind", envOr("ORCHESTRATOR_PUBLIC_ROUTE_KIND", "ingress"), "public route object: ingress or httproute (Gateway API, ADR-024)")
	publicGateway := flag.String("public-gateway", os.Getenv("ORCHESTRATOR_PUBLIC_GATEWAY"), "parent Gateway of httproute public routes: namespace/name[/sectionName]")
	publicIngressClass := flag.String("public-ingress-class", envOr("ORCHESTRATOR_PUBLIC_INGRESS_CLASS", "traefik"), "ingressClassName of ingress public routes")
	connectionCredentialStore := flag.String("connection-credential-store", os.Getenv("ORCHESTRATOR_CONNECTION_CREDENTIAL_STORE"), "UC-04 Connection credential store: none (upload registration unavailable), vault, or memory (fake adapters without persistent state only)")
	connectionVaultAddress := flag.String("connection-vault-address", os.Getenv("ORCHESTRATOR_CONNECTION_VAULT_ADDR"), "Vault API address of the Connection credential store")
	connectionVaultTokenFile := flag.String("connection-vault-token-file", os.Getenv("ORCHESTRATOR_CONNECTION_VAULT_TOKEN_FILE"), "path to the scoped Vault token file of the Connection credential store")
	connectionVaultMount := flag.String("connection-vault-mount", "kv", "KV v2 mount of the Connection credential store")
	outputSecretStore := flag.String("output-secret-store", os.Getenv("ORCHESTRATOR_OUTPUT_SECRET_STORE"), "UC-08 output-secret store: memory (default; lost on restart) or vault (durable KV v2 under orchestrator/outputs; required by the acceptance-v2 profile)")
	outputSecretAddress := flag.String("output-secret-vault-address", os.Getenv("ORCHESTRATOR_OUTPUT_SECRET_VAULT_ADDR"), "Vault API address of the output-secret store")
	outputSecretTokenFile := flag.String("output-secret-vault-token-file", os.Getenv("ORCHESTRATOR_OUTPUT_SECRET_VAULT_TOKEN_FILE"), "path to the output-secret store token file (a token scoped to orchestrator/outputs only)")
	outputSecretMount := flag.String("output-secret-vault-mount", "kv", "KV v2 mount of the output-secret store")
	deployDeadline := flag.Duration("deploy-deadline", 0, "finite hold time of one whole Deploy claim (default 45m, at most 4h); an AWS acceptance run sets it to cover the sequential VPC -> EKS -> database chain inside its recorded overall deadline")
	terraformInvocationTimeout := flag.Duration("terraform-invocation-timeout", 0, "bound of one Terraform session (default 45m); acceptance runs set a finite smaller value")
	terraformReadinessTimeout := flag.Duration("terraform-readiness-timeout", 0, "bound of the readiness query after an apply (default 5m)")
	platformVaultBootstrap := flag.Bool("platform-vault-bootstrap", envBool("ORCHESTRATOR_PLATFORM_VAULT_BOOTSTRAP"), "explicit opt-in: register the bundled Compose Vault as the ordinary verified store platform-vault at startup")
	platformVaultAddress := flag.String("platform-vault-address", os.Getenv("ORCHESTRATOR_PLATFORM_VAULT_ADDR"), "Vault API address of the platform-vault bootstrap")
	platformVaultWorkloadAddress := flag.String("platform-vault-workload-address", os.Getenv("ORCHESTRATOR_PLATFORM_VAULT_WORKLOAD_ADDR"), "Vault address used by workload Pods (default: the API address)")
	platformVaultTokenFile := flag.String("platform-vault-token-file", os.Getenv("ORCHESTRATOR_PLATFORM_VAULT_TOKEN_FILE"), "bootstrap-only file holding the application-scoped Vault token")
	platformVaultMount := flag.String("platform-vault-mount", os.Getenv("ORCHESTRATOR_PLATFORM_VAULT_MOUNT"), "KV v2 mount of the platform-vault bootstrap (default kv)")
	platformVaultAuthMount := flag.String("platform-vault-auth-mount", os.Getenv("ORCHESTRATOR_PLATFORM_VAULT_AUTH_MOUNT"), "Kubernetes auth mount of the platform-vault bootstrap (default kubernetes)")
	terraformStateKeyFile := flag.String("terraform-state-key-file", os.Getenv("ORCHESTRATOR_TERRAFORM_STATE_KEY_FILE"), "owner-only file holding the 32-byte AES-256-GCM Terraform state key (hex or base64); enables the durable state backend (PostgreSQL only)")
	terraformStateKeyID := flag.String("terraform-state-key-id", os.Getenv("ORCHESTRATOR_TERRAFORM_STATE_KEY_ID"), "identifier of the Terraform state key recorded with every encrypted state")
	terraformStateDatabaseURLFile := flag.String("terraform-state-database-url-file", os.Getenv("ORCHESTRATOR_TERRAFORM_STATE_DATABASE_URL_FILE"), "owner-only file holding the PostgreSQL URL of the State runtime login (separate from the Console login of -database-url-file, same database)")
	setupDatabase := flag.Bool("setup-database", false, "explicit owner step: create the baseline in a fresh empty database and grant the Console and State runtime roles, then exit (an initialized database is only verified)")
	ownerURLFile := flag.String("database-owner-url-file", os.Getenv("ORCHESTRATOR_DATABASE_OWNER_URL_FILE"), "owner-only file holding the PostgreSQL URL of the owner role (used only with -setup-database)")
	consoleRole := flag.String("console-database-role", os.Getenv("ORCHESTRATOR_CONSOLE_DATABASE_ROLE"), "Console runtime login role granted by -setup-database")
	stateRole := flag.String("state-database-role", os.Getenv("ORCHESTRATOR_STATE_DATABASE_ROLE"), "State runtime login role granted by -setup-database")
	flag.Parse()
	if *setupDatabase {
		os.Exit(runDatabaseSetup(*ownerURLFile, *consoleRole, *stateRole))
	}
	var platformVault *bootstrap.PlatformVaultBootstrap
	if *platformVaultBootstrap {
		platformVault = &bootstrap.PlatformVaultBootstrap{
			Address: strings.TrimRight(*platformVaultAddress, "/"), WorkloadAddress: strings.TrimRight(*platformVaultWorkloadAddress, "/"),
			TokenFile: *platformVaultTokenFile, Mount: *platformVaultMount, AuthMount: *platformVaultAuthMount,
		}
	}
	databaseURL, stateDatabaseURL := "", ""
	if *databaseURLFile != "" {
		value, err := bootstrap.ReadCredentialFile("database URL", *databaseURLFile)
		if err != nil {
			log.Printf("orchestrator: %v", err)
			os.Exit(1)
		}
		databaseURL = value
	}
	if *terraformStateDatabaseURLFile != "" {
		value, err := bootstrap.ReadCredentialFile("State database URL", *terraformStateDatabaseURLFile)
		if err != nil {
			log.Printf("orchestrator: %v", err)
			os.Exit(1)
		}
		stateDatabaseURL = value
	}
	if err := bootstrap.DistinctFiles(map[string]string{"database URL": *databaseURLFile, "State database URL": *terraformStateDatabaseURLFile, "Terraform state key": *terraformStateKeyFile, "output secret token": *outputSecretTokenFile, "connection Vault token": *connectionVaultTokenFile}); err != nil {
		log.Printf("orchestrator: %v", err)
		os.Exit(1)
	}

	opts.NamespaceIdentity = *namespace
	opts.ApplicationKey = *applicationKey
	opts.Profile = *profile
	opts.Catalog = *seedCatalog
	if opts.Profile == seed.ProfileAcceptanceV2 && opts.Catalog == "" {
		opts.Catalog = seed.CatalogNone
	}
	opts.KubeContext = *kubeContext
	opts.ClusterName = *cluster
	opts.Region = *region
	opts.AccountID = *accountID
	opts.RunID = *runID
	opts.Images.Frontend = *frontendImage
	opts.Images.Backend = *backendImage
	opts.Images.Worker = *workerImage
	opts.BaseDomain = *publicBaseDomain

	ownerTag := *owner
	if ownerTag == "" {
		ownerTag = "orchestrator"
	}
	tags := map[string]string{
		"project":     "orchestrator",
		"owner":       ownerTag,
		"environment": opts.EnvironmentKey,
		"run-id":      *runID,
		"expires-at":  time.Now().UTC().Add(*ttl).Format(time.RFC3339),
		"managed-by":  "orchestrator-verification",
	}

	mode := bootstrap.AdapterMode(*adapters)
	if mode == bootstrap.AdapterAWS {
		// The acceptance-v2 profile has no profile-bound cloud Application: every AWS Definition names its own
		// account and literal region (ADR-017), so no process-wide region exists to require.
		if *region == "" && opts.Profile != seed.ProfileAcceptanceV2 {
			log.Print("orchestrator: -adapters aws needs -region")
			os.Exit(1)
		}
		if *runID == "" {
			log.Print("orchestrator: -adapters aws needs -run-id so cloud resources carry a run id tag")
			os.Exit(1)
		}
	}

	app, err := bootstrap.Build(context.Background(), bootstrap.Options{
		ScoreK8sPath:                  *scorePath,
		RequireScoreRenderer:          true,
		Seed:                          opts,
		PublicRouteKind:               *publicRouteKind,
		PublicGateway:                 *publicGateway,
		PublicIngressClass:            *publicIngressClass,
		UIDir:                         *uiDir,
		StatePath:                     *statePath,
		DatabaseURL:                   databaseURL,
		Adapters:                      mode,
		KubectlPath:                   *kubectlPath,
		TerraformPath:                 *terraformPath,
		TerraformRoot:                 *terraformRoot,
		TerraformRecoveryDir:          *terraformRecoveryDir,
		TerraformPluginCache:          *terraformCache,
		TerraformModuleCacheDir:       *moduleCacheDir,
		TerraformModulePolicyFile:     *modulePolicyFile,
		TerraformRetainedReleasesFile: *retainedReleases,
		GitPath:                       *gitPath,
		RenderBundleCacheDir:          *renderBundleCacheDir,
		Region:                        *region,
		Tags:                          tags,
		VaultDelivery:                 *vaultDelivery,
		WorkloadDelivery:              *workloadDelivery,
		GitOpsRepoDir:                 *gitopsRepoDir,
		GitOpsBranch:                  *gitopsBranch,
		FleetGitRepoName:              *fleetGitRepoName,
		HarborRegistryHost:            *harborRegistryHost,
		DiagnosticProbeImage:          *diagnosticProbeImage,
		HarborDockerConfigFile:        *harborDockerConfigFile,
		HarborPullSecretName:          *harborPullSecretName,

		PlatformVault: platformVault,

		DeployDeadline: *deployDeadline, TerraformInvocationTimeout: *terraformInvocationTimeout, TerraformReadinessTimeout: *terraformReadinessTimeout,

		OutputSecretStore: *outputSecretStore, OutputSecretVaultAddress: *outputSecretAddress,
		OutputSecretVaultTokenFile: *outputSecretTokenFile, OutputSecretVaultMount: *outputSecretMount,

		ConnectionCredentialStore: *connectionCredentialStore,
		ConnectionVaultAddress:    *connectionVaultAddress,
		ConnectionVaultTokenFile:  *connectionVaultTokenFile,
		ConnectionVaultMount:      *connectionVaultMount,
		TerraformStateKeyFile:     *terraformStateKeyFile,
		TerraformStateKeyID:       *terraformStateKeyID,
		TerraformStateDatabaseURL: stateDatabaseURL,
	})
	if err != nil {
		log.Printf("orchestrator: startup failed: %v", err)
		os.Exit(1)
	}

	os.Exit(serve(app, *addr, *addrFile))
}

// serve runs the API until it fails or SIGINT/SIGTERM arrives, then shuts
// the server down and closes every database pool of the App.
func serve(app *bootstrap.App, addr, addrFile string) int {
	defer app.Close()
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("orchestrator: listen %s: %v", addr, err)
		return 1
	}
	resolved := listener.Addr().String()
	if addrFile != "" {
		if err := os.WriteFile(addrFile, []byte(resolved), 0o600); err != nil {
			log.Printf("orchestrator: write addr file: %v", err)
			return 1
		}
	}

	server := &http.Server{Handler: app.Server, ReadHeaderTimeout: 10 * time.Second}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)
	go func() {
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()
	fmt.Printf("orchestrator listening on http://%s (api /api/v1/, console /ui/)\n", resolved)
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Printf("orchestrator: %v", err)
		return 1
	}
	return 0
}

// runDatabaseSetup runs the explicit owner setup and returns the exit code.
// The owner URL comes only from an owner-only file; nothing secret is logged.
func runDatabaseSetup(ownerURLFile, consoleRole, stateRole string) int {
	ownerURL, err := bootstrap.ReadCredentialFile("database owner URL", ownerURLFile)
	if err != nil {
		log.Printf("orchestrator: %v", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	initialized, err := postgres.Setup(ctx, ownerURL, postgres.Roles{Console: consoleRole, State: stateRole})
	if err != nil {
		log.Printf("orchestrator: database setup failed: %v", err)
		return 1
	}
	if initialized {
		fmt.Println("orchestrator: database baseline created and runtime roles granted")
	} else {
		fmt.Println("orchestrator: database already holds the baseline and the runtime grants; nothing changed")
	}
	return 0
}

// envBool reads an opt-in switch; only "true" or "1" enables it.
func envBool(name string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return v == "true" || v == "1"
}

// envOr returns the environment value of key, or fallback when unset.
func envOr(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}
