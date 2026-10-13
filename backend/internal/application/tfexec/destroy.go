package tfexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/platform/canon"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformexec"
)

// DestroyRequest destroys one instance with its original pinned module,
// target and state. The authorization (user selection or run cleanup) was
// admitted and checked by the cleanup engine; this method never re-matches
// the current Definition, module or Connection.
type DestroyRequest struct {
	InstanceID      string
	OrganizationKey string
	// RunID is the provenance of this cleanup operation (not the run that
	// created the resource).
	RunID string
	// OnStage, when set, is told each cleanup stage as it starts
	// (CHECKING, DESTROYING, VERIFYING).
	OnStage func(stage string)
}

// DestroyOutcome is the safe result of one instance destroy.
type DestroyOutcome struct {
	InstanceID string
	Destroyed  bool
	// Remaining lists physical objects not verified absent (safe address=ID
	// pairs): the pre-destroy state objects, or the last acknowledged
	// observations when the destroy could not start.
	Remaining []string
	// RemainingUnknown is set when the instance is not destroyed and no
	// physical ID is known: an empty Remaining is then never proof of absence.
	RemainingUnknown bool
	// Stage is the last stage reached.
	Stage string
}

// snapshotDoc is the typed view of an instance target snapshot.
type snapshotDoc struct {
	Pins            []tfmodule.ProviderPin `json:"pins"`
	ProviderTargets map[string]struct {
		Provider     string `json:"provider"`
		Kind         string `json:"kind"`
		ClusterName  string `json:"clusterName"`
		Organization string `json:"organization"`
		Connection   string `json:"connection"`
		Region       string `json:"region"`
	} `json:"providerTargets"`
	AccountID string `json:"accountId"`
	Region    string `json:"region"`
}

func readSnapshot(raw map[string]any) (snapshotDoc, map[string]execution.Target, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return snapshotDoc{}, nil, err
	}
	var doc snapshotDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return snapshotDoc{}, nil, err
	}
	targets := map[string]execution.Target{}
	for descriptor, t := range doc.ProviderTargets {
		targets[descriptor] = execution.Target{Kind: t.Kind, Provider: t.Provider, ClusterName: t.ClusterName, Organization: t.Organization, Connection: t.Connection, Region: t.Region}
	}
	return doc, targets, nil
}

// Destroy destroys one instance. It must run under the cleanup operation's
// Environment claim. An unknown outcome (killed process, failed state push,
// unconfirmed stop) holds NEEDS_ATTENTION; a confirmed failure or remaining
// physical objects fail with the remaining IDs and keep the state usable for a
// retry.
func (s *Service) Destroy(ctx context.Context, req DestroyRequest) (DestroyOutcome, error) {
	out, err := s.destroy(ctx, req)
	if !out.Destroyed && errors.Is(err, ErrNoClaim) == false {
		s.fillRemaining(ctx, &out)
	}
	return out, err
}

// fillRemaining makes sure an instance that is not verified absent always
// reports its last known physical IDs, or says they are unknown.
func (s *Service) fillRemaining(ctx context.Context, out *DestroyOutcome) {
	if len(out.Remaining) > 0 || out.InstanceID == "" {
		return
	}
	pctx, cancel := persistCtx(ctx)
	defer cancel()
	attempts, err := s.deps.State.ListAttempts(pctx, out.InstanceID)
	if err == nil {
		out.Remaining = lastObserved(attempts)
	}
	out.RemainingUnknown = len(out.Remaining) == 0
}

// lastObserved returns the physical IDs of the latest attempt that recorded
// any: the last acknowledged observations of the instance.
func lastObserved(attempts []tfresource.Attempt) []string {
	for i := len(attempts) - 1; i >= 0; i-- {
		if len(attempts[i].ObservedPhysicalIDs) > 0 {
			return append([]string(nil), attempts[i].ObservedPhysicalIDs...)
		}
	}
	return nil
}

