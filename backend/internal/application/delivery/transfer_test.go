package delivery

import (
	"context"
	"encoding/json"
	"errors"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
	"testing"
	"time"
)

type transferFake struct {
	*neutralProvider
	live              map[string]string
	released, adopted int
	jobs              int
	adoptFailure      error
}

func (p *transferFake) Submit(ctx context.Context, i port.Intent, prepared port.Prepared) (port.Receipt, error) {
	r, err := p.neutralProvider.Submit(ctx, i, prepared)
	if err != nil {
		return r, err
	}
	refs, err := p.ExportArtifact(ctx, prepared.Payload)
	if err != nil {
		return r, err
	}
	inventory := []port.AppliedObject{}
	for _, m := range refs {
		ref := execution.ObjectRef{APIVersion: m.APIVersion, Kind: m.Kind, Name: m.Name}
		uid := p.live[ref.Key()]
		if uid == "" {
			uid = ids.New()
			p.live[ref.Key()] = uid
		}
		inventory = append(inventory, port.AppliedObject{ObjectRef: ref, UID: uid})
	}
	raw, _ := json.Marshal(inventory)
	r.Handle["inventory"] = string(raw)
	if i.Revision.Phase == domain.Job {
		p.jobs++
	}
	return r, nil
}
func (p *transferFake) Observe(_ context.Context, i port.Intent, r port.Receipt) (domain.Observation, error) {
	var inventory []port.AppliedObject
	if json.Unmarshal([]byte(r.Handle["inventory"]), &inventory) != nil {
		return domain.Observation{}, ErrArtifact
	}
	out := domain.Observation{Scope: i.Revision.Scope, IntendedRevision: i.Revision.ID, ObservedRevision: r.RevisionID, PayloadDigest: r.PayloadDigest, Sync: domain.InSync, Health: domain.Healthy, Drift: domain.NoDrift, Complete: true, ObservedAt: time.Now().UTC()}
	for _, o := range inventory {
		out.Objects = append(out.Objects, domain.ObjectObservation{APIVersion: o.APIVersion, Kind: o.Kind, Name: o.Name, UID: p.live[o.Key()], Namespace: i.Target.Namespace})
	}
	return out, nil
}
func (p *transferFake) ExportArtifact(_ context.Context, raw []byte) ([]execution.Manifest, error) {
	var objects []map[string]any
	if json.Unmarshal(raw, &objects) != nil {
		return nil, ErrArtifact
	}
	out := []execution.Manifest{}
	for _, obj := range objects {
		md, _ := obj["metadata"].(map[string]any)
		out = append(out, execution.Manifest{APIVersion: obj["apiVersion"].(string), Kind: obj["kind"].(string), Name: md["name"].(string), Namespace: md["namespace"].(string), Object: obj})
	}
	return out, nil
}
func (p *transferFake) ValidateTransfer(_ context.Context, _ port.Intent, _ port.Receipt, inventory []port.AppliedObject) error {
	for _, o := range inventory {
		if p.live[o.Key()] != o.UID {
			return domain.ErrStale
		}
	}
	return nil
}
func (p *transferFake) Withdraw(ctx context.Context, i port.Intent, r port.Receipt) (port.WithdrawalProof, error) {
	if err := p.guard(ctx, i); err != nil {
		return port.WithdrawalProof{}, err
	}
	p.released++
	return port.WithdrawalProof{Receipt: r, ManagementStopped: true}, nil
}
func (p *transferFake) Adopt(ctx context.Context, i port.Intent, _ port.Prepared, source port.TransferSource) (port.Receipt, error) {
	if err := p.guard(ctx, i); err != nil {
		return port.Receipt{}, err
	}
	if p.adoptFailure != nil {
		err := p.adoptFailure
		p.adoptFailure = nil
		return port.Receipt{}, err
	}
	p.adopted++
	raw, _ := json.Marshal(source.Inventory)
	return port.Receipt{IntentID: i.ID, RevisionID: i.Revision.ID, PayloadDigest: i.Revision.PayloadDigest, Handle: map[string]string{"inventory": string(raw)}}, nil
}
func TestTransferRoundTripPreservesUIDsAndDoesNotResubmitJobs(t *testing.T) {
	c, original, ctx, req, _ := coordinatorFixture(t)
	p := &transferFake{neutralProvider: original, live: map[string]string{}}
	c.Deadline = 2 * time.Second
	c.PollInterval = time.Millisecond
	c.Registry = port.NewRegistry()
	if err := c.Registry.Register("fleet", p); err != nil {
		t.Fatal(err)
	}
	if err := c.Registry.Register("direct", p); err != nil {
		t.Fatal(err)
	}
	for n := range req.Manifests {
		m := &req.Manifests[n]
		m.APIVersion = "v1"
		if m.Kind == "Job" {
			m.APIVersion = "batch/v1"
		}
		if m.Kind == "Deployment" {
			m.APIVersion = "apps/v1"
		}
		m.Name = "app"
		m.Namespace = "app"
		m.Object = map[string]any{"apiVersion": m.APIVersion, "kind": m.Kind, "metadata": map[string]any{"name": m.Name, "namespace": m.Namespace}}
	}
	if err := c.Deploy(ctx, req); err != nil {
		t.Fatal(err)
	}
	if err := c.Activate(ctx, req.ApplicationKey, req.Environment); err != nil {
		t.Fatal(err)
	}
	org, err := c.Store.GetOrganization(ctx, req.OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	originalUIDs, _ := json.Marshal(p.live)
	selection := req.Environment.DesiredDelivery
	stages := []string{}
	for _, next := range []domain.Selection{{Mode: domain.Direct}, selection} {
		owner, _ := persistence.OwnerFrom(ctx)
		if err := c.Store.ReleaseOperation(ctx, owner, environment.OpSucceeded, ""); err != nil {
			t.Fatal(err)
		}
		env, err := c.Store.GetEnvironment(context.Background(), req.ApplicationKey, req.Environment.Key)
		if err != nil {
			t.Fatal(err)
		}
		repo := c.Store.(persistence.DeliverySelectionRepository)
		env, err = repo.SelectDelivery(context.Background(), persistence.DeliverySelectionCommand{OrganizationID: org.ID, ApplicationKey: req.ApplicationKey, EnvironmentKey: env.Key, ExpectedEnvironmentVersion: env.Version, ExpectedConfigurationRevision: env.DeliveryConfigRevision, Selection: next})
		if err != nil {
			t.Fatal(err)
		}
		op, err := c.Store.ClaimEnvironment(context.Background(), persistence.OperationClaim{ID: ids.New(), ApplicationKey: req.ApplicationKey, EnvironmentKey: env.Key, Owner: "transfer", Kind: environment.OpDeploy, Deadline: time.Now().Add(time.Minute)})
		if err != nil {
			t.Fatal(err)
		}
		ctx = persistence.WithOwner(context.Background(), persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence})
		stage := func(stage string, _ map[string]any) error { stages = append(stages, stage); return nil }
		if next.Mode == domain.Direct {
			p.adoptFailure = errors.New("lost adoption acknowledgement")
			if err := c.Transfer(ctx, req.OrganizationKey, env, stage); err == nil {
				t.Fatal("interrupted transfer reported success")
			}
			owner, _ := persistence.OwnerFrom(ctx)
			if err := c.Store.SuspendOperation(ctx, owner, "transfer interrupted"); err != nil {
				t.Fatal(err)
			}
			recovered, err := c.Store.BeginRecovery(context.Background(), owner.OperationID, "new-transfer-owner", "operator")
			if err != nil {
				t.Fatal(err)
			}
			ctx = persistence.WithOwner(context.Background(), persistence.Owner{OperationID: recovered.ID, Owner: recovered.Owner, Fence: recovered.Fence})
			if err := c.ResumeTransfer(ctx, req.OrganizationKey, env, stage); err != nil {
				t.Fatal("forward recovery failed", err)
			}
		} else if err := c.Transfer(ctx, req.OrganizationKey, env, stage); err != nil {
			t.Fatal(err)
		}

		applied, err := c.Store.GetEnvironment(ctx, req.ApplicationKey, env.Key)
		if err != nil || applied.AppliedDelivery != next || applied.DeliveryEpoch != env.DeliveryEpoch+1 {
			t.Fatal("transfer acknowledgement not committed", err)
		}
	}
	currentUIDs, _ := json.Marshal(p.live)
	if string(originalUIDs) != string(currentUIDs) || p.jobs != 1 || len(p.order) != 3 || p.adopted != 6 || p.released != 6 {
		t.Fatal("transfer recreated or resubmitted workload", p.jobs, p.order, p.adopted, p.released)
	}
	if len(stages) != 12 || stages[0] != "RELEASING" || stages[6] != "COMPLETED" {
		t.Fatal("transfer stages missing", stages)
	}
}

func (p *transferFake) ValidateInventory(ctx context.Context, i port.Intent, r port.Receipt, objects []port.AppliedObject) error {
	return p.ValidateTransfer(ctx, i, r, objects)
}
