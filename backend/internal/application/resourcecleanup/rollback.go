package resourcecleanup

import (
	"context"
	"fmt"
	"sort"

	"orchestrator/internal/application/envops"
	"orchestrator/internal/application/tfexec"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/platform/canon"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformstate"
)

type RollbackInventory struct {
	Digest      string
	IDs         []string
	Descriptors []string
	Items       []tfresource.CleanupItem
	ActiveIDs   []string
}

func (s *Service) RollbackInventory(ctx context.Context, sc Scope) (RollbackInventory, error) {
	if !s.mutable() {
		return RollbackInventory{}, ErrUnavailable
	}
	if err := s.authorizeScope(ctx, sc); err != nil {
		return RollbackInventory{}, err
	}
	var out RollbackInventory
	err := s.store.ReadSnapshot(ctx, func(ctx context.Context, st persistence.Store) error {
		snap, err := loadSnapshot(ctx, st, s.reader, sc)
		if err != nil {
			return err
		}
		selected := map[string]bool{}
		for _, id := range snap.order {
			rec := snap.records[id]
			if platformResource(rec) || rec.Instance.Status == tfresource.InstanceDestroyed {
				continue
			}
			if rec.Instance.Identity.TargetGeneration != snap.env.TargetGeneration || !destroyable(rec.Instance.Status) || (rec.Attempts == 0 && rec.Instance.Status != tfresource.InstancePending) {
				return &RefusedError{Reason: ReasonOwnershipUnproven}
			}
			selected[id] = true
		}
		for _, edge := range snap.edges {
			if selected[edge.ProviderID] && !selected[edge.ConsumerID] && edge.Holds() {
				return &RefusedError{Reason: ReasonHasConsumers}
			}
		}
		for id := range selected {
			out.IDs = append(out.IDs, id)
		}
		sort.Strings(out.IDs)
		for _, id := range out.IDs {
			out.Descriptors = append(out.Descriptors, snap.records[id].Instance.Identity.Descriptor)
			inst, err := s.state.GetInstance(ctx, id)
			if err != nil {
				return err
			}
			rec := snap.records[id]
			if inst.Version != rec.Instance.Version || inst.Identity.EnvironmentID != snap.env.ID {
				return ErrStalePreview
			}
			digest, err := tfresource.TargetDigest(inst.TargetRef)
			if err != nil {
				return err
			}
			out.Items = append(out.Items, tfresource.CleanupItem{InstanceID: id, Version: inst.Version, StateKey: inst.StateKey, ModuleBundleID: inst.ModuleBundleID, TargetDigest: digest})
			if rec.Instance.ActiveResourceID != "" {
				out.ActiveIDs = append(out.ActiveIDs, rec.Instance.ActiveResourceID)
			}
		}
		out.Digest, err = canon.Hash(out.Items)
		return err
	})
	return out, err
}

// ClaimRollback keeps one immutable cleanup authorization across delivery removal
// and resource destruction. No dependency is added to the reviewed selection.
func (s *Service) ClaimRollback(ctx context.Context, sc Scope, actor string, expected RollbackInventory, env environment.Environment) (*envops.Lease, error) {
	if actor == "" || len(expected.IDs) == 0 {
		return nil, ErrForbidden
	}
	requestHash, err := canon.Hash([]string{"environment-rollback", env.ID, expected.Digest, fmt.Sprint(env.Version), fmt.Sprint(env.DraftVersion)})
	if err != nil {
		return nil, err
	}
	req := tfresource.CleanupRequest{Mode: tfresource.CleanupUserConfirmed, ActorRef: actor, RootInstanceID: expected.IDs[0], CandidateIDs: expected.IDs, Scope: tfresource.ConfirmedScope{SelectedResourceIDs: expected.IDs, Items: expected.Items}, RequestHash: requestHash, ConfirmationName: sc.ApplicationKey, ScopeSnapshot: map[string]any{"emptyRollback": true, "environmentVersion": env.Version, "currentSetId": env.CurrentDeploymentSetID}}
	claim := persistence.OperationClaim{ID: ids.New(), ApplicationKey: sc.ApplicationKey, EnvironmentKey: sc.EnvironmentKey, Owner: s.ops.InstanceID(), Kind: environment.OpResourceCleanup, Deadline: s.now().Add(envops.DeadlineFor(environment.OpResourceCleanup)), Pins: environment.OperationPins{CheckEnvVersion: true, EnvVersion: env.Version, CheckDraftVersion: true, DraftVersion: env.DraftVersion, CheckSet: true, CurrentSetID: env.CurrentDeploymentSetID}, Detail: map[string]any{"rollback": true, "emptyRollback": true}}
	op, replayed, err := s.claims.ClaimResourceCleanup(ctx, claim, req, terraformstate.AdmissionCheck(func(ctx context.Context) error {
		fresh, err := s.RollbackInventory(ctx, sc)
		if err != nil {
			return err
		}
		if fresh.Digest != expected.Digest {
			return ErrStalePreview
		}
		return nil
	}))
	if err != nil {
		return nil, err
	}
	if replayed {
		return nil, ErrStalePreview
	}
	return s.ops.Adopt(ctx, op), nil
}