func (s *Service) destroy(ctx context.Context, req DestroyRequest) (out DestroyOutcome, err error) {
	if _, ok := persistence.OwnerFrom(ctx); !ok {
		return DestroyOutcome{}, ErrNoClaim
	}
	out.InstanceID = req.InstanceID
	inst, err := s.deps.State.GetInstance(ctx, req.InstanceID)
	if err != nil {
		return out, fmt.Errorf("%w: load the instance", ErrExecution)
	}
	out.InstanceID = inst.ID
	if inst.Status == tfresource.InstanceDestroyed {
		out.Destroyed, out.Stage = true, StageDestroyed
		return out, nil
	}
	switch inst.Status {
	case tfresource.InstanceReady, tfresource.InstanceUnreferenced, tfresource.InstanceFailed, tfresource.InstancePending:
	default:
		return out, ErrInstanceHeld
	}
	attempts, err := s.deps.State.ListAttempts(ctx, inst.ID)
	if err != nil {
		return out, fmt.Errorf("%w: list attempts", ErrExecution)
	}
	has, err := s.stateExists(ctx, inst.ID)
	if err != nil {
		return out, fmt.Errorf("%w: read the state record", ErrExecution)
	}
	if err := gateDestroy(attempts, has); err != nil {
		return out, err
	}
	snapshot, targets, err := readSnapshot(inst.TargetRef)
	if err != nil {
		return out, fmt.Errorf("%w: the target history is unreadable", ErrBinding)
	}
	last := lastApplyAttempt(attempts)
	st := &execState{onStage: req.OnStage, inst: inst, created: map[string]bool{}, hasState: has, action: tfresource.ActionDestroy,
		req: ExecuteRequest{OrganizationKey: req.OrganizationKey, Pins: snapshot.Pins, Targets: targets, RunID: req.RunID}}
	prev := inst.Status

	// No state and nothing that could have changed: absence is already
	// proven by the instance's own history.
	if !has {
		return s.destroyTrivial(ctx, st, prev, last, req)
	}
	if last == nil {
		return out, ErrInstanceHeld
	}
	bundle, err := s.deps.Modules.BundleByID(ctx, inst.ModuleBundleID)
	if err != nil {
		return out, fmt.Errorf("%w: the historical module is unavailable: %w", ErrExecution, err)
	}
	st.bundle = bundle
	st.req.Source = bundle.Ref
	art, err := s.deps.Modules.VerifyHistoricalForExecution(ctx, bundle)
	if err != nil {
		return out, fmt.Errorf("%w: verify the historical module: %w", ErrExecution, err)
	}
	defer art.Close()

	env, err := s.deps.Binder.Bind(ctx, terraformexec.BindRequest{OrganizationKey: req.OrganizationKey, Pins: snapshot.Pins, Targets: targets})
	if err != nil {
		return out, fmt.Errorf("%w: %w", ErrBinding, err)
	}
	defer func() {
		if env.Close != nil && !st.retain {
			_ = env.Close()
		}
	}()
	if env.AccountID != snapshot.AccountID || env.Region != snapshot.Region {
		return out, fmt.Errorf("%w: the credential reaches another account or region than the original target", ErrBinding)
	}
	st.env = env

	vars, err := s.destroyVariables(ctx, st, last)
	if err != nil {
		return out, err
	}
	st.vars = vars

	moduleSnap, err := canon.Map(last.ModuleSnapshot)
	if err != nil {
		return out, fmt.Errorf("%w: module history", ErrExecution)
	}
	moduleSnap["action"] = string(tfresource.ActionDestroy)
	// One private pre-destroy artifact per destroy cycle: a retry after a
	// failed verification reuses the original, never the (possibly empty)
	// current state.
	handle, effects := destroyCycle(attempts)
	if handle == "" {
		handle = "verify-" + ids.New()
	}
	st.verifyHandle, st.cycleEffects = handle, effects
	moduleSnap["verifyHandle"] = handle
	attempt, err := s.deps.State.BeginAttempt(ctx, tfresource.Attempt{
		InstanceID: inst.ID, RunID: req.RunID, Action: tfresource.ActionDestroy, Ownership: tfresource.Preexisting, Stage: StageChecking,
		ModuleSnapshot: moduleSnap, TargetSnapshot: inst.TargetRef,
		Deadline: s.now().Add(2*s.cfg.InvocationTimeout + time.Duration(s.cfg.AbsencePolls)*s.cfg.AbsenceInterval),
	})
	if err != nil {
		return out, fmt.Errorf("%w: begin the destroy attempt: %w", ErrExecution, err)
	}
	st.attempt = attempt
	defer func() {
		if err != nil && s.settleOpenAttempt(ctx, st, err) {
			err = fmt.Errorf("%w: %v", ErrNeedsAttention, err)
		}
	}()
	if err := s.setInstance(ctx, st, tfresource.InstanceDestroying); err != nil {
		return out, s.settleFailure(ctx, st, prev, &stepFailure{stage: StageChecking, err: fmt.Errorf("%w: mark the instance", ErrExecution)})
	}
	token, _, err := s.deps.Tokens.IssueCapability(ctx, inst.ID, tfresource.CapabilityDestroy, s.capabilityTTL())
	if err != nil {
		return out, s.settleFailure(ctx, st, prev, &stepFailure{stage: StageChecking, err: fmt.Errorf("%w: issue the state capability", ErrExecution)})
	}
	st.token = token

	_, _, ferr := s.destroyInvocation(ctx, st, &art)
	// The pre-destroy objects are recorded before any effect, so every later
	// failure still reports them as not verified absent.
	if len(st.observed) > 0 {
		out.Remaining = append([]string(nil), st.observed...)
	}
	if ferr != nil {
		out.Stage = ferr.stage
		return out, s.settleFailure(ctx, st, prev, ferr)
	}
	out.Stage = StageVerifying
	remaining, verr := s.proveAbsence(ctx, st)
	if verr != nil {
		if ferr := s.finalizeAttempt(ctx, st, StageVerifying, tfresource.AttemptFailed, codeFailed); ferr != nil {
			return out, holdOn(ctx, "the verification result could not be recorded")
		}
		if serr := s.setInstance(ctx, st, tfresource.InstanceFailed); serr != nil {
			return out, holdOn(ctx, "the instance status could not be recorded")
		}
		out.Remaining = append([]string(nil), st.observed...)
		return out, fmt.Errorf("%w: the physical absence could not be verified", ErrExecution)
	}
	if len(remaining) > 0 {
		st.observed = remaining
		if ferr := s.finalizeAttempt(ctx, st, StageVerifying, tfresource.AttemptFailed, codeRemains); ferr != nil {
			return out, holdOn(ctx, "the verification result could not be recorded")
		}
		if serr := s.setInstance(ctx, st, tfresource.InstanceFailed); serr != nil {
			return out, holdOn(ctx, "the instance status could not be recorded")
		}
		out.Remaining = remaining
		return out, fmt.Errorf("%w: physical objects remain after destroy", ErrExecution)
	}
	if ferr := s.finalizeAttempt(ctx, st, StageDestroyed, tfresource.AttemptSucceeded, ""); ferr != nil {
		return out, holdOn(ctx, "the destroyed attempt could not be recorded")
	}
	if err := s.setInstance(ctx, st, tfresource.InstanceDestroyed); err != nil {
		return out, holdOn(ctx, "the destroyed status could not be recorded")
	}
	s.retireEdges(ctx, st)
	out.Destroyed, out.Stage, out.Remaining = true, StageDestroyed, nil
	return out, nil
}

