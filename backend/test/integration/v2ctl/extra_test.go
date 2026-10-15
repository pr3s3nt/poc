package main_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/persistence/persistencetest"
	"orchestrator/test/integration/v2run"
)

func TestKeygenWritesAPrivateKeyOnceAndNeverOverwrites(t *testing.T) {
	e := newEnv(t, "a01-0001")
	path := filepath.Join(e.dir, "state.key")
	if out, code := e.ctl("keygen", "-out", path); code != 0 {
		t.Fatal(out)
	}
	info, _ := os.Stat(path)
	raw, _ := os.ReadFile(path)
	if info.Mode().Perm() != 0o600 || len(strings.TrimSpace(string(raw))) != 64 {
		t.Fatalf("mode %v len %d", info.Mode(), len(raw))
	}
	if _, code := e.ctl("keygen", "-out", path); code == 0 {
		t.Fatal("an existing key was overwritten")
	}
}

func TestConfigGetPrintsOnlyNamedNonSecretFields(t *testing.T) {
	e := newEnv(t, "a01-0001")
	for field, want := range map[string]string{"runId": "a01-0001", "scenario": "A01", "target": "k8s-4f", "namespace": "orch-v2-test-a01-0001", "executionSeconds": "1800", "cleanupSeconds": "600", "kubeContext": "kind-k8s-4f", "imageBackend": "r/b:m04@sha256:" + strings.Repeat("b", 64)} {
		out, code := e.ctl("config-get", "-config", e.cfgPath, "-field", field)
		if code != 0 || strings.TrimSpace(out) != want {
			t.Errorf("%s = %q (%d), want %q", field, out, code, want)
		}
	}
	if _, code := e.ctl("config-get", "-config", e.cfgPath, "-field", "postgresAdminUrl"); code == 0 {
		t.Error("an unknown field was printed")
	}
}

func TestAWSConfigGetExposesPinnedKubectlWithoutKubeconfig(t *testing.T) {
	e := newEnv(t, "a01-0001")
	c := e.cfg
	c.Scenario, c.Target, c.Kube, c.NodeArch = "A03", v2run.TargetAWS, nil, "arm64"
	c.Execution = v2run.Duration{Duration: 180 * time.Minute}
	c.Images["arm64"] = c.Images["amd64"]
	c.Binaries["aws"] = v2run.Binary{Path: "/opt/pinned/aws", SHA256: strings.Repeat("c", 64)}
	c.Binaries["docker"] = v2run.Binary{Path: "/opt/pinned/docker", SHA256: strings.Repeat("d", 64)}
	c.AWS = &v2run.AWSConfig{
		AccountID: v2run.AWSAccountID, Region: v2run.AWSRegion, PrincipalARN: "arn:aws:iam::452025861381:user/codex",
		ExecutorEgress: "203.0.113.9/32", NodeInstanceType: "t4g.medium", AccessKeyFile: "/private/aws-key.json",
		EstimateFile: "/private/estimate.json", ECRRepositoryPrefix: "orch-acceptance",
		ProbePostgresImage: v2run.Image{Ref: "postgres:16-alpine", Digest: "sha256:" + strings.Repeat("e", 64)},
		ProbeRedisImage:    v2run.Image{Ref: "redis:7.4-alpine", Digest: "sha256:" + strings.Repeat("f", 64)},
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("AWS fixture must be valid without kubeconfig: %v", err)
	}
	path := filepath.Join(e.dir, "aws-config.json")
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := v2run.WriteFilePrivate(path, raw); err != nil {
		t.Fatal(err)
	}
	if out, code := e.ctl("config-get", "-config", path, "-field", "kubectl"); code != 0 || strings.TrimSpace(out) != c.Binaries["kubectl"].Path {
		t.Fatalf("AWS kubectl path = %q (%d), want pinned path %q", out, code, c.Binaries["kubectl"].Path)
	}
	for _, field := range []string{"kubeconfig", "kubeContext", "namespace", "sentinelName"} {
		if out, code := e.ctl("config-get", "-config", path, "-field", field); code == 0 {
			t.Errorf("AWS config exposed unavailable Kubernetes field %s: %q", field, out)
		}
	}
}

func TestModulePinsListTheThreeKubernetesModules(t *testing.T) {
	e := newEnv(t, "a01-0001")
	out, code := e.ctl("module-pins")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if code != 0 || len(lines) != 3 {
		t.Fatalf("%d %q", code, out)
	}
	for _, line := range lines {
		if f := strings.Split(line, "\t"); len(f) != 3 || len(f[2]) != 64 {
			t.Fatalf("pin line %q", line)
		}
	}
}

