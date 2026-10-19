// Package runcleanup destroys every Terraform instance a run left behind
// through the ORIGINAL pinned cleanup engine (tfexec RunCleanup in RUN_CLEANUP
// mode): reverse dependency order, the instance's own pinned module, original
// scoped target and encrypted state, an Environment claim with fencing, and a
// physical absence proof per instance. It is the independent safety net of the
// acceptance runner: it is a different process from the product server, built
// from the same wiring, and it never deletes Kubernetes or cloud objects
// itself, never unlocks or takes over a held Environment claim, never resumes
// an unknown outcome and never discards state: a failure or an unknown
// outcome leaves the encrypted state, claims and recovery artifacts exactly as
// the engine left them and reports NEEDS_ATTENTION.
package runcleanup

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"orchestrator/internal/application/envops"
	"orchestrator/internal/application/tfexec"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformstate"
)

// Engine is the existing tfexec cleanup engine.
type Engine interface {
	RunCleanup(ctx context.Context, organizationKey string, source tfexec.CleanupSource, report tfexec.StageReporter) (tfexec.CleanupResult, error)
}

// State reads the instances of an Environment on the State login.
type State interface {
	ListInstances(ctx context.Context, environmentID string) ([]tfresource.Instance, error)
	GetInstance(ctx context.Context, id string) (tfresource.Instance, error)
}

// Environments reads the Console Environment.
type Environments interface {
	GetEnvironment(ctx context.Context, applicationKey, environmentKey string) (environment.Environment, error)
}

// Claims admits one RUN_CLEANUP operation: it claims the Environment together
// with the persisted request, in one transaction, exactly like the user
// cleanup admission. A held Environment is ErrEnvironmentBusy.
type Claims interface {
	ClaimResourceCleanup(ctx context.Context, claim persistence.OperationClaim, req tfresource.CleanupRequest, check terraformstate.AdmissionCheck) (environment.Operation, bool, error)
	CleanupRequest(ctx context.Context, operationID string) (tfresource.CleanupRequest, error)
}

// Operations adopts an admitted operation into a lease (fencing, heartbeat).
type Operations interface {
	InstanceID() string
	Adopt(ctx context.Context, op environment.Operation) *envops.Lease
}

// Target names the Environment to clean and the TRUSTED ownership recorded by
// the runner BEFORE any create: the Terraform run IDs the run deployed with and
// the target generation of its Environment. Only instances created by one of
// these runs in that generation are ever destroyed; anything else in the
// registry (preexisting, retained, another run, another generation) is
// reported as not run-owned and left untouched.
type Target struct {
	OrganizationKey, ApplicationKey, EnvironmentKey string
	RunIDs                                          []string
	Generation                                      int64
	// InstanceIDs optionally narrows recorded-run ownership; nil retains whole-run cleanup.
	InstanceIDs []string
}

// Outcomes.
const (
	OutcomeClean     = "CLEAN"
	OutcomeAttention = "NEEDS_ATTENTION"
	OutcomeBusy      = "ENVIRONMENT_BUSY"
)

// Blocked is an instance the safety net did not (or could not) destroy.
type Blocked struct {
	InstanceID, Descriptor, Status, Reason string
}

// RunReport is the engine result of one run's instances.
type RunReport struct {
	RunID     string
	Terminal  string
	Instances []tfexec.InstanceResult
}

// Report is the safe result.
type Report struct {
	Outcome string
	Runs    []RunReport
	Blocked []Blocked
	// Remaining lists the instances not verified DESTROYED afterwards.
	Remaining []string
}

// Service runs the cleanup.
type Service struct {
	envs   Environments
	state  State
	engine Engine
	claims Claims
	ops    Operations
	// deadline is the finite hold of the RUN_CLEANUP claim (the cleanup budget
	// of the run); zero keeps envops.DeadlineFor.
	deadline time.Duration
}

// SetDeadline bounds the RUN_CLEANUP claim by the recorded cleanup budget. It
// never extends beyond envops.MaxDeadline.
func (s *Service) SetDeadline(d time.Duration) error {
	if d <= 0 || d > envops.MaxDeadline {
		return fmt.Errorf("runcleanup: the cleanup deadline must be positive and at most %s", envops.MaxDeadline)
	}
	s.deadline = d
	return nil
}

func (s *Service) holdFor() time.Duration {
	if s.deadline > 0 {
		return s.deadline
	}
	return envops.DeadlineFor(environment.OpResourceCleanup)
}

// New builds the service. Every dependency is required.
func New(envs Environments, state State, engine Engine, claims Claims, ops Operations) *Service {
	return &Service{envs: envs, state: state, engine: engine, claims: claims, ops: ops}
}

// ErrBusy means another operation owns the Environment (a runner killed
// mid-operation leaves its claim INTERRUPTED, never released): the safety net
// does not unlock it.
var ErrBusy = errors.New("runcleanup: the Environment is held by another operation; it is not unlocked or taken over")