// gateDestroy refuses destroy while an attempt is unresolved.
func gateDestroy(attempts []tfresource.Attempt, hasState bool) error {
	for _, a := range attempts {
		switch a.Status {
		case tfresource.AttemptRunning, tfresource.AttemptInterrupted, tfresource.AttemptNeedsAttention:
			return ErrInstanceHeld
		}
		if !hasState && a.Action != tfresource.ActionDestroy && effectsPossible(a.Stage) {
			return ErrInstanceHeld
		}
	}
	return nil
}

// lastApplyAttempt returns the latest CREATE/UPDATE attempt, the module
// history destroy uses.
func lastApplyAttempt(attempts []tfresource.Attempt) *tfresource.Attempt {
	var last *tfresource.Attempt
	for i := range attempts {
		a := attempts[i]
		if a.Action == tfresource.ActionDestroy {
			continue
		}
		last = &attempts[i]
		_ = a
	}
	return last
}

// destroyTrivial records the destroy of an instance that never had state.
func (s *Service) destroyTrivial(ctx context.Context, st *execState, prev tfresource.InstanceStatus, last *tfresource.Attempt, req DestroyRequest) (DestroyOutcome, error) {
	out := DestroyOutcome{InstanceID: st.inst.ID, Stage: StageDestroyed}
	module := map[string]any{"action": string(tfresource.ActionDestroy), "bundleId": st.inst.ModuleBundleID}
	attempt, err := s.deps.State.BeginAttempt(ctx, tfresource.Attempt{
		InstanceID: st.inst.ID, RunID: req.RunID, Action: tfresource.ActionDestroy, Ownership: tfresource.Preexisting, Stage: StageVerifying,
		ModuleSnapshot: module, TargetSnapshot: st.inst.TargetRef, Deadline: s.now().Add(s.cfg.InvocationTimeout),
	})
	if err != nil {
		return out, fmt.Errorf("%w: begin the destroy attempt: %w", ErrExecution, err)
	}
	st.attempt = attempt
	if err := s.finalizeAttempt(ctx, st, StageDestroyed, tfresource.AttemptSucceeded, ""); err != nil {
		return out, holdOn(ctx, "the destroyed attempt could not be recorded")
	}
	if err := s.setInstance(ctx, st, tfresource.InstanceDestroyed); err != nil {
		return out, holdOn(ctx, "the destroyed status could not be recorded")
	}
	s.retireEdges(ctx, st)
	out.Destroyed = true
	return out, nil
}

