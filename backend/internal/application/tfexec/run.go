package tfexec

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformexec"
)

// invocationOutcome is what one successful Terraform invocation produced.
type invocationOutcome struct {
	plan      terraformexec.Plan
	applied   bool
	outputs   map[string]terraformexec.Output
	resources []terraformexec.StateResource
}

// stepFailure is a failed invocation step plus everything needed to settle it
// safely. It never holds CLI output.
type stepFailure struct {
	stage string
	step  *terraformexec.StepError
	err   error
	// recovered means Terraform kept state locally because the backend refused
	// it; it is held for an operator.
	recovered bool
	// unknown means the effect of a mutating step is not proven by
	// acknowledged state: hold, never replay.
	unknown bool
}

func (f *stepFailure) Error() string { return f.err.Error() }
func (f *stepFailure) Unwrap() error { return f.err }

// run drives the instance through init, plan review, apply, outputs, state
// acknowledgement, contract checks and readiness. art is the first private
// artifact (owned by the caller); retries and sessions open their own.
func (s *Service) run(ctx context.Context, st *execState, art *tfmodule.VerifiedArtifact) (Result, error) {
	prev := st.inst.Status
	if err := s.setInstance(ctx, st, tfresource.InstanceProvisioning); err != nil {
		return Result{}, s.settleFailure(ctx, st, prev, &stepFailure{stage: StageIntent, err: fmt.Errorf("%w: mark the instance", ErrExecution)})
	}
	capAction := tfresource.CapabilityCreate
	if st.action == tfresource.ActionUpdate {
		capAction = tfresource.CapabilityUpdate
	}
	token, _, err := s.deps.Tokens.IssueCapability(ctx, st.inst.ID, capAction, s.capabilityTTL())
	if err != nil {
		return Result{}, s.settleFailure(ctx, st, prev, &stepFailure{stage: StageIntent, err: fmt.Errorf("%w: issue the state capability", ErrExecution)})
	}
	st.token = token

	var outcome invocationOutcome
	retried := false
	current := art
	for {
		outcome, err = s.invoke(ctx, st, current, retried)
		if current != art {
			_ = current.Close()
		}
		if err == nil {
			break
		}
		var failure *stepFailure
		if !errors.As(err, &failure) {
			return Result{}, s.settleFailure(ctx, st, prev, &stepFailure{stage: StagePlanning, err: err})
		}
		if !retried && retryable(failure) {
			// One bounded retry. A pre-apply failure (init, plan) had no
			// effect. After an APPLY failure a clean exit proves nothing about
			// the provider: the retry needs acknowledged state for this
			// attempt and every object the plan meant to create present in
			// that state; otherwise the outcome is unknown and the instance
			// is held (no second apply; orphan reconciliation is D13).
			if failure.stage == StageApplying {
				proven, perr := s.applyOutcomeProven(ctx, st)
				if perr != nil || !proven {
					failure.unknown = true
					return Result{}, s.settleFailure(ctx, st, prev, failure)
				}
			}
			retried = true
			s.progress(ctx, st, failure.stage, tfresource.AttemptRunning, codeTransient)
			if err := s.sleep(ctx, s.cfg.RetryBackoff); err != nil {
				return Result{}, s.settleFailure(ctx, st, prev, &stepFailure{stage: failure.stage, step: &terraformexec.StepError{Step: "retry", Class: terraformexec.ClassCancelled, Outcome: terraformexec.OutcomeUnknown}, err: failure.err})
			}
			next, verr := s.deps.Modules.VerifyForExecution(ctx, st.req.Source)
			if verr != nil {
				return Result{}, s.settleFailure(ctx, st, prev, &stepFailure{stage: failure.stage, err: fmt.Errorf("%w: verify the module artifact: %w", ErrExecution, verr)})
			}
			current = &next
			continue
		}
		return Result{}, s.settleFailure(ctx, st, prev, failure)
	}
	return s.publish(ctx, st, prev, outcome)
}

func (s *Service) capabilityTTL() time.Duration {
	ttl := 2*s.cfg.InvocationTimeout + s.cfg.RetryBackoff + 5*time.Minute
	if max := 6 * time.Hour; ttl > max {
		ttl = max
	}
	return ttl
}

