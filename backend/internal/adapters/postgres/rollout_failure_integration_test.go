package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/adapters/statecipher"
	appdelivery "orchestrator/internal/application/delivery"
	"orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/persistence/persistencetest"
)

// degradedProvider publishes through a Git-like destination and reports the
// Rollout phase Degraded, as Fleet does for a crash-looping Deployment.
type degradedProvider struct{ submits int }

func (p *degradedProvider) Describe(context.Context, port.BindingVersion) (delivery.Capabilities, error) {
	return delivery.Capabilities{}, nil
}
func (p *degradedProvider) Verify(context.Context, port.BindingVersion) (port.Verification, error) {
	return port.Verification{}, nil
}
func (p *degradedProvider) Prepare(_ context.Context, i port.Intent, m []execution.Manifest) (port.Prepared, error) {
	raw, _ := json.Marshal(m)
	sum := sha256.Sum256(raw)
	r := i.Revision
	r.PayloadDigest = hex.EncodeToString(sum[:])
	return port.Prepared{Revision: r, Payload: raw}, nil
}
func (p *degradedProvider) Submit(_ context.Context, i port.Intent, _ port.Prepared) (port.Receipt, error) {
	p.submits++
	return port.Receipt{IntentID: i.ID, RevisionID: i.Revision.ID, PayloadDigest: i.Revision.PayloadDigest, Handle: map[string]string{"release": "r"}}, nil
}
func (p *degradedProvider) Observe(_ context.Context, i port.Intent, r port.Receipt) (delivery.Observation, error) {
	health := delivery.Healthy
	if i.Revision.Phase == delivery.Rollout {
		health = delivery.Degraded
	}
	return delivery.Observation{Scope: i.Revision.Scope, IntendedRevision: i.Revision.ID, ObservedRevision: r.RevisionID, PayloadDigest: r.PayloadDigest, Sync: delivery.InSync, Health: health, Drift: delivery.NoDrift, Complete: true, ObservedAt: time.Now().UTC()}, nil
}
func (p *degradedProvider) Reconcile(context.Context, port.Intent, port.Receipt) (port.Receipt, error) {
	return port.Receipt{}, delivery.ErrUnsupported
}
func (p *degradedProvider) Withdraw(context.Context, port.Intent, port.Receipt) (port.WithdrawalProof, error) {
	return port.WithdrawalProof{}, delivery.ErrUnsupported
}
func (p *degradedProvider) PublicationDestination(port.BindingVersion) (string, error) {
	return strings.Repeat("e", 64), nil
}

// UC-06 CD-10 on real PostgreSQL: a determined rollout failure is persisted as
// FAILED, releases the Git publication lease and lets the Environment be
// released and claimed again; nothing is held.
func TestDeterminedRolloutFailureReleasesPostgresEnvironment(t *testing.T) {
	ctx := context.Background()
	db := freshDatabase(t)
	st, err := Open(ctx, db.Console)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	state, err := OpenState(ctx, db.State)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	b := persistencetest.DeliveryBindings(t, st)
	persistencetest.DeliverySelections(t, st, b)
	env, err := st.GetEnvironment(ctx, "delivery-app", "staging")
	if err != nil {
		t.Fatal(err)
	}
	op, err := st.ClaimEnvironment(ctx, persistence.OperationClaim{ID: ids.New(), ApplicationKey: "delivery-app", EnvironmentKey: "staging", Owner: "worker", Kind: environment.OpDeploy, Deadline: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	owner := persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence}
	runCtx := persistence.WithHold(persistence.WithOwner(ctx, owner))
	cipher, err := statecipher.New("delivery-test", make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	provider := &degradedProvider{}
	registry := port.NewRegistry()
	if err := registry.Register(b.ProviderKind, provider); err != nil {
		t.Fatal(err)
	}
	c := &appdelivery.Coordinator{Store: st, Registry: registry, Artifacts: &appdelivery.Artifacts{Store: state, Cipher: cipher}, PollInterval: 5 * time.Millisecond, Deadline: time.Minute}
	req := appdelivery.DeployRequest{OrganizationKey: "delivery-org", ApplicationKey: "delivery-app", Environment: env, WorkloadID: "app", DeploymentID: ids.New(), Target: execution.Target{Organization: "delivery-org", Connection: "delivery-manager", Namespace: "app"}, Manifests: []execution.Manifest{
		{APIVersion: "apps/v1", Kind: "Deployment", Name: "app", Namespace: "app", Object: map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]any{"name": "app", "namespace": "app"}}},
	}}
	if err := c.Deploy(runCtx, req); !errors.Is(err, appdelivery.ErrRolloutDegraded) {
		t.Fatal("expected a determined degraded failure", err)
	}
	if persistence.Held(runCtx) {
		t.Fatal("determined failure held the Environment")
	}
	records, err := st.ListDeliveryExecutions(ctx, b.OrganizationID, env.ID)
	if err != nil || len(records) != 1 || records[0].Mutation.Status != delivery.IntentFailed || provider.submits != 1 {
		t.Fatal("failed rollout not persisted", err, records, provider.submits)
	}
	if err := st.ReleaseOperation(runCtx, owner, environment.OpFailed, "rollout failed"); err != nil {
		t.Fatal("Environment could not be released", err)
	}
	next, err := st.ClaimEnvironment(ctx, persistence.OperationClaim{ID: ids.New(), ApplicationKey: "delivery-app", EnvironmentKey: "staging", Owner: "next-worker", Kind: environment.OpDeploy, Deadline: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal("released Environment not claimable", err)
	}
	nextCtx := persistence.WithOwner(ctx, persistence.Owner{OperationID: next.ID, Owner: next.Owner, Fence: next.Fence})
	lease := persistence.GitPublicationLease{Destination: strings.Repeat("e", 64), OrganizationID: b.OrganizationID, Owner: persistence.Owner{OperationID: next.ID, Owner: next.Owner, Fence: next.Fence}}
	if err := st.ClaimGitPublication(nextCtx, lease); err != nil {
		t.Fatal("publication lease of the failed rollout was not released", err)
	}
}
