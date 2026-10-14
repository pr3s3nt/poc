package v2run

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Targets and scenarios of the acceptance.
const (
	TargetK8s4f = "k8s-4f"
	TargetAWS   = "aws"

	AWSAccountID = "452025861381"
	AWSRegion    = "us-east-1"
	// KubeContextK8s4f is the real local context name of the k8s-4f cluster;
	// the cluster name "k8s-4f" itself is not a context.
	KubeContextK8s4f = "kind-k8s-4f"

	NamespacePrefix = "orch-v2-test-"

	// The tags below are exactly what cmd/orchestrator stamps on every Terraform
	// instance (namespace labels/annotations and AWS default tags), so the
	// manifest, the supervisor guard and the AWS tag queries agree with the
	// product.
	ProductManagedByTag   = "orchestrator-verification"
	ProductEnvironmentTag = "dev"
)

// Deadline limits (docs: terraform-v2-acceptance.md).
const (
	MaxExecutionK8s = 60 * time.Minute
	MaxExecutionAWS = 180 * time.Minute
	MaxCleanup      = 120 * time.Minute
)

var (
	runIDPattern    = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{2,22}[a-z0-9])$`)
	digestPattern   = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	scenarioTargets = map[string]string{"A01": TargetK8s4f, "E01": TargetK8s4f, "E02": TargetK8s4f, "A02": TargetAWS, "A03": TargetAWS}
)

// Binary is a pinned tool: absolute path and expected SHA-256.
type Binary struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// Image is one immutable workload image for one node architecture.
type Image struct {
	Ref     string `json:"ref"`
	Digest  string `json:"digest"`            // registry manifest digest
	ImageID string `json:"imageId,omitempty"` // local Docker config digest, when publishing to ECR
}

// Config is the private (0600, outside Git) runner configuration. Secrets are
// never inline: every credential is a file handle path.
type Config struct {
	RunID    string `json:"runId"`
	Scenario string `json:"scenario"`
	Target   string `json:"target"`
	Owner    string `json:"owner"`
	// EvidenceDir must not exist yet; the runner creates it 0700.
	EvidenceDir string `json:"evidenceDir"`

	Execution Duration `json:"execution"`
	Cleanup   Duration `json:"cleanup"`

	Binaries map[string]Binary `json:"binaries"`
	// Images by node architecture then workload (frontend, backend, worker).
	Images map[string]map[string]Image `json:"images"`
	// NodeArch is the architecture of the target nodes (amd64 on k8s-4f).
	NodeArch string `json:"nodeArch"`

	Kube *KubeConfig `json:"kube,omitempty"`
	AWS  *AWSConfig  `json:"aws,omitempty"`

	PostgresAdminURLFile string `json:"postgresAdminUrlFile"`
	// VaultBin runs a run-owned `vault server -dev`; the Vault holds the
	// scoped Connection credentials and the workload Secret refs.
	VaultBin string `json:"vaultBin"`
	// SecretStore is the EXISTING authorized Vault the Console registers as a
	// Secret Store (UC-12 settings UI, masked token field) so the acceptance
	// app's ACCEPTANCE_SECRET is a real scoped secret binding. Required for
	// A01; A02/A03 record the optional app secret row as not configured. The
	// token is a 0600 file handle and is only ever typed into the masked field.
	SecretStore *SecretStoreConfig `json:"secretStore,omitempty"`
	// RedisCABundle is NOT supported: the acceptance workloads embed the public CA
	// roots and verify CA, hostname and SNI with no option to inject or skip
	// trust. A non-empty value is refused instead of pretending it is applied.
	RedisCABundle string `json:"redisCaBundle,omitempty"`
}

// SecretStoreConfig describes an existing Vault: BackendAddress is reachable
// by the product (operator host), WorkloadAddress by the Pods (for example
// http://vault.vault.svc:8200); AuthMount is its Kubernetes auth mount.
type SecretStoreConfig struct {
	Name            string `json:"name"`
	BackendAddress  string `json:"backendAddress"`
	WorkloadAddress string `json:"workloadAddress"`
	TokenFile       string `json:"tokenFile"`
	Mount           string `json:"mount"`
	AuthMount       string `json:"authMount"`
	// RunOwned makes the runner start its OWN second Vault (own session, bound
	// to BridgeAddress so Pods reach it, Kubernetes auth enabled from the
	// EXISTING reviewer handles) instead of reusing a shared one. Its values,
	// policies and roles die with it, so no deletion right on a shared Vault is
	// needed. BackendAddress, WorkloadAddress and TokenFile are then generated.
	RunOwned        bool   `json:"runOwned,omitempty"`
	BridgeAddress   string `json:"bridgeAddress,omitempty"`   // IPv4 of the kind Docker bridge reachable from the Pods
	KubernetesHost  string `json:"kubernetesHost,omitempty"`  // https API server URL used by Vault's token review
	ReviewerJWTFile string `json:"reviewerJwtFile,omitempty"` // EXISTING reviewer token, read into a 0600 handle by the operator
	ReviewerCAFile  string `json:"reviewerCaFile,omitempty"`
}

// KubeConfig is the k8s-4f target: host kubeconfig handle (preflight and the
// supervisor safety net only, never a product fallback) and the read-only
// sentinel outside the run.
type KubeConfig struct {
	Context    string `json:"context"`
	Kubeconfig string `json:"kubeconfig"`
	Namespace  string `json:"namespace"`
	// Sentinel is an EXISTING read-only resource outside the run whose UID must
	// stay unchanged; the run never creates a forbidden outside namespace.
	Sentinel Sentinel `json:"sentinel"`
}

// Sentinel names an existing resource and the UID observed before the run.
type Sentinel struct {
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	UID       string `json:"uid"`
}

// AWSConfig is the AWS target. No credential appears here: the preflight uses
// the host chain, the product only the Connection registered in the Console.
type AWSConfig struct {
	AccountID        string `json:"accountId"`
	Region           string `json:"region"`
	PrincipalARN     string `json:"principalArn"`
	ExecutorEgress   string `json:"executorEgressCidr"` // explicit /32
	NodeInstanceType string `json:"nodeInstanceType"`
	// AccessKeyFile is the 0600 JSON handle {"accessKeyId","secretAccessKey"} of the
	// scoped key the Console registers as the AWS Connection (typed into the
	// masked fields only). The product never reads the host credential chain.
	AccessKeyFile string `json:"accessKeyFile"`
	// SharedCredentialsFile, ConfigFile and Profile are OPTIONAL handles for the
	// operator's read-only preflight and cleanup verification (host chain; never
	// the product). Empty means the default chain of the supervisor's HOME.
	SharedCredentialsFile string `json:"sharedCredentialsFile,omitempty"`
	ConfigFile            string `json:"configFile,omitempty"`
	Profile               string `json:"profile,omitempty"`
	// EstimateFile is the pre-run cost estimate (see Estimate); the AWS run is
	// refused without a complete, current one.
	EstimateFile string `json:"estimateFile"`
	// ECRRepositoryPrefix is the exact lowercase prefix used to create three
	// immutable run-owned acceptance-image repositories (one per workload).
	ECRRepositoryPrefix string `json:"ecrRepositoryPrefix"`
	// ProbePostgresImage and ProbeRedisImage are the client images of the labelled
	// technical Pods (psql / redis-cli inside the VPC). Both are pinned by digest
	// (multi-arch index digests are fine: the Pod runs on the configured node arch).
	ProbePostgresImage Image `json:"probePostgresImage"`
	ProbeRedisImage    Image `json:"probeRedisImage"`
	// I0411Evidence are the evidence directories of the PASSED k8s-4f runs
	// (A01, E01, E02): no AWS effect before I04-11 passed and was cleaned.
	I0411Evidence []string `json:"i0411Evidence"`
}

// LoadConfig reads and validates a private configuration file.
func LoadConfig(path string) (Config, error) {
	var c Config
	if err := requirePrivateFile(path); err != nil {
		return c, err
	}
	if InsideGitWorkTree(path) {
		return c, errors.New("the configuration file must be outside any Git work tree")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, fmt.Errorf("invalid configuration: %w", err)
	}
	return c, c.Validate()
}

// InsideGitWorkTree reports whether path (or an ancestor) holds a .git entry.
func InsideGitWorkTree(path string) bool {
	dir, err := filepath.Abs(path)
	if err != nil {
		return true
	}
	for {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

// Validate checks every invariant that must hold before anything is created.
func (c Config) Validate() error {
	var errs []string
	add := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }
	if !runIDPattern.MatchString(c.RunID) {
		add("runId must be 4-24 lowercase letters, digits and hyphens")
	}
	target, known := scenarioTargets[c.Scenario]
	if !known {
		add("scenario must be one of A01, E01, E02, A02, A03")
	} else if target != c.Target {
		add("scenario %s runs on %s, not %q", c.Scenario, target, c.Target)
	}
	if c.Owner == "" {
		add("owner is required for the run tags")
	}
	if !filepath.IsAbs(c.EvidenceDir) || InsideGitWorkTree(c.EvidenceDir) {
		add("evidenceDir must be absolute and outside any Git work tree")
	}
	limit := MaxExecutionK8s
	if c.Target == TargetAWS {
		limit = MaxExecutionAWS
	}
	if c.Execution.Duration <= 0 || c.Execution.Duration > limit {
		add("execution deadline must be positive and at most %s for %s", limit, c.Target)
	}
	if c.Cleanup.Duration <= 0 || c.Cleanup.Duration > MaxCleanup {
		add("cleanup budget must be positive and at most %s", MaxCleanup)
	}
	for _, name := range []string{"terraform", "score-k8s", "kubectl"} {
		b, ok := c.Binaries[name]
		if !ok || !filepath.IsAbs(b.Path) || len(b.SHA256) != 64 {
			add("binary %s needs an absolute path and a sha256", name)
		}
	}
	if c.Target == TargetAWS {
		if b, ok := c.Binaries["aws"]; !ok || !filepath.IsAbs(b.Path) || len(b.SHA256) != 64 {
			add("binary aws needs an absolute path and a sha256")
		}
		if b, ok := c.Binaries["docker"]; !ok || !filepath.IsAbs(b.Path) || len(b.SHA256) != 64 {
			add("binary docker needs an absolute path and a sha256 for owned ECR image preparation")
		}
	}
	if c.NodeArch != "amd64" && c.NodeArch != "arm64" {
		add("nodeArch must be amd64 or arm64")
	}
	c.validateImages(add)
	if !filepath.IsAbs(c.PostgresAdminURLFile) {
		add("postgresAdminUrlFile must be an absolute path to a disposable server's admin URL")
	}
	if !filepath.IsAbs(c.VaultBin) {
		add("vaultBin must be an absolute path")
	}
	if c.RedisCABundle != "" {
		add("redisCaBundle is unsupported: the workloads verify Redis TLS with their embedded public roots and cannot take custom trust roots")
	}
	c.validateSecretStore(add)
	switch c.Target {
	case TargetK8s4f:
		c.validateKube(add)
	case TargetAWS:
		c.validateAWS(add)
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.New("invalid configuration: " + strings.Join(errs, "; "))
}

func (c Config) validateImages(add func(string, ...any)) {
	arch := c.Images[c.NodeArch]
	for _, workload := range []string{"frontend", "backend", "worker"} {
		img, ok := arch[workload]
		switch {
		case !ok:
			add("no %s image for %s nodes", workload, c.NodeArch)
		case !digestPattern.MatchString(img.Digest):
			add("%s image digest must be sha256:<64 hex>", workload)
		case img.Ref == "" || strings.HasSuffix(img.Ref, ":latest") || !strings.Contains(img.Ref, ":") && !strings.Contains(img.Ref, "@"):
			add("%s image ref must be a pinned reference (not latest or untagged)", workload)
		}
	}
}

func (c Config) validateSecretStore(add func(string, ...any)) {
	// Only A01 binds the optional ACCEPTANCE_SECRET (scoped Vault through the Console). On AWS the optional binding is NOT_CONFIGURED by design (it shows FAIL while the job form
	// stays available); no unreachable secret store is required there.
	needs := c.Scenario == "A01"
	ss := c.SecretStore
	if ss == nil {
		if needs {
			add("secretStore is required: the acceptance app's secret check needs a real scoped secret binding")
		}
		return
	}
	if ss.RunOwned {
		if ss.Name == "" {
			add("secretStore.name is required")
		}
		if ip := net.ParseIP(ss.BridgeAddress); ip == nil || ip.To4() == nil || ip.IsUnspecified() || ip.IsLoopback() {
			add("secretStore.bridgeAddress must be the specific IPv4 of the Docker bridge reachable from the Pods")
		}
		if u, err := url.Parse(ss.KubernetesHost); err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			add("secretStore.kubernetesHost must be the https Kubernetes API URL")
		}
		for name, v := range map[string]string{"reviewerJwtFile": ss.ReviewerJWTFile, "reviewerCaFile": ss.ReviewerCAFile} {
			if !filepath.IsAbs(v) {
				add("secretStore.%s must be an absolute path to a private handle", name)
			}
		}
		return
	}
	for name, v := range map[string]string{"name": ss.Name, "mount": ss.Mount, "authMount": ss.AuthMount} {
		if v == "" || len(v) > 100 {
			add("secretStore.%s is required", name)
		}
	}
	for name, v := range map[string]string{"backendAddress": ss.BackendAddress, "workloadAddress": ss.WorkloadAddress} {
		if u, err := url.Parse(v); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" {
			add("secretStore.%s must be an http(s) URL without credentials", name)
		}
	}
	if !filepath.IsAbs(ss.TokenFile) {
		add("secretStore.tokenFile must be an absolute path")
	}
}

func (c Config) validateKube(add func(string, ...any)) {
	k := c.Kube
	if k == nil {
		add("kube settings are required for k8s-4f")
		return
	}
	if k.Context != KubeContextK8s4f {
		add("kube context must be %q (the cluster name k8s-4f is not a context)", KubeContextK8s4f)
	}
	if !filepath.IsAbs(k.Kubeconfig) {
		add("kube kubeconfig handle must be an absolute path")
	}
	if k.Namespace != NamespacePrefix+c.RunID {
		add("namespace must be exactly %s<runId>", NamespacePrefix)
	}
	s := k.Sentinel
	if s.Namespace == "" || s.Kind == "" || s.Name == "" || s.UID == "" {
		add("an existing read-only sentinel (namespace, kind, name, uid) outside the run is required")
	}
	if strings.HasPrefix(s.Namespace, NamespacePrefix) {
		add("the sentinel must be an existing resource outside the run, not an orch-v2-test namespace")
	}
	if c.NodeArch != "amd64" {
		add("k8s-4f nodes are amd64")
	}
}

func (c Config) validateAWS(add func(string, ...any)) {
	a := c.AWS
	if a == nil {
		add("aws settings are required for AWS scenarios")
		return
	}
	if a.AccountID != AWSAccountID || a.Region != AWSRegion {
		add("AWS is restricted to account %s in %s", AWSAccountID, AWSRegion)
	}
	if !strings.HasPrefix(a.PrincipalARN, "arn:aws:iam::"+AWSAccountID+":") {
		add("principalArn must be an IAM principal of the account")
	}
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,30}$`).MatchString(a.ECRRepositoryPrefix) {
		add("aws.ecrRepositoryPrefix must be 3-31 lowercase letters, digits, dots, underscores or hyphens")
	}
	ip, network, err := net.ParseCIDR(a.ExecutorEgress)
	if err != nil || ip.To4() == nil || strings.HasSuffix(a.ExecutorEgress, "/0") {
		add("executorEgressCidr must be an explicit IPv4 /32")
	} else if ones, _ := network.Mask.Size(); ones != 32 {
		add("executorEgressCidr must be an explicit IPv4 /32")
	}
	for name, img := range map[string]Image{"probePostgresImage": a.ProbePostgresImage, "probeRedisImage": a.ProbeRedisImage} {
		if img.Ref == "" || strings.HasSuffix(img.Ref, ":latest") || !digestPattern.MatchString(img.Digest) {
			add("aws.%s needs a pinned ref and a sha256 digest", name)
		}
	}
	for name, v := range map[string]string{"accessKeyFile": a.AccessKeyFile, "estimateFile": a.EstimateFile} {
		if !filepath.IsAbs(v) {
			add("aws.%s must be an absolute path", name)
		}
	}
	for _, dir := range a.I0411Evidence {
		if !filepath.IsAbs(dir) {
			add("aws.i0411Evidence entries must be absolute evidence directories")
		}
	}
	inst, ok := KnownInstances[a.NodeInstanceType]
	switch {
	case !ok:
		add("unknown node instance type %q", a.NodeInstanceType)
	case inst.Arch != c.NodeArch:
		add("node instance type %s is %s but nodeArch is %s", inst.Name, inst.Arch, c.NodeArch)
	case c.Scenario == "A02" || c.Scenario == "A03":
		if res, err := CheckAWSScenario(c.Scenario, a.NodeInstanceType); err != nil || !res.OK() {
			add("pod capacity of %s is insufficient for %s: %v", a.NodeInstanceType, c.Scenario, res.Failures)
		}
	}
}

