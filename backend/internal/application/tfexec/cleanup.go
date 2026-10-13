package tfexec

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/ports/persistence"
)

// CleanupSource reads the admitted cleanup authorization of an operation
// (persisted by the admission, I04-08, together with the Environment claim).
type CleanupSource interface {
	CleanupRequest(ctx context.Context, operationID string) (tfresource.CleanupRequest, error)
}

// CleanupTerminal is the terminal result of a cleanup operation.
type CleanupTerminal string

// Terminal results. NeedsAttention keeps the Environment held.
const (
	CleanupSucceeded      CleanupTerminal = "SUCCEEDED"
	CleanupFailed         CleanupTerminal = "FAILED"
	CleanupNeedsAttention CleanupTerminal = "NEEDS_ATTENTION"
)

// Cleanup stages as shown to the operation (UC-07, state contract).
const (
	CleanupRequested  = "REQUESTED"
	CleanupChecking   = StageChecking
	CleanupDestroying = StageDestroying
	CleanupVerifying  = StageVerifying
)

// InstanceResult is the safe per-instance outcome of a cleanup.
type InstanceResult struct {
	InstanceID string
	Destroyed  bool
	// Remaining lists physical objects not verified absent: the pre-destroy
	// objects of a failed destroy, or the last acknowledged observations of a
	// retained, blocked or unattempted resource.
	Remaining []string
	// RemainingUnknown is set when the resource is not verified absent and no
	// physical ID is known; an empty Remaining is never proof of absence.
	RemainingUnknown bool
	// Reason is a safe sentence for a retained or failed instance.
	Reason string
}

// CleanupResult is the safe result of RunCleanup. Terminal reports the
// operation outcome; Instances every selected instance in destroy order.
type CleanupResult struct {
	Terminal  CleanupTerminal
	Stage     string
	Instances []InstanceResult
}

// RemainingIDs lists every physical object still present, for the operation
// detail after a failure.
func (r CleanupResult) RemainingIDs() []string {
	out := []string{}
	for _, i := range r.Instances {
		out = append(out, i.Remaining...)
	}
	sort.Strings(out)
	return out
}

// StageReporter persists the operation stage (envops.Lease.Stage).
type StageReporter func(stage string, detail map[string]any) error

// RunCleanup executes the admitted cleanup of the claim owner's operation.
//
// It destroys exactly the selected set and never adds an item. Before any
// effect it re-checks, under the Environment claim: the pinned item versions,
// state keys and module bundles; ownership (run cleanup: only CREATED_BY_RUN
// of the cleanup run; user cleanup: only resources managed by the
// orchestrator in a destroyable status); and the dependency graph (no consumer
// outside the set may still hold a selected provider; no cycle). Violations
// reject the whole request before any destroy.
//
// Instances are destroyed consumers first. A consumer that fails or is held
// keeps its providers; independent branches continue after a confirmed
// failure. An unknown outcome stops everything and holds. Success needs a
// physical absence proof per instance (the executor's Destroy).
func (s *Service) RunCleanup(ctx context.Context, organizationKey string, source CleanupSource, report StageReporter) (CleanupResult, error) {
	owner, ok := persistence.OwnerFrom(ctx)
	if !ok {
		return CleanupResult{}, ErrNoClaim
	}
	say := func(stage string, detail map[string]any) {
		if report != nil {
			_ = report(stage, detail)
		}
	}
	say(CleanupRequested, nil)
	req, err := source.CleanupRequest(ctx, owner.OperationID)
	if err != nil {
		return CleanupResult{}, fmt.Errorf("%w: the admitted request is unreadable", ErrCleanupRejected)
	}
	// The pinned candidate list only bounded the admission; the stored,
	// confirmed selection is the authority from here on.
	req.CandidateIDs = req.Scope.SelectedResourceIDs
	if err := req.Validate(); err != nil {
		return CleanupResult{}, fmt.Errorf("%w: the admitted request is invalid: %v", ErrCleanupRejected, err)
	}
	say(CleanupChecking, nil)
	order, err := s.checkCleanup(ctx, req)
	if err != nil {
		return CleanupResult{Terminal: CleanupFailed, Stage: CleanupChecking}, err
	}

	result := CleanupResult{Terminal: CleanupSucceeded, Stage: CleanupVerifying}
	runID := "cleanup-" + owner.OperationID
	destroyed := map[string]bool{}
	selected := map[string]bool{}
	for _, id := range req.Scope.SelectedResourceIDs {
		selected[id] = true
	}
	held := false
	for _, id := range order.destroyOrder {
		res := InstanceResult{InstanceID: id}
		switch {
		case held:
			res.Reason = "not attempted: an earlier step needs operator attention"
		default:
			if blocker := order.blockedBy(id, destroyed); blocker != "" {
				res.Reason = "retained: a consumer in the selection was not destroyed"
				break
			}
			say(CleanupDestroying, map[string]any{"resource": id})
			out, derr := s.Destroy(ctx, DestroyRequest{InstanceID: id, OrganizationKey: organizationKey, RunID: runID, OnStage: func(stage string) { say(stage, map[string]any{"resource": id}) }})
			res.Destroyed, res.Remaining, res.RemainingUnknown = out.Destroyed, out.Remaining, out.RemainingUnknown
			switch {
			case derr == nil && out.Destroyed:
				destroyed[id] = true
			case errors.Is(derr, ErrNeedsAttention):
				held = true
				res.Reason = "needs operator attention"
			default:
				res.Reason = "failed: " + safeReason(derr)
			}
		}
		if !res.Destroyed && len(res.Remaining) == 0 && !res.RemainingUnknown {
			// Retained or not attempted: report what is still there.
			res.Remaining, res.RemainingUnknown = s.knownRemaining(ctx, id)
		}
		result.Instances = append(result.Instances, res)
	}
	for _, r := range result.Instances {
		if !r.Destroyed {
			result.Terminal = CleanupFailed
		}
	}
	if held {
		result.Terminal = CleanupNeedsAttention
	}
	return result, nil
}

