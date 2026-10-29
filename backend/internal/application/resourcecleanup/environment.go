package resourcecleanup

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"time"

	"orchestrator/internal/application/envops"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformstate"
)

// Application deletion (UC-18, ADR-025) reuses the confirmed cleanup engine
// for a whole Environment: every owned, not yet destroyed instance of one
// target generation is admitted as one RESOURCE_CLEANUP operation under the
// actor who confirmed the Application deletion. The selection is computed by
// the backend, never by the client, and is re-checked in the claim transaction.

// Inventory reasons of an instance that blocks an Application deletion.
const (
	ReasonInstanceHeld   = "RESOURCE_HELD"
	ReasonWorkloadInUse  = "WORKLOAD_IN_USE"
	ReasonNoRootEligible = "NO_UNREFERENCED_ROOT"
)

// InventoryItem is one owned instance of an Environment.
type InventoryItem struct {
	ID         string `json:"id"`
	Descriptor string `json:"descriptor"`
	Type       string `json:"type"`
	Class      string `json:"class"`
	Status     string `json:"status"`
	Generation int64  `json:"generation"`
	// Blocker is the allowlisted reason this instance blocks an Application
	// deletion before any effect, or "".
	Blocker string `json:"blocker,omitempty"`
}

// Inventory is the read-only view of the owned instances of an Environment
// that an Application deletion must destroy.
type Inventory struct {
	Busy     bool
	Items    []InventoryItem
	Platform int
}

// destroyable reports instance statuses the cleanup engine accepts.
func destroyable(s tfresource.InstanceStatus) bool {
	switch s {
	case tfresource.InstanceReady, tfresource.InstanceUnreferenced, tfresource.InstanceFailed, tfresource.InstancePending:
		return true
	}
	return false
}

// EnvironmentInventory lists every owned, not yet destroyed instance of the
// Environment (all generations). It reads persisted metadata only.
func (s *Service) EnvironmentInventory(ctx context.Context, sc Scope) (Inventory, error) {
	if !s.readable() {
		return Inventory{}, ErrUnavailable
	}
	var out Inventory
	err := s.store.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error {
		snap, err := loadSnapshot(ctx, view, s.reader, sc)
		if err != nil {
			return err
		}
		out = inventoryOf(snap)
		return nil
	})
	return out, err
}

func inventoryOf(snap *snapshot) Inventory {
	out := Inventory{Busy: snap.env.ActiveOperationID != "", Items: []InventoryItem{}}
	for _, id := range snap.order {
		rec := snap.records[id]
		if platformResource(rec) {
			out.Platform++
			continue
		}
		if rec.Instance.Status == tfresource.InstanceDestroyed {
			continue
		}
		d, _ := resource.ParseDescriptor(rec.Instance.Identity.Descriptor)
		item := InventoryItem{ID: id, Descriptor: rec.Instance.Identity.Descriptor, Type: d.Type, Class: d.Class,
			Status: safeInstanceStatus(rec.Instance.Status), Generation: rec.Instance.Identity.TargetGeneration}
		switch {
		case !destroyable(rec.Instance.Status):
			item.Blocker = ReasonInstanceHeld
		case rec.Attempts == 0 && rec.Instance.Status != tfresource.InstancePending:
			item.Blocker = ReasonOwnershipUnproven
		}
		out.Items = append(out.Items, item)
	}
	sort.Slice(out.Items, func(i, j int) bool {
		if out.Items[i].Generation != out.Items[j].Generation {
			return out.Items[i].Generation < out.Items[j].Generation
		}
		return out.Items[i].Descriptor < out.Items[j].Descriptor
	})
	return out
}

// generationSelection is the whole owned set of one generation and its root.
type generationSelection struct {
	root     string
	selected []string
}

