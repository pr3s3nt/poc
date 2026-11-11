package pending

import (
	"context"
	"errors"
	"fmt"

	deliveryapp "orchestrator/internal/application/delivery"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/envops"
	"orchestrator/internal/application/workloadconfig"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/progress"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

type WorkloadResult struct {
	WorkloadID   string        `json:"workloadId"`
	Action       domain.Action `json:"action"`
	Status       string        `json:"status"`
	DeploymentID string        `json:"deploymentId,omitempty"`
	Error        string        `json:"error,omitempty"`
	// ErrorCode is the code of a known safe summary (UC-06 CD-10), or empty.
	ErrorCode string `json:"errorCode,omitempty"`
}

type DeployReport struct {
	Status  string           `json:"status"`
	Results []WorkloadResult `json:"results"`
}

func (s *Service) SetDeployer(deployer *appsvc.Service) { s.deployer = deployer }

// Deploy accepts only the current preview token. It claims the Environment in
// one local transaction that re-verifies every pinned version and identity,
// then executes under that claim; no executor call precedes a successful claim.
// Successful workloads are committed one by one, so a later failure is visible
// and retryable.
func (s *Service) Deploy(ctx context.Context, appKey, envKey, actor, token string) (DeployReport, error) {
	return s.DeployWithReview(ctx, appKey, envKey, actor, token, "")
}

func (s *Service) DeployWithReview(ctx context.Context, appKey, envKey, actor, token, reviewRequestID string) (DeployReport, error) {
	if reviewRequestID != "" && !ids.ValidUUID(reviewRequestID) {
		return DeployReport{}, ErrInvalid
	}
	preview, err := s.Preview(ctx, appKey, envKey)
	if err != nil {
		return DeployReport{}, err
	}
	if token == "" || token != preview.Token {
		return DeployReport{}, ErrStalePreview
	}
	if s.deployer == nil {
		return DeployReport{}, fmt.Errorf("pending: deployer is unavailable")
	}
	app, err := s.store.GetApplication(ctx, appKey)
	if err != nil {
		return DeployReport{}, err
	}
	lease, err := s.operations().Begin(ctx, envops.Claim{
		ApplicationKey: appKey, EnvironmentKey: envKey, Kind: environment.OpDeploy,
		ReviewRequestID: reviewRequestID,
		Pins: environment.OperationPins{
			CheckEnvVersion: true, EnvVersion: preview.BaseVersion,
			CheckDraftVersion: true, DraftVersion: preview.DraftVersion,
			CheckConfigVersion: true, ConfigVersion: preview.ConfigVersion,
			CheckSet: true, CurrentSetID: preview.BaseSetID,
			CheckRevision: true, DesiredRevisionID: preview.ConfigRevisionID,
			CheckBinding: true, Binding: preview.Binding,
			CheckStore: true, SecretStoreKey: preview.SecretStoreKey,
		},
		Detail: map[string]any{"changes": len(preview.Changes)},
	})
	if errors.Is(err, persistence.ErrVersionConflict) {
		return DeployReport{}, ErrStalePreview
	}
	if err != nil {
		return DeployReport{}, err
	}
	for _, change := range preview.Changes {
		progress.Report(lease.Ctx, progress.Step{ID: "workload:" + change.WorkloadID, Workload: change.WorkloadID, Kind: "WORKLOAD", Status: "PENDING", Reason: "WAITING_DEPENDENCY"})
	}
	report, err := s.deployClaimed(lease, app.OrganizationKey, appKey, envKey, actor, preview)
	if err == nil && report.Status == "SUCCEEDED" {
		err = s.store.Transact(lease.Ctx, func(ctx context.Context) error {
			if len(preview.Changes) > 0 {
				if applyErr := s.store.ApplyDefinitionSelection(ctx, appKey, envKey, preview.DefinitionRevision, preview.DefinitionSnapshots, preview.CatalogPin); applyErr != nil {
					return applyErr
				}
			}
			if preview.CatalogPending {
				return s.store.ActivateCatalog(ctx, appKey, envKey, preview.CatalogRevision, "APPLIED")
			}
			return nil
		})
	} else if preview.CatalogPending {
		if catalogErr := s.store.ActivateCatalog(lease.Ctx, appKey, envKey, preview.CatalogRevision, "PARTIAL"); catalogErr != nil && err == nil {
			err = catalogErr
		}
	}

	// A failed checkpoint never becomes a selectable successful release.
	if err == nil && report.Status == "SUCCEEDED" {
		var snapshot ReleaseSnapshot
		snapshot, err = s.captureRelease(lease.Ctx, lease.Operation.ID, preview)
		if err == nil {
			err = lease.Stage("RELEASE_READY", map[string]any{"releaseSnapshot": snapshot})
		}
	}

	status, failure := environment.OpSucceeded, ""
	if err != nil || report.Status != "SUCCEEDED" {
		status, failure = environment.OpFailed, "the deploy did not complete: "+report.Status
	}
	for _, item := range report.Results {
		state := item.Status
		if state == "SKIPPED" {
			state = "PENDING"
		}
		progress.Report(lease.Ctx, progress.Step{ID: "workload:" + item.WorkloadID, Workload: item.WorkloadID, DeploymentID: item.DeploymentID, Kind: "WORKLOAD", Status: state})
	}
	if endErr := lease.End(status, failure); endErr != nil && err == nil {
		err = endErr
	}
	return report, err
}

func (s *Service) deployClaimed(lease *envops.Lease, orgKey, appKey, envKey, actor string, preview Preview) (DeployReport, error) {
	ctx := lease.Ctx
	report := DeployReport{Status: "SUCCEEDED", Results: []WorkloadResult{}}
	env, err := s.store.GetEnvironment(ctx, appKey, envKey)
	if err != nil {
		return report, err
	}
	if !preview.DeliveryTransfer && !preview.DeliveryPolicyOnly {
		staged, err := s.deployer.UsesEnvironmentBundle(ctx, orgKey, env)
		if err != nil {
			return report, err
		}
		if staged {
			// ADR-042: every change of this batch is one Environment release.
			release := appsvc.NewRelease(orgKey, appKey, env)
			if rollback, _ := lease.Operation.Detail["rollback"].(bool); rollback {
				// ADR-042: a rollback restores the app unit and never re-runs
				// migration Jobs of an older release.
				release.KeepJobs()
			}
			report, err := s.deployChanges(lease, appsvc.WithRelease(ctx, release), orgKey, appKey, envKey, actor, preview, env)
			s.finishRelease(ctx, release, &report)
			return report, err
		}
	}
	return s.deployChanges(lease, ctx, orgKey, appKey, envKey, actor, preview, env)
}

// finishRelease publishes the staged workloads once and replaces each staged
// workload's result with its delivery outcome.
func (s *Service) finishRelease(ctx context.Context, release *appsvc.Release, report *DeployReport) {
	if release.Empty() {
		return
	}
	outcome, releaseErr := s.deployer.FlushRelease(ctx, release)
	failed := false
	for index, result := range report.Results {
		cause, staged := outcome[result.WorkloadID]
		if !staged || result.Status != "SUCCEEDED" {
			continue
		}
		if cause != nil {
			report.Results[index].Status = "FAILED"
			report.Results[index].Error = publicDeployError(cause)
			report.Results[index].ErrorCode = appsvc.FailureCode(report.Results[index].Error)
			failed = true
		}
	}
	if releaseErr != nil && !failed {
		// The failing workload was not changed in this batch: the release
		// still failed, so report that workload and not a success.
		var rollout *deliveryapp.RolloutFailure
		names := []string{}
		if errors.As(releaseErr, &rollout) {
			names = rollout.Workloads
		}
		for _, name := range names {
			report.Results = append(report.Results, WorkloadResult{WorkloadID: name, Action: domain.ActionUpdate, Status: "FAILED", Error: publicDeployError(releaseErr), ErrorCode: appsvc.FailureCode(publicDeployError(releaseErr))})
		}
		failed = true
	}
	if failed {
		report.Status = statusFor(report.Results)
	}
}

func (s *Service) deployChanges(lease *envops.Lease, ctx context.Context, orgKey, appKey, envKey, actor string, preview Preview, env environment.Environment) (DeployReport, error) {
	report := DeployReport{Status: "SUCCEEDED", Results: []WorkloadResult{}}
	pin, err := deliveryapp.PinForPreview(ctx, s.store, orgKey, env)
	if err != nil {
		return report, err
	}
	if pin != preview.DeliveryBindingPin || env.DeliveryConfigRevision != preview.DeliveryConfigurationRevision || env.DesiredDelivery.Effective() != preview.DesiredDelivery || env.AppliedDelivery.Effective() != preview.AppliedDelivery {
		return report, ErrStalePreview
	}
	defSelection, err := s.store.GetDefinitionSelection(ctx, appKey, envKey)
	if err != nil {
		return report, err
	}
	if defSelection.Revision != preview.DefinitionRevision {
		return report, ErrStalePreview
	}
	catalogBinding, err := s.store.GetCatalogEnvironment(ctx, appKey, envKey)
	if err != nil {
		return report, err
	}
	if catalogBinding.Revision != preview.CatalogRevision {
		return report, ErrStalePreview
	}
	if preview.CatalogPin != "" && (catalogBinding.Desired == nil || catalogBinding.Desired.Digest() != preview.CatalogPin) {
		return report, ErrStalePreview
	}
	if preview.DeliveryTransfer {
		if err := s.deployer.TransferDelivery(ctx, orgKey, env, lease.Stage); err != nil {
			return report, err
		}
		return report, nil
	}
	if preview.DeliveryPolicyOnly {
		if err := s.deployer.UpdateDeliveryPolicy(ctx, orgKey, env, lease.Stage); err != nil {
			return report, err
		}
		return report, nil
	}
	expectedEnvVersion := preview.BaseVersion
	expectedDraftVersion := preview.DraftVersion
	for index, change := range preview.Changes {
		if lease.Lost() || ctx.Err() != nil {
			report.Results = append(report.Results, WorkloadResult{WorkloadID: change.WorkloadID, Action: change.Action, Status: "FAILED", Error: ErrInterrupted.Error()})
			report.Status = statusFor(report.Results)
			appendSkipped(&report, preview.Changes[index+1:])
			return report, nil
		}
		progress.Report(ctx, progress.Step{ID: "workload:" + change.WorkloadID, Workload: change.WorkloadID, Kind: "WORKLOAD", Status: "RUNNING"})
		if err := lease.Stage("DEPLOY "+change.WorkloadID, nil); err != nil {
			report.Results = append(report.Results, WorkloadResult{WorkloadID: change.WorkloadID, Action: change.Action, Status: "FAILED", Error: ErrInterrupted.Error()})
			report.Status = statusFor(report.Results)
			appendSkipped(&report, preview.Changes[index+1:])
			return report, nil
		}
		env, err := s.store.GetEnvironment(ctx, appKey, envKey)
		if err != nil {
			return report, err
		}
		scope, err := s.store.GetConfigurationScope(ctx, appKey, envKey)
		if err != nil {
			return report, err
		}
		if env.Version != expectedEnvVersion || env.DraftVersion != expectedDraftVersion || scope.DesiredRevisionID != preview.ConfigRevisionID {
			report.Results = append(report.Results, WorkloadResult{WorkloadID: change.WorkloadID, Action: change.Action, Status: "FAILED", Error: ErrStalePreview.Error()})
			report.Status = statusFor(report.Results)
			appendSkipped(&report, preview.Changes[index+1:])
			return report, nil
		}
		set, err := s.store.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
		if err != nil {
			return report, err
		}
		var before, after map[string]any
		if module, exists := set.Document.Modules[change.WorkloadID]; exists {
			before, err = workloadconfig.ReconstructWorkloadProven(change.WorkloadID, module, set.Document, set.LiteralProofs.ProverFor(change.WorkloadID), set.ResourceAliases[change.WorkloadID])
			if err != nil {
				return report, err
			}
		}
		draft, draftErr := s.store.GetWorkloadDraft(ctx, appKey, envKey, change.WorkloadID)
		if draftErr != nil && !isNotFound(draftErr) {
			return report, draftErr
		}
		if draftErr == nil && draft.State == environment.DraftUpsert {
			after = draft.Workload
		}
		if draftErr != nil && change.Action != domain.ActionRemove {
			after = before
		}
		result, deployErr := s.deployer.DeployWorkload(ctx, appsvc.DeployCommand{
			OrganizationKey: orgKey, ApplicationKey: appKey, EnvironmentKey: envKey,
			WorkloadID: change.WorkloadID, WorkloadBefore: before, WorkloadAfter: after, Action: change.Action,
			Actor: actor, RunID: preview.RunID,
			ReviewRequestID:  lease.Operation.ReviewRequestID,
			ExpectedPlanHash: change.PlanHash,
			ConfigRevisionID: preview.ConfigRevisionID,
			InBatch:          true,
			AcceptLiterals:   draftErr == nil && draft.State == environment.DraftUpsert,
		})
		if deployErr != nil {
			report.Results = append(report.Results, WorkloadResult{WorkloadID: change.WorkloadID, Action: change.Action, Status: "FAILED", Error: publicDeployError(deployErr), ErrorCode: appsvc.FailureCode(publicDeployError(deployErr))})
			report.Status = statusFor(report.Results)
			appendSkipped(&report, preview.Changes[index+1:])
			return report, nil
		}
		expectedEnvVersion++
		if err := s.store.Transact(ctx, func(ctx context.Context) error {
			if change.Action != domain.ActionRemove {
				instances, err := s.store.ListWorkloadInstances(ctx, appKey+"/"+envKey)
				if err != nil {
					return err
				}
				for _, instance := range instances {
					if instance.WorkloadID == change.WorkloadID {
						instance.AppliedConfigRevisionID = preview.ConfigRevisionID
						if err := s.store.UpsertWorkloadInstance(ctx, instance); err != nil {
							return err
						}
						break
					}
				}
			}
			if draftErr == nil {
				return s.store.DeleteWorkloadDraft(ctx, appKey, envKey, change.WorkloadID, expectedDraftVersion)
			}
			return nil
		}); err != nil {
			report.Results = append(report.Results, WorkloadResult{WorkloadID: change.WorkloadID, Action: change.Action, Status: "FAILED", DeploymentID: result.DeploymentID, Error: "deployed, but the applied configuration revision could not be recorded; preview again to retry"})
			report.Status = statusFor(report.Results)
			appendSkipped(&report, preview.Changes[index+1:])
			return report, nil
		}
		if draftErr == nil {
			expectedDraftVersion++
		}
		report.Results = append(report.Results, WorkloadResult{WorkloadID: change.WorkloadID, Action: change.Action, Status: "SUCCEEDED", DeploymentID: result.DeploymentID})
	}
	if preview.DeliveryPending {
		if err := s.deployer.ActivateDelivery(ctx, appKey, envKey); err != nil {
			return report, err
		}
	}
	return report, nil
}

// ErrInterrupted reports that the claim was lost; the owner stopped executing.
var ErrInterrupted = errors.New("the operation was interrupted; preview changes again")

// publicDeployError is the per-workload text of the Deploy report: the same
// safe summary the failed Deployment persists, never executor, driver,
// Kubernetes or catalog text.
func publicDeployError(err error) string {
	return appsvc.PublicFailure(err)
}

func isNotFound(err error) bool { return errors.Is(err, persistence.ErrNotFound) }

func statusFor(results []WorkloadResult) string {
	for _, result := range results {
		if result.Status == "SUCCEEDED" {
			return "PARTIAL"
		}
	}
	return "FAILED"
}

func appendSkipped(report *DeployReport, changes []Change) {
	for _, change := range changes {
		report.Results = append(report.Results, WorkloadResult{WorkloadID: change.WorkloadID, Action: change.Action, Status: "SKIPPED"})
	}
}
