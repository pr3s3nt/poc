package planning

import (
	"fmt"

	"orchestrator/internal/domain/environment"
)

// ValidatePublicRoutes checks the complete Environment desired state.
func ValidatePublicRoutes(set environment.Document) error {
	owners := map[string]string{}
	for _, workload := range set.ModuleIDs() {
		service := set.Modules[workload].Spec.Service
		if service == nil {
			continue
		}
		for _, route := range service.Routes() {
			if route.Path == "" || route.Path[0] != '/' || route.Path != "/" && (route.Path[len(route.Path)-1] == '/' || containsInvalidPath(route.Path)) {
				return fmt.Errorf("planning: invalid public path %q", route.Path)
			}
			if _, ok := service.Ports[route.Port]; !ok {
				return fmt.Errorf("planning: workload %q has undeclared public Service port %q", workload, route.Port)
			}
			if prior, ok := owners[route.Path]; ok {
				return fmt.Errorf("planning: public path %q is declared by both %s and %s", route.Path, prior, workload)
			}
			owners[route.Path] = workload
		}
	}
	return nil
}

func containsInvalidPath(path string) bool {
	for _, ch := range path {
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '/' || ch == '-' || ch == '_' || ch == '.' || ch == '~' {
			continue
		}
		return true
	}
	return false
}