// retryable reports whether a failure may be retried once: a confirmed
// transient failure of a step before or during apply. Authentication, input,
// destructive and unknown outcomes never retry.
func retryable(f *stepFailure) bool {
	return f.step != nil && f.step.Class == terraformexec.ClassTransient &&
		f.step.Outcome == terraformexec.OutcomeConfirmed && f.step.Stopped && !f.recovered
}

// invoke runs one session: init, plan, review, apply. The attempt stage is
// persisted before each boundary so a crash always leaves the last stage.
func (s *Service) invoke(ctx context.Context, st *execState, art *tfmodule.VerifiedArtifact, retried bool) (invocationOutcome, error) {
	mk := func(stage string, err error) *stepFailure {
		var step *terraformexec.StepError
		errors.As(err, &step)
		if step != nil && !step.Stopped {
			// An unconfirmed process may still use the private files: keep
			// the workspace, credentials and any recovery state.
			s.retainWorkspace(st, art)
		}
		return &stepFailure{stage: stage, step: step, err: describeStep(stage, err)}
	}
	fail := func(stage string, err error) (invocationOutcome, error) {
		return invocationOutcome{}, mk(stage, err)
	}
	sess, err := s.deps.Runner.Open(ctx, terraformexec.SessionRequest{
		Artifact: art, Backend: &terraformexec.Backend{Address: s.cfg.BackendBase + st.inst.StateKey, Token: st.token},
		Variables: st.vars.All, Env: st.env.Env, Deadline: s.now().Add(s.cfg.InvocationTimeout),
	})
	if err != nil {
		return fail(StageInit, err)
	}
	defer sess.Close()

	s.progress(ctx, st, StageInit, tfresource.AttemptRunning, "")
	if err := sess.Init(ctx); err != nil {
		return fail(StageInit, err)
	}
	if retried && st.action == tfresource.ActionCreate {
		// The first attempt may have created some objects; a retry may only
		// replace those, which are this run's own.
		resources, err := sess.StateResources(ctx)
		if err != nil {
			return fail(StageInit, err)
		}
		for _, r := range resources {
			st.created[r.Address] = true
		}
	}
	s.progress(ctx, st, StagePlanning, tfresource.AttemptRunning, "")
	plan, err := sess.Plan(ctx, terraformexec.PlanApply)
	if err != nil {
		return fail(StagePlanning, err)
	}
	if err := reviewPlan(st, plan); err != nil {
		return invocationOutcome{}, &stepFailure{stage: StagePlanned, err: err}
	}
	s.progress(ctx, st, StagePlanned, tfresource.AttemptRunning, "")

	out := invocationOutcome{plan: plan}
	st.plannedCreates = createdAddresses(plan)
	if plan.Mutating() {
		// The intent to apply is durable before the first provider effect.
		if err := s.progressStrict(ctx, st, StageApplying); err != nil {
			return invocationOutcome{}, &stepFailure{stage: StagePlanned, err: fmt.Errorf("%w: persist the apply stage", ErrExecution)}
		}
		if err := sess.Apply(ctx); err != nil {
			recovered := s.keepRecovery(ctx, st, sess, art)
			sf := mk(StageApplying, err)
			sf.recovered = recovered
			return invocationOutcome{}, sf
		}
		out.applied = true
		s.progress(ctx, st, StageApplied, tfresource.AttemptRunning, "")
	} else {
		s.progress(ctx, st, StageNoChange, tfresource.AttemptRunning, "")
	}
	outputs, err := sess.Outputs(ctx)
	if err != nil {
		return fail(StageOutputs, err)
	}
	out.outputs = outputs
	resources, err := sess.StateResources(ctx)
	if err != nil {
		return fail(StageOutputs, err)
	}
	out.resources = resources
	return out, nil
}

// describeStep turns a step error into a safe sentence.
func describeStep(stage string, err error) error {
	var step *terraformexec.StepError
	if errors.As(err, &step) {
		return fmt.Errorf("%w: %s failed (%s)", ErrExecution, stage, step.Class)
	}
	if errors.Is(err, terraformexec.ErrWorkspace) {
		return fmt.Errorf("%w: %s refused: the private workspace is not valid", ErrExecution, stage)
	}
	return fmt.Errorf("%w: %s failed", ErrExecution, stage)
}

