package tfexec

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/platform/canon"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformexec"
	"orchestrator/internal/ports/terraformstate"
)

// CapabilityIssuer mints the short-lived invocation token of one instance for
// the claim owner in ctx (implemented by statebackend.Service).
type CapabilityIssuer interface {
	IssueCapability(ctx context.Context, instanceID string, action tfresource.CapabilityAction, ttl time.Duration) (string, time.Time, error)
}

// ModuleSource verifies a pinned module for execution (private copies, no
// fetch) and resolves the historical bundle of an instance (implemented by the
// module registry).
type ModuleSource interface {
	VerifyForExecution(ctx context.Context, ref tfmodule.SourceRef) (tfmodule.VerifiedArtifact, error)
	// VerifyHistoricalForExecution verifies a persisted bundle against the
	// retained release it was created under (destroy, absence probe).
	VerifyHistoricalForExecution(ctx context.Context, bundle tfmodule.Bundle) (tfmodule.VerifiedArtifact, error)
	BundleByID(ctx context.Context, id string) (tfmodule.Bundle, error)
}

// Deps are the ports of the service.
type Deps struct {
	StateWriter interface {
		Write(context.Context, string, string, string, []byte) error
	}
	Baselines terraformstate.BaselineRepository
	Cipher    terraformstate.Cipher
	State     terraformstate.Repository
	Tokens    CapabilityIssuer
	Modules   ModuleSource
	Runner    terraformexec.Runner
	Binder    terraformexec.ProviderBinder
	Readiness terraformexec.ReadinessChecker
	// Namespaces inspects namespace contents before a namespace is destroyed;
	// nil refuses every namespace destroy.
	Namespaces terraformexec.NamespaceInspector
	Recovery   terraformexec.RecoveryStore
	// Verification keeps the private pre-destroy state of a destroy cycle so
	// absence verification can always query the original physical objects.
	Verification terraformexec.VerificationStore
	Secrets      execution.SecretStore
}

// Config bounds one invocation.
type Config struct {
	// BackendBase is the internal state backend collection URL, ending in
	// "/internal/terraform/states/".
	BackendBase string
	// Tags are stable platform tags merged into the module `tags` variable on
	// every execution.
	Tags map[string]string
	// CreationTags (for example a run ID or an expiry) are frozen into the
	// instance's physical intent when it is created and never change, so a
	// later run cannot rewrite them (the run ID is provenance only).
	CreationTags map[string]string
	// InvocationTimeout bounds each Terraform session (default 45m).
	InvocationTimeout time.Duration
	// ReadinessTimeout bounds the readiness query (default 5m).
	ReadinessTimeout time.Duration
	// RetryBackoff is the wait before the single transient retry (default 5s).
	RetryBackoff time.Duration
	// AbsencePolls and AbsenceInterval bound the physical absence proof.
	AbsencePolls    int
	AbsenceInterval time.Duration
}

// Service implements execution.ResourceExecutor and the destroy engine.
type Service struct {
	cfg   Config
	deps  Deps
	now   func() time.Time
	sleep func(context.Context, time.Duration) error
}

// New validates the dependencies. Every port is required: there is no
// fallback to a local state, the host credential chain or an always-ready
// readiness check.
func New(cfg Config, deps Deps) (*Service, error) {
	switch {
	case (deps.Baselines == nil) != (deps.Cipher == nil):
		return nil, errors.New("tfexec: baseline persistence and its State cipher must be configured together")
	case deps.State == nil || deps.Tokens == nil || deps.Modules == nil || deps.Runner == nil || deps.Binder == nil ||
		deps.Readiness == nil || deps.Recovery == nil || deps.Verification == nil || deps.Secrets == nil:
		return nil, errors.New("tfexec: state, capability, module, runner, binder, readiness, recovery and secret ports are all required")
	case !strings.HasSuffix(cfg.BackendBase, tfStatePath):
		return nil, errors.New("tfexec: the state backend base URL must end with " + tfStatePath)
	}
	if cfg.InvocationTimeout <= 0 {
		cfg.InvocationTimeout = 45 * time.Minute
	}
	if cfg.ReadinessTimeout <= 0 {
		cfg.ReadinessTimeout = 5 * time.Minute
	}
	if cfg.RetryBackoff <= 0 {
		cfg.RetryBackoff = 5 * time.Second
	}
	if cfg.AbsencePolls <= 0 {
		cfg.AbsencePolls = 3
	}
	if cfg.AbsenceInterval <= 0 {
		cfg.AbsenceInterval = 3 * time.Second
	}
	return &Service{cfg: cfg, deps: deps, now: func() time.Time { return time.Now().UTC() }, sleep: sleepContext}, nil
}

