// Package target is the single Environment execution-target resolver
// (ADR-011). Every product path that needs the Connection of an Environment
// resolves it here; the Application and the Organization default are never a
// fallback.
package target

import (
	"context"
	"errors"
	"fmt"

	appdomain "orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/planning"
	"orchestrator/internal/ports/persistence"
)

// ErrConnectionNotReady reports a selected Connection that is not READY.
var ErrConnectionNotReady = errors.New("deployment: connection is not READY")

// ErrInconsistent reports a stored binding that contradicts its Connection or
// its own invariants (for example a migrated row whose profile does not match
// the Connection kind). The Environment is never executed on a guess.
var ErrInconsistent = errors.New("target: environment execution target is inconsistent")

// ErrUnconfigured is the typed UNCONFIGURED failure of preview and deploy.
var ErrUnconfigured = planning.ErrEnvironmentUnconfigured

// Resolve returns the READY Connection selected by the Environment. It fails
// with ErrUnconfigured before any other read when the target is unset.
func Resolve(ctx context.Context, view persistence.ApplicationRepository, organizationKey string, env environment.Environment) (appdomain.Connection, error) {
	if !env.Configured() {
		return appdomain.Connection{}, fmt.Errorf("%w: environment %s/%s", ErrUnconfigured, env.ApplicationKey, env.Key)
	}
	// ADR-017: an Environment no longer carries a deployment Connection or execution profile; such a record is a retired baseline.
	return appdomain.Connection{}, fmt.Errorf("%w: environment %s/%s carries a retired deployment connection", ErrInconsistent, env.ApplicationKey, env.Key)
}

// ResolveOptional is Resolve for planning that supports Definition-placed
// Environments (ADR-016): an Environment without a deployment Connection has
// no Environment account and yields the zero Connection. A configured
// Environment (legacy, until V03.10) resolves exactly as Resolve.
func ResolveOptional(ctx context.Context, view persistence.ApplicationRepository, organizationKey string, env environment.Environment) (appdomain.Connection, error) {
	if !env.Configured() {
		return appdomain.Connection{}, nil
	}
	return Resolve(ctx, view, organizationKey, env)
}

// regionConstrained reports whether the Connection restricts its Environments
// to regions: a present allowedRegions field always does (a malformed one then
// allows none), a historical record by its single region, and only a record
// with neither is unconstrained.
func regionConstrained(conn appdomain.Connection) bool {
	_, listed := conn.Config["allowedRegions"]
	return listed || conn.ConfigString("region") != ""
}

// Accounts returns the Organization's Driver Accounts by key from the same
// snapshot view the catalog was read from, so planning sees one consistent
// account set (UC-03 V02-09). Configuration and credentials stay in the
// records; planning only reads identity, kind, status and version.
func Accounts(ctx context.Context, view persistence.ApplicationRepository, organizationKey string) (map[string]appdomain.Connection, error) {
	list, err := view.ListConnections(ctx, organizationKey)
	if err != nil {
		return nil, err
	}
	out := make(map[string]appdomain.Connection, len(list))
	for _, account := range list {
		out[account.Key] = account
	}
	return out, nil
}
