// Package workloadconfig implements UC-16 pending workload configuration.
package workloadconfig

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning/score"
	"orchestrator/internal/platform/canon"
	"orchestrator/internal/ports/persistence"
)

var ErrInvalid = errors.New("workloadconfig: invalid input")
var bindingPattern = regexp.MustCompile(`^\$\{resources\.([A-Za-z0-9_-]+)\.([A-Za-z_][A-Za-z0-9_-]*)\}$`)

type Item struct {
	ID           string                 `json:"id"`
	State        environment.DraftState `json:"state"`
	Score        map[string]any         `json:"score,omitempty"`
	Ready        bool                   `json:"ready"`
	ServicePorts []string               `json:"servicePorts,omitempty"`
}

type View struct {
	ApplicationKey string `json:"applicationKey"`
	EnvironmentKey string `json:"environmentKey"`
	DraftVersion   int64  `json:"draftVersion"`
	Workloads      []Item `json:"workloads"`
}

type Service struct{ store persistence.Store }

func NewService(store persistence.Store) *Service { return &Service{store: store} }

func (s *Service) List(ctx context.Context, app, env string) (View, error) {
	environmentRecord, err := s.store.GetEnvironment(ctx, app, env)
	if err != nil {
		return View{}, err
	}
	set, err := s.store.GetDeploymentSet(ctx, environmentRecord.CurrentDeploymentSetID)
	if err != nil {
		return View{}, err
	}
	drafts, err := s.store.ListWorkloadDrafts(ctx, app, env)
	if err != nil {
		return View{}, err
	}
	items := map[string]Item{}
	for id, module := range set.Document.Modules {
		item := Item{ID: id, Ready: true, ServicePorts: portNames(module.Spec.Service)}
		if reconstructed, err := ReconstructScore(id, module, set.Document); err == nil {
			item.Score = reconstructed
		}
		items[id] = item
	}
	for _, draft := range drafts {
		if draft.State == environment.DraftUpsert {
			if current, exists := items[draft.WorkloadID]; exists && current.Score != nil && sameVisibleScore(current.Score, draft.Score) {
				continue // a saved no-op is not a pending user-visible change
			}
		}
		item := Item{ID: draft.WorkloadID, State: draft.State, Score: draft.Score, Ready: set.Document.Modules[draft.WorkloadID].Profile != ""}
		if draft.State == environment.DraftUpsert {
			if parsed, err := score.FromMap(draft.Score); err == nil {
				item.ServicePorts = portNames(parsed.Service)
			}
		}
		items[draft.WorkloadID] = item
	}
	out := View{ApplicationKey: app, EnvironmentKey: env, DraftVersion: environmentRecord.DraftVersion, Workloads: make([]Item, 0, len(items))}
	for _, item := range items {
		out.Workloads = append(out.Workloads, item)
	}
	sort.Slice(out.Workloads, func(i, j int) bool { return out.Workloads[i].ID < out.Workloads[j].ID })
	return out, nil
}

func sameVisibleScore(current, draft map[string]any) bool {
	a, err := score.FromMap(current)
	if err != nil {
		return false
	}
	b, err := score.FromMap(draft)
	if err != nil {
		return false
	}
	a.Service = a.Service.Canonical()
	b.Service = b.Service.Canonical()
	left, err := canon.Hash(a)
	if err != nil {
		return false
	}
	right, err := canon.Hash(b)
	return err == nil && left == right
}

