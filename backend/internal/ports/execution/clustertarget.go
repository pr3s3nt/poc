package execution

import (
	"fmt"

	"orchestrator/internal/domain/resource"
)

// ClusterOutput renders the typed opaque `cluster` output of an
// existing-cluster resource (ADR-016): cluster identity plus the Driver
// Account that reaches it. It never contains credential bytes or paths.
func (t Target) ClusterOutput(descriptor string, accountVersion int64) map[string]any {
	return map[string]any{
		"kind":           resource.K8sTargetKind,
		"clusterName":    t.ClusterName,
		"context":        t.Context,
		"organization":   t.Organization,
		"account":        t.Connection,
		"accountVersion": float64(accountVersion),
		"descriptor":     descriptor,
	}
}

// ClusterRef is the decoded identity of a typed cluster output.
type ClusterRef struct {
	Target         Target
	Descriptor     string
	AccountVersion int64
}

// ParseClusterOutput decodes and validates a typed `cluster` output value.
func ParseClusterOutput(value any) (ClusterRef, error) {
	if err := resource.ValidateK8sTarget(value); err != nil {
		return ClusterRef{}, fmt.Errorf("execution: invalid cluster target: %w", err)
	}
	m := value.(map[string]any)
	text := func(key string) string { s, _ := m[key].(string); return s }
	version := int64(0)
	switch v := m["accountVersion"].(type) {
	case float64:
		version = int64(v)
	case int64:
		version = v
	case int:
		version = int64(v)
	}
	return ClusterRef{
		Target:         Target{Kind: "kubernetes", Context: text("context"), ClusterName: text("clusterName"), Organization: text("organization"), Connection: text("account")},
		Descriptor:     text("descriptor"),
		AccountVersion: version,
	}, nil
}
