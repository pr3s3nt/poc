package drift

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/envops"
	"orchestrator/internal/application/tfexec"
	domain "orchestrator/internal/domain/drift"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/persistence/persistencetest"
	"orchestrator/internal/ports/terraformstate"
	"strings"
	"testing"
)

// A coordinator fake; persistence/State fencing is separately exercised by the
// PostgreSQL tests, and saved-plan execution by the real Terraform harness.
type repairFixture struct {
	instances map[string]tfresource.Instance
	baselines map[string]terraformstate.ExecutionBaseline
	reviews   map[string]terraformstate.PrivateRepairReview
	edges     []tfresource.Dependency
	applied   []string
	prepared  []string
	root      string
	failure   string
}

func (f *repairFixture) ListInstances(context.Context, string) ([]tfresource.Instance, error) {
	out := []tfresource.Instance{}
	for _, i := range f.instances {
		out = append(out, i)
	}
	return out, nil
}
func (f *repairFixture) GetInstance(_ context.Context, id string) (tfresource.Instance, error) {
	i, ok := f.instances[id]
	if !ok {
		return i, persistence.ErrNotFound
	}
	return i, nil
}
func (f *repairFixture) LatestExecutionBaseline(_ context.Context, org, id string) (terraformstate.ExecutionBaseline, error) {
	b, ok := f.baselines[id]
	if !ok {
		return b, persistence.ErrNotFound
	}
	return b, nil
}
func (f *repairFixture) Dependencies(_ context.Context, id string) ([]tfresource.Dependency, error) {
	out := []tfresource.Dependency{}
	for _, e := range f.edges {
		if e.ProviderID == id || e.ConsumerID == id {
			out = append(out, e)
		}
	}
	return out, nil
}
func (f *repairFixture) InspectInstance(context.Context, string, string) (domain.Report, error) {
	return domain.Report{Complete: true, Proposed: []domain.Finding{{Address: "example.root", Actions: []string{"update"}}}}, nil
}
func (f *repairFixture) PrepareInstanceRepair(ctx context.Context, org, id string) (tfexec.RepairPreparation, error) {
	return f.PrepareRepairMember(ctx, org, id)
}
func (f *repairFixture) PrepareRepairMember(_ context.Context, org, id string) (tfexec.RepairPreparation, error) {
	f.prepared = append(f.prepared, id)
	report := domain.Report{Complete: true, Applyable: true, Proposed: []domain.Finding{{Address: "example.root", Actions: []string{"update"}}}}
	if id == f.root {
		report.Proposed[0].Actions = []string{"delete", "create"}
		report.OutputsUnknown = true
	}
	return tfexec.RepairPreparation{InstanceID: id, InstanceVersion: f.instances[id].Version, BaselineID: f.baselines[id].ID, Report: report, ArtifactID: ids.New()}, nil
}
func (f *repairFixture) RepairInstance(_ context.Context, org, id, artifact string) error {
	if f.failure == id {
		return errors.New("injected provider failure")
	}
	f.applied = append(f.applied, id)
	i := f.instances[id]
	i.Version++
	f.instances[id] = i
	b := f.baselines[id]
	b.ID = ids.New()
	b.InstanceVersion = i.Version
	f.baselines[id] = b
	return nil
}
func (f *repairFixture) SealReviewToken(_ context.Context, org, id string) (persistence.CipherArtifact, string, error) {
	sum := sha256.Sum256([]byte(strings.Repeat("a", 64)))
	return persistence.CipherArtifact{}, hex.EncodeToString(sum[:]), nil
}
func (f *repairFixture) OpenReviewToken(context.Context, string, string, persistence.CipherArtifact) (string, error) {
	return strings.Repeat("a", 64), nil
}
func (f *repairFixture) CreateRepairReview(_ context.Context, r terraformstate.PrivateRepairReview) error {
	f.reviews[r.Review.ID] = r
	return nil
}
func (f *repairFixture) UpdateRepairReview(_ context.Context, r terraformstate.PrivateRepairReview, _ int64) error {
	r.Review.Version++
	f.reviews[r.Review.ID] = r
	return nil
}
func (f *repairFixture) GetRepairReview(_ context.Context, org, env, id string) (terraformstate.PrivateRepairReview, error) {
	r, ok := f.reviews[id]
	if !ok || r.Review.OrganizationID != org || r.Review.EnvironmentID != env {
		return r, persistence.ErrNotFound
	}
	return r, nil
}
func (f *repairFixture) FindRepairReviewRequest(_ context.Context, org, env, actor, request string) (terraformstate.PrivateRepairReview, error) {
	for _, r := range f.reviews {
		if r.Review.EnvironmentID == env && r.Review.ActorID == actor && r.Review.RequestID == request {
			return r, nil
		}
	}
	return terraformstate.PrivateRepairReview{}, persistence.ErrNotFound
}
func (f *repairFixture) ConsumeRepairReview(ctx context.Context, org, env, id, actor, request, hash string, _ int64) (terraformstate.PrivateRepairReview, error) {
	r := f.reviews[id]
	owner, _ := persistence.OwnerFrom(ctx)
	r.Review.Status = domain.Consumed
	r.Review.ConsumedOperationID = owner.OperationID
	r.Review.ConfirmationRequestID = request
	f.reviews[id] = r
	return r, nil
}