// destroyVariables rebuilds the variable set of the last apply from the
// attempt history. Executor-owned secrets come from their persisted
// references; when a secret store no longer holds one, destroy uses a fresh
// throwaway value: destroy never changes an object through it, and it is never
// stored or shown.
func (s *Service) destroyVariables(ctx context.Context, st *execState, last *tfresource.Attempt) (variables, error) {
	public, _ := last.ModuleSnapshot["variables"].(map[string]any)
	owned := map[string]bool{}
	for _, name := range st.bundle.Contract.ExecutorOwned {
		owned[name] = true
	}
	inputs := map[string]any{}
	for name, value := range public {
		if !owned[name] {
			inputs[name] = value
		}
	}
	secrets := map[string]string{}
	refs, _ := st.inst.PhysicalIntent["secretRefs"].(map[string]any)
	for name := range owned {
		if !executorSecret(name) {
			continue
		}
		ref, _ := refs[name].(string)
		if value, ok := s.deps.Secrets.Get(ctx, ref); ok && ref != "" {
			secrets[name] = value
			continue
		}
		throwaway, err := randomSecret()
		if err != nil {
			return variables{}, fmt.Errorf("%w: draw a secret", ErrExecution)
		}
		secrets[name] = throwaway
	}
	tags := map[string]string{}
	for k, v := range s.cfg.Tags {
		tags[k] = v
	}
	for k, v := range tagsFromIntent(st.inst.PhysicalIntent) {
		tags[k] = v
	}
	name, _ := st.inst.PhysicalIntent["name"].(string)
	region, _ := public[varRegion].(string)
	if region == "" {
		doc, _, err := readSnapshot(st.inst.TargetRef)
		if err == nil {
			region = doc.Region
		}
	}
	return buildVariables(st.bundle, inputs, executorValues{Region: region, Tags: tags, Name: name, Secrets: secrets})
}

