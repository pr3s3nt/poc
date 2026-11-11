package pending

import (
	"context"
	"errors"
	"fmt"
	deliveryapp "orchestrator/internal/application/delivery"
	deliverydomain "orchestrator/internal/domain/delivery"
	"sort"
	"strings"

	"orchestrator/internal/application/envops"
	"orchestrator/internal/application/resourcecleanup"
	"orchestrator/internal/application/workloadconfig"
	"orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/canon"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

// RollbackPreview is a safe scope summary. Private release pins never enter it.
type RollbackPreview struct {
	Token             string   `json:"token"`
	TargetOperationID string   `json:"targetOperationId,omitempty"`
	Empty             bool     `json:"empty"`
	Restore           []string `json:"restore"`
	Remove            []string `json:"remove"`
	Resources         []string `json:"resources"`
}

type rollbackPlan struct {
	view                                RollbackPreview
	env                                 environment.Environment
	target                              environment.DeploymentSet
	snapshot                            ReleaseSnapshot
	config                              configuration.Scope
	cleanup                             resourcecleanup.RollbackInventory
	definitionRevision, catalogRevision int64
}

func (s *Service) PreviewRollback(ctx context.Context, appKey, envKey string) (RollbackPreview, error) {
	var plan rollbackPlan
	err := s.store.ReadSnapshot(ctx, func(ctx context.Context, st persistence.Store) error {
		var err error
		bound := *s
		bound.store = st
		plan, err = bound.rollbackPlan(ctx, appKey, envKey)
		return err
	})
	return plan.view, err
}

func (s *Service) rollbackPlan(ctx context.Context, appKey, envKey string) (rollbackPlan, error) {
	var plan rollbackPlan
	env, err := s.store.GetEnvironment(ctx, appKey, envKey)
	if err != nil {
		return plan, err
	}
	if env.Busy() {
		return plan, persistence.Busy(appKey, envKey)
	}
	plan.env = env
	snapshot, err := s.LastSuccessfulRelease(ctx, appKey, envKey)
	if errors.Is(err, persistence.ErrNotFound) {
		snapshot, err = s.initialRollbackBaseline(ctx, env)
	}
	if err != nil {
		return plan, err
	}
	target, err := s.store.GetDeploymentSet(ctx, snapshot.DeploymentSetID)
	if err != nil {
		return plan, err
	}
	app, err := s.store.GetApplication(ctx, appKey)
	if err != nil {
		return plan, err
	}
	currentPin, err := deliveryapp.PinForPreview(ctx, s.store, app.OrganizationKey, env)
	if err != nil {
		return plan, err
	}
	if snapshot.Binding != env.Binding() || snapshot.SecretStoreKey != env.SecretStoreKey || (snapshot.OperationID != "" && currentPin != snapshot.Delivery) {
		return plan, fmt.Errorf("%w: rollback target identity changed", ErrInvalid)
	}
	if snapshot.OperationID != "" && s.deployer != nil {
		op, err := s.store.GetOperation(ctx, snapshot.OperationID)
		if err != nil {
			return plan, err
		}
		if op.FinishedAt == nil {
			return plan, ErrInvalid
		}
		if err := s.deployer.ValidateRollbackRenderers(ctx, appKey, envKey, target, *op.FinishedAt); err != nil {
			return plan, err
		}
	}
	current, err := s.store.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
	if err != nil {
		return plan, err
	}
	drafts, err := s.store.ListWorkloadDrafts(ctx, appKey, envKey)
	if err != nil {
		return plan, err
	}
	scope, err := s.store.GetConfigurationScope(ctx, appKey, envKey)
	if err != nil {
		return plan, err
	}
	defs, err := s.store.GetDefinitionSelection(ctx, appKey, envKey)
	if err != nil {
		return plan, err
	}
	catalog, err := s.store.GetCatalogEnvironment(ctx, appKey, envKey)
	if err != nil {
		return plan, err
	}
	// A checkpoint captures an immutable catalog pin, not an alternate destination.
	if snapshot.OperationID != "" && catalog.Desired != nil && catalog.Desired.Digest() != snapshot.CatalogPin {
		return plan, fmt.Errorf("%w: catalog rollback requires its retained selection", ErrInvalid)
	}
	if snapshot.OperationID == "" && s.rollbackCleanup != nil {
		plan.cleanup, err = s.rollbackCleanup.RollbackInventory(ctx, resourcecleanup.Scope{OrganizationKey: app.OrganizationKey, ApplicationKey: appKey, EnvironmentKey: envKey})
		if err != nil && !s.simulation {
			return plan, err
		}
	}
	if snapshot.OperationID != "" {
		for _, module := range target.Document.Modules {
			if module.Spec.Execution != nil && strings.EqualFold(module.Spec.Execution.Kind, "Job") {
				return plan, fmt.Errorf("%w: rollback cannot replay one-shot Jobs", ErrInvalid)
			}
		}
		if repo, ok := s.store.(persistence.DeliveryExecutionRepository); ok {
			org, err := s.store.GetOrganization(ctx, app.OrganizationKey)
			if err != nil {
				return plan, err
			}
			records, err := repo.ListDeliveryExecutions(ctx, org.ID, env.ID)
			if err != nil {
				return plan, err
			}
			for _, record := range records {
				// ADR-042: rollback keeps the Environment migrate unit as it is.
				if record.Revision.Phase == deliverydomain.Job && record.Revision.Scope.TargetGeneration == env.TargetGeneration && record.Revision.Scope.UnitKey != deliverydomain.EnvironmentMigrateUnit {
					return plan, fmt.Errorf("%w: rollback cannot replay one-shot Jobs", ErrInvalid)
				}
			}
		}
	}
	view := RollbackPreview{TargetOperationID: snapshot.OperationID, Empty: snapshot.OperationID == "", Restore: []string{}, Remove: []string{}}
	for id := range target.Document.Modules {
		view.Restore = append(view.Restore, id)
	}
	for id := range current.Document.Modules {
		if _, ok := target.Document.Modules[id]; !ok {
			view.Remove = append(view.Remove, id)
		}
	}
	sort.Strings(view.Restore)
	sort.Strings(view.Remove)
	token, err := canon.Hash(struct {
		Environment                         environment.Environment
		Snapshot                            string
		Drafts                              []environment.WorkloadDraft
		Config                              configuration.Scope
		DefinitionRevision, CatalogRevision int64
	}{env, snapshot.Digest, drafts, scope, defs.Revision, catalog.Revision})
	if err != nil {
		return plan, err
	}
	view.Resources = append([]string{}, plan.cleanup.Descriptors...)
	view.Token, err = canon.Hash([]string{token, plan.cleanup.Digest})
	if err != nil {
		return plan, err
	}
	plan.view, plan.target, plan.snapshot = view, target, snapshot
	plan.config = scope
	plan.definitionRevision, plan.catalogRevision = defs.Revision, catalog.Revision
	return plan, nil
}

// Rollback explicitly restores the complete successful release under one fenced
// Environment claim. It never changes cluster identity or infers an old release
// from individual workload successes.
func (s *Service) Rollback(ctx context.Context, appKey, envKey, actor, token, confirmation string) (DeployReport, error) {
	if actor == "" || confirmation != appKey {
		return DeployReport{}, ErrInvalid
	}
	plan, err := s.rollbackPlan(ctx, appKey, envKey)
	if err != nil {
		return DeployReport{}, err
	}
	if token == "" || token != plan.view.Token {
		return DeployReport{}, ErrStalePreview
	}
	if s.deployer == nil {
		return DeployReport{}, ErrInvalid
	}
	app, err := s.store.GetApplication(ctx, appKey)
	if err != nil {
		return DeployReport{}, err
	}
	var lease *envops.Lease
	if plan.view.Empty && len(plan.cleanup.IDs) > 0 {
		lease, err = s.rollbackCleanup.ClaimRollback(ctx, resourcecleanup.Scope{OrganizationKey: app.OrganizationKey, ApplicationKey: appKey, EnvironmentKey: envKey}, actor, plan.cleanup, plan.env)
	} else {
		lease, err = s.operations().Begin(ctx, envops.Claim{ApplicationKey: appKey, EnvironmentKey: envKey, Kind: environment.OpDeploy,
			Pins:   environment.OperationPins{CheckEnvVersion: true, EnvVersion: plan.env.Version, CheckDraftVersion: true, DraftVersion: plan.env.DraftVersion, CheckSet: true, CurrentSetID: plan.env.CurrentDeploymentSetID, CheckConfigVersion: true, ConfigVersion: plan.config.Version, CheckRevision: true, DesiredRevisionID: plan.config.DesiredRevisionID},
			Detail: map[string]any{"rollback": true, "targetOperationId": plan.snapshot.OperationID}})
	}

	if err != nil {
		return DeployReport{}, err
	}
	fail := func(cause error) (DeployReport, error) {
		persistence.MarkHold(lease.Ctx)
		_ = lease.End(environment.OpFailed, "rollback did not complete; inspect operation before retry")
		return DeployReport{Status: "FAILED", Results: []WorkloadResult{}}, cause
	}
	// Recheck the token inside the claim before changing desired state. Busy is
	// permitted only for this owner; all other inputs must match the reviewed plan.
	fresh, err := s.store.GetEnvironment(lease.Ctx, appKey, envKey)
	if err != nil {
		return fail(err)
	}
	if fresh.Version != plan.env.Version || fresh.DraftVersion != plan.env.DraftVersion || fresh.CurrentDeploymentSetID != plan.env.CurrentDeploymentSetID {
		return fail(ErrStalePreview)
	}
	var preview Preview
	err = s.store.Transact(lease.Ctx, func(tx context.Context) error {
		scope, err := s.store.GetConfigurationScope(tx, appKey, envKey)
		if err != nil {
			return err
		}
		defs, err := s.store.GetDefinitionSelection(tx, appKey, envKey)
		if err != nil {
			return err
		}
		catalog, err := s.store.GetCatalogEnvironment(tx, appKey, envKey)
		if err != nil {
			return err
		}
		if scope.Version != plan.config.Version || scope.DesiredRevisionID != plan.config.DesiredRevisionID || defs.Revision != plan.definitionRevision || catalog.Revision != plan.catalogRevision {
			return ErrStalePreview
		}
		if err := s.restoreRollbackDesired(tx, plan); err != nil {
			return err
		}
		preview, err = s.Preview(tx, appKey, envKey)
		return err
	})
	if err != nil {
		// No external effect has started. The transaction reverted desired
		// changes, so a confirmed preflight refusal need not hold the claim.
		_ = lease.End(environment.OpFailed, "rollback preflight refused; no changes were applied")
		return DeployReport{Status: "FAILED", Results: []WorkloadResult{}}, err
	}
	report, err := s.deployClaimed(lease, app.OrganizationKey, appKey, envKey, actor, preview)
	if err != nil || report.Status != "SUCCEEDED" {
		if err == nil {
			err = ErrInterrupted
		}
		return fail(err)
	}
	if err = s.store.Transact(lease.Ctx, func(tx context.Context) error {
		if err := s.store.ApplyDefinitionSelection(tx, appKey, envKey, preview.DefinitionRevision, preview.DefinitionSnapshots, preview.CatalogPin); err != nil {
			return err
		}
		if preview.CatalogPending {
			return s.store.ActivateCatalog(tx, appKey, envKey, preview.CatalogRevision, "APPLIED")
		}
		return nil
	}); err != nil {
		return fail(err)
	}
	actualEnv, err := s.store.GetEnvironment(lease.Ctx, appKey, envKey)
	if err != nil {
		return fail(err)
	}
	actualSet, err := s.store.GetDeploymentSet(lease.Ctx, actualEnv.CurrentDeploymentSetID)
	if err != nil {
		return fail(err)
	}
	actualHash, err := canon.Hash(actualSet.Document)
	if err != nil {
		return fail(err)
	}
	if actualHash != plan.snapshot.DocumentHash {
		return fail(fmt.Errorf("%w: rollback target state was not fully restored", ErrInvalid))
	}
	keep := map[string]bool{}
	for id := range plan.target.Document.Modules {
		keep[id] = true
	}
	if _, err = s.deployer.RemoveRollbackRemainders(lease, app.OrganizationKey, appKey, envKey, keep); err != nil {
		return fail(err)
	}
	if plan.view.Empty && len(plan.cleanup.IDs) > 0 {
		if err = s.rollbackCleanup.FinishRollbackCleanup(lease, resourcecleanup.Scope{OrganizationKey: app.OrganizationKey, ApplicationKey: appKey, EnvironmentKey: envKey}, plan.cleanup); err != nil {
			return fail(err)
		}
	}
	snapshot, err := s.captureRelease(lease.Ctx, lease.Operation.ID, preview)
	if err != nil {
		return fail(err)
	}
	if err = lease.Stage("ROLLBACK_READY", map[string]any{"rollback": true, "targetOperationId": plan.snapshot.OperationID, "releaseSnapshot": snapshot}); err != nil {
		return fail(err)
	}
	if err = lease.End(environment.OpSucceeded, ""); err != nil {
		return report, err
	}
	return report, nil
}

func (s *Service) restoreRollbackDesired(ctx context.Context, plan rollbackPlan) error {
	// The caller holds the transaction covering desired-state restoration and Preview.
	tx := ctx
	appKey, envKey := plan.env.ApplicationKey, plan.env.Key
	env, err := s.store.GetEnvironment(tx, appKey, envKey)
	if err != nil {
		return err
	}
	drafts, err := s.store.ListWorkloadDrafts(tx, appKey, envKey)
	if err != nil {
		return err
	}
	version := env.DraftVersion
	for _, draft := range drafts {
		if err := s.store.DeleteWorkloadDraft(tx, appKey, envKey, draft.WorkloadID, version); err != nil {
			return err
		}
		version++
	}
	scope, err := s.store.GetConfigurationScope(tx, appKey, envKey)
	if err != nil {
		return err
	}
	entries := map[string]configuration.Entry{}
	if plan.snapshot.ConfigRevisionID != "" {
		revision, err := s.store.GetConfigurationRevision(tx, plan.snapshot.ConfigRevisionID)
		if err != nil {
			return err
		}
		if revision.ApplicationKey != appKey || revision.EnvironmentKey != envKey {
			return ErrInvalid
		}
		entries = revision.Entries
	}
	if err := s.store.CommitConfigurationRevision(tx, scope.Version, configuration.Revision{ID: ids.New(), ApplicationKey: appKey, EnvironmentKey: envKey, Version: scope.Version + 1, Entries: entries}); err != nil {
		return err
	}
	selection, err := s.store.GetDefinitionSelection(tx, appKey, envKey)
	if err != nil {
		return err
	}
	expected := selection.Revision
	selection.Revision++
	if plan.snapshot.OperationID != "" {
		selection.Desired = plan.snapshot.Definitions
	}
	selection.CatalogPin = plan.snapshot.CatalogPin
	app, err := s.store.GetApplication(tx, appKey)
	if err != nil {
		return err
	}
	if err := s.store.SelectDefinitionSelection(tx, app.OrganizationKey, env.Version, expected, selection); err != nil {
		return err
	}
	for _, id := range plan.view.Remove {
		if err := s.store.SaveWorkloadDraft(tx, version, environment.WorkloadDraft{ApplicationKey: appKey, EnvironmentKey: envKey, WorkloadID: id, State: environment.DraftDelete}); err != nil {
			return err
		}
		version++
	}
	for _, id := range plan.view.Restore {
		module := plan.target.Document.Modules[id]
		raw, err := workloadconfig.ReconstructWorkloadProven(id, module, plan.target.Document, plan.target.LiteralProofs.ProverFor(id), plan.target.ResourceAliases[id])
		if err != nil {
			return err
		}
		if err := s.store.SaveWorkloadDraft(tx, version, environment.WorkloadDraft{ApplicationKey: appKey, EnvironmentKey: envKey, WorkloadID: id, State: environment.DraftUpsert, Workload: raw}); err != nil {
			return err
		}
		version++
	}
	return nil
}

func (s *Service) SetRollbackCleanup(cleanup *resourcecleanup.Service) { s.rollbackCleanup = cleanup }

func (s *Service) initialRollbackBaseline(ctx context.Context, env environment.Environment) (ReleaseSnapshot, error) {
	records, err := s.store.ListDeployments(ctx, env.ApplicationKey, env.Key)
	if err != nil {
		return ReleaseSnapshot{}, err
	}
	id := env.CurrentDeploymentSetID
	for _, record := range records {
		if record.EnvironmentID != env.ID || record.TargetGeneration != env.TargetGeneration {
			return ReleaseSnapshot{}, ErrInvalid
		}
		if record.BaseDeploymentSetID != "" {
			id = record.BaseDeploymentSetID
		}
	}
	set, err := s.store.GetDeploymentSet(ctx, id)
	if err != nil {
		return ReleaseSnapshot{}, err
	}
	if set.EnvironmentID != env.ID || len(set.Document.Modules) != 0 {
		return ReleaseSnapshot{}, fmt.Errorf("%w: initial empty baseline cannot be proven", ErrInvalid)
	}
	hash, err := canon.Hash(set.Document)
	if err != nil {
		return ReleaseSnapshot{}, err
	}
	snapshot := ReleaseSnapshot{Version: 1, DeploymentSetID: set.ID, DocumentHash: hash, Binding: env.Binding(), SecretStoreKey: env.SecretStoreKey}
	snapshot.Digest, err = canon.Hash(snapshot)
	return snapshot, err
}