func repairTestService(t *testing.T) (*Service, *repairFixture, PrepareRepairCommand) {
	t.Helper()
	ctx := context.Background()
	st := store.New()
	binding := persistencetest.DeliveryBindings(t, st)
	persistencetest.DeliverySelections(t, st, binding)
	env, err := st.GetEnvironment(ctx, "delivery-app", "staging")
	if err != nil {
		t.Fatal(err)
	}
	f := &repairFixture{instances: map[string]tfresource.Instance{}, baselines: map[string]terraformstate.ExecutionBaseline{}, reviews: map[string]terraformstate.PrivateRepairReview{}}
	root, child := ids.New(), ids.New()
	f.root = root
	for _, id := range []string{root, child} {
		f.instances[id] = tfresource.Instance{ID: id, Identity: tfresource.Identity{OrganizationID: binding.OrganizationID, EnvironmentID: env.ID, TargetGeneration: env.TargetGeneration, Descriptor: "redis.default#" + id}, Version: 1, Status: tfresource.InstanceReady}
		f.baselines[id] = terraformstate.ExecutionBaseline{ID: ids.New(), InstanceVersion: 1}
	}
	f.edges = []tfresource.Dependency{{ConsumerID: child, ProviderID: root, ConsumerStatus: tfresource.InstanceReady}}
	service := &Service{Store: st, Instances: f, Inspector: f, Operations: envops.NewManager(st)}
	scan, err := service.Queue(ctx, QueueCommand{OrganizationKey: "delivery-org", ApplicationKey: "delivery-app", EnvironmentKey: "staging", RequestID: ids.New(), ExpectedEnvironmentVersion: env.Version})
	if err != nil {
		t.Fatal(err)
	}
	service.ProcessQueued(ctx)
	return service, f, PrepareRepairCommand{OrganizationKey: "delivery-org", ApplicationKey: "delivery-app", EnvironmentKey: "staging", ActorID: ids.New(), RequestID: ids.New(), ScanID: scan.ID, InstanceIDs: []string{root}, ExpectedEnvironmentVersion: env.Version}
}
func TestRepairStagesReviewConsumersWithoutReapplyingCompletedProvider(t *testing.T) {
	s, f, command := repairTestService(t)
	ctx := context.Background()
	first, err := s.PrepareRepair(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Review.Instances) != 2 || first.Review.Instances[0].InstanceID != f.root || !first.Review.Instances[1].Deferred || len(f.prepared) != 1 {
		t.Fatal("dependency stage not deferred", first)
	}
	confirm := ConfirmRepairCommand{OrganizationKey: command.OrganizationKey, ApplicationKey: command.ApplicationKey, EnvironmentKey: command.EnvironmentKey, ActorID: command.ActorID, ReviewID: first.Review.ID, ReviewToken: first.ReviewToken, RequestID: ids.New()}
	if _, err = s.ConfirmRepair(ctx, confirm); !errors.Is(err, tfexec.ErrRepairBlocked) {
		t.Fatal("destructive plan accepted without acknowledgement", err)
	}
	confirm.ConfirmDestructive = true
	opID, err := s.ConfirmRepair(ctx, confirm)
	if err != nil {
		t.Fatal(err)
	}
	op, err := s.Store.GetOperation(ctx, opID)
	if err != nil || op.Stage != "DRIFT_REPAIR_REVIEW_REQUIRED" || op.Status != environment.OpFailed {
		t.Fatal("partial repair reported success", op, err)
	}
	if _, err = s.ConfirmRepair(ctx, confirm); err != nil || len(f.applied) != 1 {
		t.Fatal("idempotent confirmation reapplied", err)
	}
	command.PreviousReviewID = first.Review.ID
	command.RequestID = ids.New()
	second, err := s.PrepareRepair(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Review.Instances[0].Completed || second.Review.Instances[1].Deferred {
		t.Fatal("completed provider not retained", second)
	}
	confirm.ReviewID = second.Review.ID
	confirm.ReviewToken = second.ReviewToken
	confirm.RequestID = ids.New()
	finalID, err := s.ConfirmRepair(ctx, confirm)
	if err != nil {
		t.Fatal(err)
	}
	final, _ := s.Store.GetOperation(ctx, finalID)
	if final.Status != environment.OpSucceeded || len(f.applied) != 2 || f.applied[0] == f.applied[1] {
		t.Fatal("dependency repair did not finish exactly once", final, f.applied)
	}
}
func TestRepairContinuationRejectsChangedCompletedPins(t *testing.T) {
	s, f, cmd := repairTestService(t)
	ctx := context.Background()
	view, err := s.PrepareRepair(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ConfirmRepair(ctx, ConfirmRepairCommand{OrganizationKey: cmd.OrganizationKey, ApplicationKey: cmd.ApplicationKey, EnvironmentKey: cmd.EnvironmentKey, ActorID: cmd.ActorID, ReviewID: view.Review.ID, ReviewToken: view.ReviewToken, RequestID: ids.New(), ConfirmDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	inst := f.instances[f.root]
	inst.Version++
	f.instances[f.root] = inst
	cmd.PreviousReviewID = view.Review.ID
	cmd.RequestID = ids.New()
	if _, err := s.PrepareRepair(ctx, cmd); !errors.Is(err, tfexec.ErrRepairStale) {
		t.Fatal("stale completed checkpoint reused", err)
	}
}

func TestRepairScopeRejectsForeignEnvironmentAndDependencyCycles(t *testing.T) {
	for _, scenario := range []string{"foreign-environment", "cycle"} {
		t.Run(scenario, func(t *testing.T) {
			s, f, cmd := repairTestService(t)
			child := f.edges[0].ConsumerID
			if scenario == "foreign-environment" {
				inst := f.instances[child]
				inst.Identity.EnvironmentID = ids.New()
				f.instances[child] = inst
			} else {
				f.edges = append(f.edges, tfresource.Dependency{ConsumerID: f.root, ProviderID: child, ConsumerStatus: tfresource.InstanceReady})
			}
			if _, err := s.PrepareRepair(context.Background(), cmd); err == nil {
				t.Fatal("unsafe repair scope accepted")
			}
			if len(f.prepared) > 0 || len(f.applied) > 0 {
				t.Fatal("unsafe scope reached provider planning")
			}
		})
	}
}
func TestRepairConfirmationRejectsAnUpdatedScopeBeforeApplying(t *testing.T) {
	s, f, cmd := repairTestService(t)
	view, err := s.PrepareRepair(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	child := f.edges[0].ConsumerID
	inst := f.instances[child]
	inst.Version++
	f.instances[child] = inst
	_, err = s.ConfirmRepair(context.Background(), ConfirmRepairCommand{OrganizationKey: cmd.OrganizationKey, ApplicationKey: cmd.ApplicationKey, EnvironmentKey: cmd.EnvironmentKey, ActorID: cmd.ActorID, ReviewID: view.Review.ID, ReviewToken: view.ReviewToken, RequestID: ids.New(), ConfirmDestructive: true})
	if !errors.Is(err, tfexec.ErrRepairStale) || len(f.applied) > 0 {
		t.Fatal("stale dependent scope applied", err)
	}
}

type repairConsumerFixture struct {
	f            *repairFixture
	refs         []domain.RepairConsumer
	refreshes    int
	dependencies map[string][]string
}

func (c *repairConsumerFixture) PreviewRepairConsumers(context.Context, string, string, string, []string) ([]domain.RepairConsumer, error) {
	return append([]domain.RepairConsumer{}, c.refs...), nil
}
func (c *repairConsumerFixture) RefreshRepairConsumers(_ context.Context, _, _, _, _ string, refs []domain.RepairConsumer) error {
	if len(c.f.applied) != 1 {
		return errors.New("consumer ran before its provider")
	}
	c.refreshes++
	for n := range c.refs {
		for _, ref := range refs {
			if ref.Descriptor == c.refs[n].Descriptor {
				c.refs[n].Version++
			}
		}
	}
	return nil
}
func (c *repairConsumerFixture) RepairTopology(context.Context, string, string, string, []string) (map[string][]string, []string, error) {
	out := []string{}
	for id := range c.f.instances {
		out = append(out, id)
	}
	return c.dependencies, out, nil
}
func TestRepairStagesDependenciesThroughNonTerraformConsumer(t *testing.T) {
	s, f, cmd := repairTestService(t)
	ctx := context.Background()
	child := f.edges[0].ConsumerID
	rootName, childName := f.instances[f.root].Identity.Descriptor, f.instances[child].Identity.Descriptor
	bridge := "workload.default#bridge"
	consumers := &repairConsumerFixture{f: f, refs: []domain.RepairConsumer{{Descriptor: bridge, WorkloadID: "bridge", DeploymentID: ids.New(), Version: 1}}, dependencies: map[string][]string{bridge: {rootName}, childName: {bridge}}}
	s.Consumers = consumers
	first, err := s.PrepareRepair(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	confirm := ConfirmRepairCommand{OrganizationKey: cmd.OrganizationKey, ApplicationKey: cmd.ApplicationKey, EnvironmentKey: cmd.EnvironmentKey, ActorID: cmd.ActorID, ReviewID: first.Review.ID, ReviewToken: first.ReviewToken, RequestID: ids.New(), ConfirmDestructive: true}
	if _, err := s.ConfirmRepair(ctx, confirm); err != nil {
		t.Fatal(err)
	}
	if consumers.refreshes != 1 || len(f.applied) != 1 {
		t.Fatal("provider/consumer stage order incorrect")
	}
	cmd.PreviousReviewID = first.Review.ID
	cmd.RequestID = ids.New()
	next, err := s.PrepareRepair(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if !next.Review.Consumers[0].Completed || next.Review.Instances[1].Deferred {
		t.Fatal("non-Terraform checkpoint was not restored")
	}
	confirm.ReviewID = next.Review.ID
	confirm.ReviewToken = next.ReviewToken
	confirm.RequestID = ids.New()
	if _, err := s.ConfirmRepair(ctx, confirm); err != nil {
		t.Fatal(err)
	}
	if consumers.refreshes != 1 || len(f.applied) != 2 {
		t.Fatal("completed consumer replayed or dependent Terraform not repaired")
	}
}
