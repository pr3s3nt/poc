package delivery

import (
	"context"
	"encoding/json"
	"errors"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/persistence"
	"testing"
	"time"
)

type receiptRecoveryFake struct {
	*transferFake
	fail          bool
	healingOnly   bool
	recoveries    int
	recoveryGuard func(context.Context, port.Intent) error
}

func TestTransferRecoveryFinishesReceiptedHealingWithoutRepeatingAdoption(t *testing.T) {
	c, original, ctx, req, _ := coordinatorFixture(t)
	p := &receiptRecoveryFake{transferFake: &transferFake{neutralProvider: original, live: map[string]string{}}, recoveryGuard: c.GuardRecovery}
	c.Registry = port.NewRegistry()
	for _, kind := range []string{"fleet", "direct"} {
		if err := c.Registry.Register(kind, p); err != nil {
			t.Fatal(err)
		}
	}
	// Begin with direct-owned objects and a completed Job, then opt into CD
	// with healing. The simulated loss happens only in the post-adoption phase.
	selection := req.Environment.DesiredDelivery
	req.Environment.DesiredDelivery = domain.Selection{Mode: domain.Direct}
	org, err := c.Store.GetOrganization(ctx, req.OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	owner, _ := persistence.OwnerFrom(ctx)
	if err := c.Store.ReleaseOperation(ctx, owner, environment.OpSucceeded, ""); err != nil {
		t.Fatal(err)
	}
	env, err := c.Store.GetEnvironment(context.Background(), req.ApplicationKey, req.Environment.Key)
	if err != nil {
		t.Fatal(err)
	}
	selections := c.Store.(persistence.DeliverySelectionRepository)
	env, err = selections.SelectDelivery(context.Background(), persistence.DeliverySelectionCommand{OrganizationID: org.ID, ApplicationKey: req.ApplicationKey, EnvironmentKey: env.Key, ExpectedEnvironmentVersion: env.Version, ExpectedConfigurationRevision: env.DeliveryConfigRevision, Selection: req.Environment.DesiredDelivery})
	if err != nil {
		t.Fatal(err)
	}
	claim := func() context.Context {
		t.Helper()
		op, err := c.Store.ClaimEnvironment(context.Background(), persistence.OperationClaim{ID: ids.New(), ApplicationKey: req.ApplicationKey, EnvironmentKey: env.Key, Owner: "transfer-healing", Kind: environment.OpDeploy, Deadline: time.Now().Add(time.Minute)})
		if err != nil {
			t.Fatal(err)
		}
		return persistence.WithOwner(context.Background(), persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence})
	}
	ctx = claim()
	req.Environment = env
	for n := range req.Manifests {
		m := &req.Manifests[n]
		m.APIVersion, m.Name, m.Namespace = "v1", "app", "app"
		if m.Kind == "Job" {
			m.APIVersion = "batch/v1"
		}
		if m.Kind == "Deployment" {
			m.APIVersion = "apps/v1"
		}
		m.Object = map[string]any{"apiVersion": m.APIVersion, "kind": m.Kind, "metadata": map[string]any{"name": m.Name, "namespace": m.Namespace}}
	}
	if err := c.Deploy(ctx, req); err != nil {
		t.Fatal(err)
	}
	if err := c.Activate(ctx, req.ApplicationKey, env); err != nil {
		t.Fatal(err)
	}
	owner, _ = persistence.OwnerFrom(ctx)
	if err := c.Store.ReleaseOperation(ctx, owner, environment.OpSucceeded, ""); err != nil {
		t.Fatal(err)
	}
	env, err = c.Store.GetEnvironment(context.Background(), req.ApplicationKey, env.Key)
	if err != nil {
		t.Fatal(err)
	}
	selection.SelfHeal = true
	bindings := c.Store.(persistence.DeliveryBindingRepository)
	binding, err := bindings.GetDeliveryBinding(ctx, org.ID, selection.BindingID, selection.BindingVersion)
	if err != nil {
		t.Fatal(err)
	}
	binding.Capabilities.SelfHeal = true
	binding.Capabilities.DriftDetection = true
	if _, err := bindings.VerifyDeliveryBinding(ctx, org.ID, binding.ID, binding.Version, binding.Revision, domain.BindingReady, binding.Capabilities, time.Now().UTC(), ""); err != nil {
		t.Fatal(err)
	}
	env, err = selections.SelectDelivery(context.Background(), persistence.DeliverySelectionCommand{OrganizationID: org.ID, ApplicationKey: req.ApplicationKey, EnvironmentKey: env.Key, ExpectedEnvironmentVersion: env.Version, ExpectedConfigurationRevision: env.DeliveryConfigRevision, Selection: selection})
	if err != nil {
		t.Fatal(err)
	}
	ctx = claim()
	p.fail = true
	p.healingOnly = true
	stage := func(string, map[string]any) error { return nil }
	if err := c.Transfer(ctx, req.OrganizationKey, env, stage); err == nil {
		t.Fatal("lost healing observation reported success")
	}
	adopted, submitted := p.adopted, len(p.order)
	before, _ := json.Marshal(p.live)
	owner, _ = persistence.OwnerFrom(ctx)
	if err := c.Store.SuspendOperation(ctx, owner, "prior worker stopped"); err != nil {
		t.Fatal(err)
	}
	op, err := c.Store.BeginRecovery(context.Background(), owner.OperationID, "healing-recovery", "operator")
	if err != nil {
		t.Fatal(err)
	}
	ctx = persistence.WithOwner(context.Background(), persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence})
	if err := c.ResumeTransfer(ctx, req.OrganizationKey, env, stage); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(p.live)
	if p.adopted != adopted || len(p.order) != submitted || p.jobs != 1 || string(before) != string(after) || p.recoveries != 1 {
		t.Fatal("healing recovery repeated effects or changed identities", p.adopted, adopted, len(p.order), submitted, p.jobs, p.recoveries, string(before) == string(after))
	}
	applied, err := c.Store.GetEnvironment(ctx, req.ApplicationKey, env.Key)
	if err != nil || applied.AppliedDelivery != selection || applied.DeliveryEpoch != env.DeliveryEpoch+1 {
		t.Fatal("transfer not activated", err)
	}
}

