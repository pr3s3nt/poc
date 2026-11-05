package tfexec_test

import (
	"context"
	"encoding/json"
	"errors"
	"orchestrator/internal/ports/terraformexec"
	"strings"
	"testing"

	driftapp "orchestrator/internal/application/drift"
	"orchestrator/internal/application/envops"
	"orchestrator/internal/application/tfexec"
	"orchestrator/internal/domain/drift"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/platform/ids"
)

// Actual PostgreSQL/encryption/Terraform planning, with a clearly FAKE drift
// observation. This proves admission fences, not provider repair conformance.
func TestRepairRejectsChangedPrivateValuesBeforeApplyDespiteSamePublicSummary(t *testing.T) {
	h := newHarness(t)
	ctx, op := h.claim("create-baseline")
	created := mustExecute(t, h, ctx, h.request("create-baseline", h.mod, inputs("database", "stable")))
	h.release(op, environment.OpSucceeded)
	digest := strings.Repeat("a", 64)
	h.runner.onDrift = func(drift.Report) (drift.Report, error) {
		return drift.Report{Complete: true, Applyable: true, Proposed: []drift.Finding{{Address: "terraform_data.db", Type: "terraform_data", Actions: []string{"update"}, Paths: []string{"/input"}}}, PrivateChangeDigest: digest}, nil
	}
	ctx, preview := h.claim("repair-preview")
	prepared, err := h.svc.PrepareInstanceRepair(ctx, h.orgID, created.InstanceID)
	if err != nil || len(prepared.Blockers) > 0 || prepared.ArtifactID == "" {
		t.Fatal("repair fixture preparation failed", err)
	}
	publicBefore, _ := json.Marshal(prepared.Report)
	if strings.Contains(string(publicBefore), digest) || strings.Contains(string(publicBefore), prepared.ArtifactID) {
		t.Fatal("private repair proof escaped review")
	}
	h.release(preview, environment.OpSucceeded)
	digest = strings.Repeat("b", 64)
	publicAfter, _ := json.Marshal(drift.Report{Complete: true, Applyable: true, Proposed: prepared.Report.Proposed, PrivateChangeDigest: digest})
	if string(publicBefore) != string(publicAfter) {
		t.Fatal("fixture no longer proves equal public summaries")
	}
	ctx, confirm := h.claim("repair-confirm")
	before := h.runner.applies
	if err := h.svc.RepairInstance(ctx, h.orgID, created.InstanceID, prepared.ArtifactID); !errors.Is(err, tfexec.ErrRepairStale) {
		t.Fatal("changed private repair values admitted", err)
	}
	if h.runner.applies != before || h.count(`SELECT count(*) FROM terraform_state_locks`) != 0 {
		t.Fatal("rejected repair applied or leaked its lock")
	}
	h.release(confirm, environment.OpFailed)
}

func TestRepairPreviewBlocksUnchangedActualPlanWithoutInventingAnApply(t *testing.T) {
	h := newHarness(t)
	ctx, op := h.claim("baseline")
	created := mustExecute(t, h, ctx, h.request("baseline", h.mod, inputs("database", "stable")))
	h.release(op, environment.OpSucceeded)
	ctx, review := h.claim("preview-no-drift")
	before := h.runner.applies
	prepared, err := h.svc.PrepareInstanceRepair(ctx, h.orgID, created.InstanceID)
	if err != nil || len(prepared.Blockers) == 0 || prepared.ArtifactID != "" || h.runner.applies != before {
		t.Fatal("unchanged plan admitted as repair", err)
	}
	h.release(review, environment.OpSucceeded)
}

