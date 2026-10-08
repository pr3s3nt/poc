// Package envops holds the Environment operation claim manager and the
// runtime-existence rule shared by Settings, deploy, store copy and transitions
// (ADR-012).
package envops

import (
	"context"
	"strings"

	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/persistence"
)

// ScopeID is the Environment resource scope identifier of one generation.
func ScopeID(applicationKey, environmentKey string, generation int64) string {
	return environment.ScopeID(applicationKey, environmentKey, generation)
}

// InGeneration reports whether an Active Resource scope belongs to exactly
// the given generation of the Environment, including workload scopes.
func InGeneration(scope resource.Scope, applicationKey, environmentKey string, generation int64) bool {
	id := ScopeID(applicationKey, environmentKey, generation)
	return scope.ID == id || strings.HasPrefix(scope.ID, id+".")
}

// belongsToEnvironment reports whether the scope belongs to any generation.
func belongsToEnvironment(scope resource.Scope, applicationKey, environmentKey string) bool {
	base := applicationKey + "." + environmentKey
	return scope.ID == base || strings.HasPrefix(scope.ID, base+".") || strings.HasPrefix(scope.ID, base+"~g")
}

// HasRuntime reports whether the Environment owns any runtime state at its
// current target generation: an applied workload instance or an Active
// Resource in its scope. Such an Environment needs an explicit transition to
// move; a metadata-only selection change is allowed only when this is false.
func HasRuntime(ctx context.Context, view persistence.Store, organizationKey string, env environment.Environment) (bool, error) {
	instances, err := view.ListWorkloadInstances(ctx, env.ApplicationKey+"/"+env.Key)
	if err != nil {
		return false, err
	}
	for _, instance := range instances {
		if instance.Status != deployment.InstanceRemoved {
			return true, nil
		}
	}
	active, err := view.ListActiveResources(ctx, organizationKey)
	if err != nil {
		return false, err
	}
	for _, resourceRecord := range active {
		if resourceRecord.Scope.Type != resource.ScopeApplication && InGeneration(resourceRecord.Scope, env.ApplicationKey, env.Key, env.TargetGeneration) {
			return true, nil
		}
		// A legacy Application-scoped VPC/EKS belongs to a LEGACY_APPLICATION binding.
		if env.Scope() == environment.ScopeLegacyApplication && resourceRecord.Scope.Type == resource.ScopeApplication && resourceRecord.Scope.ID == env.ApplicationKey &&
			strings.HasPrefix(resourceRecord.Descriptor.ID, "applications."+env.ApplicationKey) {
			return true, nil
		}
	}
	return false, nil
}
