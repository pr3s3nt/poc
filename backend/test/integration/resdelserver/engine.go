package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/adapters/secrets"
	"orchestrator/internal/adapters/statecipher"
	"orchestrator/internal/adapters/terraform/recovery"
	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/adapters/terraform/runner"
	"orchestrator/internal/application/statebackend"
	"orchestrator/internal/application/tfexec"
	deliveryhttp "orchestrator/internal/delivery/http"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/terraformexec"
)

// The cleanup engine of this launcher is the REAL tfexec service: the real
// encrypted state backend on loopback, the real Terraform CLI runner driving
// the pinned terraform binary, the real encrypted recovery store and the real
// absence verification. What is FAKE, and named so:
//
//   - the module source: one hand-built module whose only resources are the
//     built-in terraform_data (no provider is downloaded, no cloud, no cluster);
//   - the provider binder and the readiness check, which bind nothing and are
//     always ready;
//   - the "physical objects": terraform_data instances in a local state.
//
// A passing run is therefore NOT live proof of V2-A01/A02/A03.

const fixtureHCL = `
variable "note" { type = string }
variable "stable" { type = string }
variable "destroy_fails" {
  type    = bool
  default = false
}
variable "region" { type = string }
variable "tags" { type = map(string) }
variable "master_password" {
  type      = string
  sensitive = true
}
resource "terraform_data" "object" {
  input            = var.destroy_fails
  triggers_replace = var.stable
  # FAKE failure injection: a destroy-time local-exec that fails when asked to.
  provisioner "local-exec" {
    when    = destroy
    command = self.input ? "exit 1" : "true"
  }
}
resource "terraform_data" "pw" {
  input = var.master_password
}
output "host" { value = "obj-${terraform_data.object.id}" }
output "port" { value = 5432 }
output "database" { value = var.note }
output "username" { value = "app" }
output "tls" { value = false }
output "password" {
  value     = var.master_password
  sensitive = true
}
`

var fixtureTypes = []string{"postgres", "vpc", "redis"}

func fixtureType(key string) resource.Type {
	return resource.Type{Key: key, Outputs: []resource.OutputField{
		{Name: "host", Type: "string", Required: true}, {Name: "port", Type: "number", Required: true},
		{Name: "database", Type: "string"}, {Name: "username", Type: "string"},
		{Name: "password", Type: "string", Secret: true, Required: true},
	}}
}

func identityMapping() map[string]string {
	return map[string]string{"host": "host", "port": "port", "database": "database", "username": "username", "password": "password"}
}

// fixtureBundle is the verified bundle identity of the one fixture module.
func fixtureBundle(id string) tfmodule.Bundle {
	return tfmodule.Bundle{
		ID: id, Ref: tfmodule.SourceRef{SourceID: "test", ModuleID: "fixture-object", Revision: "rev1", Subdirectory: "root", ContentSHA256: strings.Repeat("a", 64)},
		TerraformVersion: "1.9.8", TerraformBinarySHA256: strings.Repeat("b", 64), ProviderLockDigest: strings.Repeat("c", 64),
		Contract: tfmodule.Contract{
			SchemaVersion: tfmodule.ContractSchemaVersion, SourceKind: tfmodule.SourceKindPackaged, ResourceTypes: fixtureTypes,
			Inputs: map[string]tfmodule.Input{
				"note": {Type: "string"}, "stable": {Type: "string"}, "destroy_fails": {Type: "bool"}, "region": {Type: "string"},
				"tags": {Type: "map(string)"}, "master_password": {Type: "string", Sensitive: true},
			},
			Outputs: map[string]tfmodule.Output{
				"host": {Type: "string"}, "port": {Type: "number"}, "database": {Type: "string"},
				"username": {Type: "string"}, "tls": {Type: "bool"}, "password": {Type: "string", Sensitive: true},
			},
			ExecutorOwned: []string{"master_password", "region", "tags"},
		},
		Providers: map[string]tfmodule.ProviderRequirement{}, Status: tfmodule.StatusVerified,
	}
}

// fixtureModules is the FAKE module source of the fixture module. It builds a
// fresh private artifact around the real pinned Terraform binary on every call.
type fixtureModules struct {
	cli     string
	workDir string
	bundle  tfmodule.Bundle
	mu      sync.Mutex
}

func (m *fixtureModules) build() (*tfmodule.VerifiedArtifact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	root, err := os.MkdirTemp(m.workDir, "artifact-")
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*tfmodule.VerifiedArtifact, error) { _ = os.RemoveAll(root); return nil, err }
	binary := filepath.Join(root, "bin", "terraform")
	if err := os.MkdirAll(filepath.Dir(binary), 0o700); err != nil {
		return fail(err)
	}
	sum, err := copyExecutable(m.cli, binary)
	if err != nil {
		return fail(err)
	}
	src := filepath.Join(root, "sources", "test@rev1")
	moduleDir := filepath.Join(src, "root")
	if err := os.MkdirAll(moduleDir, 0o700); err != nil {
		return fail(err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "main.tf"), []byte(fixtureHCL), 0o600); err != nil {
		return fail(err)
	}
	digest, err := registry.DigestDir(moduleDir)
	if err != nil {
		return fail(err)
	}
	bundle := m.bundle.Clone()
	bundle.TerraformBinarySHA256, bundle.Ref.ContentSHA256 = sum, digest
	art := &tfmodule.VerifiedArtifact{
		Bundle: bundle, Root: root, SourceDir: root, ModuleDir: moduleDir, TerraformBinary: binary,
		LockFile: []byte("# fixture lock: builtin providers only\n"),
		Cleanup:  func() error { return os.RemoveAll(root) },
		Retain: func(handle string) error {
			return os.WriteFile(filepath.Join(filepath.Dir(root), ".retained-"+handle), []byte(root), 0o600)
		},
	}
	return art, nil
}

