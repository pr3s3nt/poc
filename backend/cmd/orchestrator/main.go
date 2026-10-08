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
	"strings"
	"time"

	"orchestrator/internal/bootstrap"
	"orchestrator/internal/seed"
)

func main() {
	opts := seed.Defaults()
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	addrFile := flag.String("addr-file", "", "optional file that receives the resolved listen address")
	uiDir := flag.String("ui-dir", "../frontend/dist", "directory holding the Web Console production bundle")
	statePath := flag.String("state", "", "optional path of the JSON state snapshot")
	databaseURLFile := flag.String("database-url-file", os.Getenv("ORCHESTRATOR_DATABASE_URL_FILE"), "path to a file containing the PostgreSQL connection URL")
	adapters := flag.String("adapters", "fake", "executor adapters: fake, kubernetes or aws")
	profile := flag.String("profile", opts.Profile, "seed profile: local, test, or production")
	namespace := flag.String("namespace", opts.NamespaceIdentity, "namespace identity of the internal-k8s environment")
	cloudNamespace := flag.String("cloud-namespace", opts.CloudNamespaceIdentity, "namespace identity of the aws-eks environment")
	kubeContext := flag.String("kube-context", opts.KubeContext, "kubectl context of the registered cluster")
	cluster := flag.String("cluster", opts.ClusterName, "registered cluster name")
	region := flag.String("region", "", "AWS region; the aws-eks application is registered only when it is set")
	accountID := flag.String("account-id", "", "AWS account id for the aws-eks application")
	runID := flag.String("run-id", "", "verification run id, used in cloud resource names and tags")
	owner := flag.String("owner", "", "owner tag for cloud resources")
	ttl := flag.Duration("ttl", 2*time.Hour, "expires-at tag horizon for cloud resources")
	frontendImage := flag.String("frontend-image", opts.Images.Frontend, "acceptance frontend image")
	backendImage := flag.String("backend-image", opts.Images.Backend, "acceptance backend image")
	workerImage := flag.String("worker-image", opts.Images.Worker, "acceptance worker image")
	terraformRoot := flag.String("terraform-root", "", "directory holding Terraform run state")
	terraformCache := flag.String("terraform-plugin-cache", os.Getenv("TF_PLUGIN_CACHE_DIR"), "Terraform provider cache directory")
	kubectlPath := flag.String("kubectl", "kubectl", "kubectl binary")
	scorePath := flag.String("score-k8s", "", "explicit path to pinned score-k8s 0.15.0 renderer (optional)")
	terraformPath := flag.String("terraform", "terraform", "terraform binary")
	vaultAddress := flag.String("vault-address", os.Getenv("ORCHESTRATOR_VAULT_ADDR"), "Vault API address for UC-12")
	vaultTokenFile := flag.String("vault-token-file", os.Getenv("ORCHESTRATOR_VAULT_TOKEN_FILE"), "path to scoped Vault token file for UC-12")
	vaultAgentAddress := flag.String("vault-agent-address", os.Getenv("ORCHESTRATOR_VAULT_AGENT_ADDR"), "in-cluster Vault address used by injected workload Pods")
	vaultDelivery := flag.String("vault-delivery", "auto", "UC-12 Kubernetes delivery: auto, vso or agent")
	workloadDelivery := flag.String("workload-delivery", "direct", "workload delivery: direct or fleet-gitrepo (kind only)")
	gitopsRepoDir := flag.String("gitops-repo-dir", "", "writable local clone of the Fleet GitOps repository")
	gitopsBranch := flag.String("gitops-branch", "main", "Fleet GitOps branch")
	fleetGitRepoName := flag.String("fleet-gitrepo-name", "poc-workloads", "Fleet GitRepo resource name in fleet-local")
	harborRegistryHost := flag.String("harbor-registry", "", "Harbor registry host:port used in workload image references")
	harborDockerConfigFile := flag.String("harbor-dockerconfig-file", "", "owner-only Docker config JSON for namespace imagePullSecret")
	harborPullSecretName := flag.String("harbor-pull-secret", "harbor-pull", "namespace-local imagePullSecret name")
	connectionCredentialStore := flag.String("connection-credential-store", os.Getenv("ORCHESTRATOR_CONNECTION_CREDENTIAL_STORE"), "UC-04 Connection credential store: none (upload registration unavailable), vault, or memory (fake adapters without persistent state only)")
	connectionVaultAddress := flag.String("connection-vault-address", os.Getenv("ORCHESTRATOR_CONNECTION_VAULT_ADDR"), "Vault API address of the Connection credential store")
	connectionVaultTokenFile := flag.String("connection-vault-token-file", os.Getenv("ORCHESTRATOR_CONNECTION_VAULT_TOKEN_FILE"), "path to the scoped Vault token file of the Connection credential store")
	connectionVaultMount := flag.String("connection-vault-mount", "kv", "KV v2 mount of the Connection credential store")
	platformVaultBootstrap := flag.Bool("platform-vault-bootstrap", envBool("ORCHESTRATOR_PLATFORM_VAULT_BOOTSTRAP"), "explicit opt-in: register the bundled Compose Vault as the ordinary verified store platform-vault at startup")
	platformVaultAddress := flag.String("platform-vault-address", os.Getenv("ORCHESTRATOR_PLATFORM_VAULT_ADDR"), "Vault API address of the platform-vault bootstrap")
	platformVaultWorkloadAddress := flag.String("platform-vault-workload-address", os.Getenv("ORCHESTRATOR_PLATFORM_VAULT_WORKLOAD_ADDR"), "Vault address used by workload Pods (default: the API address)")
	platformVaultTokenFile := flag.String("platform-vault-token-file", os.Getenv("ORCHESTRATOR_PLATFORM_VAULT_TOKEN_FILE"), "bootstrap-only file holding the application-scoped Vault token")
	platformVaultMount := flag.String("platform-vault-mount", os.Getenv("ORCHESTRATOR_PLATFORM_VAULT_MOUNT"), "KV v2 mount of the platform-vault bootstrap (default kv)")
	platformVaultAuthMount := flag.String("platform-vault-auth-mount", os.Getenv("ORCHESTRATOR_PLATFORM_VAULT_AUTH_MOUNT"), "Kubernetes auth mount of the platform-vault bootstrap (default kubernetes)")
	flag.Parse()
	var platformVault *bootstrap.PlatformVaultBootstrap
	if *platformVaultBootstrap {
		platformVault = &bootstrap.PlatformVaultBootstrap{
			Address: strings.TrimRight(*platformVaultAddress, "/"), WorkloadAddress: strings.TrimRight(*platformVaultWorkloadAddress, "/"),
			TokenFile: *platformVaultTokenFile, Mount: *platformVaultMount, AuthMount: *platformVaultAuthMount,
		}
	}
	databaseURL := ""
	if *databaseURLFile != "" {
		value, err := os.ReadFile(*databaseURLFile)
		if err != nil {
			log.Printf("orchestrator: read database URL file: %v", err)
			os.Exit(1)
		}
		databaseURL = strings.TrimSpace(string(value))
		if databaseURL == "" {
			log.Print("orchestrator: database URL file is empty")
			os.Exit(1)
		}
	}

	opts.NamespaceIdentity = *namespace
	opts.Profile = *profile
	opts.CloudNamespaceIdentity = *cloudNamespace
	opts.KubeContext = *kubeContext
	opts.ClusterName = *cluster
	opts.Region = *region
	opts.AccountID = *accountID
	opts.RunID = *runID
	opts.Images.Frontend = *frontendImage
	opts.Images.Backend = *backendImage
	opts.Images.Worker = *workerImage

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
		if *region == "" {
			log.Print("orchestrator: -adapters aws needs -region")
			os.Exit(1)
		}
		if *terraformRoot == "" {
			log.Print("orchestrator: -adapters aws needs -terraform-root")
			os.Exit(1)
		}
		if *runID == "" {
			log.Print("orchestrator: -adapters aws needs -run-id so cloud resources carry a run id tag")
			os.Exit(1)
		}
	}

	app, err := bootstrap.Build(context.Background(), bootstrap.Options{
		ScoreK8sPath:           *scorePath,
		Seed:                   opts,
		UIDir:                  *uiDir,
		StatePath:              *statePath,
		DatabaseURL:            databaseURL,
		Adapters:               mode,
		KubectlPath:            *kubectlPath,
		TerraformPath:          *terraformPath,
		TerraformRoot:          *terraformRoot,
		TerraformPluginCache:   *terraformCache,
		Region:                 *region,
		Tags:                   tags,
		VaultAddress:           *vaultAddress,
		VaultTokenFile:         *vaultTokenFile,
		VaultAgentAddress:      *vaultAgentAddress,
		VaultDelivery:          *vaultDelivery,
		WorkloadDelivery:       *workloadDelivery,
		GitOpsRepoDir:          *gitopsRepoDir,
		GitOpsBranch:           *gitopsBranch,
		FleetGitRepoName:       *fleetGitRepoName,
		HarborRegistryHost:     *harborRegistryHost,
		HarborDockerConfigFile: *harborDockerConfigFile,
		HarborPullSecretName:   *harborPullSecretName,

		PlatformVault: platformVault,

		ConnectionCredentialStore: *connectionCredentialStore,
		ConnectionVaultAddress:    *connectionVaultAddress,
		ConnectionVaultTokenFile:  *connectionVaultTokenFile,
		ConnectionVaultMount:      *connectionVaultMount,
	})
	if err != nil {
		log.Printf("orchestrator: startup failed: %v", err)
		os.Exit(1)
	}

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Printf("orchestrator: listen %s: %v", *addr, err)
		os.Exit(1)
	}
	resolved := listener.Addr().String()
	if *addrFile != "" {
		if err := os.WriteFile(*addrFile, []byte(resolved), 0o600); err != nil {
			log.Printf("orchestrator: write addr file: %v", err)
			os.Exit(1)
		}
	}

	server := &http.Server{Handler: app.Server, ReadHeaderTimeout: 10 * time.Second}
	fmt.Printf("orchestrator listening on http://%s (api /api/v1/, console /ui/)\n", resolved)
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Printf("orchestrator: %v", err)
		os.Exit(1)
	}
}

// envBool reads an opt-in switch; only "true" or "1" enables it.
func envBool(name string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return v == "true" || v == "1"
}
