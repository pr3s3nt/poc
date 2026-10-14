package bootstrap

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/internal/adapters/postgres/postgrestest"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

// Real adapters with the durable SQL Terraform state backend persist outputs
// and password references: the in-process output-secret store would lose them on
// a restart while the state survives, whatever profile was selected.
func TestMemoryOutputSecretsAreRefusedWithRealAdaptersAndDurableState(t *testing.T) {
	for _, mode := range []AdapterMode{AdapterKubernetes, AdapterAWS} {
		if _, err := outputSecretStore(Options{Adapters: mode, Seed: seedtest.Defaults()}, true); err == nil || !strings.Contains(err.Error(), "durable Vault output-secret store") {
			t.Fatalf("%s with the durable state backend must refuse the memory store: %v", mode, err)
		}
		if _, err := outputSecretStore(Options{Adapters: mode, OutputSecretStore: "memory", Seed: seedtest.Defaults()}, true); err == nil {
			t.Fatalf("%s: an explicit memory store is refused too", mode)
		}
		// An explicitly labelled fixture keeps it.
		if _, err := outputSecretStore(Options{Adapters: mode, Seed: seedtest.Defaults(), EphemeralOutputSecrets: true}, true); err != nil {
			t.Fatalf("%s fixture opt-in: %v", mode, err)
		}
		// Without the durable state backend nothing outlives the process: legacy behavior is kept.
		if _, err := outputSecretStore(Options{Adapters: mode, Seed: seedtest.Defaults()}, false); err != nil {
			t.Fatalf("%s without durable state: %v", mode, err)
		}
	}
	// Fake adapters (legacy fixtures) keep the in-process store even with a state backend.
	if _, err := outputSecretStore(Options{Adapters: AdapterFake, Seed: seedtest.Defaults()}, true); err != nil {
		t.Fatalf("fake adapters: %v", err)
	}
	// The acceptance-v2 profile refuses it everywhere, opt-in or not.
	v2 := seedtest.Defaults()
	v2.Profile = seed.ProfileAcceptanceV2
	if _, err := outputSecretStore(Options{Adapters: AdapterKubernetes, Seed: v2, EphemeralOutputSecrets: true}, true); err == nil {
		t.Fatal("the real acceptance profile must refuse the memory store even with the fixture flag")
	}
}

// The guard is wired into Build: starting (and therefore reopening) the product
// with real adapters, the durable state backend and no durable store fails.
func TestBuildRefusesMemoryOutputSecretsWithRealAdaptersAndDurableState(t *testing.T) {
	host, err := exec.LookPath("terraform")
	if err != nil {
		t.Fatalf("terraform CLI is required: %v", err)
	}
	db := postgrestest.Fresh(t)
	opts := Options{Seed: seedtest.Defaults(), DatabaseURL: db.Console, TerraformStateDatabaseURL: db.State, TerraformStateKeyFile: writeKey(t), TerraformStateKeyID: "key-1",
		Adapters: AdapterKubernetes, KubectlPath: "kubectl", TerraformPath: host, TerraformRecoveryDir: filepath.Join(t.TempDir(), "recovery")}
	if app, err := Build(context.Background(), opts); err == nil || !strings.Contains(err.Error(), "durable Vault output-secret store") {
		if app != nil {
			app.Close()
		}
		t.Fatalf("Build must refuse the memory store: %v", err)
	}
	opts.EphemeralOutputSecrets = true
	app, err := Build(context.Background(), opts)
	if err != nil {
		t.Fatalf("the labelled fixture must build: %v", err)
	}
	app.Close()
}