// The provider observation is explicitly injected into a disposable State copy.
// Plan, saved-plan apply, output validation, encryption and PostgreSQL ack are real.
func TestRepairRealSavedPlanRecreatesAndReplacesWithChangedOutputs(t *testing.T) {
	for _, scenario := range []string{"recreate", "replace"} {
		t.Run(scenario, func(t *testing.T) {
			h := newHarness(t)
			ctx, op := h.claim("baseline")
			created := mustExecute(t, h, ctx, h.request("baseline", h.mod, inputs("database", "stable")))
			h.release(op, environment.OpSucceeded)
			h.runner.onLocalState = func(raw []byte) []byte {
				var state map[string]any
				if json.Unmarshal(raw, &state) != nil {
					t.Fatal("invalid State fixture")
				}
				resources := state["resources"].([]any)
				kept := []any{}
				for _, entry := range resources {
					r := entry.(map[string]any)
					if r["name"] == "db" {
						if scenario == "recreate" {
							continue
						}
						for _, instance := range r["instances"].([]any) {
							attributes := instance.(map[string]any)["attributes"].(map[string]any)
							if dynamic, ok := attributes["triggers_replace"].(map[string]any); ok {
								dynamic["value"] = "external-change"
							} else {
								attributes["triggers_replace"] = "external-change"
							}
						}
					}
					kept = append(kept, entry)
				}
				state["resources"] = kept
				result, err := json.Marshal(state)
				if err != nil {
					t.Fatal(err)
				}
				return result
			}
			ctx, preview := h.claim("preview")
			prepared, err := h.svc.PrepareInstanceRepair(ctx, h.orgID, created.InstanceID)
			if err != nil || len(prepared.Blockers) > 0 || prepared.ArtifactID == "" || !prepared.Report.ChangesOutputs() {
				t.Fatalf("full repair not prepared: %+v %v", prepared, err)
			}
			found := false
			for _, f := range prepared.Report.Proposed {
				if f.Address == "terraform_data.db" {
					for _, a := range f.Actions {
						if a == "create" {
							found = true
						}
					}
				}
			}
			if !found {
				t.Fatal("recreation/replacement not present in actual Terraform plan")
			}
			h.release(preview, environment.OpSucceeded)
			before := h.runner.applies
			ctx, confirm := h.claim("confirm")
			if err := h.svc.RepairInstance(ctx, h.orgID, created.InstanceID, prepared.ArtifactID); err != nil {
				t.Fatal("reviewed full repair failed", err)
			}
			if h.runner.applies != before+1 {
				t.Fatal("saved plan not applied exactly once")
			}
			h.runner.onLocalState = nil
			_, ack, err := h.svc.AcknowledgedRepairResult(ctx, created.InstanceID)
			if err != nil || ack.Outputs["host"] == created.Outputs["host"] {
				t.Fatal("changed outputs not acknowledged", err)
			}
			h.release(confirm, environment.OpSucceeded)
			ctx, scan := h.claim("scan")
			report, err := h.svc.InspectInstance(ctx, h.orgID, created.InstanceID)
			if err != nil || len(report.Proposed) > 0 || len(report.OutputNames) > 0 {
				t.Fatal("post-repair plan not converged", err)
			}
			h.release(scan, environment.OpSucceeded)
		})
	}
}

func TestRepairFailureAfterApplyRetainsHoldAndDoesNotRetry(t *testing.T) {
	h := newHarness(t)
	ctx, op := h.claim("baseline")
	created := mustExecute(t, h, ctx, h.request("baseline", h.mod, inputs("database", "stable")))
	h.release(op, environment.OpSucceeded)
	h.runner.onDrift = func(drift.Report) (drift.Report, error) {
		return drift.Report{Complete: true, Applyable: true, Proposed: []drift.Finding{{Address: "terraform_data.db", Type: "terraform_data", Actions: []string{"update"}}}, PrivateChangeDigest: strings.Repeat("a", 64)}, nil
	}
	ctx, preview := h.claim("preview")
	prepared, err := h.svc.PrepareInstanceRepair(ctx, h.orgID, created.InstanceID)
	if err != nil {
		t.Fatal(err)
	}
	h.release(preview, environment.OpSucceeded)
	h.runner.onApply = func(_ int, s terraformexec.Session) error {
		if err := s.Apply(context.Background()); err != nil {
			return err
		}
		return errors.New("injected lost acknowledgement")
	}
	ctx, _ = h.claim("confirm")
	before := h.runner.applies
	if err := h.svc.RepairInstance(ctx, h.orgID, created.InstanceID, prepared.ArtifactID); !errors.Is(err, tfexec.ErrNeedsAttention) {
		t.Fatal("uncertain repair was not held", err)
	}
	if h.runner.applies != before+1 {
		t.Fatal("mutating repair retried")
	}
}