// reviewPlan enforces the plan policy before apply. CREATE may only create
// (and, in a retry of its own partial create, change what it created);
// UPDATE never deletes or replaces an existing object.
func reviewPlan(st *execState, plan terraformexec.Plan) error {
	for _, change := range plan.Changes {
		for _, action := range change.Actions {
			switch action {
			case "create", "no-op", "read":
				continue
			}
			if st.action == tfresource.ActionCreate && st.created[change.Address] {
				continue
			}
			if st.action == tfresource.ActionUpdate && action == "update" {
				continue
			}
			return fmt.Errorf("%w: %s %s", ErrDestructivePlan, change.Type, action)
		}
	}
	return nil
}

// keepRecovery preserves the state Terraform kept locally when the backend
// refused it: encrypted into the private recovery store, handle audited. If
// the store fails the workspace is retained untouched (it may be the only
// copy). It reports whether recovery state existed.
func (s *Service) keepRecovery(ctx context.Context, st *execState, sess terraformexec.Session, art *tfmodule.VerifiedArtifact) bool {
	data, ok := sess.RecoveryState()
	if !ok {
		return false
	}
	pctx, cancel := persistCtx(ctx)
	defer cancel()
	handle, err := s.deps.Recovery.Save(pctx, st.inst.ID, st.attempt.ID, data)
	if err != nil {
		// The workspace may hold the only copy of the state: keep it under a
		// unique opaque handle an operator can resolve internally.
		s.retainWorkspace(st, art)
		return true
	}
	st.recovery = handle
	return true
}

// retainWorkspace keeps an invocation's private files (state, credentials, or
// a workspace an unconfirmed process may still use) and audits a unique
// opaque handle, "retained-<attempt ID>", mapped privately to the directories.
// No cache path or credential ever enters the audit handle.
func (s *Service) retainWorkspace(st *execState, art *tfmodule.VerifiedArtifact) {
	handle := "retained-" + st.attempt.ID
	if art != nil {
		if art.Retain != nil {
			_ = art.Retain(handle)
		}
		art.Cleanup = nil
	}
	if st.env.Retain != nil {
		_ = st.env.Retain(handle)
	}
	st.retain = true
	st.recovery = handle
}

// progress records a stage best-effort for informational boundaries.
func (s *Service) progress(ctx context.Context, st *execState, stage string, status tfresource.AttemptStatus, code string) {
	_ = s.write(ctx, st, stage, status, code)
}

// progressStrict records a stage that must be durable before an effect.
func (s *Service) progressStrict(ctx context.Context, st *execState, stage string) error {
	return s.write(ctx, st, stage, tfresource.AttemptRunning, "")
}

func (s *Service) write(ctx context.Context, st *execState, stage string, status tfresource.AttemptStatus, code string) error {
	pctx, cancel := persistCtx(ctx)
	defer cancel()
	updated, err := s.deps.State.UpdateAttempt(pctx, st.attempt.ID, tfresource.AttemptProgress{
		Stage: stage, Status: status, ObservedPhysicalIDs: st.observed, SafeErrorCode: code, RecoveryHandle: st.recovery,
	})
	if err == nil {
		st.attempt = updated
	}
	return err
}

// setInstance moves the instance to status with a version CAS, reloading once
// on a concurrent version change.
func (s *Service) setInstance(ctx context.Context, st *execState, status tfresource.InstanceStatus) error {
	pctx, cancel := persistCtx(ctx)
	defer cancel()
	for i := 0; i < 2; i++ {
		updated, err := s.deps.State.SetInstanceStatus(pctx, st.inst.ID, st.inst.Version, status)
		if err == nil {
			st.inst = updated
			return nil
		}
		if !errors.Is(err, persistence.ErrVersionConflict) {
			return err
		}
		latest, getErr := s.deps.State.GetInstance(pctx, st.inst.ID)
		if getErr != nil {
			return err
		}
		st.inst = latest
	}
	return persistence.ErrVersionConflict
}

