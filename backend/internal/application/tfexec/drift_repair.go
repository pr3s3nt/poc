package tfexec

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"orchestrator/internal/domain/drift"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformexec"
	"orchestrator/internal/ports/terraformstate"
)

var ErrRepairStale = errors.New("tfexec: repair review is stale")
var ErrRepairBlocked = errors.New("tfexec: repair requires normal deployment review")

type RepairPreparation struct {
	InstanceID      string       `json:"instanceId"`
	BaselineID      string       `json:"baselineId"`
	InstanceVersion int64        `json:"instanceVersion"`
	Report          drift.Report `json:"report"`
	Blockers        []string     `json:"blockers"`
	ArtifactID      string       `json:"-"`
}
type privateRepairPlan struct {
	Baseline             terraformstate.ExecutionBaseline
	InstanceVersion      int64
	Digest               string
	Report               drift.Report
	SavedPlan            []byte
	CreatedAt, ExpiresAt time.Time
}

func (s *Service) PrepareInstanceRepair(ctx context.Context, org, instance string) (prepared RepairPreparation, err error) {
	prepared.InstanceID = instance
	prepared.Blockers = []string{}
	repo, ok := s.deps.Baselines.(terraformstate.RepairArtifactRepository)
	if !ok {
		return prepared, ErrBaselineUnavailable
	}
	_, err = s.inspectInstance(ctx, org, instance, func(ctx context.Context, baseline inspectedBaseline, session terraformexec.Session, report drift.Report) error {
		prepared.BaselineID, prepared.InstanceVersion = baseline.Baseline.ID, baseline.Instance.Version
		prepared.Report = report
		prepared.Report.PrivateChangeDigest = ""
		if !report.InPlaceOnly() {
			prepared.Blockers = append(prepared.Blockers, "IN_PLACE_STABLE_OUTPUTS_REQUIRED")
			return nil
		}
		plan, err := session.(terraformexec.DriftPlanSession).ReadSavedPlan(ctx)
		if err != nil {
			return err
		}
		now := s.now()
		payload, err := json.Marshal(privateRepairPlan{Baseline: baseline.Baseline, InstanceVersion: baseline.Instance.Version, Digest: report.PrivateChangeDigest, Report: report, SavedPlan: plan, CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)})
		if err != nil {
			return ErrArtifactEncoding
		}
		artifact := persistence.CipherArtifact{ID: ids.New(), OrganizationID: org, OwnerID: instance, Purpose: "TERRAFORM_REVIEW", KeyID: s.deps.Cipher.KeyID(), AADVersion: 1, CreatedAt: now}
		artifact.Nonce, artifact.Ciphertext, err = s.deps.Cipher.Seal(payload, baselineAAD(artifact))
		if err != nil {
			return ErrArtifactEncoding
		}
		if err := repo.PutRepairArtifact(ctx, artifact); err != nil {
			return err
		}
		prepared.ArtifactID = artifact.ID
		return nil
	})
	return
}

var ErrArtifactEncoding = errors.New("tfexec: private repair artifact unavailable")

// RepairInstance applies only a freshly computed saved plan equivalent to the
// reviewed private changes. The local plan/apply is fenced by the original state
// lock; acknowledgement uses the existing State service capability protocol.
func (s *Service) RepairInstance(ctx context.Context, org, instance, artifactID string) error {
	repo, ok := s.deps.Baselines.(terraformstate.RepairArtifactRepository)
	if !ok || s.deps.StateWriter == nil {
		return ErrBaselineUnavailable
	}
	artifact, err := repo.ReadRepairArtifact(ctx, org, artifactID)
	if err != nil || artifact.OwnerID != instance || artifact.OrganizationID != org || artifact.ID != artifactID || artifact.Purpose != "TERRAFORM_REVIEW" || artifact.AADVersion != 1 {
		return ErrArtifactEncoding
	}
	plain, err := s.deps.Cipher.Open(artifact.KeyID, artifact.Nonce, artifact.Ciphertext, baselineAAD(artifact))
	if err != nil {
		return ErrArtifactEncoding
	}
	var review privateRepairPlan
	if json.Unmarshal(plain, &review) != nil || review.Baseline.InstanceID != instance || review.Baseline.OrganizationID != org || !s.now().Before(review.ExpiresAt) || review.CreatedAt.After(s.now()) || len(review.SavedPlan) == 0 {
		return ErrRepairStale
	}
	_, err = s.inspectInstance(ctx, org, instance, func(ctx context.Context, baseline inspectedBaseline, session terraformexec.Session, report drift.Report) error {
		if !s.now().Before(review.ExpiresAt) || !sameBaseline(review.Baseline, baseline.Baseline) || review.InstanceVersion != baseline.Instance.Version || report.PrivateChangeDigest == "" || report.PrivateChangeDigest != review.Digest {
			return ErrRepairStale
		}
		if !report.InPlaceOnly() {
			return ErrRepairBlocked
		}
		return s.applyInstanceRepair(ctx, baseline, session, report)
	})
	return err
}

