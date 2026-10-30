package tfexec

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"orchestrator/internal/domain/drift"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformexec"
	"orchestrator/internal/ports/terraformstate"
)

var ErrBaselineUnavailable = errors.New("tfexec: acknowledged execution baseline unavailable")

// InspectInstance plans against a private copy of acknowledged state and exact
// execution inputs. It cannot apply, adopt refreshed state or publish outputs.
// The caller owns the Environment operation throughout this bounded invocation.
func (s *Service) InspectInstance(ctx context.Context, org, instanceID string) (report drift.Report, err error) {
	return s.inspectInstance(ctx, org, instanceID, nil)
}

type inspectedBaseline struct {
	Baseline   terraformstate.ExecutionBaseline
	Input      baselinePayload
	Instance   tfresource.Instance
	State      *tfresource.StateBlob
	PlainState []byte
	Bound      terraformexec.ProviderEnv
	Retain     func(string)
}

func (s *Service) inspectInstance(ctx context.Context, org, instanceID string, after func(context.Context, inspectedBaseline, terraformexec.Session, drift.Report) error) (report drift.Report, err error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if s.deps.Baselines == nil || s.deps.Cipher == nil {
		return report, ErrBaselineUnavailable
	}
	owner, ok := persistence.OwnerFrom(ctx)
	if !ok {
		return report, ErrNoClaim
	}
	instance, err := s.deps.State.GetInstance(ctx, instanceID)
	if err != nil {
		return report, err
	}
	if instance.Identity.OrganizationID != org || (instance.Status != tfresource.InstanceReady && instance.Status != tfresource.InstanceUnreferenced) {
		return report, ErrBaselineUnavailable
	}
	baseline, err := s.deps.Baselines.LatestExecutionBaseline(ctx, org, instanceID)
	if err != nil {
		return report, ErrBaselineUnavailable
	}
	a, err := s.deps.Baselines.ReadBaselineArtifact(ctx, org, baseline.ArtifactID)
	if err != nil || a.OrganizationID != org || a.ID != baseline.ArtifactID || a.Purpose != "TERRAFORM_BASELINE" || a.OwnerID != instance.ID || a.AADVersion != 1 {
		return report, ErrBaselineUnavailable
	}
	raw, err := s.deps.Cipher.Open(a.KeyID, a.Nonce, a.Ciphertext, baselineAAD(a))
	if err != nil {
		return report, ErrBaselineUnavailable
	}
	var input baselinePayload
	if json.Unmarshal(raw, &input) != nil || input.Request.Identity != instance.Identity || baseline.ModuleBundleID != instance.ModuleBundleID {
		return report, ErrBaselineUnavailable
	}
	token, _, err := s.deps.Tokens.IssueCapability(ctx, instance.ID, tfresource.CapabilityPlan, 15*time.Minute)
	if err != nil {
		return report, err
	}
	sum := sha256.Sum256([]byte(token))
	lockID := ids.New()
	var snapshot *tfresource.StateBlob
	err = s.deps.State.Transact(ctx, func(ctx context.Context) error {
		if _, err := s.deps.State.AuthorizeForUpdate(ctx, hex.EncodeToString(sum[:]), instance.StateKey, s.now()); err != nil {
			return err
		}
		if held, err := s.deps.State.LockForUpdate(ctx, instance.ID); err != nil {
			return err
		} else if held != nil {
			return terraformstate.ErrLockConflict
		}
		snapshot, err = s.deps.State.StateForUpdate(ctx, instance.ID)
		if err != nil {
			return err
		}
		if snapshot == nil || snapshot.Version != baseline.StateVersion || snapshot.Lineage != baseline.Lineage || snapshot.Serial != baseline.Serial {
			return ErrBaselineUnavailable
		}
		for _, pin := range baseline.Dependencies {
			provider, err := s.deps.State.GetInstance(ctx, pin.InstanceID)
			if err != nil {
				return err
			}
			state, err := s.deps.State.StateForUpdate(ctx, pin.InstanceID)
			if err != nil {
				return err
			}
			version := int64(0)
			if state != nil {
				version = state.Version
			}
			if provider.Version != pin.Version || version != pin.StateVersion {
				if !s.stableDependencyOutputs(ctx, org, pin, provider, state) {
					return ErrBaselineUnavailable
				}
			}
		}
		inserted, err := s.deps.State.InsertLock(ctx, tfresource.Lock{InstanceID: instance.ID, LockID: lockID, OperationID: owner.OperationID, Owner: owner.Owner, Fence: owner.Fence, AcquiredAt: s.now(), Deadline: s.now().Add(10 * time.Minute)})
		if err != nil {
			return err
		}
		if !inserted {
			return terraformstate.ErrLockConflict
		}
		return nil
	})
	if err != nil {
		return report, err
	}
	stopped := true
	defer func() {
		if !stopped {
			persistence.MarkHold(ctx)
			return
		}
		cleanup, cancel := persistCtx(ctx)
		defer cancel()
		if unlockErr := s.deps.State.Transact(cleanup, func(ctx context.Context) error {
			if _, err := s.deps.State.AuthorizeForUpdate(ctx, hex.EncodeToString(sum[:]), instance.StateKey, s.now()); err != nil {
				return err
			}
			return s.deps.State.DeleteLock(ctx, instance.ID, lockID, owner.OperationID, owner.Fence)
		}); unlockErr != nil {
			persistence.MarkHold(ctx)
			err = unlockErr
		}
	}()
	state, err := s.deps.Cipher.Open(snapshot.KeyID, snapshot.Nonce, snapshot.Ciphertext, tfresource.StateAAD(org, instance.StateKey, snapshot.KeyID, snapshot.AADRevision))
	if err != nil {
		return report, ErrBaselineUnavailable
	}
	bundle, err := s.deps.Modules.BundleByID(ctx, baseline.ModuleBundleID)
	if err != nil {
		return report, err
	}
	art, err := s.deps.Modules.VerifyHistoricalForExecution(ctx, bundle)
	if err != nil {
		return report, err
	}
	defer func() {
		if stopped {
			_ = art.Close()
		}
	}()
	bound, err := s.deps.Binder.Bind(ctx, terraformexec.BindRequest{OrganizationKey: input.Request.OrganizationKey, Pins: input.Request.Pins, Targets: input.Request.Targets})
	if err != nil {
		return report, err
	}
	defer func() {
		if stopped && bound.Close != nil {
			_ = bound.Close()
		}
	}()
	deadline := s.now().Add(10 * time.Minute)
	session, err := s.deps.Runner.Open(ctx, terraformexec.SessionRequest{Artifact: &art, LocalState: state, Variables: input.Variables, Env: bound.Env, Deadline: deadline})
	if err != nil {
		return report, err
	}
	defer func() {
		if stopped {
			_ = session.Close()
		}
	}()
	retried := false
	step := func(call func(context.Context) error) error {
		for attempt := 0; attempt < 2; attempt++ {
			err := call(ctx)
			var failure *terraformexec.StepError
			if errors.As(err, &failure) {
				if !failure.Stopped {
					stopped = false
					handle := "retained-drift-" + owner.OperationID + "-" + instance.ID
					if art.Retain != nil {
						_ = art.Retain(handle)
					}
					if bound.Retain != nil {
						_ = bound.Retain(handle)
					}
				}
				if !retried && attempt == 0 && failure.Class == terraformexec.ClassTransient && failure.Stopped && failure.Outcome == terraformexec.OutcomeConfirmed {
					retried = true
					if err := s.sleep(ctx, s.cfg.RetryBackoff); err != nil {
						return err
					}
					continue
				}
			}
			return err
		}
		return ErrExecution
	}
	if err := step(session.Init); err != nil {
		return report, describeStep("drift init", err)
	}
	if err := step(func(ctx context.Context) error { _, err := session.Plan(ctx, terraformexec.PlanApply); return err }); err != nil {
		return report, describeStep("drift plan", err)
	}
	inspection, ok := session.(terraformexec.DriftPlanSession)
	if !ok {
		return report, ErrBaselineUnavailable
	}
	if err := step(func(ctx context.Context) error { report, err = inspection.InspectDrift(ctx); return err }); err != nil {
		return drift.Report{}, describeStep("drift inspection", err)
	}
	if after != nil {
		err = after(ctx, inspectedBaseline{Baseline: baseline, Input: input, Instance: instance, State: snapshot, PlainState: state, Bound: bound, Retain: func(handle string) {
			stopped = false
			if art.Retain != nil {
				_ = art.Retain(handle)
			}
			if bound.Retain != nil {
				_ = bound.Retain(handle)
			}
		}}, session, report)
	}
	return report, err
}
