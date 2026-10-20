package v2run_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"orchestrator/internal/domain/environment"
	"orchestrator/test/integration/v2run"
)

func goodConfig(t *testing.T) v2run.Config {
	t.Helper()
	bin := func(n string) v2run.Binary { return v2run.Binary{Path: "/opt/" + n, SHA256: strings.Repeat("a", 64)} }
	img := func(w string) v2run.Image {
		return v2run.Image{Ref: "registry.example/acceptance-" + w + ":m04-1", Digest: "sha256:" + strings.Repeat("b", 64)}
	}
	return v2run.Config{
		RunID: "a01-0001", Scenario: "A01", Target: v2run.TargetK8s4f, Owner: "codex", EvidenceDir: filepath.Join(t.TempDir(), "evidence"),
		Execution: v2run.Duration{Duration: 60 * time.Minute}, Cleanup: v2run.Duration{Duration: 120 * time.Minute},
		Binaries: map[string]v2run.Binary{"terraform": bin("terraform"), "score-k8s": bin("score"), "kubectl": bin("kubectl")},
		NodeArch: "amd64", Images: map[string]map[string]v2run.Image{"amd64": {"frontend": img("frontend"), "backend": img("backend"), "worker": img("worker")}},
		SecretStore: &v2run.SecretStoreConfig{Name: "platform-vault", BackendAddress: "http://127.0.0.1:18200", WorkloadAddress: "http://vault.vault.svc:8200", TokenFile: "/priv/secret-store.token", Mount: "kv", AuthMount: "kubernetes"},
		Kube: &v2run.KubeConfig{Context: v2run.KubeContextK8s4f, Kubeconfig: "/priv/kubeconfig", Namespace: "orch-v2-test-a01-0001",
			Sentinel: v2run.Sentinel{Namespace: "kube-public", Kind: "configmap", Name: "cluster-info", UID: "uid-1"}},
		PostgresAdminURLFile: "/priv/pg-admin", VaultBin: "/opt/vault",
	}
}