// selectGeneration computes the complete selection of one generation after
// every workload was removed. Any blocker refuses the whole generation.
func selectGeneration(snap *snapshot, generation int64) (generationSelection, error) {
	if snap.env.ActiveOperationID != "" {
		return generationSelection{}, persistence.Busy(snap.scope.ApplicationKey, snap.scope.EnvironmentKey)
	}
	if len(snap.doc.ModuleIDs()) > 0 {
		return generationSelection{}, &RefusedError{Reason: ReasonWorkloadInUse}
	}
	for _, draft := range snap.drafts {
		if draft.State == environment.DraftUpsert {
			return generationSelection{}, &RefusedError{Reason: ReasonWorkloadInUse}
		}
	}
	inSet := map[string]bool{}
	var selected []string
	for _, id := range snap.order {
		rec := snap.records[id]
		if platformResource(rec) || rec.Instance.Identity.TargetGeneration != generation || rec.Instance.Status == tfresource.InstanceDestroyed {
			continue
		}
		if !destroyable(rec.Instance.Status) {
			return generationSelection{}, &RefusedError{Reason: ReasonInstanceHeld}
		}
		if rec.Attempts == 0 && rec.Instance.Status != tfresource.InstancePending {
			return generationSelection{}, &RefusedError{Reason: ReasonOwnershipUnproven}
		}
		inSet[id] = true
		selected = append(selected, id)
	}
	if len(selected) == 0 {
		return generationSelection{}, persistence.ErrNotFound
	}
	// A consumer outside the generation's owned set that still holds a
	// selected provider refuses the whole set.
	for _, e := range snap.edges {
		if inSet[e.ProviderID] && !inSet[e.ConsumerID] && e.Holds() {
			return generationSelection{}, &RefusedError{Reason: ReasonHasConsumers}
		}
	}
	sort.Slice(selected, func(i, j int) bool {
		return snap.records[selected[i]].Instance.Identity.Descriptor < snap.records[selected[j]].Instance.Identity.Descriptor
	})
	// The engine requires an UNREFERENCED root of a user-confirmed cleanup;
	// prefer one that no selected resource consumes.
	consumed := map[string]bool{}
	for _, e := range snap.edges {
		if inSet[e.ProviderID] && inSet[e.ConsumerID] {
			consumed[e.ProviderID] = true
		}
	}
	root := ""
	for _, pass := range []bool{false, true} {
		for _, id := range selected {
			if snap.records[id].Instance.Status == tfresource.InstanceUnreferenced && (pass || !consumed[id]) {
				root = id
				break
			}
		}
		if root != "" {
			break
		}
	}
	if root == "" {
		return generationSelection{}, &RefusedError{Reason: ReasonNoRootEligible}
	}
	return generationSelection{root: root, selected: selected}, nil
}

// EnvironmentCleanupInput identifies the Application deletion an
// Environment cleanup belongs to.
type EnvironmentCleanupInput struct {
	// DeletionID is the Application deletion operation; together with the
	// Environment, generation and attempt it makes the request hash, so a
	// repeated admission of the same step is a replay.
	DeletionID string
	Attempt    int
	Generation int64
}