// destroyInvocation runs init, the pre-destroy state read, the destroy plan,
// its review and apply in one private session.
func (s *Service) destroyInvocation(ctx context.Context, st *execState, art *tfmodule.VerifiedArtifact) ([]terraformexec.StateResource, []byte, *stepFailure) {
	mk := func(stage string, err error) *stepFailure {
		var step *terraformexec.StepError
		errors.As(err, &step)
		if step != nil && !step.Stopped {
			s.retainWorkspace(st, art)
		}
		return &stepFailure{stage: stage, step: step, err: describeStep(stage, err)}
	}
	sess, err := s.deps.Runner.Open(ctx, terraformexec.SessionRequest{
		Artifact: art, Backend: &terraformexec.Backend{Address: s.cfg.BackendBase + st.inst.StateKey, Token: st.token},
		Variables: st.vars.All, Env: st.env.Env, Deadline: s.now().Add(s.cfg.InvocationTimeout),
	})
	if err != nil {
		return nil, nil, mk(StageChecking, err)
	}
	defer sess.Close()
	s.progress(ctx, st, StageChecking, tfresource.AttemptRunning, "")
	if st.onStage != nil {
		st.onStage(StageChecking)
	}
	if err := sess.Init(ctx); err != nil {
		return nil, nil, mk(StageChecking, err)
	}
	pre, err := sess.StateResources(ctx)
	if err != nil {
		return nil, nil, mk(StageChecking, err)
	}
	preState, err := sess.PullState(ctx)
	if err != nil {
		return nil, nil, mk(StageChecking, err)
	}
	// The ORIGINAL pre-destroy state of this cycle, kept privately before any
	// effect. On a retry it is loaded; the current backend state may already
	// be empty and proves nothing about the provider.
	original, found, lerr := s.deps.Verification.LoadVerification(ctx, st.verifyHandle, st.inst.ID)
	if lerr != nil {
		return nil, nil, &stepFailure{stage: StageChecking, unknown: true, err: fmt.Errorf("%w: the verification artifact cannot be read", ErrExecution)}
	}
	switch {
	case found:
		st.verifyState = original
		// IDs of the original objects, from the earlier attempts' audit.
		attempts, _ := s.deps.State.ListAttempts(ctx, st.inst.ID)
		st.observed = lastObserved(attempts)
	case st.cycleEffects:
		// An earlier attempt may have destroyed objects but the artifact that
		// proves what they were is gone: unprovable, so held, never "absent".
		return nil, nil, &stepFailure{stage: StageChecking, unknown: true, err: fmt.Errorf("%w: the original verification state is missing", ErrExecution)}
	default:
		if err := s.deps.Verification.SaveVerification(ctx, st.verifyHandle, st.inst.ID, preState); err != nil {
			return nil, nil, &stepFailure{stage: StageChecking, err: fmt.Errorf("%w: the pre-destroy state could not be preserved", ErrExecution)}
		}
		st.verifyState = preState
		st.observed = allIdentities(pre)
	}
	// A namespace delete cascades: inspect the real contents immediately
	// before, and refuse (with nothing destroyed) on anything not accounted for.
	if err := s.inspectNamespaces(ctx, st, pre); err != nil {
		return nil, nil, &stepFailure{stage: StageChecking, err: err}
	}
	plan, err := sess.Plan(ctx, terraformexec.PlanDestroy)
	if err != nil {
		return nil, nil, mk(StageChecking, err)
	}
	if !plan.OnlyDeletes() {
		return nil, nil, &stepFailure{stage: StageChecking, err: fmt.Errorf("%w: the destroy plan would do more than delete", ErrDestructivePlan)}
	}
	if plan.Mutating() {
		if err := s.progressStrict(ctx, st, StageDestroying); err != nil {
			return nil, nil, &stepFailure{stage: StageChecking, err: fmt.Errorf("%w: persist the destroy stage", ErrExecution)}
		}
		if st.onStage != nil {
			st.onStage(StageDestroying)
		}
		if err := sess.Apply(ctx); err != nil {
			f := mk(StageDestroying, err)
			f.recovered = s.keepRecovery(ctx, st, sess, art)
			return nil, nil, f
		}
		acked, aerr := s.stateAcknowledged(ctx, st)
		if aerr != nil || !acked {
			return nil, nil, &stepFailure{stage: StageDestroying, recovered: true, err: fmt.Errorf("%w: the state was not acknowledged", ErrExecution)}
		}
	}
	return pre, preState, nil
}

// allIdentities renders every state object as a safe address=ID pair,
// built-in state-only objects included (audit of what existed).
func allIdentities(resources []terraformexec.StateResource) []string {
	out := []string{}
	for _, r := range resources {
		out = append(out, r.Address+"="+r.ID)
	}
	sort.Strings(out)
	return out
}

// identify renders safe address=ID pairs of managed objects that exist
// outside Terraform's own state (built-in resources are excluded).
func identify(resources []terraformexec.StateResource) []string {
	out := []string{}
	for _, r := range resources {
		if strings.Contains(r.Provider, "builtin/terraform") {
			continue
		}
		out = append(out, r.Address+"="+r.ID)
	}
	sort.Strings(out)
	return out
}

// destroyCycle returns the private verification handle of the current
// destroy cycle and whether an earlier destroy attempt of it may already have
// changed provider objects. A cycle starts after the last successful destroy
// or the last CREATE/UPDATE and ends with a successful destroy.
func destroyCycle(attempts []tfresource.Attempt) (handle string, effects bool) {
	for _, a := range attempts {
		if a.Action != tfresource.ActionDestroy {
			handle, effects = "", false
			continue
		}
		if a.Status == tfresource.AttemptSucceeded {
			handle, effects = "", false
			continue
		}
		if h, _ := a.ModuleSnapshot["verifyHandle"].(string); h != "" && handle == "" {
			handle = h
		}
		if effectsPossible(a.Stage) {
			effects = true
		}
	}
	return handle, effects
}

