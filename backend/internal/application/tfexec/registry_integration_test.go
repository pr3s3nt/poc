package tfexec_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/adapters/terraform/runner/runnertest"
	"orchestrator/internal/application/tfexec"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/ports/moduleregistry"
)

// REAL module registry (private verified copies, transitive call tree,
// readonly provider lock) + REAL PostgreSQL + REAL HTTP state backend + the
// ACTUAL Terraform CLI, with an OUTPUT-ONLY module: no resources, providers,
// cluster or cloud. Binder and readiness remain fakes.

const outputOnlyRoot = `
variable "word" { type = string }
module "inner" {
  source = "./child"
  word   = var.word
}
output "host" { value = module.inner.greeting }
output "port" { value = 5432 }
output "password" {
  value     = "generated-${var.word}"
  sensitive = true
}
`

const outputOnlyChild = `
variable "word" { type = string }
output "greeting" { value = "hello-${var.word}" }
`

func treeDigest(files map[string]string) string {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	for i := 1; i < len(paths); i++ {
		for j := i; j > 0 && paths[j] < paths[j-1]; j-- {
			paths[j], paths[j-1] = paths[j-1], paths[j]
		}
	}
	h := sha256.New()
	for _, p := range paths {
		sum := sha256.Sum256([]byte(files[p]))
		h.Write([]byte(p))
		h.Write([]byte{0})
		h.Write([]byte(hex.EncodeToString(sum[:])))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// buildRegistry prepares a registry for one platform release over a shared
// private cache: revision names the packaged source revision, cli the Terraform
// binary the release pins, retained the trusted historical releases.
func buildRegistry(t *testing.T, h *harness, root, cacheDir, revision, cli string, retained []registry.RetainedRelease) (*registry.Service, registry.Release, tfmodule.SourceRef) {
	t.Helper()
	files := map[string]string{"main.tf": root, "child/main.tf": outputOnlyChild}
	release, err := registry.DefaultRelease()
	if err != nil {
		t.Fatal(err)
	}
	sum, err := fileDigest(cli)
	if err != nil {
		t.Fatal(err)
	}
	release.Terraform.BinarySHA256 = sum
	release.Providers = nil
	release.ExecutorVariables = nil
	release.Packaged = registry.PackagedSource{SourceID: "platform", Revision: revision, Modules: []registry.PackagedModule{
		{ID: "outputs", Subdirectory: "outputs", ContentSHA256: treeDigest(files), ResourceTypes: []string{"postgres"}},
	}}
	packaged := fstest.MapFS{"outputs/main.tf": {Data: []byte(files["main.tf"])}, "outputs/child/main.tf": {Data: []byte(files["child/main.tf"])}}
	cache, err := registry.OpenCache(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := registry.New(registry.Config{Release: release, Packaged: packaged, Cache: cache, Store: h.store, TerraformBinary: cli, Retained: retained})
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Prepare(context.Background()); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	return reg, release, release.Ref(release.Packaged.Modules[0])
}

// registryHarness builds the harness with the real registry as module source.
func registryHarness(t *testing.T, root string) (*harness, *registry.Service, tfmodule.SourceRef, string) {
	t.Helper()
	h := newHarness(t)
	cacheDir := t.TempDir()
	if err := os.Chmod(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	reg, _, ref := buildRegistry(t, h, root, cacheDir, "v1-test", h.cli, nil)
	return h, reg, ref, cacheDir
}

func fileDigest(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func registryService(t *testing.T, h *harness, reg *registry.Service) *tfexec.Service {
	t.Helper()
	svc, err := tfexec.New(tfexec.Config{BackendBase: h.base, InvocationTimeout: 2 * time.Minute, ReadinessTimeout: time.Minute, RetryBackoff: time.Millisecond},
		tfexec.Deps{State: h.state, Tokens: h.backend, Modules: reg, Runner: h.runner, Binder: h.binder, Readiness: h.ready, Recovery: h.recover, Verification: h.recover, Secrets: h.secrets})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func outputType() resource.Type {
	return resource.Type{Key: "postgres", Outputs: []resource.OutputField{
		{Name: "host", Type: "string", Required: true}, {Name: "port", Type: "number", Required: true}, {Name: "password", Type: "string", Secret: true, Required: true},
	}}
}

func registryRequest(h *harness, ref tfmodule.SourceRef, run string) tfexec.ExecuteRequest {
	return tfexec.ExecuteRequest{
		RunID: run, Descriptor: defaultDescriptor, Scope: defaultScope(), Identity: h.identity(defaultDescriptor, defaultScope()),
		Source: ref, Type: outputType(), Inputs: map[string]any{"word": "world"}, OutputMapping: map[string]string{"host": "host", "port": "port", "password": "password"}, OrganizationKey: h.orgKey,
	}
}

// The registry installs the verified transitive call tree without any
// download, writes the pinned readonly lock, and the runtime outputs are
// checked against the module and Type contracts.
func TestRegistryArtifactRunsOutputOnlyModuleWithTransitiveCallTree(t *testing.T) {
	h, reg, ref, _ := registryHarness(t, outputOnlyRoot)
	svc := registryService(t, h, reg)
	ctx, op := h.claim("proc-a")
	res, err := svc.Execute(ctx, registryRequest(h, ref, "run-1"))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.Outputs["host"] != "hello-world" || res.Outputs["port"] != float64(5432) || !res.Sensitive["password"] || res.Sensitive["host"] {
		t.Fatalf("outputs %+v sensitive %+v", res.Outputs, res.Sensitive)
	}
	inst, _ := h.instance(ctx, defaultDescriptor, defaultScope())
	attempts := h.attempts(inst.ID)
	if inst.Status != tfresource.InstanceReady || attempts[0].Status != tfresource.AttemptSucceeded {
		t.Fatalf("instance %s attempts %+v", inst.Status, attempts)
	}
	// The module's call tree is in the immutable bundle contract.
	bundle, err := reg.BundleByID(context.Background(), inst.ModuleBundleID)
	if err != nil || len(bundle.Contract.Transitive) != 1 || bundle.Contract.ModuleCalls[0].Key != "inner" {
		t.Fatalf("bundle %+v: %v", bundle.Contract.ModuleCalls, err)
	}
	h.release(op, environment.OpSucceeded)
}

// Drift of the verified transitive content in the shared cache stops before
// any effect: no instance state, no attempt, no Terraform process.
func TestTransitivePinDriftStopsBeforeProviderExecution(t *testing.T) {
	h, reg, ref, cacheDir := registryHarness(t, outputOnlyRoot)
	svc := registryService(t, h, reg)
	var childFile string
	_ = filepath.WalkDir(cacheDir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, filepath.Join("outputs", "child", "main.tf")) {
			childFile = p
		}
		return nil
	})
	if childFile == "" {
		t.Fatal("transitive module not cached")
	}
	if err := os.WriteFile(childFile, []byte(`output "greeting" { value = "tampered" }`), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, _ := h.claim("proc-a")
	opens := h.runner.opens
	_, err := svc.Execute(ctx, registryRequest(h, ref, "run-1"))
	if !errors.Is(err, moduleregistry.ErrDrift) {
		t.Fatalf("drifted transitive module: %v", err)
	}
	if h.runner.opens != opens || h.count(`SELECT count(*) FROM terraform_resource_instances`) != 0 || h.count(`SELECT count(*) FROM terraform_resource_attempts`) != 0 {
		t.Fatal("drift reached state or Terraform")
	}
}

// A deferred static output type (no literal to derive it from) is checked at
// runtime: a string where the Type contract says number never publishes.
func TestDeferredOutputTypeIsCheckedAtRuntime(t *testing.T) {
	root := strings.Replace(outputOnlyRoot, `output "port" { value = 5432 }`, `output "port" { value = var.word }`, 1)
	h, reg, ref, _ := registryHarness(t, root)
	svc := registryService(t, h, reg)
	ctx, _ := h.claim("proc-a")
	_, err := svc.Execute(ctx, registryRequest(h, ref, "run-1"))
	if !errors.Is(err, tfexec.ErrOutputContract) {
		t.Fatalf("deferred type mismatch: %v", err)
	}
	if len(h.ready.calls) != 0 {
		t.Fatal("readiness ran after an output contract failure")
	}
}

// Cleanup after a platform release upgrade runs the ORIGINAL bundle under the
// retained trusted release it was created with (binary, version, lock), never
// the current release, and never downloads or re-matches.
func TestDestroyUsesRetainedHistoricalReleaseNeverTheCurrentOne(t *testing.T) {
	h, regA, refA, cacheDir := registryHarness(t, outputOnlyRoot)
	svcA := registryService(t, h, regA)
	ctx, op := h.claim("proc-a")
	created, err := svcA.Execute(ctx, registryRequest(h, refA, "run-1"))
	if err != nil {
		t.Fatalf("create under release A: %v", err)
	}
	h.release(op, environment.OpSucceeded)

	// Release B pins a different Terraform binary; its stand-in records any run.
	ran := filepath.Join(t.TempDir(), "release-b-ran")
	cliB := runnertest.Script(t, "echo ran > "+ran+"\nexit 1\n")
	releaseA, err := registry.DefaultRelease()
	if err != nil {
		t.Fatal(err)
	}
	sumA, _ := fileDigest(h.cli)
	releaseA.Terraform.BinarySHA256, releaseA.Providers, releaseA.ExecutorVariables = sumA, nil, nil
	releaseA.Packaged = regA.Release().Packaged

	regB, _, _ := buildRegistry(t, h, outputOnlyRoot, cacheDir, "v2-test", cliB, []registry.RetainedRelease{{Release: releaseA, TerraformBinary: h.cli}})
	svcB := registryService(t, h, regB)
	ctx2, op2 := h.claim("proc-clean")
	out, err := svcB.Destroy(ctx2, tfexec.DestroyRequest{InstanceID: created.InstanceID, OrganizationKey: h.orgKey, RunID: "cleanup-" + op2.ID})
	if err != nil || !out.Destroyed {
		t.Fatalf("destroy under release B with A retained: %+v %v", out, err)
	}
	if _, statErr := os.Stat(ran); !os.IsNotExist(statErr) {
		t.Fatal("the current release's binary ran during a historical destroy")
	}
	h.release(op2, environment.OpSucceeded)

	// Without the retained release the historical bundle fails closed and
	// reports what is still there (nothing is re-matched or upgraded).
	h2, regA2, refA2, cacheDir2 := registryHarness(t, outputOnlyRoot)
	ctxA2, opA2 := h2.claim("proc-a")
	created2, err := registryService(t, h2, regA2).Execute(ctxA2, registryRequest(h2, refA2, "run-1"))
	if err != nil {
		t.Fatal(err)
	}
	h2.release(opA2, environment.OpSucceeded)
	regNoRetain, _, _ := buildRegistry(t, h2, outputOnlyRoot, cacheDir2, "v2-test", cliB, nil)
	ctx3, op3 := h2.claim("proc-clean")
	out2, err := registryService(t, h2, regNoRetain).Destroy(ctx3, tfexec.DestroyRequest{InstanceID: created2.InstanceID, OrganizationKey: h2.orgKey, RunID: "cleanup-" + op3.ID})
	if !errors.Is(err, moduleregistry.ErrReleaseMismatch) || out2.Destroyed || (len(out2.Remaining) == 0 && !out2.RemainingUnknown) {
		t.Fatalf("destroy without a retained release: %+v %v", out2, err)
	}
	if h2.status(created2.InstanceID) != tfresource.InstanceReady {
		t.Fatal("a refused historical destroy changed the instance")
	}
	if _, statErr := os.Stat(ran); !os.IsNotExist(statErr) {
		t.Fatal("release B ran")
	}
}
