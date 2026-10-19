// Command v2cleanup is the independent safety net of the real Terraform v2
// acceptance runner. The cleanup supervisor starts it (its own session and
// process group) after the runner, the browser or the product server died or
// the execution expired. It rebuilds the SAME real product wiring from private
// handles (scoped Console/State logins, state key, Vault tokens, pinned
// binaries) and never touches Kubernetes or cloud objects with anything but
// the product's own code:
//
//	v2cleanup delivery  -handles H -manifest M   remove the delivered Workloads (the product's own Deployer.Remove)
//	v2cleanup terraform -handles H   reverse-destroy every Terraform instance through the original pinned engine (RUN_CLEANUP)
//	v2cleanup status    -handles H   exit 0 only when every instance of the Environment is DESTROYED
//
// It never deletes a namespace to skip children, never unlocks or takes over a
// held Environment claim, never resumes an unknown outcome and never drops
// state, claims or recovery artifacts. Exit: 0 done, 3 needs attention (held,
// failed or blocked; evidence kept), 4 Environment busy, 2 usage.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"orchestrator/internal/application/runcleanup"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
	"orchestrator/test/integration/v2run"
)

const (
	exitDone      = 0
	exitAttention = 3
	exitBusy      = 4
	exitUsage     = 2
)

// options rebuilds the product options of the real acceptance profile.
func options(h v2run.CleanupHandles) (bootstrap.Options, error) {
	databaseURL, err := bootstrap.ReadCredentialFile("database URL", h.DatabaseURLFile)
	if err != nil {
		return bootstrap.Options{}, err
	}
	stateURL, err := bootstrap.ReadCredentialFile("State database URL", h.StateDatabaseURLFile)
	if err != nil {
		return bootstrap.Options{}, err
	}
	seedOpts := seed.Defaults()
	seedOpts.Profile, seedOpts.Catalog = seed.ProfileAcceptanceV2, seed.CatalogNone
	seedOpts.OrganizationKey, seedOpts.ApplicationKey, seedOpts.EnvironmentKey = h.OrganizationKey, h.ApplicationKey, h.EnvironmentKey
	seedOpts.NamespaceIdentity, seedOpts.RunID = h.NamespaceIdentity, h.RunID
	// ADR-017: no process-wide account or region exists. The safety net rebuilds exactly the wiring of the product (which seeds no
	// profile-bound cloud Application) and reaches every AWS node through its ORIGINAL pinned account and region.
	return bootstrap.Options{
		Seed: seedOpts, Adapters: adapterMode(h), DatabaseURL: databaseURL, TerraformStateDatabaseURL: stateURL,
		KubectlPath: h.Kubectl, TerraformPath: h.Terraform, ScoreK8sPath: h.ScoreK8s, RequireScoreRenderer: true, GitPath: h.Git,
		TerraformRoot: h.TerraformRoot, TerraformPluginCache: h.PluginCacheDir, TerraformModuleCacheDir: h.ModuleCacheDir,
		TerraformRecoveryDir: h.RecoveryDir, TerraformStateKeyFile: h.StateKeyFile, TerraformStateKeyID: h.StateKeyID,
		ConnectionCredentialStore: "vault", ConnectionVaultAddress: h.VaultAddress, ConnectionVaultTokenFile: h.ConnectionTokenFile, ConnectionVaultMount: h.VaultMount,
		OutputSecretStore: "vault", OutputSecretVaultAddress: h.VaultAddress, OutputSecretVaultTokenFile: h.OutputTokenFile, OutputSecretVaultMount: h.VaultMount,
		WorkloadDelivery: "direct", VaultDelivery: vaultDelivery(h),
		TerraformInvocationTimeout: h.InvocationTimeout.Duration, TerraformReadinessTimeout: h.ReadinessTimeout.Duration,
		Tags: map[string]string{"project": "orchestrator", "owner": h.Owner, "environment": v2run.ProductEnvironmentTag, "run-id": h.RunID, "managed-by": v2run.ProductManagedByTag,
			"expires-at": time.Now().UTC().Add(time.Hour).Format(time.RFC3339)},
	}, nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: v2cleanup delivery|terraform|status -handles FILE")
		os.Exit(exitUsage)
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	handlesFile := fs.String("handles", "", "private cleanup handles (0600)")
	manifestFile := fs.String("manifest", "", "private run manifest: the trusted record of the run's Terraform run IDs and target generation")
	descriptorsFile := fs.String("selected-descriptors", "", "optional private exact failed-attempt probe descriptors; narrows recorded-run cleanup")
	_ = fs.Parse(os.Args[2:])
	handles, err := v2run.LoadCleanupHandles(*handlesFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "v2cleanup:", err)
		os.Exit(exitUsage)
	}
	opts, err := options(handles)
	if err != nil {
		fmt.Fprintln(os.Stderr, "v2cleanup:", err)
		os.Exit(exitUsage)
	}
	if err := bootstrap.ValidateRealProfile(opts); err != nil {
		fmt.Fprintln(os.Stderr, "v2cleanup:", err)
		os.Exit(exitUsage)
	}
	// The whole safety net is bounded by the recorded cleanup budget (never extended).
	ctx, cancel := context.WithTimeout(context.Background(), handles.CleanupBudget.Duration)
	defer cancel()
	app, err := bootstrap.Build(ctx, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "v2cleanup: the product wiring could not be rebuilt (a missing Vault, database or key keeps everything in place):", err)
		os.Exit(exitAttention)
	}
	defer app.Close()
	if app.RunCleanup == nil {
		fmt.Fprintln(os.Stderr, "v2cleanup: the Terraform safety net is unavailable")
		os.Exit(exitAttention)
	}
	manifest, err := v2run.LoadManifest(*manifestFile)
	if err != nil || manifest.RunID != handles.RunID {
		fmt.Fprintln(os.Stderr, "v2cleanup: the run manifest is missing or belongs to another run; nothing is touched")
		os.Exit(exitAttention)
	}
	if err := app.RunCleanup.SetDeadline(handles.CleanupBudget.Duration); err != nil {
		fmt.Fprintln(os.Stderr, "v2cleanup:", err)
		os.Exit(exitUsage)
	}
	target := runcleanup.Target{OrganizationKey: handles.OrganizationKey, ApplicationKey: handles.ApplicationKey, EnvironmentKey: handles.EnvironmentKey,
		RunIDs: manifest.TerraformRuns, Generation: manifest.Generation}
	if *descriptorsFile != "" {
		if cmd != "terraform" {
			fmt.Fprintln(os.Stderr, "v2cleanup: selection only supports terraform")
			os.Exit(exitUsage)
		}
		ids, selectErr := selectFailedInstances(ctx, app, target, opts.TerraformStateDatabaseURL, *descriptorsFile)
		if selectErr != nil {
			fmt.Fprintln(os.Stderr, "v2cleanup: exact failed selection is invalid")
			os.Exit(exitAttention)
		}
		target.InstanceIDs = ids
	}
	switch cmd {
	case "delivery":
		os.Exit(delivery(ctx, app, handles, manifest))
	case "terraform":
		os.Exit(terraform(ctx, app, target))
	case "status":
		os.Exit(status(ctx, app, target))
	}
	fmt.Fprintf(os.Stderr, "v2cleanup: unknown command %q\n", cmd)
	os.Exit(exitUsage)
}

