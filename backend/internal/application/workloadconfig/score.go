package workloadconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/mod/semver"
	catalogapp "orchestrator/internal/application/catalogrelease"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/templatesource"
	"orchestrator/internal/planning/workloadspec"
	"orchestrator/internal/ports/persistence"
)

var ErrScoreProfile = fmt.Errorf("%w: invalid Score profile", ErrInvalidRequest)

// ImportDocument is read-only admission. Profile resolution reads the same
// matching rules and selected Definition revisions as Preview; it never changes
// Definitions, resources, placement or the dependency graph.
func (s *Service) ImportDocument(ctx context.Context, app, env string, raw, extension map[string]any) (map[string]any, error) {
	if raw["apiVersion"] != workloadspec.ScoreAPIVersion {
		if extension != nil {
			return nil, fmt.Errorf("%w: extensions require Score", ErrScoreProfile)
		}
		if err := s.ValidateImport(ctx, app, env, raw); err != nil {
			return nil, err
		}
		d, err := workloadspec.FromMap(raw)
		if err != nil {
			return nil, err
		}
		return d.Map(), nil
	}
	converted, err := workloadspec.DecodeScore(raw, extension)
	if err != nil {
		return nil, err
	}
	// Resolve a profile before validating runtime intent: it supplies the kind,
	// but never workload identity or dependency declarations.
	meta := converted.Workload["metadata"].(map[string]any)
	application, err := s.store.GetApplication(ctx, app)
	if err != nil {
		return nil, err
	}
	target, err := s.store.GetEnvironment(ctx, app, env)
	if err != nil {
		return nil, err
	}
	defs, _, err := catalogapp.ResolveDefinitions(ctx, s.store, application.OrganizationKey, app, env)
	if err != nil {
		return nil, err
	}
	class, _ := meta["class"].(string)
	if class == "" {
		class = "default"
	}
	name, _ := meta["name"].(string)
	ranking := resource.Rank(defs, "workload", resource.MatchContext{EnvironmentType: target.Type, ApplicationID: app, EnvironmentID: env, ResourceID: "modules." + name, Class: class}, nil)
	if len(ranking.Top) != 1 {
		return nil, fmt.Errorf("%w: profile needs one matching workload Definition", ErrScoreProfile)
	}
	def := ranking.Top[0].Definition
	values, _ := def.DriverInputs["values"].(map[string]any)
	variables, _ := values["variables"].(map[string]any)
	sourceID, _ := variables[resource.VariableTemplateSource].(string)
	records, err := persistence.TemplateSourcesOf(ctx, s.store, application.OrganizationKey)
	if err != nil {
		return nil, err
	}
	bundle, ok := records[sourceID]
	if !ok || def.DriverType != resource.DriverTemplate {
		return nil, fmt.Errorf("%w: profile source unavailable", ErrScoreProfile)
	}
	templates, err := templatesource.TemplateCatalog(map[string]templatesource.Bundle{sourceID: bundle})
	if err != nil {
		return nil, err
	}
	if converted.Profile == "orchestrator/custom" {
		doc, err := workloadspec.FromMap(converted.Workload)
		if err != nil {
			return nil, err
		}
		if doc.Execution.HasTemplate() {
			return nil, fmt.Errorf("%w: custom profile cannot contain a template pin", ErrScoreProfile)
		}
		if err := templatesource.ValidateWorkloadCapabilities(bundle.Contract.Manifest, doc.Containers, doc.Execution); err != nil {
			return nil, err
		}
		if err := s.ValidateImport(ctx, app, env, doc.Map()); err != nil {
			return nil, err
		}
		return doc.Map(), nil
	}
	selected, err := resolveScoreProfile(converted.Profile, templates)
	if err != nil {
		return nil, err
	}
	execution, ok := converted.Workload["workloadExecution"].(map[string]any)
	if !ok {
		execution = map[string]any{"version": 1}
		converted.Workload["workloadExecution"] = execution
	}
	if kind, ok := execution["kind"]; ok && kind != selected.Kind {
		return nil, fmt.Errorf("%w: profile runtime mismatch", ErrScoreProfile)
	}
	if supplied, ok := execution["template"]; ok {
		var pin environment.TemplatePin
		b, err := json.Marshal(supplied)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &pin); err != nil || pin.ID != selected.ID || pin.Version != selected.Version || pin.SHA256 != selected.SHA256() {
			return nil, fmt.Errorf("%w: profile pin mismatch", ErrScoreProfile)
		}
	}
	execution["kind"] = selected.Kind
	execution["template"] = environment.TemplatePin{ID: selected.ID, Version: selected.Version, SHA256: selected.SHA256()}
	doc, err := workloadspec.FromMap(converted.Workload)
	if err != nil {
		return nil, err
	}
	if err := templatesource.ValidateWorkloadCapabilities(bundle.Contract.Manifest, doc.Containers, doc.Execution); err != nil {
		return nil, err
	}
	if _, err := templatesource.ExecutionInputs(bundle.Contract.Manifest, doc.Execution); err != nil {
		return nil, err
	}
	resolved, _, err := s.ResolveDefaults(ctx, app, env, doc.Map())
	if err != nil {
		return nil, err
	}
	if e, ok := resolved["workloadExecution"].(*environment.WorkloadExecution); ok {
		doc.Execution.TemplateDefaults = e.TemplateDefaults
	}
	if err := s.ValidateImport(ctx, app, env, doc.Map()); err != nil {
		return nil, err
	}
	return doc.Map(), nil
}
func resolveScoreProfile(profile string, templates []templatesource.WorkloadTemplate) (templatesource.WorkloadTemplate, error) {
	kind := ""
	switch profile {
	case "humanitec/default-module":
		kind = "Deployment"
	case "humanitec/default-cronjob":
		kind = "CronJob"
	}
	id, version, _ := strings.Cut(profile, "@")
	var matches []templatesource.WorkloadTemplate
	for _, t := range templates {
		if kind != "" {
			if t.Kind == kind {
				matches = append(matches, t)
			}
		} else if t.ID == id && (version == "" || version == t.Version) {
			matches = append(matches, t)
		}
	}
	if kind != "" {
		ids := map[string]bool{}
		for _, t := range matches {
			ids[t.ID] = true
		}
		if len(ids) != 1 {
			return templatesource.WorkloadTemplate{}, fmt.Errorf("%w: default profile requires one template ID; specify profile", ErrScoreProfile)
		}
	}
	if len(matches) == 0 {
		return templatesource.WorkloadTemplate{}, fmt.Errorf("%w: profile unavailable in selected source", ErrScoreProfile)
	}
	selected := matches[0]
	for _, t := range matches[1:] {
		if semver.Compare("v"+t.Version, "v"+selected.Version) > 0 {
			selected = t
		}
	}
	return selected, nil
}
