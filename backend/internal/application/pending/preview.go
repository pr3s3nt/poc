// Package pending coordinates UC-05 preview and UC-06/07 deployment of UC-12/16 desired changes.
package pending

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/workloadconfig"
	"orchestrator/internal/domain/configuration"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/planning/score"
	"orchestrator/internal/platform/canon"
	"orchestrator/internal/ports/persistence"
)

var ErrStalePreview = errors.New("pending: preview is stale; preview changes again")
var ErrInvalid = errors.New("pending: invalid desired configuration")

type Change struct {
	WorkloadID string                  `json:"workloadId"`
	Action     domain.Action           `json:"action"`
	Delta      domain.DeltaDocument    `json:"delta"`
	Resources  planning.Classification `json:"resources"`
	PlanHash   string                  `json:"planHash"`
}

type Preview struct {
	Token            string   `json:"token"`
	ApplicationKey   string   `json:"applicationKey"`
	EnvironmentKey   string   `json:"environmentKey"`
	BaseSetID        string   `json:"baseDeploymentSetId"`
	BaseVersion      int64    `json:"baseVersion"`
	DraftVersion     int64    `json:"draftVersion"`
	ConfigRevisionID string   `json:"configRevisionId,omitempty"`
	RunID            string   `json:"runId"`
	Changes          []Change `json:"changes"`
}

type Service struct {
	store     persistence.Store
	planner   *planning.Service
	workloads *workloadconfig.Service
	terraform planning.ModuleInspector
	deployer  *appsvc.Service
}

func NewService(store persistence.Store, planner *planning.Service, workloads *workloadconfig.Service, inspector planning.ModuleInspector) *Service {
	return &Service{store: store, planner: planner, workloads: workloads, terraform: inspector}
}

func (s *Service) Preview(ctx context.Context, appKey, envKey string) (Preview, error) {
	app, err := s.store.GetApplication(ctx, appKey)
	if err != nil {
		return Preview{}, err
	}
	env, err := s.store.GetEnvironment(ctx, appKey, envKey)
	if err != nil {
		return Preview{}, err
	}
	connection, err := s.store.GetConnection(ctx, app.ConnectionKey)
	if err != nil {
		return Preview{}, err
	}
	if connection.Status != "READY" {
		return Preview{}, fmt.Errorf("%w: connection is not ready", ErrInvalid)
	}
	set, err := s.store.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
	if err != nil {
		return Preview{}, err
	}
	scope, err := s.store.GetConfigurationScope(ctx, appKey, envKey)
	if err != nil {
		return Preview{}, err
	}
	desired := configuration.Revision{Entries: map[string]configuration.Entry{}}
	if scope.DesiredRevisionID != "" {
		desired, err = s.store.GetConfigurationRevision(ctx, scope.DesiredRevisionID)
		if err != nil {
			return Preview{}, err
		}
	}
	drafts, err := s.store.ListWorkloadDrafts(ctx, appKey, envKey)
	if err != nil {
		return Preview{}, err
	}
	instances, err := s.store.ListWorkloadInstances(ctx, appKey+"/"+envKey)
	if err != nil {
		return Preview{}, err
	}
	instanceByID := map[string]domain.WorkloadInstance{}
	for _, instance := range instances {
		instanceByID[instance.WorkloadID] = instance
	}

	pending := map[string]environment.WorkloadDraft{}
	for _, draft := range drafts {
		pending[draft.WorkloadID] = draft
	}
	for id, module := range set.Document.Modules {
		if _, exists := pending[id]; exists {
			continue
		}
		changed, err := usesChangedConfiguration(ctx, s.store, module, desired, instanceByID[id].AppliedConfigRevisionID)
		if err != nil {
			return Preview{}, err
		}
		if !changed {
			continue
		}
		raw, err := workloadconfig.ReconstructScore(id, module, set.Document)
		if err != nil {
			return Preview{}, fmt.Errorf("%w: workload %s cannot be rebuilt: %v", ErrInvalid, id, err)
		}
		pending[id] = environment.WorkloadDraft{ApplicationKey: appKey, EnvironmentKey: envKey, WorkloadID: id, State: environment.DraftUpsert, Score: raw}
	}
	ordered, err := orderDrafts(pending)
	if err != nil {
		return Preview{}, err
	}
	types, err := s.store.ListResourceTypes(ctx)
	if err != nil {
		return Preview{}, err
	}
	definitions, err := s.store.ListResourceDefinitions(ctx)
	if err != nil {
		return Preview{}, err
	}
	active, err := s.store.ListActiveResources(ctx, app.OrganizationKey)
	if err != nil {
		return Preview{}, err
	}
	typeMap := map[string]resource.Type{}
	for _, typ := range types {
		typeMap[typ.Key] = typ
	}
	catalog := planning.Catalog{Types: typeMap, Definitions: definitions}
	preview := Preview{ApplicationKey: appKey, EnvironmentKey: envKey, BaseSetID: set.ID, BaseVersion: env.Version, DraftVersion: env.DraftVersion, ConfigRevisionID: scope.DesiredRevisionID, Changes: []Change{}}
	// Keep the execution identity stable across revisions of one environment.
	// The preview token separately pins the mutable base/draft/configuration state.
	pinHash, err := canon.Hash([]string{appKey, envKey})
	if err != nil {
		return Preview{}, err
	}
	preview.RunID = "pending-" + pinHash[:16]
	base := set.Document
	for _, id := range ordered {
		draft := pending[id]
		var before, after *score.Document
		if module, exists := base.Modules[id]; exists {
			raw, err := workloadconfig.ReconstructScore(id, module, base)
			if err != nil {
				return Preview{}, fmt.Errorf("%w: workload %s cannot be updated: %v", ErrInvalid, id, err)
			}
			before, err = score.FromMap(raw)
			if err != nil {
				return Preview{}, err
			}
		}
		if draft.State == environment.DraftUpsert {
			if err := s.workloads.ValidateImport(ctx, appKey, envKey, draft.Score); err != nil {
				return Preview{}, err
			}
			after, err = score.FromMap(draft.Score)
			if err != nil {
				return Preview{}, err
			}
		}
		action := domain.ActionDeploy
		if after == nil {
			action = domain.ActionRemove
		} else if before != nil {
			action = domain.ActionUpdate
		}
		plan, err := s.planner.Plan(planning.Request{OrganizationKey: app.OrganizationKey, App: app, Env: env, Connection: connection, BaseSet: base, Before: before, After: after, WorkloadID: id, Action: action, Catalog: catalog, Active: active, Terraform: s.terraform, RunID: preview.RunID})
		if err != nil {
			return Preview{}, err
		}
		preview.Changes = append(preview.Changes, Change{WorkloadID: id, Action: action, Delta: plan.Delta, Resources: plan.Classification, PlanHash: plan.PlanHash})
		base = plan.CandidateSet
	}
	preview.Token, err = canon.Hash(preview)
	return preview, err
}

