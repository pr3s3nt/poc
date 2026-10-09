package planning

import (
	"errors"
	"fmt"
	"reflect"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/platform/canon"
)

// BuiltinClusterKey is the reserved system-owned Definition identity of the
// implicit internal-k8s cluster node (ADR-013).
const BuiltinClusterKey = "builtin-existing-cluster"

// BindingEnvironmentConnection marks a Match produced from the Environment
// Connection binding instead of Definition criteria matching.
const BindingEnvironmentConnection = "environment-connection"

// ErrReservedDefinition reports a stored Definition that uses the reserved
// builtin key with content other than the trusted system definition.
var ErrReservedDefinition = errors.New("planning: definition key " + BuiltinClusterKey + " is reserved for the system")

// BuiltinClusterDefinition returns the exact trusted system Definition. It is
// constructed by code, never selected by matching criteria. The criterion only
// satisfies Definition validation; the matcher never evaluates it.
func BuiltinClusterDefinition() resource.Definition {
	return resource.Definition{
		Key:              BuiltinClusterKey,
		ResourceTypeKey:  TypeCluster,
		ExecutionProfile: string(application.ProfileInternalK8s),
		DriverType:       resource.DriverExistingCluster,
		DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{
			"name":        "${context.connection.cluster}",
			"kubeContext": "${context.connection.context}",
		}}},
		Criteria: []resource.Criterion{{Class: ClassInternal}},
	}
}

// IsBuiltinClusterDefinition reports whether def equals the trusted definition.
func IsBuiltinClusterDefinition(def resource.Definition) bool {
	left, err := canon.Map(def)
	if err != nil {
		return false
	}
	right, err := canon.Map(BuiltinClusterDefinition())
	if err != nil {
		return false
	}
	return reflect.DeepEqual(left, right)
}

// isImplicitCluster reports whether node is the Environment-bound cluster of
// an internal-k8s Environment.
func (c Context) isImplicitCluster(node *Node) bool {
	if c.ReferenceCluster || c.Env.Profile != application.ProfileInternalK8s || node.ResourceType != TypeCluster || node.Class != ClassInternal {
		return false
	}
	want, err := InternalClusterDescriptor(c.Env.ConnectionKey)
	return err == nil && node.Descriptor == want.String()
}

// ValidateBuiltinMatch checks a stored Match and its node against the
// Environment binding for defense in depth at execution time. It rejects a
// forged Binding or the reserved key on any other node.
func ValidateBuiltinMatch(ctx Context, node Node, m Match) error {
	if m.DefinitionKey != BuiltinClusterKey {
		if m.Binding != "" {
			return fmt.Errorf("planning: match for %s carries a binding without the builtin definition", m.Descriptor)
		}
		return nil
	}
	want, err := InternalClusterDescriptor(ctx.Env.ConnectionKey)
	if err != nil {
		return err
	}
	scope, err := ScopeFor(ctx, want)
	if err != nil {
		return err
	}
	if ctx.Env.Profile != application.ProfileInternalK8s || m.Binding != BindingEnvironmentConnection ||
		m.ConnectionKey != ctx.Env.ConnectionKey || m.DriverType != resource.DriverExistingCluster ||
		m.Descriptor != want.String() || node.Descriptor != want.String() ||
		node.Kind != NodeResource || node.ResourceType != TypeCluster || node.Class != ClassInternal || node.Scope != scope {
		return fmt.Errorf("planning: builtin cluster match for %s does not follow the environment connection", m.Descriptor)
	}
	return nil
}

// ErrClusterConnection reports an Environment Connection that cannot back the
// implicit cluster: wrong identity, kind, status or Organization.
var ErrClusterConnection = errors.New("planning: the environment connection cannot back the implicit cluster")

// checkImplicitConnection fails closed before the implicit cluster is planned.
func (c Context) checkImplicitConnection() error {
	if c.ReferenceCluster || c.Env.Profile != application.ProfileInternalK8s {
		return nil
	}
	conn := c.Connection
	if conn.Key != c.Env.ConnectionKey || conn.Kind != application.ConnectionKubernetes ||
		conn.Status != application.ConnectionReady || conn.OrganizationKey != c.OrganizationKey {
		return fmt.Errorf("%w: connection %q for environment %q", ErrClusterConnection, c.Env.ConnectionKey, c.Env.Key)
	}
	return nil
}