// ValidateFresh is Validate plus the start-of-run rule that the evidence
// directory does not exist yet (the runner creates it 0700 and keeps it).
func (c Config) ValidateFresh() error {
	if err := c.Validate(); err != nil {
		return err
	}
	if _, err := os.Lstat(c.EvidenceDir); err == nil {
		return errors.New("invalid configuration: evidenceDir already exists; choose a new path")
	}
	return nil
}

// Tags are the run tags every created asset carries.
func (c Config) Tags(now time.Time) map[string]string {
	return map[string]string{
		"project": "orchestrator", "owner": c.Owner, "environment": ProductEnvironmentTag,
		"run-id": c.RunID, "expires-at": now.Add(c.Execution.Duration).UTC().Format(time.RFC3339), "managed-by": ProductManagedByTag,
	}
}

// PublicView is the redacted configuration safe for evidence: handles and
// secrets' locations are omitted, pins and deadlines kept.
func (c Config) PublicView() map[string]any {
	bins := map[string]string{}
	for k, b := range c.Binaries {
		bins[k] = b.SHA256
	}
	view := map[string]any{
		"runId": c.RunID, "scenario": c.Scenario, "target": c.Target, "nodeArch": c.NodeArch,
		"executionDeadline": c.Execution.String(), "cleanupBudget": c.Cleanup.String(), "binarySha256": bins,
	}
	images := map[string]string{}
	for w, img := range c.Images[c.NodeArch] {
		images[w] = img.Ref + "@" + img.Digest
	}
	view["images"] = images
	if c.Kube != nil {
		view["kubeContext"], view["namespace"] = c.Kube.Context, c.Kube.Namespace
	}
	if c.AWS != nil {
		view["awsAccount"], view["awsRegion"], view["nodeInstanceType"] = c.AWS.AccountID, c.AWS.Region, c.AWS.NodeInstanceType
	}
	return view
}

// ApplicationKey is the unique per-run key of the fixture Application. Vault
// paths, policies and roles of the secret row are scoped by it, so a run never
// touches the values of an existing acceptance Application.
func (c Config) ApplicationKey() string { return "acc-" + c.RunID }