// settleFailure records a failed execution and returns the error to surface.
// Unknown outcomes, a failed state push and an unstopped process hold the
// instance NEEDS_ATTENTION (the Environment stays held, no duplicate CREATE).
// A confirmed failure leaves the acknowledged state usable by cleanup and
// restores the previous status when nothing could have changed.
func (s *Service) settleFailure(ctx context.Context, st *execState, prev tfresource.InstanceStatus, f *stepFailure) error {
	// A clean CLI exit after a mutating step proves nothing about the
	// provider, whatever the error class and however many retries ran: every
	// failed APPLY/DESTROY must be accounted for by acknowledged state, or the
	// instance is held (no replay; orphan reconciliation is D13).
	if !f.unknown && !f.recovered && (f.stage == StageApplying || f.stage == StageDestroying) &&
		(f.step == nil || (f.step.Outcome == terraformexec.OutcomeConfirmed && f.step.Stopped)) {
		if !s.failedMutationProven(ctx, st, f.stage) {
			f.unknown = true
		}
	}
	hold := f.recovered || f.unknown || (f.step != nil && (f.step.Outcome == terraformexec.OutcomeUnknown || !f.step.Stopped)) ||
		(f.step != nil && f.step.Class == terraformexec.ClassBackend && f.stage == StageApplying)
	code := codeFailed
	switch {
	case f.step != nil && f.step.Class == terraformexec.ClassTimeout:
		code = codeTimeout
	case f.step != nil && f.step.Class == terraformexec.ClassCancelled:
		code = codeCancelled
	case f.step != nil && f.step.Class == terraformexec.ClassBackend:
		code = codeBackend
	case f.step != nil && f.step.Class == terraformexec.ClassAuth:
		code = codeAuth
	case f.step != nil && f.step.Class == terraformexec.ClassInput:
		code = codeInput
	case f.step != nil && f.step.Class == terraformexec.ClassTransient:
		code = codeTransient
	case errors.Is(f.err, ErrDestructivePlan):
		code = codeDestructive
	case errors.Is(f.err, ErrOutputContract):
		code = codeOutputs
	}
	if hold {
		if f.recovered {
			code = codeBackend
		} else if f.unknown {
			code = codeUnknown
		} else if f.step != nil && f.step.Outcome == terraformexec.OutcomeUnknown && code == codeFailed {
			code = codeUnknown
		}
		st.heldStage = f.stage
		persistence.MarkHold(ctx)
		_ = s.finalizeAttempt(ctx, st, f.stage, tfresource.AttemptNeedsAttention, code)
		_ = s.setInstance(ctx, st, tfresource.InstanceNeedsAttention)
		return fmt.Errorf("%w: %s", ErrNeedsAttention, f.err.Error())
	}
	// A terminal record that cannot be confirmed durable may leave the attempt
	// RUNNING: never release the Environment as an ordinary failure then.
	if err := s.finalizeAttempt(ctx, st, f.stage, tfresource.AttemptFailed, code); err != nil {
		return holdOn(ctx, "the failed attempt could not be recorded")
	}
	target := previousStatus(prev)
	if effectsPossible(f.stage) {
		target = tfresource.InstanceFailed
	}
	// Nothing applied keeps the previous status; otherwise FAILED.
	if err := s.setInstance(ctx, st, target); err != nil {
		return holdOn(ctx, "the instance status could not be recorded")
	}
	return f.err
}

// previousStatus is the status restored when an execution failed before any
// effect: a never-applied or healthy instance keeps its status.
func previousStatus(prev tfresource.InstanceStatus) tfresource.InstanceStatus {
	switch prev {
	case tfresource.InstanceProvisioning, tfresource.InstanceDestroying:
		return tfresource.InstanceFailed
	}
	return prev
}

func (s *Service) finalizeAttempt(ctx context.Context, st *execState, stage string, status tfresource.AttemptStatus, code string) error {
	return s.write(ctx, st, stage, status, code)
}

// holdOn marks the claim held and returns the hold error: used when a durable
// terminal record could not be confirmed, so the attempt may still read as
// RUNNING and the Environment must never be released as an ordinary failure.
func holdOn(ctx context.Context, why string) error {
	persistence.MarkHold(ctx)
	return fmt.Errorf("%w: %s", ErrNeedsAttention, why)
}