func (p *receiptRecoveryFake) Observe(ctx context.Context, i port.Intent, r port.Receipt) (domain.Observation, error) {
	if p.fail && i.Revision.Phase == domain.Rollout && (!p.healingOnly || i.Revision.Selection.SelfHeal) {
		p.fail = false
		return domain.Observation{}, errors.New("observation acknowledgement unavailable")
	}
	return p.transferFake.Observe(ctx, i, r)
}
func (p *receiptRecoveryFake) RecoverSubmission(ctx context.Context, i port.Intent, _ port.Prepared, r port.Receipt, _ port.TransferSource) (port.Receipt, error) {
	if err := p.recoveryGuard(ctx, i); err != nil {
		return r, err
	}
	p.recoveries++
	return r, nil
}
func TestReceiptedSubmissionRecoveryObservesWithoutReplayOrJobRerun(t *testing.T) {
	c, original, ctx, req, _ := coordinatorFixture(t)
	p := &receiptRecoveryFake{transferFake: &transferFake{neutralProvider: original, live: map[string]string{}}, fail: true, recoveryGuard: c.GuardRecovery}
	c.Registry = port.NewRegistry()
	if err := c.Registry.Register("fleet", p); err != nil {
		t.Fatal(err)
	}
	for n := range req.Manifests {
		m := &req.Manifests[n]
		m.APIVersion, m.Name, m.Namespace = "v1", "app", "app"
		if m.Kind == "Job" {
			m.APIVersion = "batch/v1"
		}
		if m.Kind == "Deployment" {
			m.APIVersion = "apps/v1"
		}
		m.Object = map[string]any{"apiVersion": m.APIVersion, "kind": m.Kind, "metadata": map[string]any{"name": m.Name, "namespace": m.Namespace}}
	}
	if err := c.Deploy(ctx, req); err == nil {
		t.Fatal("lost observation was reported successful")
	}
	submitted := len(p.order)
	before, _ := json.Marshal(p.live)
	owner, _ := persistence.OwnerFrom(ctx)
	if err := c.Store.SuspendOperation(ctx, owner, "prior executor stopped"); err != nil {
		t.Fatal(err)
	}
	op, err := c.Store.BeginRecovery(context.Background(), owner.OperationID, "receipt-recovery", "operator")
	if err != nil {
		t.Fatal(err)
	}
	recovery := persistence.WithOwner(context.Background(), persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence})
	if err := c.RecoverSubmissions(recovery, req.OrganizationKey, req.Environment); err != nil {
		t.Fatal(err)
	}
	if err := c.Deploy(recovery, req); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(p.live)
	if p.recoveries != 1 || len(p.order) != submitted || p.jobs != 1 || string(before) != string(after) {
		t.Fatal("recovery replayed mutation or changed UID")
	}
	if err := c.Activate(recovery, req.ApplicationKey, req.Environment); err != nil {
		t.Fatal(err)
	}
}
