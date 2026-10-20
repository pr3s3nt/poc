package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformstate"
)

// seedProgress writes persisted Terraform attempts for three Deployments of one
// Environment through the real Console and State stores, the way the executor
// does (write-ahead attempt, then stage updates). The attempts are SEEDED, not
// executed: no Terraform, provider or cluster runs. One Deployment carries
// attempt text that no executor would write (written as the database owner) to
// prove the Console never forwards it. It prints the Deployment IDs as JSON.
func seedProgress(args []string) int {
	fs := flag.NewFlagSet("seed-progress", flag.ExitOnError)
	dir := fs.String("private-dir", "", "private directory holding console.url, state.url and owner.url")
	app := fs.String("application", "", "Application key")
	env := fs.String("environment", "staging", "Environment key")
	_ = fs.Parse(args)
	read := func(name string) string {
		v, err := bootstrap.ReadCredentialFile(name, filepath.Join(*dir, name))
		if err != nil {
			fmt.Fprintln(os.Stderr, "defworkserver seed-progress: cannot read", name)
			os.Exit(1)
		}
		return strings.TrimSpace(v)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	console, err := postgres.Open(ctx, read("console.url"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "defworkserver seed-progress: console login failed")
		return 1
	}
	defer console.Close()
	state, err := postgres.OpenState(ctx, read("state.url"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "defworkserver seed-progress: state login failed")
		return 1
	}
	defer state.Close()
	owner, err := pgx.Connect(ctx, read("owner.url"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "defworkserver seed-progress: owner login failed")
		return 1
	}
	defer owner.Close(ctx)

	fail := func(what string, err error) int {
		fmt.Fprintf(os.Stderr, "defworkserver seed-progress: %s: %v\n", what, err)
		return 1
	}
	environ, err := console.GetEnvironment(ctx, *app, *env)
	if err != nil {
		return fail("environment", err)
	}
	application, err := console.GetApplication(ctx, *app)
	if err != nil {
		return fail("application", err)
	}
	bundles, err := console.ListModuleBundles(ctx)
	if err != nil {
		return fail("bundles", err)
	}
	bundleOf := map[string]string{}
	for _, b := range bundles {
		bundleOf[b.Ref.ModuleID] = b.ID
	}
	if bundleOf["redis-k8s"] == "" || bundleOf["postgres-k8s"] == "" {
		return fail("bundles", fmt.Errorf("the verified redis-k8s/postgres-k8s bundles are not registered yet"))
	}
	op, err := console.ClaimEnvironment(ctx, persistence.OperationClaim{ApplicationKey: *app, EnvironmentKey: *env, Owner: "seed-progress", Kind: environment.OpDeploy, Deadline: time.Now().Add(time.Hour)})
	if err != nil {
		return fail("claim", err)
	}
	ownerCtx := persistence.WithOwner(ctx, persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence})
	register := func(descriptor, scopeID, module string) (tfresource.Instance, error) {
		return state.RegisterInstance(ownerCtx, terraformstate.NewInstance{Descriptor: descriptor, ScopeType: "SHARED", ScopeID: scopeID, ModuleBundleID: bundleOf[module], TargetRef: map[string]any{"connection": "seeded"}, PhysicalIntent: map[string]any{"name": scopeID}, CreatedByRunID: "run-seed"})
	}
	redis, err := register("redis.in-cluster#shared.progress-cache", "progress-cache", "redis-k8s")
	if err != nil {
		return fail("instance", err)
	}
	pg, err := register("postgres.in-cluster#shared.progress-db", "progress-db", "postgres-k8s")
	if err != nil {
		return fail("instance", err)
	}
	save := func(status deployment.Status, reason string) (string, error) {
		d := deployment.Deployment{ID: ids.New(), EnvironmentID: environ.ID, OrganizationKey: application.OrganizationKey, ApplicationKey: *app, EnvironmentKey: *env, Action: deployment.ActionDeploy, WorkloadID: "seeded", ActorRef: "seed-progress", Status: status, BaseEnvironmentVersion: environ.Version, StartedAt: time.Now().UTC()}
		if err := console.SaveDeployment(ctx, d); err != nil {
			return "", err
		}
		if status == deployment.StatusFailed {
			d.FailureReason = reason
			return d.ID, console.SaveDeployment(ctx, d)
		}
		return d.ID, nil
	}
	attempt := func(instance tfresource.Instance, deploymentID string, stages ...tfresource.AttemptProgress) error {
		a, err := state.BeginAttempt(ownerCtx, tfresource.Attempt{InstanceID: instance.ID, DeploymentID: deploymentID, RunID: "run-seed", Action: tfresource.ActionUpdate, Ownership: tfresource.Preexisting, Stage: "INTENT",
			ModuleSnapshot: map[string]any{"verifyHandle": "seeded-private-handle"}, TargetSnapshot: map[string]any{"connection": "seeded"}, Deadline: time.Now().Add(time.Hour)})
		if err != nil {
			return err
		}
		for _, stage := range stages {
			if _, err := state.UpdateAttempt(ownerCtx, a.ID, stage); err != nil {
				return err
			}
		}
		return nil
	}
	out := map[string]string{}
	if out["good"], err = save(deployment.StatusPlanning, ""); err != nil {
		return fail("deployment", err)
	}
	if err = attempt(redis, out["good"], tfresource.AttemptProgress{Stage: "APPLYING", Status: tfresource.AttemptRunning}, tfresource.AttemptProgress{Stage: "READY", Status: tfresource.AttemptSucceeded}); err != nil {
		return fail("attempt", err)
	}
	if err = attempt(pg, out["good"], tfresource.AttemptProgress{Stage: "APPLYING", Status: tfresource.AttemptFailed, SafeErrorCode: "TIMEOUT"}); err != nil {
		return fail("attempt", err)
	}
	if out["hostile"], err = save(deployment.StatusFailed, "seeded"); err != nil {
		return fail("deployment", err)
	}
	if err = attempt(redis, out["hostile"], tfresource.AttemptProgress{Stage: "APPLYING", Status: tfresource.AttemptFailed, SafeErrorCode: "AUTH"}); err != nil {
		return fail("attempt", err)
	}
	if _, err := owner.Exec(ctx, `UPDATE terraform_resource_attempts SET stage='AKIAPRIVATE /var/lib/private/state', safe_error_code='AUTH vault://PRIVATE-TOKEN' WHERE deployment_id=$1::uuid`, out["hostile"]); err != nil {
		return fail("hostile text", err)
	}
	if out["empty"], err = save(deployment.StatusPlanning, ""); err != nil {
		return fail("deployment", err)
	}
	if err := console.ReleaseOperation(ctx, persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence}, environment.OpSucceeded, ""); err != nil {
		return fail("release", err)
	}
	encoded, _ := json.Marshal(out)
	fmt.Println(string(encoded))
	return 0
}