const tfStatePath = "/internal/terraform/states/"

// SetSleep replaces the wait function (tests).
func (s *Service) SetSleep(f func(context.Context, time.Duration) error) { s.sleep = f }

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// VerifiesModules implements execution.SelfVerifyingExecutor.
func (s *Service) VerifiesModules() bool { return true }

var (
	_ execution.ResourceExecutor      = (*Service)(nil)
	_ execution.SelfVerifyingExecutor = (*Service)(nil)
)

// persistCtx returns a bounded context that survives caller cancellation but
// keeps the claim owner, so a cancelled execution still records its outcome.
func persistCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
}

// ExecuteRequest is one Terraform node handed to the executor.
type ExecuteRequest struct {
	RunID        string
	DeploymentID string
	Descriptor   string
	Scope        resource.Scope
	Generation   int64
	// Identity is the canonical instance identity tuple (never a run ID); the
	// registered instance must carry exactly its state key.
	Identity        tfresource.Identity
	Source          tfmodule.SourceRef
	Type            resource.Type
	Inputs          map[string]any
	OutputMapping   map[string]string
	Pins            []tfmodule.ProviderPin
	Targets         map[string]execution.Target
	ProviderIDs     []string
	OrganizationKey string
}

// Result is the published outcome of one execution.
type Result struct {
	InstanceID string
	StateKey   string
	Action     tfresource.Action
	// Outputs are module outputs by module name, runtime values.
	Outputs   map[string]any
	Sensitive map[string]bool
	Resources []terraformexec.StateResource
	Target    *execution.Target
}

// Provision implements execution.ResourceExecutor for Terraform nodes.
func (s *Service) Provision(ctx context.Context, req execution.ProvisionRequest) (execution.ProvisionResult, error) {
	if req.Source == nil {
		return execution.ProvisionResult{}, fmt.Errorf("%w: the node has no pinned module", ErrInput)
	}
	res, err := s.Execute(ctx, ExecuteRequest{
		RunID: req.RunID, DeploymentID: req.DeploymentID, Descriptor: req.Descriptor, Scope: req.Scope, Generation: req.Generation, Identity: req.Identity,
		Source: *req.Source, Type: req.Type, Inputs: req.Inputs, OutputMapping: req.OutputMapping,
		Pins: req.ProviderPins, Targets: req.ProviderTargets, ProviderIDs: req.ProviderInstanceIDs, OrganizationKey: req.OrganizationKey,
	})
	if err != nil {
		return execution.ProvisionResult{}, err
	}
	return execution.ProvisionResult{
		Outputs: res.Outputs, Sensitive: res.Sensitive, Target: res.Target, InstanceID: res.InstanceID,
		State: map[string]any{"driver": "terraform", "instanceId": res.InstanceID, "stateKey": res.StateKey, "module": req.Source.ModuleID, "action": string(res.Action)},
	}, nil
}

// execState is the per-execution working record.
type execState struct {
	req      ExecuteRequest
	bundle   tfmodule.Bundle
	inst     tfresource.Instance
	attempt  tfresource.Attempt
	action   tfresource.Action
	vars     variables
	env      terraformexec.ProviderEnv
	token    string
	hasState bool
	// verifyHandle names the destroy cycle's private pre-destroy artifact;
	// cycleEffects reports an earlier destroy attempt of the cycle that may
	// have changed provider objects; verifyState is the original state.
	verifyHandle   string
	cycleEffects   bool
	verifyState    []byte
	plannedCreates []string
	retain         bool
	onStage        func(string)
	observed       []string
	recovery       string
	heldStage      string
	// created holds the addresses this attempt created, which a retry of the
	// same CREATE may replace (they are the run's own new objects).
	created map[string]bool
}