// settleOpenAttempt is the safety net of Execute and Destroy: an attempt that
// is still RUNNING when an error returns is held, never silently dropped. When
// the attempts cannot even be read, the hold is conservative: the Environment
// is marked held. It reports whether it held.
func (s *Service) settleOpenAttempt(ctx context.Context, st *execState, err error) (held bool) {
	pctx, cancel := persistCtx(ctx)
	defer cancel()
	attempts, listErr := s.deps.State.ListAttempts(pctx, st.inst.ID)
	if listErr != nil {
		persistence.MarkHold(ctx)
		return true
	}
	for _, a := range attempts {
		if a.ID == st.attempt.ID && a.Status == tfresource.AttemptRunning {
			st.attempt = a
			persistence.MarkHold(ctx)
			_ = s.finalizeAttempt(ctx, st, a.Stage, tfresource.AttemptNeedsAttention, codeUnknown)
			_ = s.setInstance(ctx, st, tfresource.InstanceNeedsAttention)
			held = true
		}
	}
	return held
}

// failedMutationProven reports whether a failed apply or destroy is accounted
// for by acknowledged state.
func (s *Service) failedMutationProven(ctx context.Context, st *execState, stage string) bool {
	if stage == StageDestroying {
		acked, err := s.stateAcknowledged(ctx, st)
		return err == nil && acked
	}
	proven, err := s.applyOutcomeProven(ctx, st)
	return err == nil && proven
}

// applyOutcomeProven reports whether a failed APPLY is fully accounted for
// before it may be retried: the backend acknowledged a state of this attempt
// and every object the plan meant to create is present in it. A clean exit of
// the CLI proves neither.
func (s *Service) applyOutcomeProven(ctx context.Context, st *execState) (bool, error) {
	acked, err := s.stateAcknowledged(ctx, st)
	if err != nil || !acked {
		return false, err
	}
	art, err := s.deps.Modules.VerifyForExecution(ctx, st.req.Source)
	if err != nil {
		return false, err
	}
	defer art.Close()
	sess, err := s.deps.Runner.Open(ctx, terraformexec.SessionRequest{
		Artifact: &art, Backend: &terraformexec.Backend{Address: s.cfg.BackendBase + st.inst.StateKey, Token: st.token},
		Variables: st.vars.All, Env: st.env.Env, Deadline: s.now().Add(s.cfg.InvocationTimeout),
	})
	if err != nil {
		return false, err
	}
	defer sess.Close()
	if err := sess.Init(ctx); err != nil {
		return false, err
	}
	resources, err := sess.StateResources(ctx)
	if err != nil {
		return false, err
	}
	present := map[string]bool{}
	for _, r := range resources {
		present[r.Address] = true
	}
	for _, address := range st.plannedCreates {
		if !present[address] {
			return false, nil
		}
	}
	return true, nil
}

// createdAddresses lists the addresses a plan creates.
func createdAddresses(plan terraformexec.Plan) []string {
	var out []string
	for _, c := range plan.Changes {
		for _, a := range c.Actions {
			if a == "create" {
				out = append(out, c.Address)
				break
			}
		}
	}
	return out
}

// publish verifies state acknowledgement and the output and readiness
// contracts, then marks the instance READY. Order matters: nothing is
// published and no consumer may run before all three hold.
func (s *Service) publish(ctx context.Context, st *execState, prev tfresource.InstanceStatus, out invocationOutcome) (Result, error) {
	observed := make([]string, 0, len(out.resources))
	for _, r := range out.resources {
		if r.ID != "" {
			observed = append(observed, r.Address+"="+r.ID)
		}
	}
	sort.Strings(observed)
	st.observed = observed

	// State acknowledgement: a mutating apply must have advanced the
	// acknowledged serial of this attempt in the backend.
	if out.applied {
		acked, err := s.stateAcknowledged(ctx, st)
		if err != nil || !acked {
			return Result{}, s.settleFailure(ctx, st, prev, &stepFailure{stage: StageApplied, recovered: true, err: fmt.Errorf("%w: the state was not acknowledged", ErrExecution)})
		}
	}

	execOutputs := s.executorOutputs(st, out.outputs)
	checked, err := checkOutputs(st.bundle, st.req.OutputMapping, st.req.Type, out.outputs, execOutputs)
	if err != nil {
		return Result{}, s.settleFailure(ctx, st, prev, &stepFailure{stage: StageOutputs, err: err})
	}

	if err := s.checkReadiness(ctx, st, out, checked); err != nil {
		return Result{}, err
	}
	if err := s.finalizeAttempt(ctx, st, StageReady, tfresource.AttemptSucceeded, ""); err != nil {
		return Result{}, holdOn(ctx, "the successful attempt could not be recorded")
	}
	if err := s.setInstance(ctx, st, tfresource.InstanceReady); err != nil {
		return Result{}, holdOn(ctx, "the instance status could not be recorded")
	}
	if len(st.req.ProviderIDs) == 0 {
		// Nothing to retire.
	}
	result := Result{InstanceID: st.inst.ID, StateKey: st.inst.StateKey, Action: st.action, Outputs: checked.Module, Sensitive: checked.Sensitive, Resources: out.resources}
	result.Target = s.eksTarget(st, checked)
	return result, nil
}

