package terraform

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/execution"
)

func newTestExecutor(t *testing.T) *Executor {
	t.Helper()
	e, err := New(Options{Root: t.TempDir(), Region: "us-east-1", Tags: map[string]string{"run-id": "test"}})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	return e
}

// TestBuildVarsPassesResolvedInputsThrough proves the executor applies exactly
// the variables planning checked against the module contract, plus the ones the
// executor owns.
func TestBuildVarsPassesResolvedInputsThrough(t *testing.T) {
	e := newTestExecutor(t)
	vars, err := e.buildVars(ModuleEKS, t.TempDir(), execution.ProvisionRequest{
		Descriptor: "k8s-cluster.eks#applications.acceptance-cloud",
		Inputs: map[string]any{
			"name":               "acceptance-run",
			"subnet_ids":         []any{"subnet-a", "subnet-b"},
			"kubernetes_version": "1.34",
			"node_capacity_type": "SPOT",
		},
	})
	if err != nil {
		t.Fatalf("build vars: %v", err)
	}
	subnets, ok := vars["subnet_ids"].([]any)
	if !ok || len(subnets) != 2 {
		t.Fatalf("unexpected subnet ids: %#v", vars["subnet_ids"])
	}
	if vars["node_capacity_type"] != "SPOT" || vars["kubernetes_version"] != "1.34" {
		t.Fatalf("unexpected vars: %#v", vars)
	}
	if vars["region"] != "us-east-1" {
		t.Fatalf("the region must come from the executor: %#v", vars["region"])
	}
	if _, ok := vars["master_password"]; ok {
		t.Fatal("only the Aurora module receives a master password")
	}
}

func TestAuroraPasswordIsReusedAcrossDeployments(t *testing.T) {
	e := newTestExecutor(t)
	dir := t.TempDir()
	req := execution.ProvisionRequest{
		Descriptor: "postgres.default#shared.acceptance-db",
		Inputs:     map[string]any{"subnet_ids": []any{"subnet-a", "subnet-b"}, "vpc_id": "vpc-1"},
	}
	first, err := e.buildVars(ModuleAurora, dir, req)
	if err != nil {
		t.Fatalf("build vars: %v", err)
	}
	if err := writeVars(dir, first); err != nil {
		t.Fatalf("write vars: %v", err)
	}
	second, err := e.buildVars(ModuleAurora, dir, req)
	if err != nil {
		t.Fatalf("build vars again: %v", err)
	}
	if first["master_password"] != second["master_password"] {
		t.Fatal("the Aurora master password must stay stable across deployments of one run")
	}
	if password, ok := second["master_password"].(string); !ok || len(password) < 16 {
		t.Fatalf("unexpected password shape: %#v", second["master_password"])
	}
}

func TestVarsFileIsNotWorldReadable(t *testing.T) {
	dir := t.TempDir()
	if err := writeVars(dir, map[string]any{"master_password": "secret"}); err != nil {
		t.Fatalf("write vars: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "terraform.tfvars.json"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("variables file mode is %v", info.Mode().Perm())
	}
}

func TestWorkspaceNameIsFilesystemSafe(t *testing.T) {
	if got := workspaceName("postgres.default#shared.acceptance-db"); got != "postgres-default-shared-acceptance-db" {
		t.Fatalf("unexpected workspace name %q", got)
	}
}

func TestModulesAreEmbedded(t *testing.T) {
	for _, module := range []string{ModuleVPC, ModuleEKS, ModuleAurora} {
		if _, err := fs.Stat(PackagedModules(), module+"/main.tf"); err != nil {
			t.Fatalf("module %s has no main.tf: %v", module, err)
		}
	}
}

// The executor runs only a registry-verified artifact: without one, or with
// a binary other than the verified one, it refuses before any workspace or
// Terraform call.
func TestProvisionRequiresVerifiedArtifact(t *testing.T) {
	e := newTestExecutor(t)
	req := execution.ProvisionRequest{Descriptor: "vpc.default#x", Module: ModuleVPC}
	if _, err := e.Provision(context.Background(), req); err == nil || !strings.Contains(err.Error(), "verified module artifact") {
		t.Fatalf("missing artifact must be refused, got %v", err)
	}
	req.Artifact = &tfmodule.VerifiedArtifact{TerraformBinary: "/other/terraform", ModuleDir: t.TempDir()}
	if _, err := e.Provision(context.Background(), req); err == nil || !strings.Contains(err.Error(), "verified Terraform binary") {
		t.Fatalf("unverified binary must be refused, got %v", err)
	}
	entries, _ := os.ReadDir(e.Root)
	if len(entries) != 0 {
		t.Fatalf("a refused run created workspaces: %v", entries)
	}
}

// writeModule copies exactly the verified module files plus the release lock.
func TestWriteModuleCopiesVerifiedArtifactAndLock(t *testing.T) {
	e := newTestExecutor(t)
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "main.tf"), []byte("# verified\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "ws")
	artifact := tfmodule.VerifiedArtifact{ModuleDir: src, LockFile: []byte("# lock\n")}
	if err := e.writeModule(artifact, dir); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "main.tf")); string(got) != "# verified\n" {
		t.Fatalf("module file not copied: %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, ".terraform.lock.hcl")); string(got) != "# lock\n" {
		t.Fatalf("lock file not written: %q", got)
	}
}

// ADR-011: new AWS Environments derive distinct workspaces (and so state) from
// their Environment descriptors; a legacy Environment keeps the exact
// application-scoped workspace it always used.
func TestWorkspaceNamesSeparateEnvironmentScopedInfrastructureFromLegacy(t *testing.T) {
	names := map[string]string{
		"legacy vpc":     workspaceName("vpc.default#applications.pay"),
		"legacy eks":     workspaceName("k8s-cluster.eks#applications.pay"),
		"staging vpc":    workspaceName("vpc.default#environments.pay.staging"),
		"production vpc": workspaceName("vpc.default#environments.pay.production"),
		"staging eks":    workspaceName("k8s-cluster.eks#environments.pay.staging"),
		"production eks": workspaceName("k8s-cluster.eks#environments.pay.production"),
	}
	if names["legacy vpc"] != "vpc-default-applications-pay" || names["legacy eks"] != "k8s-cluster-eks-applications-pay" {
		t.Fatalf("legacy workspaces changed: %v", names)
	}
	seen := map[string]string{}
	for label, name := range names {
		if other, dup := seen[name]; dup {
			t.Fatalf("%s and %s share workspace %q", label, other, name)
		}
		seen[name] = label
	}
}

func TestWorkspaceForIsolatesTargetGenerations(t *testing.T) {
	d := "postgres.default#shared.acceptance-db"
	if workspaceFor(d, 0) != workspaceName(d) {
		t.Fatal("generation 0 must keep the legacy workspace and state")
	}
	seen := map[string]int64{}
	for g := int64(0); g < 5; g++ {
		w := workspaceFor("vpc.default#environments.pay.staging", g)
		if previous, dup := seen[w]; dup {
			t.Fatalf("generations %d and %d share workspace %q", previous, g, w)
		}
		seen[w] = g
	}
}
