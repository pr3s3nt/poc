package accountdiag

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"orchestrator/internal/application/pending"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/configuration"
	domain "orchestrator/internal/domain/diagnostics"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/planning"
	"orchestrator/internal/planning/score"
	probes "orchestrator/internal/ports/diagnostics"
	"orchestrator/internal/ports/persistence"
)

// ContextStore is the read-only slice of the platform store that requirement
// resolution needs. No method here can change an Application, Environment,
// draft or Deployment Set.
type ContextStore interface {
	GetApplication(ctx context.Context, key string) (application.Application, error)
	GetEnvironment(ctx context.Context, applicationKey, environmentKey string) (environment.Environment, error)
	GetDeploymentSet(ctx context.Context, id string) (environment.DeploymentSet, error)
	GetConfigurationScope(ctx context.Context, applicationKey, environmentKey string) (configuration.Scope, error)
	ListWorkloadDrafts(ctx context.Context, applicationKey, environmentKey string) ([]environment.WorkloadDraft, error)
	ListResourceDefinitions(ctx context.Context, organizationKey string) ([]resource.Definition, error)
	GetModuleBundle(ctx context.Context, ref tfmodule.SourceRef) (tfmodule.Bundle, error)
}

// Planner is the existing pure pending-change planning pipeline: one read-only
// snapshot of drafts, applied set, configuration and catalog. Diagnostics reuse
// it, so before-deploy problems are the ones Preview and Deploy would meet.
type Planner interface {
	Preview(ctx context.Context, applicationKey, environmentKey string) (pending.Preview, error)
}

