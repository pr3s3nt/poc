package deployment

import (
	"context"
	"errors"
	"fmt"

	appdomain "orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/ports/persistence"
)

// ErrConnectionNotReady reports that the Application connection cannot be
// used for planning (UC-05 PRE-04, UC-06 PRE-04).
var ErrConnectionNotReady = errors.New("deployment: connection is not READY")

// ErrStalePlan reports that planning no longer matches the pinned preview.
var ErrStalePlan = errors.New("deployment: preview plan is stale")

// Safe failure summaries. A failed Deployment persists one of these as its
// FailureReason (shown by UC-09) instead of the raw cause, which may quote
// executor, driver, Kubernetes, store or catalog text. The typed cause is
// still returned to callers for errors.Is decisions.
const (
	FailureStale    = "the Environment changed after this change was planned; preview changes again"
	FailureNotReady = "the Application connection is not READY; ask a platform engineer to verify it"
	FailureRuntime  = "runtime deployment failed during resource provisioning, workload apply/readiness or removal; preview changes again to retry"
	FailureNotFound = "a required Application, Environment or catalog record was not found"
)

// FailureLegacy replaces a stored FailureReason that is not a known safe
// summary, such as raw text recorded before summaries existed. Records are
// not rewritten; only the UC-09 read projection changes.
const FailureLegacy = "deployment failed; its recorded diagnostic is not shown. Preview changes again to retry"

// SafeFailureReason returns reason when it is a known safe summary (or
// empty) and FailureLegacy otherwise.
func SafeFailureReason(reason string) string {
	switch reason {
	case "", FailureStale, FailureNotReady, FailureRuntime, FailureNotFound:
		return reason
	}
	if planning.IsPublicMessage(reason) {
		return reason
	}
	return FailureLegacy
}

// PublicFailure returns the safe summary for a deployment failure.
func PublicFailure(err error) string {
	switch {
	case errors.Is(err, ErrStalePlan), errors.Is(err, persistence.ErrVersionConflict):
		return FailureStale
	case errors.Is(err, ErrConnectionNotReady):
		return FailureNotReady
	case errors.Is(err, persistence.ErrNotFound):
		return FailureNotFound
	}
	if message, ok := planning.PublicMessage(err); ok {
		return message
	}
	return FailureRuntime
}

// PlanningSnapshot is the input of one planning run, read from one consistent
// point in time (UC-05 BR-01, OC-06/07). UC-05 Preview and UC-06 Deploy share
// it so both plan from identical input semantics.
type PlanningSnapshot struct {
	App         appdomain.Application
	Env         environment.Environment
	Connection  appdomain.Connection
	BaseSetID   string
	BaseSet     environment.Document
	Catalog     planning.Catalog
	Types       map[string]resource.Type
	Definitions map[string]resource.Definition
	Active      []resource.ActiveResource
}

// LoadPlanningSnapshot reads the Application, Environment, current Deployment
// Set, connection, catalog and Active Resources inside one read-only store
// snapshot. An Application outside organizationKey is reported as not found.
func LoadPlanningSnapshot(ctx context.Context, st persistence.Store, organizationKey, applicationKey, environmentKey string) (PlanningSnapshot, error) {
	var out PlanningSnapshot
	err := st.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error {
		app, err := view.GetApplication(ctx, applicationKey)
		if err != nil {
			return err
		}
		if app.OrganizationKey != organizationKey {
			return fmt.Errorf("deployment: application %q: %w", applicationKey, persistence.ErrNotFound)
		}
		env, err := view.GetEnvironment(ctx, applicationKey, environmentKey)
		if err != nil {
			return err
		}
		conn, err := view.GetConnection(ctx, app.OrganizationKey, app.ConnectionKey)
		if err != nil {
			return err
		}
		if conn.Status != appdomain.ConnectionReady {
			return fmt.Errorf("%w: connection %q is %s, want READY", ErrConnectionNotReady, conn.Key, conn.Status)
		}
		base := environment.NewDocument()
		if env.CurrentDeploymentSetID != "" {
			set, err := view.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
			if err != nil {
				return err
			}
			base = set.Document
		}
		typeList, err := view.ListResourceTypes(ctx, organizationKey)
		if err != nil {
			return err
		}
		defList, err := view.ListResourceDefinitions(ctx, organizationKey)
		if err != nil {
			return err
		}
		active, err := view.ListActiveResources(ctx, organizationKey)
		if err != nil {
			return err
		}
		types := map[string]resource.Type{}
		for _, t := range typeList {
			types[t.Key] = t
		}
		definitions := map[string]resource.Definition{}
		for _, d := range defList {
			definitions[d.Key] = d
		}
		out = PlanningSnapshot{
			App: app, Env: env, Connection: conn,
			BaseSetID: env.CurrentDeploymentSetID, BaseSet: base,
			Catalog: planning.Catalog{Types: types, Definitions: defList},
			Types:   types, Definitions: definitions, Active: active,
		}
		return nil
	})
	return out, err
}
