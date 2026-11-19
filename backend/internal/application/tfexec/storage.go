package tfexec

import (
	"context"
	"errors"
	"sort"
	"strings"

	"orchestrator/internal/ports/terraformexec"
)

var (
	errStorageRemaining = errors.New("tfexec: Kubernetes volumes still refer to deleted application claims")
	errStorageUnknown   = errors.New("tfexec: Kubernetes volume reclamation could not be verified")
)

func (s *Service) remainingApplicationVolumes(ctx context.Context, st *execState, original []terraformexec.StateResource) ([]string, error) {
	claims := map[string]map[string]string{}
	for _, r := range original {
		if ownKinds[r.Type] != "PersistentVolumeClaim" {
			continue
		}
		namespace, name, found := strings.Cut(r.ID, "/")
		if !found || namespace == "" || name == "" || r.UID == "" {
			return nil, errStorageUnknown
		}
		if claims[namespace] == nil {
			claims[namespace] = map[string]string{}
		}
		claims[namespace][name] = r.UID
	}
	for _, namespace := range namespacesOf(original) {
		// Namespace ownership was independently checked by destroy. Include
		// dynamically generated claims even if Terraform never managed them.
		claims[namespace.Name] = nil
	}
	if len(claims) == 0 {
		return nil, nil
	}
	inspector, ok := s.deps.Namespaces.(terraformexec.VolumeInspector)
	if !ok {
		return nil, errStorageUnknown
	}
	var remaining []string
	for namespace, ids := range claims {
		current, err := inspector.RemainingVolumes(ctx, st.env.Env, namespace, ids)
		if err != nil {
			return nil, errStorageUnknown
		}
		remaining = append(remaining, current...)
	}
	sort.Strings(remaining)
	return remaining, nil
}