// FinishRollbackCleanup is invoked only after all workload delivery is verified
// removed under this lease. It does not end or release the parent operation.
func (s *Service) FinishRollbackCleanup(lease *envops.Lease, sc Scope, inventory RollbackInventory) error {
	req, err := s.claims.CleanupRequest(lease.Ctx, lease.Operation.ID)
	if err != nil {
		return err
	}
	if req.ScopeSnapshot["emptyRollback"] != true || req.ConfirmationName != sc.ApplicationKey || !tfresource.SameSelection(req.Scope.SelectedResourceIDs, inventory.IDs) {
		return ErrStalePreview
	}
	env, err := s.store.GetEnvironment(lease.Ctx, sc.ApplicationKey, sc.EnvironmentKey)
	if err != nil {
		return err
	}
	set, err := s.store.GetDeploymentSet(lease.Ctx, env.CurrentDeploymentSetID)
	if err != nil {
		return err
	}
	if len(set.Document.Modules) != 0 || env.ActiveOperationID != lease.Operation.ID {
		return ErrStalePreview
	}
	instances, err := s.store.ListWorkloadInstances(lease.Ctx, sc.ApplicationKey+"/"+sc.EnvironmentKey)
	if err != nil {
		return err
	}
	for _, instance := range instances {
		if instance.Status != "REMOVED" {
			return &RefusedError{Reason: ReasonWorkloadInUse}
		}
	}
	for index, item := range req.Scope.Items {
		inst, err := s.state.GetInstance(lease.Ctx, item.InstanceID)
		if err != nil {
			return err
		}
		digest, err := tfresource.TargetDigest(inst.TargetRef)
		if err != nil {
			return err
		}
		if inst.Identity.EnvironmentID != env.ID || inst.Identity.TargetGeneration != env.TargetGeneration || inst.StateKey != item.StateKey || inst.ModuleBundleID != item.ModuleBundleID || digest != item.TargetDigest {
			return ErrStalePreview
		}
		// A failed first Deploy can own resources outside the committed Set.
		// Only after every workload is verified absent may this claim retire
		// their reference metadata, using the reviewed version and identity.
		if inst.Version == item.Version && (inst.Status == tfresource.InstanceReady || inst.Status == tfresource.InstanceFailed) {
			writer, ok := s.state.(interface {
				SetInstanceStatus(context.Context, string, int64, tfresource.InstanceStatus) (tfresource.Instance, error)
			})
			if !ok {
				return ErrUnavailable
			}
			inst, err = writer.SetInstanceStatus(lease.Ctx, inst.ID, inst.Version, tfresource.InstanceUnreferenced)
			if err != nil {
				return err
			}
		}
		if inst.Version != item.Version {
			if inst.Version != item.Version+1 || inst.Status != tfresource.InstanceUnreferenced {
				return ErrStalePreview
			}
			req.Scope.Items[index].Version = inst.Version
		}
	}
	s.setActive(lease.Ctx, sc, inventory.ActiveIDs, resource.StatusDestroying)
	source := rollbackCleanupSource{request: req, operationID: lease.Operation.ID}
	result, err := s.engine.RunCleanup(lease.Ctx, sc.OrganizationKey, source, func(stage string, detail map[string]any) error { return lease.Stage(stage, nil) })
	if err != nil {
		return err
	}
	destroyed := map[string]bool{}
	keep := map[string]bool{}
	for _, item := range result.Instances {
		if item.Destroyed {
			destroyed[item.InstanceID] = true
		}
		if item.Code == tfexec.CodeNeedsAttention {
			keep[item.InstanceID] = true
		}
	}
	s.finalizeActive(lease.Ctx, sc, inventory.IDs, inventory.ActiveIDs, destroyed, keep)
	if result.Terminal != tfexec.CleanupSucceeded || len(destroyed) != len(inventory.IDs) {
		return fmt.Errorf("rollback resource cleanup is incomplete")
	}
	return nil
}

type rollbackCleanupSource struct {
	request     tfresource.CleanupRequest
	operationID string
}

func (s rollbackCleanupSource) CleanupRequest(ctx context.Context, op string) (tfresource.CleanupRequest, error) {
	owner, ok := persistence.OwnerFrom(ctx)
	if !ok || op != s.operationID || owner.OperationID != op {
		return tfresource.CleanupRequest{}, persistence.ErrOperationLost
	}
	return s.request, nil
}