func portNames(service *environment.Service) []string {
	if service == nil {
		return nil
	}
	out := make([]string, 0, len(service.Ports))
	for name := range service.Ports {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (s *Service) Save(ctx context.Context, app, env, workload string, raw map[string]any, expectedVersion int64) (View, error) {
	doc, err := score.FromMap(raw)
	if err != nil {
		return View{}, err
	}
	if doc.Metadata.Name != workload {
		return View{}, fmt.Errorf("%w: workload name does not match URL", ErrInvalid)
	}
	e, err := s.store.GetEnvironment(ctx, app, env)
	if err != nil {
		return View{}, err
	}
	if e.DraftVersion != expectedVersion {
		return View{}, persistence.ErrVersionConflict
	}
	if err := s.validateBindings(ctx, app, env, *doc); err != nil {
		return View{}, err
	}
	if err := s.store.SaveWorkloadDraft(ctx, expectedVersion, environment.WorkloadDraft{ApplicationKey: app, EnvironmentKey: env, WorkloadID: workload, State: environment.DraftUpsert, Score: raw}); err != nil {
		return View{}, err
	}
	return s.List(ctx, app, env)
}

// ValidateImport applies the same UC-16 rules without saving a draft.
func (s *Service) ValidateImport(ctx context.Context, app, env string, raw map[string]any) error {
	if _, err := s.store.GetEnvironment(ctx, app, env); err != nil {
		return err
	}
	doc, err := score.FromMap(raw)
	if err != nil {
		return err
	}
	return s.validateBindings(ctx, app, env, *doc)
}

func (s *Service) Delete(ctx context.Context, app, env, workload string, expectedVersion int64) (View, error) {
	e, err := s.store.GetEnvironment(ctx, app, env)
	if err != nil {
		return View{}, err
	}
	if e.DraftVersion != expectedVersion {
		return View{}, persistence.ErrVersionConflict
	}
	set, err := s.store.GetDeploymentSet(ctx, e.CurrentDeploymentSetID)
	if err != nil {
		return View{}, err
	}
	prior, priorErr := s.store.GetWorkloadDraft(ctx, app, env, workload)
	if priorErr != nil && !errors.Is(priorErr, persistence.ErrNotFound) {
		return View{}, priorErr
	}
	_, deployed := set.Document.Modules[workload]
	if !deployed && priorErr != nil {
		return View{}, persistence.ErrNotFound
	}
	if !deployed && priorErr == nil && prior.State == environment.DraftUpsert {
		if err := s.store.DeleteWorkloadDraft(ctx, app, env, workload, expectedVersion); err != nil {
			return View{}, err
		}
		return s.List(ctx, app, env)
	}
	draft := environment.WorkloadDraft{ApplicationKey: app, EnvironmentKey: env, WorkloadID: workload, State: environment.DraftDelete}
	if priorErr == nil && prior.State == environment.DraftUpsert {
		draft.Score = prior.Score
	}
	if err := s.store.SaveWorkloadDraft(ctx, expectedVersion, draft); err != nil {
		return View{}, err
	}
	return s.List(ctx, app, env)
}

func (s *Service) Undo(ctx context.Context, app, env, workload string, expectedVersion int64) (View, error) {
	e, err := s.store.GetEnvironment(ctx, app, env)
	if err != nil {
		return View{}, err
	}
	if e.DraftVersion != expectedVersion {
		return View{}, persistence.ErrVersionConflict
	}
	draft, err := s.store.GetWorkloadDraft(ctx, app, env, workload)
	if err != nil {
		return View{}, err
	}
	if draft.State != environment.DraftDelete {
		return View{}, ErrInvalid
	}
	if draft.Score != nil {
		draft.State = environment.DraftUpsert
		err = s.store.SaveWorkloadDraft(ctx, expectedVersion, draft)
	} else {
		err = s.store.DeleteWorkloadDraft(ctx, app, env, workload, expectedVersion)
	}
	if err != nil {
		return View{}, err
	}
	return s.List(ctx, app, env)
}

func (s *Service) validateBindings(ctx context.Context, app, env string, doc score.Document) error {
	scope, err := s.store.GetConfigurationScope(ctx, app, env)
	if err != nil {
		return err
	}
	entries := map[string]configuration.Entry{}
	if scope.DesiredRevisionID != "" {
		rev, err := s.store.GetConfigurationRevision(ctx, scope.DesiredRevisionID)
		if err != nil {
			return err
		}
		entries = rev.Entries
	}
	applicationRecord, err := s.store.GetApplication(ctx, app)
	if err != nil {
		return err
	}
	types, err := s.store.ListResourceTypes(ctx, applicationRecord.OrganizationKey)
	if err != nil {
		return err
	}
	typeMap := map[string]resource.Type{}
	for _, typ := range types {
		typeMap[typ.Key] = typ
	}
	if err := validateResourceParams(doc, typeMap); err != nil {
		return err
	}
	for containerName, container := range doc.Containers {
		for key, value := range container.Variables {
			match := bindingPattern.FindStringSubmatch(value)
			if match == nil {
				return fmt.Errorf("%w: container %s variable %s must be one reference", ErrInvalid, containerName, key)
			}
			alias, output := match[1], match[2]
			resourceSpec, ok := doc.Resources[alias]
			if !ok {
				return fmt.Errorf("%w: undeclared resource %s", ErrInvalid, alias)
			}
			switch resourceSpec.Type {
			case "environment":
				if alias != "env" || resourceSpec.ID != "" {
					return fmt.Errorf("%w: Application keys require resources.env", ErrInvalid)
				}
				if _, ok := entries[output]; !ok {
					return fmt.Errorf("%w: Application key %s is not configured in %s", ErrInvalid, output, env)
				}
			case "service":
				if output != "url" {
					return fmt.Errorf("%w: service output must be url", ErrInvalid)
				}
				if err := s.validateService(ctx, app, env, doc.Metadata.Name, resourceSpec); err != nil {
					return err
				}
			default:
				typ, ok := typeMap[resourceSpec.Type]
				if !ok {
					return fmt.Errorf("%w: unknown resource type %s", ErrInvalid, resourceSpec.Type)
				}
				if _, ok := typ.Output(output); !ok {
					return fmt.Errorf("%w: resource %s has no output %s", ErrInvalid, alias, output)
				}
			}
		}
	}
	return nil
}

// validateResourceParams checks every declared non-virtual resource, bound or
// not, against the Organization Resource Type catalog (UC-16 BR-14). It uses
// the Score planning input subset plus an explicit null rejection. Errors
// name the field path, never the submitted value.
func validateResourceParams(doc score.Document, types map[string]resource.Type) error {
	for _, alias := range doc.ResourceAliases() {
		spec := doc.Resources[alias]
		if spec.Type == "environment" || spec.Type == "service" {
			continue // virtual resources keep their dedicated validation
		}
		path := "resources." + alias
		typ, ok := types[spec.Type]
		if !ok {
			return fmt.Errorf("%w: %s.type is not a registered resource type", ErrInvalid, path)
		}
		names := make([]string, 0, len(spec.Params))
		for name := range spec.Params {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			field, ok := typ.Input(name)
			if !ok {
				return fmt.Errorf("%w: %s.params.%s is not declared by resource type %s", ErrInvalid, path, name, typ.Key)
			}
			value := spec.Params[name]
			if value == nil {
				return fmt.Errorf("%w: %s.params.%s must not be null", ErrInvalid, path, name)
			}
			if err := field.CheckValue(value); err != nil {
				return fmt.Errorf("%w: %s.params.%s must be %s", ErrInvalid, path, name, field.Type)
			}
		}
		for _, field := range typ.Inputs {
			if _, ok := spec.Params[field.Name]; field.Required && !ok {
				return fmt.Errorf("%w: %s.params.%s is required by resource type %s", ErrInvalid, path, field.Name, typ.Key)
			}
		}
	}
	return nil
}

func (s *Service) validateService(ctx context.Context, app, env, source string, spec score.ResourceSpec) error {
	workload, _ := spec.Params["workload"].(string)
	portName, _ := spec.Params["port"].(string)
	if workload == "" || portName == "" || strings.ContainsAny(workload, "/.") {
		return fmt.Errorf("%w: service requires workload and port", ErrInvalid)
	}
	if workload == source {
		return fmt.Errorf("%w: workload cannot reference its own service", ErrInvalid)
	}
	if draft, err := s.store.GetWorkloadDraft(ctx, app, env, workload); err == nil {
		if draft.State == environment.DraftDelete {
			return fmt.Errorf("%w: referenced service is pending deletion", ErrInvalid)
		}
		parsed, err := score.FromMap(draft.Score)
		if err != nil {
			return fmt.Errorf("%w: referenced service has an invalid draft", ErrInvalid)
		}
		if parsed.Service != nil {
			if _, ok := parsed.Service.Ports[portName]; ok {
				return nil
			}
		}
		return fmt.Errorf("%w: service %s/%s is not declared in the current draft", ErrInvalid, workload, portName)
	} else if !errors.Is(err, persistence.ErrNotFound) {
		return err
	}
	e, err := s.store.GetEnvironment(ctx, app, env)
	if err != nil {
		return err
	}
	set, err := s.store.GetDeploymentSet(ctx, e.CurrentDeploymentSetID)
	if err != nil {
		return err
	}
	if module, ok := set.Document.Modules[workload]; ok && module.Spec.Service != nil {
		if _, ok := module.Spec.Service.Ports[portName]; ok {
			return nil
		}
	}
	return fmt.Errorf("%w: service %s/%s does not exist in %s", ErrInvalid, workload, portName, env)
}