// Real State/Console PostgreSQL roles, encrypted reviews and saved Terraform
// apply through the production coordinator. Only the observed missing resource
// is simulated in its private copy, with no cloud/cluster mutation.
func TestRepairCoordinatorPersistsFullPlanConfirmationAndRescan(t *testing.T) {
	h := newHarness(t)
	ctx, op := h.claim("baseline")
	created := mustExecute(t, h, ctx, h.request("baseline", h.mod, inputs("database", "stable")))
	h.release(op, environment.OpSucceeded)
	actor := identity.UserAccount{ID: ids.New(), OrganizationKey: h.orgKey, Username: "repair-engineer", PasswordHash: "test-only-hash", Role: identity.RolePlatformEngineer, Status: identity.AccountActive}
	if err := h.store.SaveUserAccount(context.Background(), actor); err != nil {
		t.Fatal(err)
	}
	h.runner.onLocalState = func(raw []byte) []byte {
		var state map[string]any
		_ = json.Unmarshal(raw, &state)
		kept := []any{}
		for _, entry := range state["resources"].([]any) {
			if entry.(map[string]any)["name"] != "db" {
				kept = append(kept, entry)
			}
		}
		state["resources"] = kept
		result, _ := json.Marshal(state)
		return result
	}
	service := &driftapp.Service{Store: h.store, Instances: h.state, Inspector: h.svc, Operations: envops.NewManager(h.store), Consumers: emptyRepairConsumers{}}
	env, err := h.store.GetEnvironment(context.Background(), h.appKey, "staging")
	if err != nil {
		t.Fatal(err)
	}
	scan, err := service.Queue(context.Background(), driftapp.QueueCommand{OrganizationKey: h.orgKey, ApplicationKey: h.appKey, EnvironmentKey: "staging", RequestID: ids.New(), ExpectedEnvironmentVersion: env.Version})
	if err != nil {
		t.Fatal(err)
	}
	service.ProcessQueued(context.Background())
	view, err := service.PrepareRepair(context.Background(), driftapp.PrepareRepairCommand{OrganizationKey: h.orgKey, ApplicationKey: h.appKey, EnvironmentKey: "staging", ActorID: actor.ID, ScanID: scan.ID, InstanceIDs: []string{created.InstanceID}, RequestID: ids.New(), ExpectedEnvironmentVersion: env.Version})
	if err != nil || view.Review.Status != drift.Ready {
		t.Fatal("full review not persisted", view.Review.Status, err)
	}
	command := driftapp.ConfirmRepairCommand{OrganizationKey: h.orgKey, ApplicationKey: h.appKey, EnvironmentKey: "staging", ActorID: actor.ID, ReviewID: view.Review.ID, ReviewToken: view.ReviewToken, RequestID: ids.New()}
	// Recreate has no delete action and needs the ordinary explicit confirmation.
	before := h.runner.applies
	operation, err := service.ConfirmRepair(context.Background(), command)
	if err != nil {
		t.Fatal("confirmation failed", err)
	}
	repeated, err := service.ConfirmRepair(context.Background(), command)
	if err != nil || repeated != operation || h.runner.applies != before+1 {
		t.Fatal("confirmation was not idempotent", err)
	}
	h.runner.onLocalState = nil
	service.ProcessQueued(context.Background())
	scans, err := service.List(context.Background(), h.orgKey, h.appKey, "staging", "", 10)
	if err != nil || len(scans) < 2 || scans[0].Result != drift.InSync {
		t.Fatal("post-repair rescan did not converge", err)
	}
	stored, err := service.GetRepair(context.Background(), h.orgKey, h.appKey, "staging", actor.ID, view.Review.ID)
	if err != nil || stored.Review.Status != drift.Consumed || stored.ReviewToken != "" {
		t.Fatal("consumed review or token state incorrect", err)
	}
}

// Empty JSON collections round-trip through PostgreSQL as omitted/null fields;
// the immutable safe-scope check must treat these representations equally.
type emptyRepairConsumers struct{}

func (emptyRepairConsumers) PreviewRepairConsumers(context.Context, string, string, string, []string) ([]drift.RepairConsumer, error) {
	return []drift.RepairConsumer{}, nil
}
func (emptyRepairConsumers) RefreshRepairConsumers(context.Context, string, string, string, string, []drift.RepairConsumer) error {
	return errors.New("no consumer should execute")
}
func (emptyRepairConsumers) RepairTopology(context.Context, string, string, string, []string) (map[string][]string, []string, error) {
	return map[string][]string{}, []string{}, nil
}