func print(v any) {
	out, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(out))
}

// delivery removes the delivered Workloads of the recorded generation with the
// product's own removal, under the Environment claim: a held Environment is
// reported before any effect and an unverified removal keeps the claim.
func delivery(ctx context.Context, app *bootstrap.App, h v2run.CleanupHandles, m v2run.Manifest) int {
	env, err := app.Store.GetEnvironment(ctx, h.ApplicationKey, h.EnvironmentKey)
	if err != nil {
		fmt.Fprintln(os.Stderr, "v2cleanup: the Environment cannot be read")
		return exitAttention
	}
	report, err := app.Deployments.RemoveDelivered(ctx, h.OrganizationKey, h.ApplicationKey, h.EnvironmentKey, m.Generation)
	print(map[string]any{"removed": report.Removed, "protected": report.Protected, "failed": report.Failed, "secretObjects": report.SecretObjects, "namespace": env.Namespace()})
	switch {
	case errors.Is(err, persistence.ErrEnvironmentBusy):
		fmt.Fprintln(os.Stderr, "v2cleanup: the Environment is held by another operation; it is not unlocked and nothing was removed")
		return exitBusy
	case err != nil || len(report.Failed) > 0 || len(report.Protected) > 0:
		return exitAttention
	}
	return exitDone
}

func terraform(ctx context.Context, app *bootstrap.App, t runcleanup.Target) int {
	report, err := app.RunCleanup.Run(ctx, t)
	print(report)
	switch {
	case report.Outcome == runcleanup.OutcomeBusy:
		fmt.Fprintln(os.Stderr, "v2cleanup: the Environment is held by another operation; it is not unlocked")
		return exitBusy
	case err != nil || report.Outcome != runcleanup.OutcomeClean:
		return exitAttention
	}
	return exitDone
}

func status(ctx context.Context, app *bootstrap.App, t runcleanup.Target) int {
	env, err := app.Store.GetEnvironment(ctx, t.ApplicationKey, t.EnvironmentKey)
	if err != nil {
		return exitAttention
	}
	remaining, err := app.RunCleanup.Remaining(ctx, env)
	print(map[string]any{"remaining": remaining})
	if err != nil || len(remaining) > 0 {
		return exitAttention
	}
	return exitDone
}

func adapterMode(h v2run.CleanupHandles) bootstrap.AdapterMode {
	if h.Adapters == "aws" {
		return bootstrap.AdapterAWS
	}
	return bootstrap.AdapterKubernetes
}

func vaultDelivery(h v2run.CleanupHandles) string {
	if adapterMode(h) == bootstrap.AdapterAWS {
		return "agent"
	}
	return "vso"
}
