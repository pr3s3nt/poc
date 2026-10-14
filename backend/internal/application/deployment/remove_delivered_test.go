package deployment_test

import (
	"context"
	"errors"
	"testing"

	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/envops"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
)

func deliveredApp(t *testing.T) (context.Context, appsvc.RemoveInputs, *hookDeployer, func() []domain.WorkloadInstance) {
	t.Helper()
	ctx := context.Background()
	app, opts, deployer := uc07App(t)
	scores := seed.AcceptanceScores(opts)
	for _, id := range []string{"backend", "worker"} {
		if _, err := uc07Deploy(t, app, opts, id, nil, scores[id]); err != nil {
			t.Fatal(err)
		}
	}
	in := appsvc.RemoveInputs{Service: app.Deployments, Store: app.Store, Ops: app.Operations, Organization: opts.OrganizationKey, Application: opts.ApplicationKey, Environment: opts.EnvironmentKey}
	return ctx, in, deployer, func() []domain.WorkloadInstance {
		list, err := app.Store.ListWorkloadInstances(ctx, opts.ApplicationKey+"/"+opts.EnvironmentKey)
		if err != nil {
			t.Fatal(err)
		}
		return list
	}
}

func TestRemoveDeliveredUsesTheClaimAndEachOriginalTarget(t *testing.T) {
	ctx, in, deployer, instances := deliveredApp(t)
	before := deployer.removes
	report, err := in.Service.RemoveDelivered(ctx, in.Organization, in.Application, in.Environment, 0)
	if err != nil || len(report.Removed) != 2 || deployer.removes != before+2 {
		t.Fatalf("report=%+v err=%v removes=%d", report, err, deployer.removes-before)
	}
	for _, w := range instances() {
		if w.Status != domain.InstanceRemoved {
			t.Fatalf("instance %s is %s", w.WorkloadID, w.Status)
		}
	}
	if _, held, _ := in.Store.ActiveOperation(ctx, in.Application, in.Environment); held {
		t.Fatal("a successful removal must release the Environment claim")
	}
	again, err := in.Service.RemoveDelivered(ctx, in.Organization, in.Application, in.Environment, 0)
	if err != nil || len(again.Removed) != 0 || deployer.removes != before+2 {
		t.Fatalf("a second call must be a no-op: %+v %v", again, err)
	}
}

func TestRemoveDeliveredChecksTheHoldBeforeAnyEffect(t *testing.T) {
	ctx, in, deployer, instances := deliveredApp(t)
	lease, err := in.Ops.Begin(ctx, envops.Claim{ApplicationKey: in.Application, EnvironmentKey: in.Environment, Kind: environment.OpDeploy})
	if err != nil {
		t.Fatal(err)
	}
	defer lease.End(environment.OpFailed, "test")
	before := deployer.removes
	_, err = in.Service.RemoveDelivered(ctx, in.Organization, in.Application, in.Environment, 0)
	if !errors.Is(err, persistence.ErrEnvironmentBusy) {
		t.Fatalf("a held Environment must be busy, got %v", err)
	}
	if deployer.removes != before {
		t.Fatal("a held Environment must not see any removal")
	}
	for _, w := range instances() {
		if w.Status == domain.InstanceRemoving || w.Status == domain.InstanceRemoved || w.Status == domain.InstanceFailed {
			t.Fatalf("a held Environment must not change instance %s (%s)", w.WorkloadID, w.Status)
		}
	}
}

func TestRemoveDeliveredUnknownOutcomeKeepsTheClaimAndTheFailedInstance(t *testing.T) {
	ctx, in, deployer, instances := deliveredApp(t)
	deployer.removeErr = errors.New("kubectl: connection lost")
	_, err := in.Service.RemoveDelivered(ctx, in.Organization, in.Application, in.Environment, 0)
	if err == nil {
		t.Fatal("an unverified removal must fail")
	}
	failed := 0
	for _, w := range instances() {
		if w.Status == domain.InstanceFailed {
			failed++
		}
	}
	if failed == 0 {
		t.Fatal("the unverified instance must stay FAILED, not hidden")
	}
	active, held, _ := in.Store.ActiveOperation(ctx, in.Application, in.Environment)
	if !held || active.Status != environment.OpInterrupted {
		t.Fatalf("the claim must stay INTERRUPTED (no unlock): held=%v %+v", held, active)
	}
	deployer.removeErr = nil
	before := deployer.removes
	if _, err := in.Service.RemoveDelivered(ctx, in.Organization, in.Application, in.Environment, 0); !errors.Is(err, persistence.ErrEnvironmentBusy) {
		t.Fatalf("no automatic resume after an unknown outcome: %v", err)
	}
	if deployer.removes != before {
		t.Fatal("no removal may run while the claim is held")
	}
}

