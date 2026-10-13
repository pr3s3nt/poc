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
	if !env.Profile.Valid() || !env.InfrastructureScopeValid() ||
		(env.RuntimeStatus != appdomain.RuntimePending && env.RuntimeStatus != appdomain.RuntimeReady) ||
		(env.Profile == appdomain.ProfileAWSEKS && env.Region == "") {
		return appdomain.Connection{}, fmt.Errorf("%w: environment %s/%s", ErrInconsistent, env.ApplicationKey, env.Key)
	}
	conn, err := view.GetConnection(ctx, organizationKey, env.ConnectionKey)
	if err != nil {
		return appdomain.Connection{}, err
	}
	// The profile must match the Connection kind and, for AWS, the region pinned
	// when the binding was set, so a mixed kind never reaches another account.
	switch env.Profile {
	case appdomain.ProfileInternalK8s:
		if conn.Kind != appdomain.ConnectionKubernetes {
			return appdomain.Connection{}, fmt.Errorf("%w: environment %s/%s", ErrInconsistent, env.ApplicationKey, env.Key)
		}
	case appdomain.ProfileAWSEKS:
		if conn.Kind != appdomain.ConnectionAWS || (regionConstrained(conn) && !conn.AllowsRegion(env.Region)) {
			return appdomain.Connection{}, fmt.Errorf("%w: environment %s/%s", ErrInconsistent, env.ApplicationKey, env.Key)
		}
	}
	if conn.Status != appdomain.ConnectionReady {
		return appdomain.Connection{}, fmt.Errorf("%w: connection %q is %s, want READY", ErrConnectionNotReady, conn.Key, conn.Status)
	}
	return conn, nil
}

// regionConstrained reports whether the Connection restricts its Environments
// to regions: a present allowedRegions field always does (a malformed one then
// allows none), a historical record by its single region, and only a record
// with neither is unconstrained.
func regionConstrained(conn appdomain.Connection) bool {
	_, listed := conn.Config["allowedRegions"]
	return listed || conn.ConfigString("region") != ""
}