func destroyable(s tfresource.InstanceStatus) bool {
	switch s {
	case tfresource.InstanceReady, tfresource.InstanceUnreferenced, tfresource.InstanceFailed, tfresource.InstancePending:
		return true
	}
	return false
}

// Run destroys every non-destroyed instance of the Environment's current
// target generation, grouped by the run that created it.
func (s *Service) Run(ctx context.Context, t Target) (Report, error) {
	env, err := s.envs.GetEnvironment(ctx, t.ApplicationKey, t.EnvironmentKey)
	if err != nil {
		return Report{Outcome: OutcomeAttention}, fmt.Errorf("runcleanup: the Environment cannot be read: %w", err)
	}
	instances, err := s.state.ListInstances(ctx, env.ID)
	if err != nil {
		return Report{Outcome: OutcomeAttention}, fmt.Errorf("runcleanup: the state registry cannot be read: %w", err)
	}
	report := Report{}
	if env.TargetGeneration != t.Generation {
		// The Environment moved to another target generation than the one the run recorded: nothing here is provably the run's.
		report.Outcome = OutcomeAttention
		return report, fmt.Errorf("runcleanup: the Environment is on target generation %d, the run recorded %d; nothing was touched", env.TargetGeneration, t.Generation)
	}
	owned := map[string]bool{}
	for _, id := range t.RunIDs {
		owned[id] = true
	}
	selected := map[string]bool{}
	if t.InstanceIDs != nil {
		if len(t.InstanceIDs) == 0 {
			return Report{Outcome: OutcomeAttention}, errors.New("runcleanup: explicit selection is empty")
		}
		byID := map[string]tfresource.Instance{}
		for _, inst := range instances {
			byID[inst.ID] = inst
		}
		for _, id := range t.InstanceIDs {
			inst, found := byID[id]
			if id == "" || selected[id] || !found || inst.Identity.TargetGeneration != t.Generation || !owned[inst.CreatedByRunID] {
				return Report{Outcome: OutcomeAttention}, errors.New("runcleanup: explicit selection identity or ownership is invalid")
			}
			selected[id] = true
		}
	}
	byRun := map[string][]tfresource.Instance{}
	for _, inst := range instances {
		switch {
		case inst.Status == tfresource.InstanceDestroyed:
		case inst.Identity.TargetGeneration != t.Generation:
			report.Blocked = append(report.Blocked, Blocked{inst.ID, inst.Identity.Descriptor, string(inst.Status), "OTHER_TARGET_GENERATION"})
		case !owned[inst.CreatedByRunID]:
			// Not created by a run the manifest recorded: never self-authorize destruction of it.
			report.Blocked = append(report.Blocked, Blocked{inst.ID, inst.Identity.Descriptor, string(inst.Status), "NOT_RUN_OWNED"})
		case t.InstanceIDs != nil && !selected[inst.ID]:
			report.Blocked = append(report.Blocked, Blocked{inst.ID, inst.Identity.Descriptor, string(inst.Status), "NOT_SELECTED"})
		case !destroyable(inst.Status):
			report.Blocked = append(report.Blocked, Blocked{inst.ID, inst.Identity.Descriptor, string(inst.Status), "NOT_DESTROYABLE_STATUS"})
		default:
			byRun[inst.CreatedByRunID] = append(byRun[inst.CreatedByRunID], inst)
		}
	}
	if len(byRun) == 0 && len(report.Blocked) == 0 {
		report.Outcome = OutcomeClean
		return report, nil
	}
	if len(byRun) == 0 {
		report.Outcome = OutcomeAttention
		return report, nil
	}
	pending := make([]string, 0, len(byRun))
	for run := range byRun {
		pending = append(pending, run)
	}
	sort.Strings(pending)
	held := false
	// Instances of different runs may depend on each other: retry rejected runs
	// while any run still makes progress; a rejection never forces an order.
	for len(pending) > 0 && !held {
		var next []string
		progressed := false
		for _, run := range pending {
			rr, outcome, cerr := s.cleanRun(ctx, t, env, run, byRun[run])
			if rr.RunID != "" {
				report.Runs = append(report.Runs, rr)
			}
			switch outcome {
			case runBusy:
				report.Outcome = OutcomeBusy
				return report, cerr
			case runHeld:
				held = true
			case runRejected:
				next = append(next, run)
			default:
				progressed = true
			}
			if held {
				break
			}
		}
		if !progressed && !held {
			for _, run := range next {
				for _, inst := range byRun[run] {
					report.Blocked = append(report.Blocked, Blocked{inst.ID, inst.Identity.Descriptor, string(inst.Status), "REJECTED_BY_ENGINE"})
				}
			}
			break
		}
		pending = next
	}
	// Independent proof of the registry state afterwards.
	after, listErr := s.state.ListInstances(ctx, env.ID)
	if listErr != nil {
		report.Remaining = append(report.Remaining, "registry-unreadable")
	}
	for _, inst := range after {
		if inst.Status != tfresource.InstanceDestroyed {
			report.Remaining = append(report.Remaining, inst.Identity.Descriptor+"="+string(inst.Status))
		}
	}
	sort.Strings(report.Remaining)
	if !held && len(report.Remaining) == 0 && len(report.Blocked) == 0 {
		report.Outcome = OutcomeClean
	} else {
		report.Outcome = OutcomeAttention
	}
	return report, nil
}

