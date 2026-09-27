package pending

import (
	"context"
	"errors"
	"fmt"

	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/workloadconfig"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
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

// Deploy accepts only the current preview token. Successful workloads are
// committed one by one, so a later failure is visible and retryable.
func (s *Service) Deploy(ctx context.Context, appKey, envKey, actor, token string) (DeployReport, error) {
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
	report := DeployReport{Status: "SUCCEEDED", Results: []WorkloadResult{}}
	expectedEnvVersion := preview.BaseVersion
	expectedDraftVersion := preview.DraftVersion
	for index, change := range preview.Changes {
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
			before, err = workloadconfig.ReconstructScore(change.WorkloadID, module, set.Document)
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
			OrganizationKey: app.OrganizationKey, ApplicationKey: appKey, EnvironmentKey: envKey,
			WorkloadID: change.WorkloadID, ScoreBefore: before, ScoreAfter: after, Action: change.Action,
			Actor: actor, RunID: preview.RunID,
			ExpectedPlanHash: change.PlanHash,
			ConfigRevisionID: preview.ConfigRevisionID,
		})
		if deployErr != nil {
			report.Results = append(report.Results, WorkloadResult{WorkloadID: change.WorkloadID, Action: change.Action, Status: "FAILED", Error: deployErr.Error()})
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
			report.Results = append(report.Results, WorkloadResult{WorkloadID: change.WorkloadID, Action: change.Action, Status: "FAILED", DeploymentID: result.DeploymentID, Error: "deployed but failed to record applied revision: " + err.Error()})
			report.Status = statusFor(report.Results)
			appendSkipped(&report, preview.Changes[index+1:])
			return report, nil
		}
		if draftErr == nil {
			expectedDraftVersion++
		}
		report.Results = append(report.Results, WorkloadResult{WorkloadID: change.WorkloadID, Action: change.Action, Status: "SUCCEEDED", DeploymentID: result.DeploymentID})
	}
	return report, nil
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
