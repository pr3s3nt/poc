// Package envops holds the Environment operation claim manager.
package envops

import (
	"strings"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
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
