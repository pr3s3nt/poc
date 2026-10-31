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
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

type WorkloadResult struct {
	WorkloadID   string        `json:"workloadId"`
	Action       domain.Action `json:"action"`
	Status       string        `json:"status"`
	DeploymentID string        `json:"deploymentId,omitempty"`
	Error        string        `json:"error,omitempty"`
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
	report, err := s.deployClaimed(lease, app.OrganizationKey, appKey, envKey, actor, preview)
	if preview.CatalogPending {
		catalogStatus := "PARTIAL"
		if err == nil && report.Status == "SUCCEEDED" {
			catalogStatus = "APPLIED"
		}
		if catalogErr := s.store.ActivateCatalog(lease.Ctx, appKey, envKey, preview.CatalogRevision, catalogStatus); catalogErr != nil && err == nil {
			err = catalogErr
		}
	}

	status, failure := environment.OpSucceeded, ""
	if err != nil || report.Status != "SUCCEEDED" {
		status, failure = environment.OpFailed, "the deploy did not complete: "+report.Status
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
	pin, err := deliveryapp.PinForPreview(ctx, s.store, orgKey, env)
	if err != nil {
		return report, err
	}
	if pin != preview.DeliveryBindingPin || env.DeliveryConfigRevision != preview.DeliveryConfigurationRevision || env.DesiredDelivery.Effective() != preview.DesiredDelivery || env.AppliedDelivery.Effective() != preview.AppliedDelivery {
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
	if len(preview.Changes) > 0 {
		if err := s.store.SetPublicRoutesPending(ctx, appKey, envKey, true); err != nil {
			return report, err
		}
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
			before, err = workloadconfig.ReconstructScoreProven(change.WorkloadID, module, set.Document, set.LiteralProofs.ProverFor(change.WorkloadID), set.ResourceAliases[change.WorkloadID])
			if err != nil {
				return report, err
			}
		}
		draft, draftErr := s.store.GetWorkloadDraft(ctx, appKey, envKey, change.WorkloadID)
		if draftErr != nil && !isNotFound(draftErr) {
			return report, draftErr
		}
		if draftErr == nil && draft.State == environment.DraftUpsert {
			after = draft.Score
		}
		if draftErr != nil && change.Action != domain.ActionRemove {
			after = before
		}
		result, deployErr := s.deployer.DeployWorkload(ctx, appsvc.DeployCommand{
			OrganizationKey: orgKey, ApplicationKey: appKey, EnvironmentKey: envKey,
			WorkloadID: change.WorkloadID, ScoreBefore: before, ScoreAfter: after, Action: change.Action,
			Actor: actor, RunID: preview.RunID,
			ReviewRequestID:   lease.Operation.ReviewRequestID,
			ExpectedPlanHash:  change.PlanHash,
			ConfigRevisionID:  preview.ConfigRevisionID,
			DeferPublicRoutes: true,
			AcceptLiterals:    draftErr == nil && draft.State == environment.DraftUpsert,
		})
		if deployErr != nil {
			report.Results = append(report.Results, WorkloadResult{WorkloadID: change.WorkloadID, Action: change.Action, Status: "FAILED", Error: publicDeployError(deployErr)})
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
	if len(preview.Changes) > 0 || preview.RoutePending || preview.DeliveryPending {
		if err := s.deployer.ReconcilePublicRoutes(ctx, appKey, envKey); err != nil {
			report.Status = "PARTIAL"
			return report, ErrRouteReconcile
		}
		if err := s.store.SetPublicRoutesPending(ctx, appKey, envKey, false); err != nil {
			return report, err
		}
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

// ErrRouteReconcile reports that workloads committed but public routes did not;
// the route cause stays out of the public message.
var ErrRouteReconcile = errors.New("pending: workloads were applied but public routes could not be reconciled; preview again to retry public routes")

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
