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