// proveAbsence asks the provider, not Terraform's state, whether each object
// of the ORIGINAL pre-destroy state (the private verification artifact) still
// exists: a refresh-only plan over a private copy of that state reports every
// object gone as drift. The current backend state is never consulted for what
// to verify: after a destroy it is empty. It polls within the configured
// bound so eventually consistent deletions can settle. An unreadable original
// is an error, never an empty want.
func (s *Service) proveAbsence(ctx context.Context, st *execState) ([]string, error) {
	s.progress(ctx, st, StageVerifying, tfresource.AttemptRunning, "")
	if st.onStage != nil {
		st.onStage(StageVerifying)
	}
	if len(st.verifyState) == 0 {
		return nil, errors.New("the original verification state is unavailable")
	}
	var remaining []string
	for poll := 0; poll < s.cfg.AbsencePolls; poll++ {
		if poll > 0 {
			if err := s.sleep(ctx, s.cfg.AbsenceInterval); err != nil {
				return nil, err
			}
		}
		resources, drift, err := s.refreshDrift(ctx, st, st.verifyState)
		if err != nil {
			return nil, err
		}
		want := map[string]terraformexec.StateResource{}
		for _, r := range resources {
			if strings.Contains(r.Provider, "builtin/terraform") {
				continue
			}
			want[r.Address] = r
		}
		if len(want) == 0 {
			return nil, nil
		}
		gone := map[string]bool{}
		for _, address := range drift {
			gone[address] = true
		}
		remaining = remaining[:0]
		for address, r := range want {
			if !gone[address] {
				remaining = append(remaining, address+"="+r.ID)
			}
		}
		if len(remaining) == 0 {
			return nil, nil
		}
	}
	sort.Strings(remaining)
	return remaining, nil
}

// refreshDrift runs the local refresh-only session over the original state
// and returns that state's managed objects with the ones the provider no
// longer has.
func (s *Service) refreshDrift(ctx context.Context, st *execState, original []byte) ([]terraformexec.StateResource, []string, error) {
	art, err := s.deps.Modules.VerifyHistoricalForExecution(ctx, st.bundle)
	if err != nil {
		return nil, nil, err
	}
	defer art.Close()
	sess, err := s.deps.Runner.Open(ctx, terraformexec.SessionRequest{
		Artifact: &art, LocalState: original, Variables: st.vars.All, Env: st.env.Env, Deadline: s.now().Add(s.cfg.InvocationTimeout),
	})
	if err != nil {
		return nil, nil, err
	}
	defer sess.Close()
	retain := func(err error) error {
		var step *terraformexec.StepError
		if errors.As(err, &step) && !step.Stopped {
			s.retainWorkspace(st, &art)
		}
		return err
	}
	if err := sess.Init(ctx); err != nil {
		return nil, nil, retain(err)
	}
	resources, err := sess.StateResources(ctx)
	if err != nil {
		return nil, nil, retain(err)
	}
	plan, err := sess.Plan(ctx, terraformexec.PlanRefreshOnly)
	if err != nil {
		return nil, nil, retain(err)
	}
	return resources, plan.Drift, nil
}

// retireEdges drops the destroyed instance's provider edges once absence is
// proven; edges of failed or unknown consumers are never retired.
func (s *Service) retireEdges(ctx context.Context, st *execState) {
	pctx, cancel := persistCtx(ctx)
	defer cancel()
	deps, err := s.deps.State.Dependencies(pctx, st.inst.ID)
	if err != nil {
		return
	}
	providers := []string{}
	for _, d := range deps {
		if d.ConsumerID == st.inst.ID {
			providers = append(providers, d.ProviderID)
		}
	}
	if len(providers) == 0 {
		return
	}
	latest, err := s.deps.State.GetInstance(pctx, st.inst.ID)
	if err != nil {
		return
	}
	_, _ = s.deps.State.RetireDependencies(pctx, st.inst.ID, latest.Version, providers)
}
