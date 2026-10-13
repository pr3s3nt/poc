package bootstrap

import (
	"context"
	"errors"
	"orchestrator/internal/seed/seedtest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/internal/adapters/postgres/postgrestest"
	tfregistry "orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/ports/moduleregistry"
)

// Real adapters with the durable state backend wire the Terraform v2
// executor in BOTH modes (in-cluster Kubernetes as well as AWS), resolving the
// pinned Terraform binary in both; they refuse to start without a private
// recovery directory, and AWS refuses to start without the state backend.
func TestTerraformExecutorWiringInKubernetesAndAWSModes(t *testing.T) {
	ctx := context.Background()
	host, err := exec.LookPath("terraform")
	if err != nil {
		t.Skip("terraform CLI not found")
	}
	db := postgrestest.Fresh(t)
	key := writeKey(t)
	recovery := filepath.Join(t.TempDir(), "recovery")
	base := func(mode AdapterMode) Options {
		return Options{Seed: seedtest.Defaults(), DatabaseURL: db.Console, TerraformStateDatabaseURL: db.State, TerraformStateKeyFile: key, TerraformStateKeyID: "key-1",
			Adapters: mode, KubectlPath: "kubectl", TerraformPath: host, TerraformRecoveryDir: recovery, Region: "us-east-1"}
	}
	for _, mode := range []AdapterMode{AdapterKubernetes, AdapterAWS} {
		app, err := Build(ctx, base(mode))
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		// The pinned Terraform binary is resolved: execution verification
		// returns a private copy, never the host path.
		refs, err := tfregistry.PackagedRefs()
		if err != nil {
			t.Fatal(err)
		}
		artifact, err := app.Modules.VerifyForExecution(ctx, refs["vpc"])
		if err != nil || artifact.TerraformBinary == host || !strings.HasSuffix(artifact.TerraformBinary, "bin/terraform") {
			t.Fatalf("%s: verified artifact %q: %v", mode, artifact.TerraformBinary, err)
		}
		_ = artifact.Close()
		app.Close()
	}

	for label, change := range map[string]func(*Options){
		"no recovery directory": func(o *Options) { o.TerraformRecoveryDir = "" },
		"relative recovery dir": func(o *Options) { o.TerraformRecoveryDir = "recovery" },
	} {
		opts := base(AdapterKubernetes)
		change(&opts)
		if app, err := Build(ctx, opts); err == nil {
			app.Close()
			t.Errorf("%s: built", label)
		}
	}
	noBackend := Options{Seed: seedtest.Defaults(), DatabaseURL: db.Console, Adapters: AdapterAWS, TerraformPath: host, Region: "us-east-1"}
	if app, err := Build(ctx, noBackend); err == nil || !strings.Contains(err.Error(), "state backend") {
		if app != nil {
			app.Close()
		}
		t.Fatalf("AWS adapters without the state backend: %v", err)
	}
	// Kubernetes mode without the backend runs no Terraform (and needs no binary).
	k8s := Options{Seed: seedtest.Defaults(), DatabaseURL: db.Console, Adapters: AdapterKubernetes, KubectlPath: "kubectl", TerraformPath: "/nonexistent/terraform"}
	app, err := Build(ctx, k8s)
	if err != nil {
		t.Fatalf("kubernetes mode without state backend: %v", err)
	}
	refs, err := tfregistry.PackagedRefs()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Modules.VerifyForExecution(ctx, refs["vpc"]); !errors.Is(err, moduleregistry.ErrBinaryUnverified) {
		t.Fatalf("no Terraform binary must fail closed: %v", err)
	}
	app.Close()
}