// Execute runs CREATE or non-destructive UPDATE of one registered module
// instance. It must run under an Environment claim (the owner in ctx).
func (s *Service) Execute(ctx context.Context, req ExecuteRequest) (result Result, err error) {
	if _, ok := persistence.OwnerFrom(ctx); !ok {
		return Result{}, ErrNoClaim
	}
	if err := req.Source.Validate(); err != nil || req.Descriptor == "" || req.RunID == "" || req.Scope.Validate() != nil || req.Identity.Validate() != nil {
		return Result{}, fmt.Errorf("%w: the request is incomplete", ErrInput)
	}
	// Verify the pinned module and copy it privately. Nothing has an effect
	// yet; drift or a release mismatch stops here.
	probe, err := s.deps.Modules.VerifyForExecution(ctx, req.Source)
	if err != nil {
		return Result{}, fmt.Errorf("%w: verify the module artifact: %w", ErrExecution, err)
	}
	defer probe.Close()
	st := &execState{req: req, bundle: probe.Bundle, created: map[string]bool{}}
	if !st.bundle.ServesType(req.Type.Key) {
		return Result{}, fmt.Errorf("%w: the module does not serve the Resource Type", ErrInput)
	}
	if err := s.checkPins(st); err != nil {
		return Result{}, err
	}

	// Provider binding: resolve the scoped credential and verify account and
	// region before any state write or provider call.
	env, err := s.deps.Binder.Bind(ctx, terraformexec.BindRequest{OrganizationKey: req.OrganizationKey, Pins: req.Pins, Targets: req.Targets})
	if err != nil {
		return Result{}, fmt.Errorf("%w: %w", ErrBinding, err)
	}
	st.env = env
	defer func() {
		if env.Close != nil && !st.retain {
			_ = env.Close()
		}
	}()

	if err := s.prepareInstance(ctx, st, env); err != nil {
		return Result{}, err
	}
	defer func() {
		// The attempt must not stay RUNNING on a path that returned early.
		if err != nil && st.attempt.ID != "" && s.settleOpenAttempt(ctx, st, err) {
			err = fmt.Errorf("%w: %v", ErrNeedsAttention, err)
		}
	}()
	return s.run(ctx, st, &probe)
}

// checkPins requires exactly one pin per configurable provider of the bundle
// and refuses account/region overrides: a pin carries only the scoped
// identity resolved at planning.
func (s *Service) checkPins(st *execState) error {
	byName := map[string]tfmodule.ProviderPin{}
	for _, pin := range st.req.Pins {
		if _, dup := byName[pin.Name]; dup {
			return fmt.Errorf("%w: duplicate provider pin", ErrBinding)
		}
		byName[pin.Name] = pin
	}
	names := make([]string, 0, len(st.bundle.Providers))
	for name := range st.bundle.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		provider := st.bundle.Providers[name]
		if provider.Binding == tfmodule.BindingNone {
			continue
		}
		pin, ok := byName[name]
		if !ok || pin.Kind != provider.Binding {
			return fmt.Errorf("%w: a provider has no matching pin", ErrBinding)
		}
		delete(byName, name)
	}
	if len(byName) != 0 {
		return fmt.Errorf("%w: a pin names no provider of the module", ErrBinding)
	}
	return nil
}