func usesChangedConfiguration(ctx context.Context, store persistence.Store, module environment.Module, desired configuration.Revision, appliedID string) (bool, error) {
	if desired.ID == appliedID {
		return false, nil
	}
	applied := configuration.Revision{Entries: map[string]configuration.Entry{}}
	if appliedID != "" {
		loaded, err := store.GetConfigurationRevision(ctx, appliedID)
		if err != nil {
			return false, err
		}
		applied = loaded
	}
	for _, container := range module.Spec.Containers {
		for _, value := range container.Variables {
			if !strings.HasPrefix(value, "${context.uc12.") || !strings.HasSuffix(value, "}") {
				continue
			}
			key := strings.TrimSuffix(strings.TrimPrefix(value, "${context.uc12."), "}")
			if desired.Entries[key] != applied.Entries[key] {
				return true, nil
			}
		}
	}
	return false, nil
}

func orderDrafts(drafts map[string]environment.WorkloadDraft) ([]string, error) {
	ids := make([]string, 0, len(drafts))
	for id := range drafts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	state := map[string]int{}
	ordered := []string{}
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == 1 {
			return fmt.Errorf("%w: Service reference cycle", ErrInvalid)
		}
		if state[id] == 2 {
			return nil
		}
		state[id] = 1
		draft := drafts[id]
		if draft.State == environment.DraftUpsert {
			parsed, err := score.FromMap(draft.Score)
			if err != nil {
				return err
			}
			for _, alias := range parsed.ResourceAliases() {
				resource := parsed.Resources[alias]
				if resource.Type != "service" {
					continue
				}
				target, _ := resource.Params["workload"].(string)
				if dep, exists := drafts[target]; exists && dep.State == environment.DraftUpsert {
					if err := visit(target); err != nil {
						return err
					}
				}
			}
		}
		state[id] = 2
		ordered = append(ordered, id)
		return nil
	}
	for _, id := range ids {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}