type runOutcome int

const (
	runDone runOutcome = iota
	runRejected
	runHeld
	runBusy
)

// cleanRun admits and runs one RUN_CLEANUP operation for one run's instances.
func (s *Service) cleanRun(ctx context.Context, t Target, env environment.Environment, run string, instances []tfresource.Instance) (RunReport, runOutcome, error) {
	rr := RunReport{RunID: run}
	sort.Slice(instances, func(i, j int) bool { return instances[i].ID < instances[j].ID })
	req := tfresource.CleanupRequest{Mode: tfresource.CleanupRunCleanup, ActorRef: "acceptance-safety-net", RunID: run, RootInstanceID: instances[0].ID, ScopeSnapshot: map[string]any{"generation": env.TargetGeneration}}
	for _, inst := range instances {
		full, err := s.state.GetInstance(ctx, inst.ID)
		if err != nil {
			rr.Terminal = "STATE_UNREADABLE"
			return rr, runHeld, nil
		}
		digest, err := tfresource.TargetDigest(full.TargetRef)
		if err != nil {
			rr.Terminal = "TARGET_UNREADABLE"
			return rr, runHeld, nil
		}
		req.Scope.SelectedResourceIDs = append(req.Scope.SelectedResourceIDs, full.ID)
		req.Scope.Items = append(req.Scope.Items, tfresource.CleanupItem{InstanceID: full.ID, Version: full.Version, StateKey: full.StateKey, ModuleBundleID: full.ModuleBundleID, TargetDigest: digest})
	}
	req.CandidateIDs = req.Scope.SelectedResourceIDs
	if err := req.Validate(); err != nil {
		rr.Terminal = "INVALID_REQUEST"
		return rr, runRejected, nil
	}
	claim := persistence.OperationClaim{
		ID: ids.New(), ApplicationKey: t.ApplicationKey, EnvironmentKey: t.EnvironmentKey, Owner: s.ops.InstanceID(),
		Kind: environment.OpResourceCleanup, Deadline: time.Now().UTC().Add(s.holdFor()),
		Detail: map[string]any{"kind": "RUN_CLEANUP", "origin": "acceptance-safety-net", "runId": run},
	}
	op, _, err := s.claims.ClaimResourceCleanup(ctx, claim, req, nil)
	switch {
	case errors.Is(err, persistence.ErrEnvironmentBusy):
		return rr, runBusy, ErrBusy
	case err != nil:
		rr.Terminal = "NOT_ADMITTED"
		return rr, runRejected, nil
	}
	lease := s.ops.Adopt(ctx, op)
	result, runErr := s.engine.RunCleanup(lease.Ctx, t.OrganizationKey, s.claims, nil)
	rr.Terminal, rr.Instances = string(result.Terminal), result.Instances
	switch {
	case runErr != nil && errors.Is(runErr, tfexec.ErrCleanupRejected):
		rr.Terminal = "REJECTED"
		_ = lease.End(environment.OpFailed, "CLEANUP_REJECTED")
		return rr, runRejected, nil
	case runErr != nil, result.Terminal == tfexec.CleanupNeedsAttention:
		// Unknown outcomes and engine errors hold: no retry, no resume, the claim stays.
		_ = lease.Suspend("CLEANUP_NEEDS_ATTENTION")
		return rr, runHeld, nil
	case result.Terminal != tfexec.CleanupSucceeded:
		_ = lease.End(environment.OpFailed, "CLEANUP_INCOMPLETE")
		return rr, runDone, nil
	}
	_ = lease.End(environment.OpSucceeded, "")
	return rr, runDone, nil
}

// Remaining lists the instances of the Environment's current target generation
// that are not verified DESTROYED in the registry (read-only).
func (s *Service) Remaining(ctx context.Context, env environment.Environment) ([]string, error) {
	instances, err := s.state.ListInstances(ctx, env.ID)
	if err != nil {
		return []string{"registry-unreadable"}, err
	}
	var out []string
	for _, inst := range instances {
		if inst.Status != tfresource.InstanceDestroyed {
			out = append(out, inst.Identity.Descriptor+"="+string(inst.Status))
		}
	}
	sort.Strings(out)
	return out, nil
}
