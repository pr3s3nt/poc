package accountdiag

import (
	"context"
	"errors"
	"fmt"
	"sort"

	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/pending"
	"orchestrator/internal/domain/application"
	domain "orchestrator/internal/domain/diagnostics"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/planning"
	probes "orchestrator/internal/ports/diagnostics"
	"orchestrator/internal/ports/persistence"
)

// Planner plans the complete desired state of an Environment (drafts on top of
// the applied set, unchanged workloads included) on ONE snapshot, returning the
// records it used. Diagnostics reuse it, so before-deploy problems are the ones
// Preview and Deploy would meet and nothing is read from a different version.
type Planner interface {
	Desired(ctx context.Context, applicationKey, environmentKey string) (pending.Desired, error)
}

// ContextStore validates the selection against the Organization. It is only
// used to refuse a foreign or unknown Application/Environment; every fact about
// the desired state comes from the Planner's single snapshot.
type ContextStore interface {
	GetApplication(ctx context.Context, key string) (application.Application, error)
	GetEnvironment(ctx context.Context, applicationKey, environmentKey string) (environment.Environment, error)
}

// ErrContextUnavailable means the desired state could not be read at all; the
// run is refused rather than started on guessed, empty facts.
var ErrContextUnavailable = errors.New("accountdiag: the desired state could not be read")

// resolveContext derives the read-only requirements of the selected
// Application/Environment from one planning snapshot. A foreign or unknown key
// is persistence.ErrNotFound, never disclosed.
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
	need.EnvironmentKey, need.EnvironmentType, need.Namespace, need.SecretStoreKey = env.Key, env.Type, env.Namespace(), env.SecretStoreKey
	if app.Subdomain != "" && s.cfg.BaseDomain != "" {
		need.PublicHost = appsvc.PublicHost(app.Subdomain, env.Key, s.cfg.BaseDomain)
	}
	ref.EnvironmentKey, ref.EnvironmentVersion, ref.EnvironmentType, ref.CurrentDeploymentID, ref.DraftVersion = env.Key, env.Version, env.Type, env.CurrentDeploymentSetID, env.DraftVersion
	if s.planner == nil {
		need.PlanUnavailable = true
		return ref, need, nil
	}
	desired, err := s.planner.Desired(ctx, app.Key, env.Key)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return nil, nil, fmt.Errorf("%w: environment", persistence.ErrNotFound)
		}
		return nil, nil, fmt.Errorf("%w: %w", ErrContextUnavailable, err)
	}
	// Pins come from the same snapshot the plan was made on.
	ref.ApplicationVersion, ref.EnvironmentVersion, ref.CurrentDeploymentID = desired.Application.Version, desired.Environment.Version, desired.Environment.CurrentDeploymentSetID
	ref.DraftVersion, ref.ConfigVersion = desired.Environment.DraftVersion, desired.ConfigVersion
	need.Namespace, need.SecretStoreKey = desired.Environment.Namespace(), desired.Environment.SecretStoreKey
	fillFinal(desired.Final, need)
	if desired.PlanErr != nil {
		var staged *planning.StageError
		switch {
		case errors.As(desired.PlanErr, &staged):
			need.PlanStage, need.PlanReason = string(staged.Stage), staged.Reason
		default:
			need.PlanStage, need.PlanReason = "pending", "invalid"
		}
		// A rejected plan still pins the drafts and catalog it was judged on.
		return ref, need, nil
	}
	need.Resolved = true
	need.PendingChanges = len(desired.Preview.Changes)
	byKey := map[string]resource.Definition{}
	for _, d := range desired.Definitions {
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
	for _, change := range desired.Preview.Changes {
		pin(change.Rendering.DefinitionKey)
		for _, intent := range change.ResourceIntents {
			if intent.Intent == planning.IntentRetain && intent.Ownership == planning.OwnerPreexisting {
				continue
			}
			pin(intent.DefinitionKey)
			n := needOf(intent, byKey[intent.DefinitionKey], desired.Bundles[intent.Descriptor])
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
	return ref, need, nil
}

// fillFinal counts the workloads and public routes of the final desired state.
func fillFinal(final environment.Document, need *probes.Requirements) {
	need.Workloads = len(final.Modules)
	for _, module := range final.Modules {
		if module.Spec.Service != nil {
			need.PublicRoutes += len(module.Spec.Service.PublicRoutes)
		}
	}
}

// needOf derives one planned resource's requirements from its matched
// Definition and the verified module contract read on the same snapshot.
func needOf(intent planning.ResourceIntent, def resource.Definition, bundle tfmodule.Bundle) probes.ResourceNeed {
	n := probes.ResourceNeed{Descriptor: intent.Descriptor, Type: intent.ResourceType, Class: intent.Class, Definition: intent.DefinitionKey, DriverType: intent.DriverType}
	for _, p := range intent.Providers {
		n.Providers = append(n.Providers, probes.ProviderTarget{Kind: p.Kind, Connection: p.ConnectionKey, Region: p.Region})
	}
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
		if bundle.Ref.ContentSHA256 == "" {
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