// knownRemaining returns the last acknowledged physical IDs of an instance
// that was not destroyed; unknown when none are recorded.
func (s *Service) knownRemaining(ctx context.Context, id string) ([]string, bool) {
	pctx, cancel := persistCtx(ctx)
	defer cancel()
	attempts, err := s.deps.State.ListAttempts(pctx, id)
	if err != nil {
		return nil, true
	}
	ids := lastObserved(attempts)
	return ids, len(ids) == 0
}

func safeReason(err error) string {
	switch {
	case errors.Is(err, ErrInstanceHeld):
		return "an earlier attempt is unresolved"
	case errors.Is(err, ErrBinding):
		return "the original target cannot be reached"
	case errors.Is(err, ErrDestructivePlan):
		return "the destroy plan was not a pure delete"
	case errors.Is(err, ErrNamespaceContents):
		return "the namespace holds objects that are not owned by the selection"
	}
	return "the destroy did not complete"
}

// cleanupOrder is the verified destroy plan of a selected set.
type cleanupOrder struct {
	destroyOrder []string
	// consumers maps a selected provider to its selected consumers.
	consumers map[string][]string
}

// blockedBy returns a selected consumer of id that is not destroyed.
func (o cleanupOrder) blockedBy(id string, destroyed map[string]bool) string {
	for _, c := range o.consumers[id] {
		if !destroyed[c] {
			return c
		}
	}
	return ""
}