func TestConfigAcceptsTheK8s4fShapeAndRefusesUnsafeOnes(t *testing.T) {
	if err := goodConfig(t).Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*v2run.Config){
		"context is the cluster name":  func(c *v2run.Config) { c.Kube.Context = "k8s-4f" },
		"namespace is not run-owned":   func(c *v2run.Config) { c.Kube.Namespace = "orch-v2-test-other" },
		"namespace outside the prefix": func(c *v2run.Config) { c.Kube.Namespace = "production" },
		"sentinel inside a test ns":    func(c *v2run.Config) { c.Kube.Sentinel.Namespace = "orch-v2-test-x" },
		"no sentinel uid":              func(c *v2run.Config) { c.Kube.Sentinel.UID = "" },
		"execution over 60 minutes":    func(c *v2run.Config) { c.Execution.Duration = 61 * time.Minute },
		"no deadline":                  func(c *v2run.Config) { c.Execution.Duration = 0 },
		"cleanup over 120 minutes":     func(c *v2run.Config) { c.Cleanup.Duration = 121 * time.Minute },
		"latest tag": func(c *v2run.Config) {
			c.Images["amd64"]["worker"] = v2run.Image{Ref: "r/w:latest", Digest: "sha256:" + strings.Repeat("b", 64)}
		},
		"mutable image without digest": func(c *v2run.Config) { c.Images["amd64"]["backend"] = v2run.Image{Ref: "r/b:1", Digest: "sha256:abc"} },
		"wrong scenario target":        func(c *v2run.Config) { c.Scenario = "A02" },
		"unknown scenario":             func(c *v2run.Config) { c.Scenario = "A09" },
		"relative terraform": func(c *v2run.Config) {
			c.Binaries["terraform"] = v2run.Binary{Path: "terraform", SHA256: strings.Repeat("a", 64)}
		},
		"arm nodes on k8s-4f": func(c *v2run.Config) { c.NodeArch = "arm64" },
		"bad run id":          func(c *v2run.Config) { c.RunID = "A" },
	} {
		c := goodConfig(t)
		mutate(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func awsConfig(t *testing.T, scenario, instance string) v2run.Config {
	c := goodConfig(t)
	c.Scenario, c.Target, c.Kube, c.NodeArch = scenario, v2run.TargetAWS, nil, "arm64"
	c.Execution = v2run.Duration{Duration: 180 * time.Minute}
	c.Binaries["aws"] = v2run.Binary{Path: "/opt/aws", SHA256: strings.Repeat("a", 64)}
	c.Binaries["docker"] = v2run.Binary{Path: "/opt/docker", SHA256: strings.Repeat("a", 64)}
	c.Images["arm64"] = c.Images["amd64"]
	c.AWS = &v2run.AWSConfig{AccountID: v2run.AWSAccountID, Region: v2run.AWSRegion, PrincipalARN: "arn:aws:iam::452025861381:user/x", ExecutorEgress: "203.0.113.9/32", NodeInstanceType: instance, AccessKeyFile: "/priv/aws-key.json", EstimateFile: "/priv/estimate.json", ECRRepositoryPrefix: "orch-acceptance", ProbePostgresImage: v2run.Image{Ref: "postgres:16-alpine", Digest: "sha256:" + strings.Repeat("c", 64)}, ProbeRedisImage: v2run.Image{Ref: "redis:7.4-alpine", Digest: "sha256:" + strings.Repeat("d", 64)}}
	return c
}

func TestAWSConfigGatesAccountRegionEgressAndPodCapacity(t *testing.T) {
	if err := awsConfig(t, "A03", "t4g.medium").Validate(); err != nil {
		t.Fatal(err)
	}
	if err := awsConfig(t, "A03", "t4g.small").Validate(); err == nil || !strings.Contains(err.Error(), "pod capacity") {
		t.Fatalf("t4g.small must fail the capacity gate before create: %v", err)
	}
	for name, mutate := range map[string]func(*v2run.Config){
		"other account":   func(c *v2run.Config) { c.AWS.AccountID = "111122223333" },
		"other region":    func(c *v2run.Config) { c.AWS.Region = "eu-west-1" },
		"wide egress":     func(c *v2run.Config) { c.AWS.ExecutorEgress = "203.0.113.0/24" },
		"open egress":     func(c *v2run.Config) { c.AWS.ExecutorEgress = "0.0.0.0/0" },
		"foreign account": func(c *v2run.Config) { c.AWS.PrincipalARN = "arn:aws:iam::111122223333:user/x" },
		"x86 type on arm": func(c *v2run.Config) { c.AWS.NodeInstanceType = "t3.medium" },
		"over 180":        func(c *v2run.Config) { c.Execution.Duration = 181 * time.Minute },
	} {
		c := awsConfig(t, "A03", "t4g.medium")
		mutate(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestLoadConfigRequiresPrivateFileOutsideGit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := writeSpecAny(path, goodConfig(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := v2run.LoadConfig(path); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(path, 0o640)
	if _, err := v2run.LoadConfig(path); err == nil {
		t.Fatal("group-readable configuration accepted")
	}
	_ = os.Chmod(path, 0o600)
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := v2run.LoadConfig(path); err == nil {
		t.Fatal("a configuration inside a Git work tree was accepted")
	}
}

func newManifest(t *testing.T) (string, v2run.Manifest) {
	t.Helper()
	m := v2run.NewManifest(goodConfig(t), time.Now())
	path := filepath.Join(t.TempDir(), "manifest.json")
	return path, m
}

func TestManifestRecordsTheExactNamespaceNeverAGuess(t *testing.T) {
	_, m := newManifest(t)
	exact := environment.NamespaceFor("orch-v2-test-a01-0001", 2)
	if exact == "orch-v2-test-a01-0001" {
		t.Fatal("generation suffix expected")
	}
	if err := m.RecordNamespace("orch-v2-test-a01-0001", 2, "orch-v2-test-a01-0001"); err == nil {
		t.Fatal("a guessed name without the generation suffix was accepted")
	}
	if err := m.RecordNamespace("orch-v2-test-a01-0001", 2, exact); err != nil {
		t.Fatal(err)
	}
	if err := m.RecordNamespace("other-identity", 0, "other-identity"); err == nil {
		t.Fatal("a foreign namespace was accepted")
	}
}

func TestManifestArmsOnlyWithPinsCountsAndTagsAndThenFreezes(t *testing.T) {
	_, m := newManifest(t)
	if err := m.Arm(); err == nil {
		t.Fatal("armed without pins and counts")
	}
	_ = m.AddPin(v2run.Pin{Name: "terraform", Version: "1.9.8", SHA256: strings.Repeat("a", 64)})
	_ = m.AddPlanned("kubernetes_namespace_v1", 1)
	if err := m.Arm(); err != nil {
		t.Fatal(err)
	}
	if err := m.AddPin(v2run.Pin{Name: "x"}); err == nil {
		t.Fatal("pins changed after arming")
	}
	if err := m.AddPlanned("x", 1); err == nil {
		t.Fatal("counts changed after arming")
	}
}

func TestManifestOwnershipNeverUpgradesAndOrdersReverse(t *testing.T) {
	_, m := newManifest(t)
	add := func(a v2run.Asset) error { return m.AddAsset(a) }
	_ = add(v2run.Asset{Kind: "namespace", ID: "ns", Layer: v2run.LayerNamespace, Owner: v2run.OwnerCreatedByRun})
	_ = add(v2run.Asset{Kind: "deployment", ID: "web", Layer: v2run.LayerDelivery, Owner: v2run.OwnerCreatedByRun})
	_ = add(v2run.Asset{Kind: "statefulset", ID: "redis", Layer: v2run.LayerAppResources, Owner: v2run.OwnerCreatedByRun})
	_ = add(v2run.Asset{Kind: "pvc", ID: "data", Layer: v2run.LayerAppResources, Owner: v2run.OwnerRetained})
	_ = add(v2run.Asset{Kind: "namespace", ID: "sentinel", Layer: v2run.LayerNamespace, Owner: v2run.OwnerPreexisting})
	if err := add(v2run.Asset{Kind: "namespace", ID: "sentinel", Layer: v2run.LayerNamespace, Owner: v2run.OwnerCreatedByRun}); err == nil {
		t.Fatal("a preexisting asset was upgraded to CREATED_BY_RUN")
	}
	if err := add(v2run.Asset{Kind: "pvc", ID: "data", Layer: v2run.LayerAppResources, Owner: v2run.OwnerCreatedByRun}); err == nil {
		t.Fatal("a retained asset was upgraded to CREATED_BY_RUN")
	}
	// A later observation of the same object records its original creation UID;
	// repeating a name-only observation must preserve that identity in durable JSON.
	if err := add(v2run.Asset{Kind: "deployment", ID: "web", Layer: v2run.LayerDelivery, Owner: v2run.OwnerCreatedByRun, UID: "c3efcf94-c21a-4f44-9620-e9ded4c580bb"}); err != nil {
		t.Fatal(err)
	}
	if err := add(v2run.Asset{Kind: "deployment", ID: "web", Layer: v2run.LayerDelivery, Owner: v2run.OwnerCreatedByRun}); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var durable v2run.Manifest
	if err := json.Unmarshal(raw, &durable); err != nil {
		t.Fatal(err)
	}
	if durable.OwnedByRun()[0].UID != "c3efcf94-c21a-4f44-9620-e9ded4c580bb" {
		t.Fatal("original UID was not durably preserved")
	}
	var got []string
	for _, a := range m.OwnedByRun() {
		got = append(got, a.ID)
	}
	if strings.Join(got, ",") != "web,redis,ns" {
		t.Fatalf("cleanup order %v", got)
	}
	if len(m.Protected()) != 2 {
		t.Fatalf("protected %v", m.Protected())
	}
}

// Concurrent writers (runner, helpers) never lose an update or leave a torn file.
func TestManifestConcurrentUpdatesAreSerialized(t *testing.T) {
	path, m := newManifest(t)
	if err := v2run.SaveManifest(path, m); err != nil {
		t.Fatal(err)
	}
	if err := v2run.SaveManifest(path, m); err == nil {
		t.Fatal("manifest overwritten")
	}
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := v2run.UpdateManifest(path, func(m *v2run.Manifest) error {
				return m.AddAsset(v2run.Asset{Kind: "pod", ID: fmt.Sprintf("p%02d", i), Layer: v2run.LayerDelivery, Owner: v2run.OwnerCreatedByRun})
			})
			if err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	got, err := v2run.LoadManifest(path)
	if err != nil || len(got.Assets) != 40 {
		t.Fatalf("assets=%d err=%v", len(got.Assets), err)
	}
	// A failing update writes nothing.
	_ = v2run.UpdateManifest(path, func(m *v2run.Manifest) error {
		m.Assets = nil
		return fmt.Errorf("reject")
	})
	if again, _ := v2run.LoadManifest(path); len(again.Assets) != 40 {
		t.Fatal("a failed update changed the manifest")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode())
	}
}

func TestValidateFreshRequiresAnUnusedEvidenceDirectory(t *testing.T) {
	c := goodConfig(t)
	if err := c.ValidateFresh(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(c.EvidenceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := c.ValidateFresh(); err == nil {
		t.Fatal("an existing evidence directory was accepted at the start of a run")
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("later commands must still load the configuration: %v", err)
	}
}

// The documented example is the configuration interface of the runner: it must
// parse with unknown fields refused, for the k8s-4f shape it describes.
func TestConfigExampleParsesStrictly(t *testing.T) {
	raw, err := os.ReadFile("testdata/config.example.json")
	if err != nil {
		t.Fatal(err)
	}
	var c v2run.Config
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		t.Fatal(err)
	}
	if c.Target != v2run.TargetK8s4f || c.Kube == nil || c.Kube.Context != v2run.KubeContextK8s4f {
		t.Fatalf("%+v", c)
	}
	// Placeholders are rejected until a real run fills them in.
	if err := c.Validate(); err == nil {
		t.Fatal("the example with placeholders must not validate")
	}
	if c.Images["amd64"]["frontend"].Digest != "sha256:e06d251281a53cbab5a7eed2f4764398040feb4daf4c645b9e2c10a3ad738eff" || c.Images["arm64"]["worker"].Digest != "sha256:eedf635439402e6ab98c8ef0fdba8e2959bda858499a1668e2f4183c04731bc3" {
		t.Fatal("the example lost the supplied immutable image digests")
	}
}

func TestRunOwnedSecretStoreNeedsBridgeReviewerHandlesAndNoSharedVault(t *testing.T) {
	c := goodConfig(t)
	c.SecretStore = &v2run.SecretStoreConfig{Name: "run-vault", RunOwned: true, BridgeAddress: "172.18.0.1", KubernetesHost: "https://172.18.0.2:6443",
		ReviewerJWTFile: "/priv/reviewer.jwt", ReviewerCAFile: "/priv/reviewer.ca"}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*v2run.SecretStoreConfig){
		"loopback bridge": func(s *v2run.SecretStoreConfig) { s.BridgeAddress = "127.0.0.1" },
		"any address":     func(s *v2run.SecretStoreConfig) { s.BridgeAddress = "0.0.0.0" },
		"http host":       func(s *v2run.SecretStoreConfig) { s.KubernetesHost = "http://172.18.0.2:6443" },
		"relative jwt":    func(s *v2run.SecretStoreConfig) { s.ReviewerJWTFile = "reviewer.jwt" },
	} {
		bad := goodConfig(t)
		store := *c.SecretStore
		mutate(&store)
		bad.SecretStore = &store
		if err := bad.Validate(); err == nil {
			t.Fatalf("%s must be refused", name)
		}
	}
	steps := v2run.K8sCleanupSteps("/abs/v2ctl", "/abs/v2cleanup", "/p/m", "/p/h", "/p/c", "/p/s", c)
	for _, s := range steps {
		if s.Name == "vault-config" {
			t.Fatal("a run-owned Vault needs no cleanup step on a shared Vault")
		}
	}
}

func TestProbeImagesMustBePinnedAndAnUnsupportedRedisCABundleIsRefused(t *testing.T) {
	c := awsConfig(t, "A02", "t4g.medium")
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*v2run.Config){
		"unpinned postgres probe": func(c *v2run.Config) { c.AWS.ProbePostgresImage.Digest = "" },
		"latest redis probe":      func(c *v2run.Config) { c.AWS.ProbeRedisImage.Ref = "redis:latest" },
		"bad digest":              func(c *v2run.Config) { c.AWS.ProbeRedisImage.Digest = "sha256:abc" },
		"redis CA bundle":         func(c *v2run.Config) { c.RedisCABundle = "/priv/ca.pem" },
	} {
		bad := awsConfig(t, "A02", "t4g.medium")
		mutate(&bad)
		if err := bad.Validate(); err == nil {
			t.Fatalf("%s must be refused", name)
		}
	}
}
