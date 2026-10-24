package accountdiag

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"orchestrator/internal/domain/application"
	domain "orchestrator/internal/domain/diagnostics"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/secretstore"
	"orchestrator/internal/planning"
	probes "orchestrator/internal/ports/diagnostics"
	"orchestrator/internal/ports/persistence"
)

// ContextStore is the read-only slice of the platform store that requirement
// resolution needs. No method here can change an Application, Environment or
// Deployment Set.
type ContextStore interface {
	GetApplication(ctx context.Context, key string) (application.Application, error)
	GetEnvironment(ctx context.Context, applicationKey, environmentKey string) (environment.Environment, error)
	GetDeploymentSet(ctx context.Context, id string) (environment.DeploymentSet, error)
	ListResourceDefinitions(ctx context.Context, organizationKey string) ([]resource.Definition, error)
	GetSecretStore(ctx context.Context, organizationKey, key string) (secretstore.Store, error)
}

// resolveContext validates an Application/Environment selection against the
// Organization and derives the read-only requirements of its desired state.
// A foreign or unknown key is persistence.ErrNotFound, never disclosed.
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
	ref.EnvironmentKey, ref.EnvironmentVersion, ref.EnvironmentType, ref.CurrentDeploymentID = env.Key, env.Version, env.Type, env.CurrentDeploymentSetID
	need.EnvironmentKey, need.EnvironmentType, need.Namespace = env.Key, env.Type, env.Namespace()
	need.SecretStoreKey = env.SecretStoreKey
	if env.SecretStoreKey != "" {
		store, err := s.context.GetSecretStore(ctx, org, env.SecretStoreKey)
		need.SecretStoreReady = err == nil && store.Status == secretstore.StatusReady
	}
	if env.CurrentDeploymentSetID == "" {
		return ref, need, nil
	}
	set, err := s.context.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return ref, need, nil
		}
		return nil, nil, err
	}
	defs, err := s.context.ListResourceDefinitions(ctx, org)
	if err != nil {
		return nil, nil, err
	}
	need.Resolved = true
	need.Workloads = len(set.Document.Modules)
	for _, id := range set.Document.ModuleIDs() {
		module := set.Document.Modules[id]
		if module.Spec.Service != nil {
			need.PublicRoutes += len(module.Spec.Service.PublicRoutes)
		}
		for _, name := range module.ExternalNames() {
			entry := module.Externals[name]
			descriptor, err := planning.PrivateDescriptor(id, name, entry)
			if err != nil {
				continue
			}
			need.Resources = append(need.Resources, matchNeed(app.Key, env, descriptor, entry, defs))
		}
	}
	for _, id := range set.Document.SharedIDs() {
		entry := set.Document.Shared[id]
		descriptor, err := planning.SharedDescriptor(id, entry)
		if err != nil {
			continue
		}
		need.Resources = append(need.Resources, matchNeed(app.Key, env, descriptor, entry, defs))
	}
	sort.Slice(need.Resources, func(i, j int) bool { return need.Resources[i].Alias < need.Resources[j].Alias })
	return ref, need, nil
}

// matchNeed ranks the Definition catalog exactly as the planner does and
// derives the storage requirement from the winner's declared variables.
func matchNeed(appKey string, env environment.Environment, d resource.Descriptor, entry environment.ResourceEntry, defs []resource.Definition) probes.ResourceNeed {
	class := entry.Class
	if class == "" {
		class = planning.ClassDefault
	}
	need := probes.ResourceNeed{Alias: d.ID, Type: entry.Type, Class: class}
	ranking := resource.Rank(defs, entry.Type, resource.MatchContext{EnvironmentType: env.Type, ApplicationID: appKey, EnvironmentID: env.Key, ResourceID: d.ID, Class: class}, nil)
	switch len(ranking.Top) {
	case 0:
		need.Outcome = "NO_MATCH"
	case 1:
		win := ranking.Top[0].Definition
		need.Outcome, need.Definition, need.DriverType, need.DriverAccount = "MATCHED", win.Key, string(win.DriverType), win.DriverAccount
		vars := win.Variables()
		if raw, ok := vars["storage_class"]; ok {
			need.NeedsStorage = true
			if text, ok := raw.(string); ok {
				need.StorageClass = unquoteLiteral(text)
			}
		} else if _, ok := vars["storage"]; ok {
			need.NeedsStorage = true
		}
	default:
		need.Outcome = "AMBIGUOUS"
	}
	return need
}

// unquoteLiteral returns the value of a Terraform string literal such as
// "gp2", or "" for a reference or expression that is not a plain literal.
func unquoteLiteral(text string) string {
	if len(text) >= 2 && text[0] == '"' && text[len(text)-1] == '"' {
		inner := text[1 : len(text)-1]
		for _, r := range inner {
			if r == '"' || r == '$' || r == '\\' {
				return ""
			}
		}
		return inner
	}
	return ""
}
