package terraform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

// TestInspectorReadsTheEmbeddedModules is the planning-time Terraform contract
// (UC-06 BR-09) read from the real module sources.
func TestInspectorReadsTheEmbeddedModules(t *testing.T) {
	inspector := NewInspector()
	for module, wantOutputs := range map[string][]string{
		"vpc":    {"cidr", "id", "subnetIds"},
		"eks":    {"endpoint", "name"},
		"aurora": {"database", "host", "password", "port", "username"},
	} {
		contract, err := inspector.Inspect(module)
		if err != nil {
			t.Fatalf("inspect %s: %v", module, err)
		}
		if !strings.HasPrefix(contract.Fingerprint, "sha256:") {
			t.Fatalf("module %s has no fingerprint", module)
		}
		for _, output := range wantOutputs {
			found := false
			for _, declared := range contract.Outputs {
				if declared == output {
					found = true
				}
			}
			if !found {
				t.Fatalf("module %s does not declare output %q: %v", module, output, contract.Outputs)
			}
		}
		if _, ok := contract.Variables["region"]; !ok {
			t.Fatalf("module %s does not declare the region variable", module)
		}
	}
	if _, err := inspector.Inspect("not-a-module"); err == nil {
		t.Fatal("expected an error for an unknown module")
	}
}

func TestInspectorFingerprintIsStable(t *testing.T) {
	first, err := NewInspector().Inspect("vpc")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	second, err := NewInspector().Inspect("vpc")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if first.Fingerprint != second.Fingerprint {
		t.Fatal("the module fingerprint must be deterministic")
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
	e := newTestExecutor(t)
	for _, module := range []string{ModuleVPC, ModuleEKS, ModuleAurora} {
		dir := filepath.Join(t.TempDir(), module)
		if err := e.writeModule(module, dir); err != nil {
			t.Fatalf("write module %s: %v", module, err)
		}
		if _, err := os.Stat(filepath.Join(dir, "main.tf")); err != nil {
			t.Fatalf("module %s has no main.tf: %v", module, err)
		}
	}
}