// resolveContext validates an Application/Environment selection against the
// Organization and derives the read-only requirements of its desired state
// (drafts and applied configuration together). A foreign or unknown key is
// persistence.ErrNotFound, never disclosed.
func (s *Service) resolveContext(ctx context.Context, org string, req domain.Request) (*domain.ContextRef, *probes.Requirements, error) {
	if req.Application == "" {
		return nil, nil, nil
	}
	app, err := s.context.GetApplication(ctx, req.Application)
	if err != nil || app.OrganizationKey != org || app.Key != req.Application {
		if err != nil && !errors.Is(err, persistence.ErrNotFound) {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("%w: application", persistence.ErrNotFound)
	}
	ref := &domain.ContextRef{ApplicationKey: app.Key, ApplicationName: app.Name, ApplicationVersion: app.Version}
	need := &probes.Requirements{ApplicationKey: app.Key, SecretDelivery: s.cfg.SecretDelivery}
	if app.Subdomain != "" && s.cfg.BaseDomain != "" {
		need.PublicHost = app.Subdomain + "." + s.cfg.BaseDomain
	}
	if req.Environment == "" {
		return ref, need, nil
	}
	env, err := s.context.GetEnvironment(ctx, app.Key, req.Environment)
	if err != nil || env.ApplicationKey != app.Key {
		if err != nil && !errors.Is(err, persistence.ErrNotFound) {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("%w: environment", persistence.ErrNotFound)
	}
	ref.EnvironmentKey, ref.EnvironmentVersion, ref.EnvironmentType, ref.CurrentDeploymentID = env.Key, env.Version, env.Type, env.CurrentDeploymentSetID
	ref.DraftVersion = env.DraftVersion
	need.EnvironmentKey, need.EnvironmentType, need.Namespace, need.SecretStoreKey = env.Key, env.Type, env.Namespace(), env.SecretStoreKey
	if scope, err := s.context.GetConfigurationScope(ctx, app.Key, env.Key); err == nil {
		ref.ConfigVersion = scope.Version
	}
	defs, err := s.context.ListResourceDefinitions(ctx, org)
	if err != nil {
		return nil, nil, err
	}
	s.plan2(ctx, app.Key, env, defs, need, ref)
	return ref, need, nil
}

// plan2 runs the planning pipeline and fills the requirements from its result.
func (s *Service) plan2(ctx context.Context, appKey string, env environment.Environment, defs []resource.Definition, need *probes.Requirements, ref *domain.ContextRef) {
	if s.planner == nil {
		need.PlanUnavailable = true
		return
	}
	preview, err := s.planner.Preview(ctx, appKey, env.Key)
	if err != nil {
		var staged *planning.StageError
		switch {
		case errors.As(err, &staged):
			need.PlanStage, need.PlanReason = string(staged.Stage), staged.Reason
		case errors.Is(err, pending.ErrInvalid):
			need.PlanStage, need.PlanReason = "pending", "invalid"
		default:
			need.PlanUnavailable = true
		}
		s.fillWorkloads(ctx, appKey, env, need)
		return
	}
	need.Resolved = true
	need.PendingChanges = len(preview.Changes)
	s.fillWorkloads(ctx, appKey, env, need)
	byKey := map[string]resource.Definition{}
	for _, d := range defs {
		byKey[d.Key] = d
	}
	pins := map[string]int64{}
	pin := func(key string) {
		if d, ok := byKey[key]; ok && len(pins) < domain.MaxProbeItems {
			pins[d.Key] = d.Version
		}
	}
	seen := map[string]int{}
	seenPlacement := map[string]bool{}
	byNode := map[string]planning.Placement{}
	for _, change := range preview.Changes {
		pin(change.Rendering.DefinitionKey)
		for _, intent := range change.ResourceIntents {
			if intent.Intent == planning.IntentRetain && intent.Ownership == planning.OwnerPreexisting {
				continue
			}
			pin(intent.DefinitionKey)
			n := s.needOf(ctx, intent, byKey)
			if at, dup := seen[n.Descriptor]; dup {
				need.Resources[at] = n
				continue
			}
			seen[n.Descriptor] = len(need.Resources)
			need.Resources = append(need.Resources, n)
		}
		for _, p := range change.Placements {
			key := p.Account + "|" + p.Cluster + "|" + p.Namespace
			if !seenPlacement[key] {
				seenPlacement[key] = true
				need.Placements = append(need.Placements, probes.PlacementNeed{Cluster: p.Cluster, Account: p.Account, Namespace: p.Namespace, Provider: p.Provider, Region: p.Region})
			}
			byNode[p.Node] = p
		}
	}
	for i := range need.Resources {
		if p, ok := byNode[need.Resources[i].Descriptor]; ok {
			need.Resources[i].Cluster, need.Resources[i].Namespace = p.Account, p.Namespace
		}
	}
	sort.Slice(need.Resources, func(i, j int) bool { return need.Resources[i].Descriptor < need.Resources[j].Descriptor })
	for key, version := range pins {
		ref.Definitions = append(ref.Definitions, domain.DefinitionPin{Key: key, Version: version})
	}
	sort.Slice(ref.Definitions, func(i, j int) bool { return ref.Definitions[i].Key < ref.Definitions[j].Key })
}

// fillWorkloads counts the workloads and public routes of the desired final
// state: the applied set with every pending draft applied on top.
func (s *Service) fillWorkloads(ctx context.Context, appKey string, env environment.Environment, need *probes.Requirements) {
	routes := map[string]int{}
	if set, err := s.context.GetDeploymentSet(ctx, env.CurrentDeploymentSetID); err == nil {
		for id, module := range set.Document.Modules {
			if module.Spec.Service != nil {
				routes[id] = len(module.Spec.Service.PublicRoutes)
			} else {
				routes[id] = 0
			}
		}
	}
	if drafts, err := s.context.ListWorkloadDrafts(ctx, appKey, env.Key); err == nil {
		for _, d := range drafts {
			if d.State == environment.DraftDelete {
				delete(routes, d.WorkloadID)
				continue
			}
			routes[d.WorkloadID] = 0
			if doc, err := score.FromMap(d.Score); err == nil && doc.Service != nil {
				routes[d.WorkloadID] = len(doc.Service.PublicRoutes)
			}
		}
	}
	need.Workloads = len(routes)
	for _, n := range routes {
		need.PublicRoutes += n
	}
}

// needOf derives one planned resource's requirements from its matched
// Definition and the verified module contract.
func (s *Service) needOf(ctx context.Context, intent planning.ResourceIntent, defs map[string]resource.Definition) probes.ResourceNeed {
	n := probes.ResourceNeed{Descriptor: intent.Descriptor, Type: intent.ResourceType, Class: intent.Class, Definition: intent.DefinitionKey, DriverType: intent.DriverType}
	for _, p := range intent.Providers {
		n.Providers = append(n.Providers, probes.ProviderTarget{Kind: p.Kind, Connection: p.ConnectionKey, Region: p.Region})
	}
	def := defs[intent.DefinitionKey]
	switch resource.DriverType(intent.DriverType) {
	case resource.DriverKubernetes:
		// The native executor claims storage from the cluster default class.
		if domain.CapabilityOf(intent.ResourceType) != "" {
			n.Storage = probes.StorageNeed{Required: true, Resolved: true}
		}
	case resource.DriverTerraform:
		if intent.Module == nil {
			return n
		}
		n.Module = intent.Module.Module
		bundle, err := s.context.GetModuleBundle(ctx, tfmodule.SourceRef{SourceID: intent.Module.SourceID, ModuleID: intent.Module.Module, Revision: intent.Module.Revision, Subdirectory: intent.Module.Subdirectory, ContentSHA256: intent.Module.ContentSHA256})
		if err != nil {
			return n
		}
		n.Packaged = bundle.Contract.SourceKind == tfmodule.SourceKindPackaged
		n.Storage = storageOf(def, bundle)
	}
	return n
}

// storageOf reads the storage requirement from the module contract's declared
// input and the Definition's literal value. A module that declares no storage
// input is unresolved, not storage-free; a non-literal value is unresolved,
// not the default class.
func storageOf(def resource.Definition, bundle tfmodule.Bundle) probes.StorageNeed {
	input, declared := bundle.Contract.Inputs["storage_class"]
	if !declared {
		return probes.StorageNeed{}
	}
	if raw, set := def.Variables()["storage_class"]; set {
		value, ok := plainString(raw)
		return probes.StorageNeed{Required: true, Class: value, Resolved: ok}
	}
	if input.HasDefault {
		if value, ok := input.Default.(string); ok {
			return probes.StorageNeed{Required: true, Class: value, Resolved: true}
		}
	}
	return probes.StorageNeed{Required: true}
}

// plainString returns the value of a plain quoted HCL string literal; any
// reference, interpolation or non-string is not resolvable.
func plainString(raw any) (string, bool) {
	text, ok := raw.(string)
	if !ok || len(text) < 2 || text[0] != '"' || text[len(text)-1] != '"' {
		return "", false
	}
	inner := text[1 : len(text)-1]
	for _, r := range inner {
		if r == '"' || r == '$' || r == '\\' || r == '%' {
			return "", false
		}
	}
	return inner, true
}