func (m *fixtureModules) VerifyForExecution(context.Context, tfmodule.SourceRef) (tfmodule.VerifiedArtifact, error) {
	art, err := m.build()
	if err != nil {
		return tfmodule.VerifiedArtifact{}, err
	}
	return *art, nil
}

func (m *fixtureModules) VerifyHistoricalForExecution(ctx context.Context, bundle tfmodule.Bundle) (tfmodule.VerifiedArtifact, error) {
	return m.VerifyForExecution(ctx, bundle.Ref)
}

func (m *fixtureModules) BundleByID(_ context.Context, id string) (tfmodule.Bundle, error) {
	if id != m.bundle.ID {
		return tfmodule.Bundle{}, os.ErrNotExist
	}
	art, err := m.build()
	if err != nil {
		return tfmodule.Bundle{}, err
	}
	defer art.Close()
	return art.Bundle, nil
}

func copyExecutable(src, dst string) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(out, h), in); err != nil {
		out.Close()
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(dst, 0o500); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// fakeBinder is the FAKE provider binder: it binds nothing.
type fakeBinder struct{}

func (fakeBinder) Bind(context.Context, terraformexec.BindRequest) (terraformexec.ProviderEnv, error) {
	return terraformexec.ProviderEnv{Env: []string{"FIXTURE_PROVIDER=fake"}, AccountID: "123456789012", Region: "us-east-1", Close: func() error { return nil }, Retain: func(string) error { return nil }}, nil
}

// fakeReadiness is the FAKE readiness check: every applied instance is ready.
type fakeReadiness struct{}

func (fakeReadiness) Check(context.Context, terraformexec.ReadinessRequest) error { return nil }

// fakeInspector is the FAKE namespace inspector: the fixture has no namespaces.
type fakeInspector struct{}

func (fakeInspector) Inspect(context.Context, []string, string) (terraformexec.NamespaceInspection, error) {
	return terraformexec.NamespaceInspection{UID: "uid-fixture"}, nil
}

// engine is the assembled executor plus the pieces the launcher closes.
type engine struct {
	*tfexec.Service
	state    *postgres.StateStore
	listener *deliveryhttp.TerraformStateLoopback
	bundleID string
}

func (e *engine) close() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = e.listener.Close(ctx)
	e.state.Close()
}

// buildEngine wires the real executor over the real State login, state backend
// and Terraform runner, with the FAKE module source, binder and readiness.
func buildEngine(ctx context.Context, states *statebackend.Service, handler *deliveryhttp.TerraformStateHandler, stateURL, keyFile, keyID, terraformPath, workDir, bundleID string) (*engine, error) {
	if states == nil || handler == nil {
		return nil, fmt.Errorf("the Terraform state backend is not configured")
	}
	state, err := postgres.OpenState(ctx, stateURL)
	if err != nil {
		return nil, fmt.Errorf("open the State login: %w", err)
	}
	cipher, err := statecipher.LoadKeyFile(keyID, keyFile)
	if err != nil {
		state.Close()
		return nil, fmt.Errorf("load the state key: %w", err)
	}
	recoveryStore, err := recovery.New(filepath.Join(workDir, "recovery"), cipher)
	if err != nil {
		state.Close()
		return nil, err
	}
	listener, err := deliveryhttp.ServeTerraformStateLoopback(handler)
	if err != nil {
		state.Close()
		return nil, err
	}
	svc, err := tfexec.New(tfexec.Config{BackendBase: listener.Address, Tags: map[string]string{"platform": "fixture"}, InvocationTimeout: 3 * time.Minute, ReadinessTimeout: time.Minute, RetryBackoff: time.Second, AbsencePolls: 2, AbsenceInterval: 200 * time.Millisecond},
		tfexec.Deps{
			State: state, Tokens: states, Modules: &fixtureModules{cli: terraformPath, workDir: workDir, bundle: fixtureBundle(bundleID)},
			Runner: runner.New(runner.Options{Grace: time.Second, StopConfirm: 10 * time.Second}), Binder: fakeBinder{}, Readiness: fakeReadiness{}, Namespaces: fakeInspector{},
			Recovery: recoveryStore, Verification: recoveryStore, Secrets: secrets.NewMemory(),
		})
	if err != nil {
		_ = listener.Close(ctx)
		state.Close()
		return nil, err
	}
	return &engine{Service: svc, state: state, listener: listener, bundleID: bundleID}, nil
}