// targetSnapshot is the immutable nonsecret target history of an instance.
func targetSnapshot(pins []tfmodule.ProviderPin, targets map[string]execution.Target, accountID, region string) (map[string]any, error) {
	sorted := append([]tfmodule.ProviderPin(nil), pins...)
	// The Driver Account version is an operational CAS counter (rename, verify,
	// same-target credential replacement). It guards Preview/admission but is not
	// part of the physical destination, so it never enters the durable target
	// identity: a version-only account change must not force replacement.
	for i := range sorted {
		sorted[i].AccountVersion = 0
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	provider := map[string]any{}
	for descriptor, t := range targets {
		provider[descriptor] = map[string]any{"provider": t.Provider, "kind": t.Kind, "clusterName": t.ClusterName, "organization": t.Organization, "connection": t.Connection, "region": t.Region}
	}
	return canon.Map(map[string]any{"pins": sorted, "providerTargets": provider, "accountId": accountID, "region": region})
}

// prepareInstance finds or registers the instance (write-ahead identity,
// target, module and physical intent), enforces the replacement and unresolved
// attempt gates, builds the variables, records dependencies and the attempt.
func (s *Service) prepareInstance(ctx context.Context, st *execState, env terraformexec.ProviderEnv) error {
	req := st.req
	snapshot, err := targetSnapshot(req.Pins, s.relevantTargets(st), env.AccountID, env.Region)
	if err != nil {
		return fmt.Errorf("%w: target snapshot", ErrBinding)
	}
	inst, found, err := s.deps.State.FindInstance(ctx, req.Generation, req.Descriptor, string(req.Scope.Type), req.Scope.ID)
	if err != nil {
		return fmt.Errorf("%w: find the instance: %w", ErrExecution, err)
	}
	owned := st.bundle.Contract.ExecutorOwned
	if !found {
		intent, err := s.newIntent(ctx, st, owned)
		if err != nil {
			return err
		}
		inst, err = s.deps.State.RegisterInstance(ctx, terraformstate.NewInstance{
			TargetGeneration: req.Generation, Descriptor: req.Descriptor, ScopeType: string(req.Scope.Type), ScopeID: req.Scope.ID,
			ModuleBundleID: st.bundle.ID, TargetRef: snapshot, PhysicalIntent: intent, CreatedByRunID: req.RunID,
		})
		if err != nil {
			return fmt.Errorf("%w: register the instance: %w", ErrExecution, err)
		}
	}
	if inst.StateKey != req.Identity.StateKey() {
		return fmt.Errorf("%w: the instance identity differs from the request", ErrBinding)
	}
	st.inst = inst
	// Class, module or target replacement is never an ordinary execution.
	sameTarget, err := sameJSON(inst.TargetRef, snapshot)
	if err != nil || inst.ModuleBundleID != st.bundle.ID || !sameTarget {
		return ErrReplacementRequired
	}
	switch inst.Status {
	case tfresource.InstancePending, tfresource.InstanceReady, tfresource.InstanceFailed, tfresource.InstanceUnreferenced, tfresource.InstanceDestroyed:
	default:
		return ErrInstanceHeld
	}
	attempts, err := s.deps.State.ListAttempts(ctx, inst.ID)
	if err != nil {
		return fmt.Errorf("%w: list attempts: %w", ErrExecution, err)
	}
	has, err := s.stateExists(ctx, inst.ID)
	if err != nil {
		return fmt.Errorf("%w: read the state record: %w", ErrExecution, err)
	}
	st.hasState = has
	if err := gateAttempts(attempts, has); err != nil {
		return err
	}
	st.action = tfresource.ActionCreate
	if has && inst.Status != tfresource.InstanceDestroyed {
		st.action = tfresource.ActionUpdate
	}

	secrets, err := s.executorSecrets(ctx, inst, owned)
	if err != nil {
		return err
	}
	tags := map[string]string{}
	for k, v := range s.cfg.Tags {
		tags[k] = v
	}
	for k, v := range tagsFromIntent(inst.PhysicalIntent) {
		tags[k] = v
	}
	name, _ := inst.PhysicalIntent["name"].(string)
	st.vars, err = buildVariables(st.bundle, req.Inputs, executorValues{Region: env.Region, Tags: tags, Name: name, Secrets: secrets})
	if err != nil {
		return err
	}
	if len(req.ProviderIDs) > 0 {
		updated, err := s.deps.State.RecordDependencies(ctx, inst.ID, inst.Version, req.ProviderIDs)
		if err != nil {
			return fmt.Errorf("%w: record dependencies: %w", ErrExecution, err)
		}
		st.inst = updated
	}
	ownership := ownershipFor(attempts, req.RunID, st.inst, has)
	moduleSnap, err := s.moduleSnapshot(st)
	if err != nil {
		return err
	}
	attempt, err := s.deps.State.BeginAttempt(ctx, tfresource.Attempt{
		InstanceID: st.inst.ID, DeploymentID: req.DeploymentID, RunID: req.RunID, Action: st.action, Ownership: ownership,
		Stage: StageIntent, ModuleSnapshot: moduleSnap, TargetSnapshot: snapshot,
		Deadline: s.now().Add(2*s.cfg.InvocationTimeout + s.cfg.RetryBackoff + s.cfg.ReadinessTimeout),
	})
	if err != nil {
		return fmt.Errorf("%w: begin the attempt: %w", ErrExecution, err)
	}
	st.attempt = attempt
	return nil
}

// relevantTargets returns the completed opaque targets of the cluster nodes
// the pins name.
func (s *Service) relevantTargets(st *execState) map[string]execution.Target {
	out := map[string]execution.Target{}
	for _, pin := range st.req.Pins {
		if pin.Provider == "" {
			continue
		}
		if t, ok := st.req.Targets[pin.Provider]; ok {
			out[pin.Provider] = t
		}
	}
	return out
}

func sameJSON(a, b map[string]any) (bool, error) {
	x, err := canon.Hash(a)
	if err != nil {
		return false, err
	}
	y, err := canon.Hash(b)
	if err != nil {
		return false, err
	}
	return x == y, nil
}

// stateExists reports whether an acknowledged state record exists.
func (s *Service) stateExists(ctx context.Context, instanceID string) (bool, error) {
	exists := false
	err := s.deps.State.Transact(ctx, func(ctx context.Context) error {
		blob, err := s.deps.State.StateForUpdate(ctx, instanceID)
		exists = blob != nil
		return err
	})
	return exists, err
}

// gateAttempts refuses a new execution while an earlier attempt is
// unresolved, and refuses CREATE-like execution when an earlier attempt could
// have created objects but no state was ever acknowledged (unknown outcome).
func gateAttempts(attempts []tfresource.Attempt, hasState bool) error {
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

// effectsPossible reports whether an attempt that stopped at stage may have
// changed provider objects.
func effectsPossible(stage string) bool {
	switch stage {
	case StageApplying, StageApplied, StageOutputs, StageReadiness, StageReady, StageDestroying, StageVerifying, StageDestroyed:
		return true
	}
	return false
}

// ownershipFor classifies the attempt: the run's earlier classification is
// kept; a new instance created by this run with no prior state is
// CREATED_BY_RUN; anything older is UPDATED_BY_RUN.
func ownershipFor(attempts []tfresource.Attempt, runID string, inst tfresource.Instance, hasState bool) tfresource.Ownership {
	for _, a := range attempts {
		if a.RunID == runID {
			return a.Ownership
		}
	}
	if inst.CreatedByRunID == runID && !hasState {
		return tfresource.CreatedByRun
	}
	return tfresource.UpdatedByRun
}

// newIntent builds the immutable physical intent of a new instance: the
// stable physical name, stable tags and the opaque reference of each
// executor-owned secret, created once and kept across attempts.
func (s *Service) newIntent(ctx context.Context, st *execState, owned []string) (map[string]any, error) {
	key := physicalKey(st)
	refs := map[string]any{}
	for _, name := range owned {
		if !executorSecret(name) {
			continue
		}
		value, err := randomSecret()
		if err != nil {
			return nil, fmt.Errorf("%w: draw a secret", ErrExecution)
		}
		ref, err := s.deps.Secrets.Put(ctx, "terraform/"+key+"/"+name, value)
		if err != nil {
			return nil, fmt.Errorf("%w: store a secret", ErrExecution)
		}
		refs[name] = ref
	}
	tags := map[string]any{"orchestrator-managed": "true", "orchestrator-instance": key, "orchestrator-created-by-run": st.req.RunID}
	for k, v := range s.cfg.CreationTags {
		tags[k] = v
	}
	return map[string]any{
		"name":       "orc-" + key,
		"tags":       tags,
		"secretRefs": refs,
	}, nil
}

// physicalKey is the stable short identity of the instance used in physical
// names and tags: a prefix of the opaque state key, so it never changes with a
// run, a retry or a Definition version.
func physicalKey(st *execState) string { return st.req.Identity.StateKey()[:16] }

func tagsFromIntent(intent map[string]any) map[string]string {
	out := map[string]string{}
	if tags, ok := intent["tags"].(map[string]any); ok {
		for k, v := range tags {
			if text, ok := v.(string); ok {
				out[k] = text
			}
		}
	}
	return out
}

// executorSecrets reads the stable executor-owned secret values of the
// instance through their persisted references.
func (s *Service) executorSecrets(ctx context.Context, inst tfresource.Instance, owned []string) (map[string]string, error) {
	out := map[string]string{}
	refs, _ := inst.PhysicalIntent["secretRefs"].(map[string]any)
	for _, name := range owned {
		if !executorSecret(name) {
			continue
		}
		ref, _ := refs[name].(string)
		if ref == "" {
			return nil, fmt.Errorf("%w: the secret reference of the instance is missing", ErrInput)
		}
		value, ok := s.deps.Secrets.Get(ctx, ref)
		if !ok {
			return nil, fmt.Errorf("%w: an executor secret is unavailable", ErrInput)
		}
		out[name] = value
	}
	return out, nil
}

// moduleSnapshot is the nonsecret module history of an attempt.
func (s *Service) moduleSnapshot(st *execState) (map[string]any, error) {
	refs := map[string]any{}
	if intentRefs, ok := st.inst.PhysicalIntent["secretRefs"].(map[string]any); ok {
		for _, name := range st.vars.SecretNames {
			refs[name] = intentRefs[name]
		}
	}
	return canon.Map(map[string]any{
		"source": st.req.Source.Map(), "bundleId": st.bundle.ID, "terraformVersion": st.bundle.TerraformVersion,
		"providerLockDigest": st.bundle.ProviderLockDigest, "variables": st.vars.Public, "secretVariables": refs, "action": string(st.action),
		"outputMapping": st.req.OutputMapping,
	})
}
