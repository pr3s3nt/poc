package planning

import (
	"fmt"
	"strings"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
)

// Resource type keys and classes the orchestrator itself understands.
const (
	TypeNamespace = "k8s-namespace"
	TypeCluster   = "k8s-cluster"
	TypeVPC       = "vpc"
	TypeWorkload  = "workload"
	ClassEKS      = "eks"
	ClassInternal = "internal"
	ClassDefault  = "default"
)

// Identity path prefixes. The Resource ID of every node is the path of the
// thing it belongs to, which is what makes the identity derivable from scope
// (UC-06 "Resource Descriptor convention").
const (
	PathModules      = "modules."
	PathShared       = "shared."
	PathApplications = "applications."
	PathEnvironments = "environments."
	PathConnections  = "connections."
)

// WorkloadDescriptor returns `workload.default#modules.<workload-id>`.
func WorkloadDescriptor(workloadID string) (resource.Descriptor, error) {
	return resource.NewDescriptor(TypeWorkload, ClassDefault, PathModules+workloadID)
}

// PrivateDescriptor returns `<type>.<class>#modules.<workload-id>.externals.<name>`.
func PrivateDescriptor(workloadID, name string, entry environment.ResourceEntry) (resource.Descriptor, error) {
	return resource.NewDescriptor(entry.Type, entryClass(entry), PathModules+workloadID+".externals."+name)
}

// SharedDescriptor returns `<type>.<class>#shared.<shared-id>`.
func SharedDescriptor(sharedID string, entry environment.ResourceEntry) (resource.Descriptor, error) {
	return resource.NewDescriptor(entry.Type, entryClass(entry), PathShared+sharedID)
}

// VPCDescriptor returns the VPC node of the aws-eks profile: Application
// scoped for a LEGACY_APPLICATION binding, Environment scoped otherwise.
func VPCDescriptor(ctx Context) (resource.Descriptor, error) {
	return resource.NewDescriptor(TypeVPC, ClassDefault, ctx.InfraPath())
}

// EKSDescriptor returns the EKS node of the aws-eks profile, scoped like the VPC.
func EKSDescriptor(ctx Context) (resource.Descriptor, error) {
	return resource.NewDescriptor(TypeCluster, ClassEKS, ctx.InfraPath())
}

// InternalClusterDescriptor returns the registered-cluster node of internal-k8s.
func InternalClusterDescriptor(connectionKey string) (resource.Descriptor, error) {
	return resource.NewDescriptor(TypeCluster, ClassInternal, PathConnections+connectionKey)
}

// NamespaceDescriptor returns the Environment-scoped namespace node.
func NamespaceDescriptor(applicationKey, environmentKey string) (resource.Descriptor, error) {
	return resource.NewDescriptor(TypeNamespace, ClassDefault, PathEnvironments+applicationKey+"."+environmentKey)
}

func entryClass(entry environment.ResourceEntry) string {
	if entry.Class == "" {
		return ClassDefault
	}
	return entry.Class
}

// ScopeFor derives the Active Resource scope from the identity path.
func ScopeFor(ctx Context, d resource.Descriptor) (resource.Scope, error) {
	envScope := ctx.App.Key + "." + ctx.Env.Key
	switch {
	case strings.HasPrefix(d.ID, PathModules):
		rest := strings.TrimPrefix(d.ID, PathModules)
		workload := rest
		if idx := strings.Index(rest, "."); idx >= 0 {
			workload = rest[:idx]
		}
		return resource.Scope{Type: resource.ScopeWorkload, ID: envScope + "." + workload}, nil
	case strings.HasPrefix(d.ID, PathShared):
		return resource.Scope{Type: resource.ScopeShared, ID: envScope}, nil
	case strings.HasPrefix(d.ID, PathEnvironments):
		return resource.Scope{Type: resource.ScopeEnvironment, ID: envScope}, nil
	case strings.HasPrefix(d.ID, PathApplications), strings.HasPrefix(d.ID, PathConnections):
		return resource.Scope{Type: resource.ScopeApplication, ID: ctx.App.Key}, nil
	default:
		return resource.Scope{}, fmt.Errorf("planning: descriptor %s has no scope prefix (modules./shared./applications./environments./connections.)", d)
	}
}

// ParseDescriptorText reads a Resource Reference descriptor written as
// `TYPE[.CLASS][#ID]`. A missing or `@` class or ID is inherited from the node
// that owns the reference. Inside the ID, the tokens `@app`, `@env`, `@connection` and
// `@infra` expand to the current Application, Environment and Connection
// key and the AWS infrastructure path, which keeps a Definition reusable across Applications.
func ParseDescriptorText(raw string, current *Node, ctx Context) (resource.Descriptor, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.NewReplacer(
		"@app", ctx.App.Key,
		"@env", ctx.Env.Key,
		"@connection", ctx.Connection.Key,
		"@infra", ctx.InfraPath(),
	).Replace(raw)
	if raw == "" {
		return resource.Descriptor{}, fmt.Errorf("planning: empty resource reference")
	}
	if strings.ContainsAny(raw, "<>") {
		return resource.Descriptor{}, fmt.Errorf("planning: resource reference %q uses an unsupported selector", raw)
	}
	head, id := raw, ""
	if idx := strings.Index(raw, "#"); idx >= 0 {
		head, id = raw[:idx], raw[idx+1:]
	}
	typ, class := head, ""
	if idx := strings.Index(head, "."); idx >= 0 {
		typ, class = head[:idx], head[idx+1:]
	}
	if typ == "" {
		return resource.Descriptor{}, fmt.Errorf("planning: resource reference %q has no type", raw)
	}
	if class == "" || class == "@" {
		class = ClassDefault
		if current != nil {
			class = current.Class
		}
	}
	if id == "" || id == "@" {
		if current == nil {
			return resource.Descriptor{}, fmt.Errorf("planning: resource reference %q has no resource id", raw)
		}
		id = descriptorID(current.Descriptor)
	}
	return resource.NewDescriptor(typ, class, id)
}