// checkCleanup performs every pre-effect check and returns the order.
func (s *Service) checkCleanup(ctx context.Context, req tfresource.CleanupRequest) (cleanupOrder, error) {
	reject := func(why string) (cleanupOrder, error) {
		return cleanupOrder{}, fmt.Errorf("%w: %s", ErrCleanupRejected, why)
	}
	selected := map[string]bool{}
	for _, id := range req.Scope.SelectedResourceIDs {
		selected[id] = true
	}
	items := map[string]tfresource.CleanupItem{}
	for _, item := range req.Scope.Items {
		items[item.InstanceID] = item
	}
	instances := map[string]tfresource.Instance{}
	var envID, orgID string
	var generation int64
	for id := range selected {
		inst, err := s.deps.State.GetInstance(ctx, id)
		if err != nil {
			return reject("a selected resource does not exist")
		}
		item := items[id]
		if inst.Version != item.Version || inst.StateKey != item.StateKey || inst.ModuleBundleID != item.ModuleBundleID {
			return reject("a selected resource changed after admission; review it again")
		}
		if same, err := sameJSON(inst.TargetRef, item.TargetRef); err != nil || !same {
			return reject("a selected resource target changed after admission; review it again")
		}
		if envID == "" {
			envID, orgID, generation = inst.Identity.EnvironmentID, inst.Identity.OrganizationID, inst.Identity.TargetGeneration
		}
		if inst.Identity.EnvironmentID != envID || inst.Identity.OrganizationID != orgID {
			return reject("the selection spans Environments")
		}
		if inst.Identity.TargetGeneration != generation {
			return reject("the selection spans target generations")
		}
		switch inst.Status {
		case tfresource.InstanceReady, tfresource.InstanceUnreferenced, tfresource.InstanceFailed, tfresource.InstancePending:
		default:
			return reject("a selected resource is not in a destroyable status")
		}
		// A user deletion starts from an unreferenced (or failed) root; the
		// admission checked this, and the claim must not have changed it.
		if req.Mode == tfresource.CleanupUserConfirmed && id == req.RootInstanceID && inst.Status != tfresource.InstanceUnreferenced {
			return reject("the root resource must be UNREFERENCED")
		}
		instances[id] = inst
	}
	// Ownership.
	for id, inst := range instances {
		attempts, err := s.deps.State.ListAttempts(ctx, id)
		if err != nil {
			return reject("attempt history is unavailable")
		}
		if err := checkOwnership(req, inst, attempts); err != nil {
			return reject(err.Error())
		}
	}
	// Graph: consumers outside the set that still hold a provider reject the
	// request; selected consumers order the destroy.
	consumers := map[string][]string{}
	providersOf := map[string][]string{}
	for id := range selected {
		deps, err := s.deps.State.Dependencies(ctx, id)
		if err != nil {
			return reject("the dependency graph is unavailable")
		}
		for _, d := range deps {
			if d.ProviderID != id {
				continue
			}
			if selected[d.ConsumerID] {
				consumers[id] = append(consumers[id], d.ConsumerID)
				providersOf[d.ConsumerID] = append(providersOf[d.ConsumerID], id)
				continue
			}
			if d.Holds() {
				return reject("a consumer outside the selection still holds a selected resource")
			}
		}
	}
	for id := range consumers {
		sort.Strings(consumers[id])
	}
	// Reverse topological order: a resource comes after all its consumers.
	order := []string{}
	done := map[string]bool{}
	ids := make([]string, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for len(order) < len(ids) {
		progressed := false
		for _, id := range ids {
			if done[id] {
				continue
			}
			ready := true
			for _, c := range consumers[id] {
				if !done[c] {
					ready = false
					break
				}
			}
			if ready {
				done[id] = true
				order = append(order, id)
				progressed = true
			}
		}
		if !progressed {
			return reject("the selection has a dependency cycle")
		}
	}
	return cleanupOrder{destroyOrder: order, consumers: consumers}, nil
}

// checkOwnership applies the authorization-specific ownership rule.
func checkOwnership(req tfresource.CleanupRequest, inst tfresource.Instance, attempts []tfresource.Attempt) error {
	switch req.Mode {
	case tfresource.CleanupRunCleanup:
		// Only objects this run created: its first attempt is CREATED_BY_RUN
		// and the instance was registered by it. Anything updated by a later
		// run, preexisting or unknown is not the run's to delete.
		if inst.CreatedByRunID != req.RunID {
			return errors.New("a selected resource was not created by the cleanup run")
		}
		var first *tfresource.Attempt
		for i := range attempts {
			if attempts[i].RunID == req.RunID && attempts[i].Action != tfresource.ActionDestroy {
				first = &attempts[i]
				break
			}
		}
		if first == nil || first.Ownership != tfresource.CreatedByRun {
			return errors.New("a selected resource is not CREATED_BY_RUN for the cleanup run")
		}
		// Another run's attempt that changed, or may have changed, the
		// object blocks run cleanup whatever its final status: a failed or
		// interrupted UPDATE past the point of effect is as disqualifying as
		// a successful one.
		for _, a := range attempts {
			if a.RunID == req.RunID || a.Action == tfresource.ActionDestroy {
				continue
			}
			if a.Status == tfresource.AttemptSucceeded || a.Status == tfresource.AttemptRunning || a.Status == tfresource.AttemptInterrupted ||
				a.Status == tfresource.AttemptNeedsAttention || effectsPossible(a.Stage) {
				return errors.New("a selected resource was changed, or may have been changed, by another run")
			}
		}
	case tfresource.CleanupUserConfirmed:
		// Durable ownership: the instance is an orchestrator-managed Terraform
		// instance of the claimed Environment (it exists in this registry);
		// platform and preexisting resources have no instance.
		if len(attempts) == 0 && inst.Status != tfresource.InstancePending {
			return errors.New("a selected resource has no execution history")
		}
	default:
		return errors.New("the cleanup authorization is unknown")
	}
	return nil
}
