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

// ResourceNeed is one resource of the Environment's desired state matched
// against the Definition catalog. It carries only identifiers and booleans
// derived from the configuration, never parameter values.
type ResourceNeed struct {
	Alias, Type, Class string
	// Outcome is MATCHED, NO_MATCH or AMBIGUOUS.
	Outcome       string
	Definition    string
	DriverType    string
	DriverAccount string
	// NeedsStorage and StorageClass come from the winner's declared
	// storage variables; an empty class selects the cluster default.
	NeedsStorage bool
	StorageClass string
}

// Requirements are the read-only facts resolved from an Application and
// Environment. Nil means no context was selected: nothing about matching,
// images, secrets or routes may be assumed.
type Requirements struct {
	ApplicationKey, EnvironmentKey, EnvironmentType, Namespace string
	// Resolved is true once the Environment's current Deployment Set was
	// read; until then nothing about resources, routes or workloads is known.
	Resolved                bool
	Workloads, PublicRoutes int
	SecretStoreKey          string
	SecretStoreReady        bool
	// SecretDelivery is the platform's configured workload secret delivery
	// ("vso", "agent" or empty).
	SecretDelivery string
	Resources      []ResourceNeed
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