func TestManifestNamespaceIsDerivedWithTheProductFunctionAndRefusesOthers(t *testing.T) {
	e := newEnv(t, "a01-0001")
	// A fresh manifest without the namespace.
	m := v2run.NewManifest(e.cfg, time.Now())
	path := filepath.Join(e.private, "manifest-2.json")
	if err := v2run.SaveManifest(path, m); err != nil {
		t.Fatal(err)
	}
	out, code := e.ctl("manifest-namespace", "-manifest", path, "-identity", "orch-v2-test-a01-0001", "-generation", "2")
	want := environment.NamespaceFor("orch-v2-test-a01-0001", 2)
	if code != 0 || strings.TrimSpace(out) != want {
		t.Fatalf("%q (%d) want %q", out, code, want)
	}
	if _, code := e.ctl("manifest-namespace", "-manifest", path, "-identity", "other", "-generation", "0"); code == 0 {
		t.Fatal("a foreign identity was recorded")
	}
	if _, code := e.ctl("manifest-namespace", "-manifest", path, "-identity", "orch-v2-test-a01-0001", "-generation", "2", "-observed", "orch-v2-test-a01-0001"); code == 0 {
		t.Fatal("a guessed observed name was accepted")
	}
}

func TestAssertionCommandAppendsAndKeepsOrder(t *testing.T) {
	e := newEnv(t, "a01-0001")
	file := filepath.Join(e.dir, "assertions.json")
	for _, id := range []string{"a", "b"} {
		if out, code := e.ctl("assertion", "-file", file, "-id", id, "-phase", "CLEANUP", "-pass=true", "-detail", "x"); code != 0 {
			t.Fatal(out)
		}
	}
	var list []v2run.Assertion
	raw, _ := os.ReadFile(file)
	if err := json.Unmarshal(raw, &list); err != nil || len(list) != 2 || list[0].ID != "a" || list[1].Source != "private-read" {
		t.Fatalf("%v %v", list, err)
	}
}

// Real PostgreSQL: pg-provision creates one database and three separate
// logins; the product setup step can run on it; pg-drop removes exactly those.
func TestPostgresProvisionAndDropOnARealServer(t *testing.T) {
	base := persistencetest.PostgresTestURL(t)
	e := newEnv(t, "a01-0001")
	admin := filepath.Join(e.dir, "admin.url")
	if err := v2run.WriteFilePrivate(admin, []byte(base)); err != nil {
		t.Fatal(err)
	}
	if out, code := e.ctl("pg-provision", "-admin-url-file", admin, "-out-dir", e.private); code != 0 {
		t.Fatal(out)
	}
	manifest := filepath.Join(e.private, "pg-manifest.json")
	roles := map[string]string{}
	for _, kind := range []string{"owner", "console", "state"} {
		out, code := e.ctl("pg-role", "-manifest", manifest, "-kind", kind)
		if code != 0 || !strings.HasPrefix(strings.TrimSpace(out), "i0410_") {
			t.Fatalf("%s role %q", kind, out)
		}
		roles[kind] = strings.TrimSpace(out)
	}
	if roles["owner"] == roles["console"] || roles["console"] == roles["state"] {
		t.Fatal("the three logins must be separate roles")
	}
	for _, f := range []string{"owner.url", "console.url", "state.url", "pg-manifest.json"} {
		if info, err := os.Stat(filepath.Join(e.private, f)); err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("%s: %v %v", f, info, err)
		}
	}
	ownerURL, _ := os.ReadFile(filepath.Join(e.private, "owner.url"))
	consoleURL, _ := os.ReadFile(filepath.Join(e.private, "console.url"))
	stateURL, _ := os.ReadFile(filepath.Join(e.private, "state.url"))
	if string(ownerURL) == string(consoleURL) || string(consoleURL) == string(stateURL) {
		t.Fatal("the logins share a URL")
	}
	ctx := context.Background()
	if _, err := postgres.Setup(ctx, string(ownerURL), postgres.Roles{Console: roles["console"], State: roles["state"]}); err != nil {
		t.Fatalf("the product owner setup step must work on the provisioned database: %v", err)
	}
	// A refusing drop for a manifest naming a role that was not provisioned here.
	bad := filepath.Join(e.dir, "bad.json")
	_ = v2run.WriteFilePrivate(bad, []byte(`{"database":"i0410_x","roles":["postgres"]}`))
	if _, code := e.ctl("pg-drop", "-admin-url-file", admin, "-manifest", bad); code == 0 {
		t.Fatal("a manifest with a foreign role was dropped")
	}
	if out, code := e.ctl("pg-drop", "-admin-url-file", admin, "-manifest", manifest); code != 0 {
		t.Fatal(out)
	}
	conn, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var n int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM pg_roles WHERE rolname = ANY($1)`, []string{roles["owner"], roles["console"], roles["state"]}).Scan(&n); err != nil || n != 0 {
		t.Fatalf("roles left: %d %v", n, err)
	}
}
