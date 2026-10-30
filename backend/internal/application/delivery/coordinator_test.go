package delivery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"orchestrator/internal/adapters/statecipher"
	"orchestrator/internal/adapters/store"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/persistence/persistencetest"
)

type neutralProvider struct {
	order       []domain.Phase
	removed     []domain.Phase
	submitError error
	guard       func(context.Context, port.Intent) error
}

func (p *neutralProvider) Describe(context.Context, port.BindingVersion) (domain.Capabilities, error) {
	return domain.Capabilities{}, nil
}
func (p *neutralProvider) Verify(context.Context, port.BindingVersion) (port.Verification, error) {
	return port.Verification{}, nil
}
func (p *neutralProvider) Prepare(_ context.Context, i port.Intent, m []execution.Manifest) (port.Prepared, error) {
	objects := []map[string]any{}
	for _, obj := range m {
		objects = append(objects, obj.Object)
	}
	raw, _ := json.Marshal(objects)
	sum := sha256.Sum256(raw)
	r := i.Revision
	r.PayloadDigest = hex.EncodeToString(sum[:])
	return port.Prepared{Revision: r, Payload: raw}, nil
}
func (p *neutralProvider) Submit(ctx context.Context, i port.Intent, prepared port.Prepared) (port.Receipt, error) {
	if err := p.guard(ctx, i); err != nil {
		return port.Receipt{}, err
	}
	p.order = append(p.order, i.Revision.Phase)
	if p.submitError != nil {
		return port.Receipt{}, p.submitError
	}
	return port.Receipt{IntentID: i.ID, RevisionID: i.Revision.ID, PayloadDigest: i.Revision.PayloadDigest, Handle: map[string]string{"second-provider": "receipt"}}, nil
}
func (p *neutralProvider) Observe(_ context.Context, i port.Intent, r port.Receipt) (domain.Observation, error) {
	return domain.Observation{Scope: i.Revision.Scope, IntendedRevision: i.Revision.ID, ObservedRevision: r.RevisionID, PayloadDigest: r.PayloadDigest, Sync: domain.InSync, Health: domain.Healthy, Drift: domain.NoDrift, Complete: true, ObservedAt: time.Now().UTC()}, nil
}
func (p *neutralProvider) Reconcile(context.Context, port.Intent, port.Receipt) (port.Receipt, error) {
	return port.Receipt{}, domain.ErrUnsupported
}
func (p *neutralProvider) Withdraw(context.Context, port.Intent, port.Receipt) (port.WithdrawalProof, error) {
	return port.WithdrawalProof{}, domain.ErrUnsupported
}
func (p *neutralProvider) RemoveRetainedObjects(ctx context.Context, i port.Intent, _ port.Receipt, _ []port.AppliedObject) error {
	if err := p.guard(ctx, i); err != nil {
		return err
	}
	p.removed = append(p.removed, i.Revision.Phase)
	return nil
}
func (p *neutralProvider) SameExecution(_ context.Context, old []byte, manifests []execution.Manifest) (bool, error) {
	objects := []map[string]any{}
	for _, m := range manifests {
		objects = append(objects, m.Object)
	}
	raw, err := json.Marshal(objects)
	return string(raw) == string(old), err
}
func coordinatorFixture(t *testing.T) (*Coordinator, *neutralProvider, context.Context, DeployRequest, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.json")
	st, err := store.NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	b := persistencetest.DeliveryBindings(t, st)
	persistencetest.DeliverySelections(t, st, b)
	env, err := st.GetEnvironment(context.Background(), "delivery-app", "staging")
	if err != nil {
		t.Fatal(err)
	}
	op, err := st.ClaimEnvironment(context.Background(), persistence.OperationClaim{ID: ids.New(), ApplicationKey: "delivery-app", EnvironmentKey: "staging", Owner: "coordinator", Kind: environment.OpDeploy, Deadline: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	ctx := persistence.WithOwner(context.Background(), persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence})
	cipher, err := statecipher.New("delivery-test", make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	registry := port.NewRegistry()
	provider := &neutralProvider{}
	if err := registry.Register(b.ProviderKind, provider); err != nil {
		t.Fatal(err)
	}
	coordinator := &Coordinator{Store: st, Registry: registry, Artifacts: &Artifacts{Store: st, Cipher: cipher}}
	provider.guard = coordinator.Guard
	req := DeployRequest{OrganizationKey: "delivery-org", ApplicationKey: "delivery-app", Environment: env, WorkloadID: "app", DeploymentID: ids.New(), Target: execution.Target{Organization: "delivery-org", Connection: "delivery-manager", Namespace: "app"}, Manifests: []execution.Manifest{
		{Kind: "Deployment", Object: map[string]any{"kind": "Deployment"}},
		{Kind: "Job", Object: map[string]any{"kind": "Job"}},
		{Kind: "ConfigMap", Object: map[string]any{"kind": "ConfigMap"}},
	}}
	return coordinator, provider, ctx, req, path
}
func TestCoordinatorUsesProviderContractAndPersistsReplayBeforeCalls(t *testing.T) {
	c, p, ctx, req, path := coordinatorFixture(t)
	if err := c.Deploy(ctx, req); err != nil {
		t.Fatal(err)
	}
	want := []domain.Phase{domain.Prerequisite, domain.Job, domain.Rollout}
	if len(p.order) != len(want) {
		t.Fatal(p.order)
	}
	for n, phase := range want {
		if p.order[n] != phase {
			t.Fatal("ordering changed", p.order)
		}
	}
	// Same operation/deployment after backend restart does not submit a phase again.
	reopened, err := store.NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	c.Store = reopened
	c.Artifacts.Store = reopened
	if err := c.Deploy(ctx, req); err != nil {
		t.Fatal(err)
	}
	if len(p.order) != 3 {
		t.Fatal("replayed acknowledged side effects")
	}
	b, err := reopened.ListDeliveryExecutions(ctx, req.Environment.DesiredDelivery.BindingID, req.Environment.ID)
	if err != nil || len(b) != 0 {
		t.Fatal("foreign organization read exposed execution")
	}
}
func TestCoordinatorHoldsUnknownSubmissionWithoutBlindReplay(t *testing.T) {
	c, p, ctx, req, _ := coordinatorFixture(t)
	p.submitError = errors.New("lost external ack")
	if err := c.Deploy(ctx, req); err == nil {
		t.Fatal("unknown submission reported success")
	}
	p.submitError = nil
	if err := c.Deploy(ctx, req); !errors.Is(err, domain.ErrOutcomeUnknown) {
		t.Fatal("unknown replay allowed", err)
	}
	if len(p.order) != 1 {
		t.Fatal("external submission repeated")
	}
}

func TestCoordinatorRetiresOmittedPhaseAndDoesNotRemoveItTwice(t *testing.T) {
	c, p, ctx, req, _ := coordinatorFixture(t)
	if err := c.Deploy(ctx, req); err != nil {
		t.Fatal(err)
	}
	req.DeploymentID = ids.New()
	req.Manifests = req.Manifests[:1] // only rollout remains
	if err := c.Deploy(ctx, req); err != nil {
		t.Fatal(err)
	}
	if len(p.removed) != 2 || p.removed[0] != domain.Job || p.removed[1] != domain.Prerequisite {
		t.Fatal("omitted controllers remain active or withdrawal order changed", p.removed)
	}
	if err := c.RemoveWorkload(ctx, req.OrganizationKey, req.Environment.ID, req.WorkloadID); err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveWorkload(ctx, req.OrganizationKey, req.Environment.ID, req.WorkloadID); err != nil {
		t.Fatal(err)
	}
	if len(p.removed) != 3 || p.removed[2] != domain.Rollout {
		t.Fatal("withdrawn revisions were reused as active ownership", p.removed)
	}
}

func TestCoordinatorReusesCompletedJobAcrossDeploymentAndRejectsMutation(t *testing.T) {
	c, p, ctx, req, _ := coordinatorFixture(t)
	if err := c.Deploy(ctx, req); err != nil {
		t.Fatal(err)
	}
	req.DeploymentID = ids.New()
	if err := c.Deploy(ctx, req); err != nil {
		t.Fatal(err)
	}
	jobs := 0
	for _, phase := range p.order {
		if phase == domain.Job {
			jobs++
		}
	}
	if jobs != 1 {
		t.Fatal("completed Job was executed again", p.order)
	}
	before := len(p.order)
	req.DeploymentID = ids.New()
	req.Manifests[1].Object = map[string]any{"kind": "Job", "spec": map[string]any{"changed": true}}
	if err := c.Deploy(ctx, req); !errors.Is(err, domain.ErrUnsupported) {
		t.Fatal("immutable execution changed", err)
	}
	if len(p.order) != before {
		t.Fatal("immutable change was rejected after external writes")
	}
}

func (p *neutralProvider) ValidateConfiguration(port.BindingVersion) error { return nil }

func TestBindingVersionRetainsOriginalConsumerConfiguration(t *testing.T) {
	c, _, ctx, req, _ := coordinatorFixture(t)
	service := NewBindings(c.Store.(bindingStore), c.Registry)
	org, err := c.Store.GetOrganization(ctx, req.OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	selection := req.Environment.DesiredDelivery
	original, err := c.Store.GetDeliveryBinding(ctx, org.ID, selection.BindingID, selection.BindingVersion)
	if err != nil {
		t.Fatal(err)
	}
	account, err := c.Store.GetConnection(ctx, req.OrganizationKey, "delivery-manager")
	if err != nil {
		t.Fatal(err)
	}
	revised, err := service.Register(ctx, req.OrganizationKey, RegisterBinding{BindingID: original.ID, PreviousVersion: original.Version, ExpectedRevision: original.Revision, RetainWriterCredential: true, Name: "Revised CD", ProviderKind: original.ProviderKind, ManagementAccountKey: account.Key, ManagementAccountVersion: account.Version, Config: map[string]string{"workspace": "different"}})
	if err != nil {
		t.Fatal(err)
	}
	if revised.Version != 3 || revised.Status != domain.Unverified || revised.WriterCredentialRef != original.WriterCredentialRef {
		t.Fatal("new version reused verification or lost private identity")
	}
	old, err := c.Store.GetDeliveryBinding(ctx, org.ID, original.ID, original.Version)
	if err != nil || !old.SameConfiguration(original) {
		t.Fatal("historical binding rewritten", err)
	}
	env, err := c.Store.GetEnvironment(ctx, req.ApplicationKey, req.Environment.Key)
	if err != nil || env.DesiredDelivery != selection {
		t.Fatal("version creation repointed environment", err)
	}
	details, err := service.Details(ctx, req.OrganizationKey, original.ID, original.Version)
	if err != nil || len(details.Usage) == 0 || details.ManagementAccountKey != account.Key {
		t.Fatal("binding usage unavailable", err)
	}
	if _, err := service.Details(ctx, "foreign-org", original.ID, original.Version); err == nil {
		t.Fatal("foreign binding details exposed")
	}
}