// stateAcknowledged reports whether the backend recorded a state serial for
// the running attempt.
func (s *Service) stateAcknowledged(ctx context.Context, st *execState) (bool, error) {
	pctx, cancel := persistCtx(ctx)
	defer cancel()
	attempts, err := s.deps.State.ListAttempts(pctx, st.inst.ID)
	if err != nil {
		return false, err
	}
	for _, a := range attempts {
		if a.ID == st.attempt.ID {
			return a.StateSerial != nil, nil
		}
	}
	return false, nil
}

// executorOutputs supplies the outputs the executor itself adds (the cluster
// context name); a kubeconfig is never produced or persisted.
func (s *Service) executorOutputs(st *execState, actual map[string]terraformexec.Output) map[string]any {
	out := map[string]any{}
	for _, name := range st.bundle.Contract.ExecutorOutputs {
		if name == "kubeContext" {
			if v, ok := actual["name"]; ok {
				out[name] = v.Value
			}
		}
	}
	return out
}

// checkReadiness runs the real bounded readiness query once the state is
// acknowledged and the outputs are valid. A deterministic not-ready result is
// a retained confirmed failure (state, provenance and ownership stay usable by
// cleanup); an unavailable query is also not an unknown apply outcome.
func (s *Service) checkReadiness(ctx context.Context, st *execState, out invocationOutcome, checked checkedOutputs) error {
	s.progress(ctx, st, StageReadiness, tfresource.AttemptRunning, "")
	deadline := s.now().Add(s.cfg.ReadinessTimeout)
	rctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	err := s.deps.Readiness.Check(rctx, terraformexec.ReadinessRequest{
		OrganizationKey: st.req.OrganizationKey, ResourceType: st.req.Type.Key, Module: st.req.Source.ModuleID, Pins: st.req.Pins,
		Targets: st.req.Targets, Resources: out.resources, Outputs: checked.Module, Env: st.env.Env, Deadline: deadline,
	})
	if err == nil {
		return nil
	}
	code, safe := codeReadinessUnav, fmt.Errorf("%w: the readiness query could not complete", ErrExecution)
	if errors.Is(err, terraformexec.ErrNotReady) {
		code, safe = codeNotReady, fmt.Errorf("%w: %w", terraformexec.ErrNotReady, ErrExecution)
	}
	if ferr := s.finalizeAttempt(ctx, st, StageReadiness, tfresource.AttemptFailed, code); ferr != nil {
		return holdOn(ctx, "the readiness result could not be recorded")
	}
	if serr := s.setInstance(ctx, st, tfresource.InstanceFailed); serr != nil {
		return holdOn(ctx, "the instance status could not be recorded")
	}
	return safe
}

// eksTarget returns the completed opaque cluster target of an EKS node: the
// AWS Connection, region and cluster name, with no endpoint, token or
// kubeconfig.
func (s *Service) eksTarget(st *execState, checked checkedOutputs) *execution.Target {
	if st.req.Type.Key != "k8s-cluster" {
		return nil
	}
	var aws *tfmodule.ProviderPin
	for i := range st.req.Pins {
		if st.req.Pins[i].Kind == tfmodule.BindingAWS {
			aws = &st.req.Pins[i]
		}
	}
	if aws == nil {
		return nil
	}
	name := ""
	for module, target := range st.req.OutputMapping {
		if target == "name" {
			name, _ = checked.Module[module].(string)
		}
	}
	if name == "" {
		return nil
	}
	return &execution.Target{Kind: "kubernetes", Provider: "eks", ClusterName: name, Organization: st.req.OrganizationKey, Connection: aws.ConnectionKey, Region: aws.Region}
}