func TestRemoveDeliveredRetryUsesOriginalOwnerAfterRemovalSnapshotChanged(t *testing.T) {
	ctx, in, deployer, instances := deliveredApp(t)
	var instance domain.WorkloadInstance
	for _, candidate := range instances() {
		if candidate.WorkloadID == "worker" {
			instance = candidate
		}
	}
	if instance.WorkloadID == "" {
		t.Fatal("worker workload missing")
	}
	originalOwner := instance.LastDeploymentID
	instance.TargetRef = cloneTargetRef(instance.TargetRef)
	instance.TargetRef["orchestrator.io/owner-deployment-id"] = originalOwner
	instance.LastDeploymentID = "removal-attempt"
	instance.Status = domain.InstanceFailed
	if err := in.Store.UpsertWorkloadInstance(ctx, instance); err != nil {
		t.Fatal(err)
	}
	if _, err := in.Service.RemoveDelivered(ctx, in.Organization, in.Application, in.Environment, 0); err != nil {
		t.Fatal(err)
	}
	if got := deployer.removeTarget.Extra["deployment"]; got != originalOwner {
		t.Fatalf("retry used deployment %q; renderer objects belong to %q", got, originalOwner)
	}
}

func cloneTargetRef(ref map[string]any) map[string]any {
	clone := make(map[string]any, len(ref))
	for key, value := range ref {
		clone[key] = value
	}
	return clone
}

func TestRemoveDeliveredRefusesAWrongGenerationAndProtectsOthers(t *testing.T) {
	ctx, in, deployer, instances := deliveredApp(t)
	before := deployer.removes
	if _, err := in.Service.RemoveDelivered(ctx, in.Organization, in.Application, in.Environment, 3); !errors.Is(err, appsvc.ErrGenerationMismatch) {
		t.Fatalf("a different recorded generation must be refused: %v", err)
	}
	other := instances()[0]
	other.ID, other.Generation, other.WorkloadID = "other-gen", 1, "legacy"
	if err := in.Store.UpsertWorkloadInstance(ctx, other); err != nil {
		t.Fatal(err)
	}
	report, err := in.Service.RemoveDelivered(ctx, in.Organization, in.Application, in.Environment, 0)
	if err != nil || len(report.Protected) != 1 || deployer.removes != before+2 {
		t.Fatalf("another generation must be protected and untouched: %+v %v", report, err)
	}
	for _, w := range instances() {
		_ = w
	}
	got, _ := in.Store.ListWorkloadInstancesFor(ctx, in.Application+"/"+in.Environment, 1)
	if len(got) != 1 || got[0].Status == domain.InstanceRemoved {
		t.Fatalf("the other generation must stay as it was: %+v", got)
	}
}

type removingSync struct {
	calls   int
	app     string
	failure error
}

func (*removingSync) Sync(context.Context, execution.Target, execution.ConfigBundle) error {
	return nil
}
func (r *removingSync) RemoveSynchronized(_ context.Context, target execution.Target, app, env string) ([]string, error) {
	r.calls++
	r.app = app + "/" + env
	return []string{"secret/orch-x"}, r.failure
}

func TestRemoveDeliveredAlsoRemovesTheApplicationsSecretDeliveryObjects(t *testing.T) {
	ctx, in, _, _ := deliveredApp(t)
	sync := &removingSync{}
	in.Service.SetConfigSecretSynchronizer(sync)
	report, err := in.Service.RemoveDelivered(ctx, in.Organization, in.Application, in.Environment, 0)
	if err != nil || sync.calls != 1 || sync.app != in.Application+"/"+in.Environment || len(report.SecretObjects) != 1 {
		t.Fatalf("report=%+v err=%v sync=%+v", report, err, sync)
	}
	// A failed secret-object removal is an unverified outcome: the claim stays held.
	ctx2, in2, _, _ := deliveredApp(t)
	failing := &removingSync{failure: errors.New("kubectl failed")}
	in2.Service.SetConfigSecretSynchronizer(failing)
	if _, err := in2.Service.RemoveDelivered(ctx2, in2.Organization, in2.Application, in2.Environment, 0); err == nil {
		t.Fatal("a failed secret object removal must fail the delivery layer")
	}
	if active, held, _ := in2.Store.ActiveOperation(ctx2, in2.Application, in2.Environment); !held || active.Status != environment.OpInterrupted {
		t.Fatal("the claim must stay held after an unverified secret object removal")
	}
}
