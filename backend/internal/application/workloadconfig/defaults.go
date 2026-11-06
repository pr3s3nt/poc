package workloadconfig

import (
	"context"
	"fmt"
	catalogapp "orchestrator/internal/application/catalogrelease"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/templatesource"
	"orchestrator/internal/planning/workloadspec"
	"orchestrator/internal/ports/persistence"
	"reflect"
)

func (s *Service) ResolveDefaults(ctx context.Context, app, env string, raw map[string]any) (map[string]any, map[string]string, error) {
	doc, err := workloadspec.FromMap(raw)
	if err != nil {
		return nil, nil, err
	}
	if e := doc.Execution; e != nil && e.TemplateDefaults != nil && e.TemplateDefaults.Environment != env {
		return nil, nil, fmt.Errorf("%w: policy environment mismatch", ErrInvalidRequest)
	}
	var profile *environment.RuntimeDefaults
	if e := doc.Execution; e.HasTemplate() {
		a, err := s.store.GetApplication(ctx, app)
		if err != nil {
			return nil, nil, err
		}
		records, err := persistence.TemplateSourcesOf(ctx, s.store, a.OrganizationKey)
		if err != nil {
			return nil, nil, err
		}
		records, err = catalogapp.TemplateSources(ctx, s.store, a.OrganizationKey, app, env, records)
		if err != nil {
			return nil, nil, err
		}
		templates, err := templatesource.TemplateCatalog(records)
		if err != nil {
			return nil, nil, err
		}
		t, _, err := templatesource.MatchWorkloadTemplate(templates, e.Template.ID, e.Template.Version, e.Template.SHA256, e.Kind, e.Inputs)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: template unavailable", ErrInvalidRequest)
		}
		profile = t.RuntimeDefaults.ForEnvironment(env)
		if doc.Execution.TemplateDefaults != nil && doc.Execution.TemplateDefaults.Profile != nil && !reflect.DeepEqual(doc.Execution.TemplateDefaults.Profile, profile) {
			return nil, nil, fmt.Errorf("%w: template defaults snapshot differs from pin", ErrInvalidRequest)
		}
	}
	if doc.Execution.HasTemplate() && profile != nil {
		doc.Execution.TemplateDefaults = &environment.TemplateDefaultsSnapshot{Environment: env, Profile: profile}
	}
	var replicas *int
	if _, ok := doc.Map()["replicas"]; ok {
		v := doc.Replicas
		replicas = &v
	}
	effective, sources := (environment.ModuleSpec{Execution: doc.Execution, Containers: doc.Containers, Service: doc.Service, Replicas: replicas}).ResolveDefaults(profile)
	for _, c := range effective.Containers {
		if c.Image == "" {
			return nil, nil, fmt.Errorf("%w: image required", workloadspec.ErrInvalid)
		}
	}
	if err := effective.ValidateComputePolicy(); err != nil {
		return nil, nil, err
	}
	resources := map[string]bool{}
	for name := range doc.Resources {
		resources[name] = true
	}
	if err := effective.Execution.ValidateResolved(effective.Containers, effective.ReplicaCount(), effective.Service != nil && len(effective.Service.Ports) > 0, resources); err != nil {
		return nil, nil, err
	}
	out := doc.Map()
	out["containers"] = effective.Containers
	if effective.Service != nil {
		out["service"] = effective.Service
	}
	if effective.Execution != nil {
		out["workloadExecution"] = effective.Execution
	}
	if effective.Replicas != nil {
		out["replicas"] = *effective.Replicas
	}
	return out, sources, nil
}
