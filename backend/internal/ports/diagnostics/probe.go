package diagnostics

import (
	"context"

	"orchestrator/internal/domain/application"
	domain "orchestrator/internal/domain/diagnostics"
)

// Check is one planned probe. Providers plan checks in dependency order; the
// service owns identity, dependencies and affected use cases, so a probe can
// only choose a verdict, a fixed reason and registered facts.
type Check struct {
	ID    string
	Group string
	// DependsOn names checks that must not have FAILed or been BLOCKED.
	DependsOn []string
	// AffectedUseCases are the selected use cases this check supports.
	AffectedUseCases []string
	// Execution marks a check that may create run-owned objects.
	Execution bool
	// Always runs after the other checks even when the run is cancelled or an
	// earlier check failed (run-owned cleanup).
	Always bool
}

// ProviderTarget is one resolved provider target of a planned resource: the
// safe identity the planner pinned (no credential, endpoint or state).
type ProviderTarget struct {
	Kind, Connection, Region string
}

// StorageNeed says whether a planned resource needs persistent storage and
// which StorageClass it selects. It is derived from the matched Definition and
// the verified module contract; an input that is a reference or an expression
// is Unresolved and is never converted to the default class.
type StorageNeed struct {
	Required bool
	// Class is the explicit StorageClass; empty with Required means the cluster
	// default class. Meaningful only when Resolved.
	Class    string
	Resolved bool
}

// ResourceNeed is one planned resource, derived from the actual desired state
// (drafts plus the applied set) through the planning pipeline.
type ResourceNeed struct {
	Descriptor, Type, Class string
	Definition              string
	DriverType              string
	// Module is the verified module name; Packaged says it ships with the
	// platform (its capability groups are therefore known).
	Module   string
	Packaged bool
	// Providers are the resolved provider targets; Cluster and Namespace come
	// from the placement of the resource.
	Providers []ProviderTarget
	Cluster   string
	Namespace string
	Storage   StorageNeed
}

// Requirements are the read-only facts resolved from an Application and
// Environment. Nil means no context was selected: nothing about matching,
// images, secrets or routes may be assumed.
type Requirements struct {
	ApplicationKey, EnvironmentKey, EnvironmentType, Namespace string
	// Resolved is true when the planning pipeline produced a plan from the
	// desired drafts and the applied configuration on one snapshot.
	Resolved bool
	// PlanStage and PlanReason (planner enums) are set when planning rejected
	// the desired state; PlanUnavailable when it could not run at all.
	PlanStage, PlanReason string
	PlanUnavailable       bool
	// Workloads is the number of workloads in the desired final state and
	// PendingChanges the number of workload changes that deploy would apply.
	Workloads, PendingChanges int
	PublicRoutes              int
	PublicHost                string
	SecretStoreKey            string
	// SecretDelivery is the platform's workload secret delivery mode.
	SecretDelivery string
	Resources      []ResourceNeed
	// Placements are the distinct workload/resource cluster placements.
	Placements []PlacementNeed
}

// PlacementNeed is a planned cluster placement.
type PlacementNeed struct{ Cluster, Account, Namespace, Provider, Region string }

// StorageClassesNeeded returns the distinct explicit classes the selected
// capability resources require, whether the default class is needed and
// whether some requirement could not be resolved.
func (r *Requirements) StorageClassesNeeded(resources []ResourceNeed) (classes []string, needDefault, unresolved bool) {
	seen := map[string]bool{}
	for _, res := range resources {
		if !res.Storage.Required {
			continue
		}
		switch {
		case !res.Storage.Resolved:
			unresolved = true
		case res.Storage.Class == "":
			needDefault = true
		case !seen[res.Storage.Class]:
			seen[res.Storage.Class] = true
			classes = append(classes, res.Storage.Class)
		}
	}
	return classes, needDefault, unresolved
}

// StoreInspector reads real, safe evidence about a scoped workload secret
// store (TLS trust and expiry, availability, token acceptance). It never
// reads a workload secret value.
type StoreInspector interface {
	Inspect(ctx context.Context, org, storeKey string) domain.Finding
}

// RouteInspector reads real TLS evidence of a public application host.
type RouteInspector interface {
	Inspect(ctx context.Context, host string) domain.Finding
}

// Recorder persists run-owned resources the moment a probe intends or
// observes them, before the next remote call.
type Recorder interface {
	Record(ctx context.Context, resource domain.ProbeResource) error
	// RunID is the report ID that owns every object the run creates.
	RunID() string
}

// Session is one provider run bound to the saved account credential. It is
// created per run and closed afterwards.
type Session interface {
	// Run executes one check and returns its verdict. It must not return raw
	// provider text: only a fixed reason code and registered facts.
	Run(ctx context.Context, check Check, prior map[string]domain.Finding, req *Requirements, rec Recorder) domain.Finding
	// Cleanup deletes the given run-owned resources after validating
	// ownership and UID, and returns every resource with its observed state.
	Cleanup(ctx context.Context, resources []domain.ProbeResource, runID string) []domain.ProbeResource
	Close()
}

// Provider plans and opens diagnostics for one account type.
type Provider interface {
	// Plan lists the provider's checks for scope in execution order.
	Plan(scope domain.Scope, req *Requirements) []Check
	// Open resolves the saved scoped credential. It never falls back to a host
	// credential. A resolution problem is reported by the session's first
	// check, not by a nil session.
	Open(ctx context.Context, account application.Connection, scope domain.Scope) Session
}