// AdmitEnvironmentCleanup admits the whole owned set of one generation of the
// Environment for destruction (UC-18 MS-06). The caller is the Application
// deletion service, which already checked the session, role and typed
// confirmation. The selection and every guard are re-checked inside the claim
// transaction. It returns the cleanup operation ID; Wait or GetCleanup follow it.
func (s *Service) AdmitEnvironmentCleanup(ctx context.Context, sc Scope, actor Actor, in EnvironmentCleanupInput) (AdmissionView, error) {
	if err := s.authorizeScope(ctx, sc); err != nil {
		return AdmissionView{}, err
	}
	if !actor.MayDelete || actor.Username == "" || in.DeletionID == "" {
		return AdmissionView{}, ErrForbidden
	}
	if !s.mutable() {
		return AdmissionView{}, ErrUnavailable
	}
	var (
		pre *snapshot
		sel generationSelection
	)
	if err := s.store.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error {
		snap, err := loadSnapshot(ctx, view, s.reader, sc)
		if err != nil {
			return err
		}
		pre = snap
		sel, err = selectGeneration(snap, in.Generation)
		return err
	}); err != nil {
		return AdmissionView{}, err
	}
	items := make([]tfresource.CleanupItem, 0, len(sel.selected))
	for _, id := range sel.selected {
		rec := pre.records[id]
		inst, err := s.state.GetInstance(ctx, id)
		if err != nil || inst.Identity.EnvironmentID != pre.env.ID || inst.Identity.OrganizationID != rec.Instance.Identity.OrganizationID {
			return AdmissionView{}, ErrUnavailable
		}
		digest, err := tfresource.TargetDigest(inst.TargetRef)
		if err != nil {
			return AdmissionView{}, ErrUnavailable
		}
		items = append(items, tfresource.CleanupItem{InstanceID: id, Version: inst.Version, StateKey: inst.StateKey, ModuleBundleID: inst.ModuleBundleID, TargetDigest: digest})
	}
	requestHash := requestHashOf(sc.OrganizationKey, "app-deletion|"+in.DeletionID+"|"+sc.EnvironmentKey+"|"+strconv.FormatInt(in.Generation, 10)+"|"+strconv.Itoa(in.Attempt))
	rootName := pre.records[sel.root].Instance.Identity.Descriptor
	req := tfresource.CleanupRequest{
		Mode: tfresource.CleanupUserConfirmed, ActorRef: actor.Username, RootInstanceID: sel.root, CandidateIDs: append([]string(nil), sel.selected...),
		Scope:       tfresource.ConfirmedScope{SelectedResourceIDs: append([]string(nil), sel.selected...), Items: items, RetainedReasons: map[string]string{}},
		RequestHash: requestHash, ConfirmationName: rootName,
		ScopeSnapshot: map[string]any{"environmentVersion": pre.env.Version, "draftVersion": pre.env.DraftVersion, "currentSetId": pre.env.CurrentDeploymentSetID,
			"generation": in.Generation, "applicationDeletionId": in.DeletionID},
	}
	if err := req.Validate(); err != nil {
		return AdmissionView{}, invalid("selectedResourceIds")
	}
	expected := append([]string(nil), sel.selected...)
	check := func(ctx context.Context) error {
		snap, err := loadSnapshot(ctx, s.store, s.reader, sc)
		if err != nil {
			return err
		}
		if snap.env.ID != pre.env.ID {
			return ErrStalePreview
		}
		again, err := selectGeneration(snap, in.Generation)
		if err != nil {
			return err
		}
		if again.root != sel.root || !tfresource.SameSelection(again.selected, expected) {
			return ErrStalePreview
		}
		return nil
	}
	claim := persistence.OperationClaim{
		ID: ids.New(), ApplicationKey: sc.ApplicationKey, EnvironmentKey: sc.EnvironmentKey, Owner: s.ops.InstanceID(),
		Kind: environment.OpResourceCleanup, Deadline: s.now().Add(envops.DeadlineFor(environment.OpResourceCleanup)),
		Pins: environment.OperationPins{CheckEnvVersion: true, EnvVersion: pre.env.Version, CheckDraftVersion: true, DraftVersion: pre.env.DraftVersion,
			CheckSet: true, CurrentSetID: pre.env.CurrentDeploymentSetID},
		Detail: map[string]any{"rootId": sel.root, "applicationDeletionId": in.DeletionID},
	}
	op, replayed, err := s.claims.ClaimResourceCleanup(ctx, claim, req, terraformstate.AdmissionCheck(check))
	switch {
	case errors.Is(err, terraformstate.ErrCleanupSelectionChanged), errors.Is(err, persistence.ErrVersionConflict):
		return AdmissionView{}, ErrStalePreview
	case err != nil:
		return AdmissionView{}, err
	}
	view := AdmissionView{OperationID: op.ID, Status: safeOperationStatus(string(op.Status)), Stage: safeStage(op.Stage), Replayed: replayed}
	if replayed {
		return view, nil
	}
	actives := make([]string, 0, len(sel.selected))
	for _, id := range sel.selected {
		if aid := pre.records[id].Instance.ActiveResourceID; aid != "" {
			actives = append(actives, aid)
		}
	}
	s.wg.Add(1)
	go s.run(op, sc, sel.selected, actives)
	return view, nil
}

// WaitCleanup polls the persisted cleanup operation until it is no longer
// held or ctx ends. It reads persistence only.
func (s *Service) WaitCleanup(ctx context.Context, sc Scope, operationID string, every time.Duration) (CleanupView, error) {
	for {
		view, err := s.GetCleanup(ctx, sc, operationID)
		if err != nil {
			return view, err
		}
		if view.Status != string(environment.OpActive) && view.Status != string(environment.OpRecovering) {
			return view, nil
		}
		select {
		case <-ctx.Done():
			return view, ctx.Err()
		case <-time.After(every):
		}
	}
}
