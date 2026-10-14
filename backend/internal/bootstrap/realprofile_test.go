package bootstrap

import (
	"context"

	"orchestrator/internal/application/tfexec"
	"strings"
	"testing"

	"orchestrator/internal/seed"
)

func realOptions() Options {
	o := seed.Defaults()
	o.Profile, o.Catalog = seed.ProfileAcceptanceV2, seed.CatalogNone
	return Options{
		Seed: o, Adapters: AdapterKubernetes, DatabaseURL: "postgres://console", TerraformStateDatabaseURL: "postgres://state",
		TerraformStateKeyFile: "/k", TerraformRecoveryDir: "/r", ScoreK8sPath: "/s", RequireScoreRenderer: true,
		ConnectionCredentialStore: "vault", ConnectionVaultAddress: "http://v", ConnectionVaultTokenFile: "/t",
		OutputSecretStore: "vault", OutputSecretVaultAddress: "http://v", OutputSecretVaultTokenFile: "/o",
	}
}

func TestRealProfileAcceptsOnlyTheFullRealWiring(t *testing.T) {
	if err := validateRealProfile(realOptions()); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Options){
		"fake adapters":       func(o *Options) { o.Adapters = AdapterFake },
		"json state":          func(o *Options) { o.StatePath = "/x" },
		"no state backend":    func(o *Options) { o.TerraformStateKeyFile = "" },
		"no recovery dir":     func(o *Options) { o.TerraformRecoveryDir = "" },
		"native renderer":     func(o *Options) { o.RequireScoreRenderer = false },
		"no score binary":     func(o *Options) { o.ScoreK8sPath = "" },
		"memory credentials":  func(o *Options) { o.ConnectionCredentialStore = "memory" },
		"fleet delivery":      func(o *Options) { o.WorkloadDelivery = "fleet-gitrepo" },
		"memory output store": func(o *Options) { o.OutputSecretStore = "memory" },
		"cleanup override":    func(o *Options) { o.CleanupEngineOverride = stubEngine{} },
	} {
		o := realOptions()
		mutate(&o)
		if err := validateRealProfile(o); err == nil || !strings.Contains(err.Error(), "real acceptance profile refused") {
			t.Errorf("%s: %v", name, err)
		}
	}
	other := realOptions()
	other.Seed.Profile, other.Seed.Catalog, other.Adapters = "local", "", AdapterFake
	if err := validateRealProfile(other); err != nil {
		t.Fatalf("legacy profiles are unaffected: %v", err)
	}
}

type stubEngine struct{}

func (stubEngine) RunCleanup(context.Context, string, tfexec.CleanupSource, tfexec.StageReporter) (tfexec.CleanupResult, error) {
	return tfexec.CleanupResult{}, nil
}