func (s *Service) applyInstanceRepair(ctx context.Context, baseline inspectedBaseline, session terraformexec.Session, report drift.Report) error {
	owner, ok := persistence.OwnerFrom(ctx)
	if !ok {
		return ErrNoClaim
	}
	before, err := session.StateResources(ctx)
	if err != nil {
		return describeStep("repair inventory", err)
	}
	st := &execState{req: baseline.Input.Request, inst: baseline.Instance, action: tfresource.ActionUpdate, vars: variables{All: baseline.Input.Variables}, env: baseline.Bound, created: map[string]bool{}}
	st.bundle, err = s.deps.Modules.BundleByID(ctx, baseline.Baseline.ModuleBundleID)
	if err != nil {
		return err
	}
	st.attempt, err = s.deps.State.BeginAttempt(ctx, tfresource.Attempt{InstanceID: st.inst.ID, RunID: owner.OperationID, Action: tfresource.ActionUpdate, Ownership: tfresource.UpdatedByRun, Stage: "DRIFT_REPAIR", ModuleSnapshot: map[string]any{"bundleId": st.bundle.ID}, TargetSnapshot: st.inst.TargetRef, Deadline: s.now().Add(10 * time.Minute)})
	if err != nil {
		return err
	}
	prev := st.inst.Status
	fail := func(stage string, err error) error {
		var step *terraformexec.StepError
		errors.As(err, &step)
		if stage == StageApplying || stage == StageApplied || stage == StageOutputs || stage == StageReadiness {
			// Any partial repair remains held. Never retry apply or silently
			// acknowledge provider effects after a failed/uncertain mutation.
			baseline.Retain("retained-repair-" + st.attempt.ID)
			if state, readErr := session.PullState(context.WithoutCancel(ctx)); readErr == nil {
				if handle, saveErr := s.deps.Recovery.Save(context.WithoutCancel(ctx), st.inst.ID, st.attempt.ID, state); saveErr == nil {
					st.recovery = handle
				}
			}
			persistence.MarkHold(ctx)
			_ = s.finalizeAttempt(ctx, st, stage, tfresource.AttemptNeedsAttention, codeUnknown)
			_ = s.setInstance(ctx, st, tfresource.InstanceNeedsAttention)
			return ErrNeedsAttention
		}
		return s.settleFailure(ctx, st, prev, &stepFailure{stage: stage, step: step, err: err})
	}
	if err := s.setInstance(ctx, st, tfresource.InstanceProvisioning); err != nil {
		return fail(StagePlanned, err)
	}
	token, _, err := s.deps.Tokens.IssueCapability(ctx, st.inst.ID, tfresource.CapabilityUpdate, 15*time.Minute)
	if err != nil {
		return fail(StagePlanned, err)
	}
	st.token = token
	if err := s.progressStrict(ctx, st, StageApplying); err != nil {
		return fail(StagePlanned, err)
	}
	if err := session.Apply(ctx); err != nil {
		return fail(StageApplying, err)
	}
	outputs, err := session.Outputs(ctx)
	if err != nil {
		return fail(StageOutputs, err)
	}
	if !reflect.DeepEqual(outputs, baseline.Input.Outputs) {
		return fail(StageOutputs, ErrRepairBlocked)
	}
	resources, err := session.StateResources(ctx)
	if err != nil {
		return fail(StageOutputs, err)
	}
	if !samePhysicalInventory(before, resources) {
		return fail(StageOutputs, ErrRepairBlocked)
	}
	checked, err := checkOutputs(st.bundle, st.req.OutputMapping, st.req.Type, outputs, s.executorOutputs(st, outputs))
	if err != nil {
		return fail(StageOutputs, err)
	}
	if err := s.deps.Readiness.Check(ctx, terraformexec.ReadinessRequest{OrganizationKey: st.req.OrganizationKey, ResourceType: st.req.Type.Key, Module: st.bundle.Ref.ModuleID, Pins: st.req.Pins, Targets: st.req.Targets, Resources: resources, Outputs: checked.Module, Env: baseline.Bound.Env, Deadline: s.now().Add(s.cfg.ReadinessTimeout)}); err != nil {
		return fail(StageReadiness, err)
	}
	state, err := session.PullState(ctx)
	if err != nil {
		return fail(StageApplied, err)
	}
	lock, err := s.deps.State.Lock(ctx, st.inst.ID)
	if err != nil || lock == nil || lock.OperationID != owner.OperationID || lock.Fence != owner.Fence {
		return fail(StageApplied, persistence.ErrOperationLost)
	}
	if err := s.deps.StateWriter.Write(ctx, token, st.inst.StateKey, lock.LockID, state); err != nil {
		return fail(StageApplied, err)
	}
	if err := s.finalizeAttempt(ctx, st, StageReady, tfresource.AttemptSucceeded, ""); err != nil {
		return fail(StageApplied, err)
	}
	if err := s.setInstance(ctx, st, prev); err != nil {
		return fail(StageApplied, err)
	}
	if err := s.recordBaseline(ctx, st, invocationOutcome{outputs: outputs, resources: resources, applied: true}); err != nil {
		return fail(StageApplied, err)
	}
	return nil
}
func samePhysicalInventory(a, b []terraformexec.StateResource) bool {
	if len(a) != len(b) {
		return false
	}
	index := map[string]terraformexec.StateResource{}
	for _, resource := range a {
		index[resource.Address] = resource
	}
	for _, resource := range b {
		prior, exists := index[resource.Address]
		if !exists || prior != resource {
			return false
		}
	}
	return true
}
func sameBaseline(a, b terraformstate.ExecutionBaseline) bool {
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}
	right, err := json.Marshal(b)
	return err == nil && string(left) == string(right)
}
