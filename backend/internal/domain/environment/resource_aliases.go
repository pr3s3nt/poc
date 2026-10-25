package environment

// WorkloadResourceAliases preserves Score names erased by shared-ID planning.
// It is private display metadata, never authority to reveal literal values.
type WorkloadResourceAliases struct {
	Shared     map[string]string `json:"shared"`
	References map[string]string `json:"references,omitempty"`
}

// ResourceAliases is scoped by workload, because two workloads can use
// different names for the same shared resource.
type ResourceAliases map[string]WorkloadResourceAliases
