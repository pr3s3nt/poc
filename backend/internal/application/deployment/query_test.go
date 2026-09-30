package deployment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"orchestrator/internal/adapters/store"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/domain/application"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/persistence/persistencetest"
	"orchestrator/internal/seed"
)

func TestQueryServiceScopesFiltersAndOrdersHistory(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	for _, fixture := range []struct{ org, app, env string }{
		{"org-a", "app-a", "staging"},
		{"org-b", "app-b", "production"},
	} {
		if err := st.SaveApplication(ctx, application.Application{Key: fixture.app, OrganizationKey: fixture.org}); err != nil {
			t.Fatal(err)
		}
		if err := st.SaveEnvironment(ctx, environment.Environment{Key: fixture.env, ApplicationKey: fixture.app}); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	for _, record := range []domain.Deployment{
		{ID: "old", OrganizationKey: "org-a", ApplicationKey: "app-a", EnvironmentKey: "staging", Status: domain.StatusPlanning, StartedAt: now.Add(-time.Minute)},
		{ID: "new", OrganizationKey: "org-a", ApplicationKey: "app-a", EnvironmentKey: "staging", Status: domain.StatusFailed, StartedAt: now},
		{ID: "other", OrganizationKey: "org-b", ApplicationKey: "app-b", EnvironmentKey: "production", Status: domain.StatusFailed, StartedAt: now.Add(time.Minute)},
	} {
		if err := st.SaveDeployment(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	queries := appsvc.NewQueryService(st)

	list, err := queries.ListDeployments(ctx, appsvc.ListDeploymentsQuery{OrganizationKey: "org-a", ApplicationKey: "app-a", EnvironmentKey: "staging"})
	if err != nil || len(list) != 2 || list[0].ID != "new" || list[1].ID != "old" {
		t.Fatalf("newest scoped history = %#v, %v", list, err)
	}
	failed, err := queries.ListDeployments(ctx, appsvc.ListDeploymentsQuery{OrganizationKey: "org-a", ApplicationKey: "app-a", EnvironmentKey: "staging", Status: domain.StatusFailed})
	if err != nil || len(failed) != 1 || failed[0].ID != "new" {
		t.Fatalf("failed filter = %#v, %v", failed, err)
	}
	if _, err := queries.ListDeployments(ctx, appsvc.ListDeploymentsQuery{Status: "NOT_A_STATUS"}); !errors.Is(err, appsvc.ErrInvalidStatus) {
		t.Fatalf("invalid status = %v", err)
	}

	for name, query := range map[string]appsvc.GetDeploymentQuery{
		"organization": {OrganizationKey: "org-b", ApplicationKey: "app-a", EnvironmentKey: "staging", DeploymentID: "new"},
		"application":  {OrganizationKey: "org-a", ApplicationKey: "app-b", EnvironmentKey: "production", DeploymentID: "new"},
		"environment":  {OrganizationKey: "org-a", ApplicationKey: "app-a", EnvironmentKey: "production", DeploymentID: "new"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := queries.GetDeployment(ctx, query); !errors.Is(err, persistence.ErrNotFound) {
				t.Fatalf("scope mismatch = %v", err)
			}
		})
	}
}

func TestGetDeploymentReadsItsOwnWorkloadSnapshot(t *testing.T) {
	st := store.New()
	fixture := persistencetest.WorkloadSnapshots(t, st)
	queries := appsvc.NewQueryService(st)
	opts := fixture.Options
	for id, digest := range map[string]string{fixture.FirstID: "sha256:first", fixture.SecondID: "sha256:second"} {
		view, err := queries.GetDeployment(context.Background(), appsvc.GetDeploymentQuery{
			OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey,
			EnvironmentKey: opts.EnvironmentKey, DeploymentID: id,
		})
		if err != nil || len(view.Workloads) != 1 || view.Workloads[0].ManifestDigest != digest || view.Workloads[0].LastDeploymentID != id {
			t.Fatalf("view of %s = %#v, %v", id, view, err)
		}
	}
}

// countingStore records which unit-of-work a query used and can simulate a
// storage outage on scope reads.
type countingStore struct {
	persistence.Store
	transacts, snapshots int
	outage               error
}

func (c *countingStore) Transact(ctx context.Context, fn func(context.Context) error) error {
	c.transacts++
	return c.Store.Transact(ctx, fn)
}

func (c *countingStore) ReadSnapshot(ctx context.Context, fn func(context.Context, persistence.Store) error) error {
	c.snapshots++
	return c.Store.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error {
		return fn(ctx, &outageView{Store: view, outage: c.outage})
	})
}

type outageView struct {
	persistence.Store
	outage error
}

func (o *outageView) GetApplication(ctx context.Context, key string) (application.Application, error) {
	if o.outage != nil {
		return application.Application{}, o.outage
	}
	return o.Store.GetApplication(ctx, key)
}

func queryFixture(t *testing.T) (*countingStore, seed.Options, string) {
	t.Helper()
	ctx := context.Background()
	st := store.New()
	opts := seed.Defaults()
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	record := persistencetest.SaveDeployment(t, st, opts, domain.StatusFailed, time.Now().UTC())
	return &countingStore{Store: st}, opts, record.ID
}

func TestQueriesUseOneReadSnapshotAndNeverTransact(t *testing.T) {
	st, opts, id := queryFixture(t)
	queries := appsvc.NewQueryService(st)
	ctx := context.Background()
	if _, err := queries.GetDeployment(ctx, appsvc.GetDeploymentQuery{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, DeploymentID: id}); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.ListDeployments(ctx, appsvc.ListDeploymentsQuery{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey}); err != nil {
		t.Fatal(err)
	}
	if st.snapshots != 2 || st.transacts != 0 {
		t.Fatalf("snapshots=%d transacts=%d, want 2 and 0", st.snapshots, st.transacts)
	}
}

func TestScopeReadOutageIsNotReportedAsNotFound(t *testing.T) {
	st, opts, id := queryFixture(t)
	outage := errors.New("database unavailable")
	st.outage = outage
	queries := appsvc.NewQueryService(st)
	ctx := context.Background()
	if _, err := queries.GetDeployment(ctx, appsvc.GetDeploymentQuery{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, DeploymentID: id}); !errors.Is(err, outage) || errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("detail outage = %v", err)
	}
	if _, err := queries.ListDeployments(ctx, appsvc.ListDeploymentsQuery{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey}); !errors.Is(err, outage) {
		t.Fatalf("history outage = %v", err)
	}
}

func TestOutputRedactionFailsClosed(t *testing.T) {
	ctx := context.Background()
	raw, opts, id := queryFixture(t)
	st := raw.Store
	if err := st.SaveResourceType(ctx, opts.OrganizationKey, resource.Type{Key: "classified", Outputs: []resource.OutputField{
		{Name: "host", Type: "string"},
		{Name: "password", Type: "string", Secret: true},
		{Name: "config", Type: "string"},
		{Name: "endpoints", Type: "string"},
	}}); err != nil {
		t.Fatal(err)
	}
	for _, row := range []domain.Resource{
		{DeploymentID: id, NodeDescriptor: "classified.main", ResourceTypeKey: "classified", Status: domain.ResourceStatus("SUCCEEDED"), OutputSnapshot: map[string]any{
			"host":       "db.internal",
			"password":   "hunter2",
			"undeclared": "surprise",
			"config":     map[string]any{"nested": map[string]any{"secretRef": "vault://x"}},
			"endpoints":  []any{map[string]any{"secretRef": "vault://y"}},
		}},
		{DeploymentID: id, NodeDescriptor: "unknown.main", ResourceTypeKey: "no-such-type", Status: domain.ResourceStatus("SUCCEEDED"), OutputSnapshot: map[string]any{"host": "leak.internal"}},
	} {
		if err := st.SaveDeploymentResource(ctx, row); err != nil {
			t.Fatal(err)
		}
	}
	view, err := appsvc.NewQueryService(st).GetDeployment(ctx, appsvc.GetDeploymentQuery{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, DeploymentID: id})
	if err != nil {
		t.Fatal(err)
	}
	outputs := map[string]map[string]any{}
	for _, r := range view.Resources {
		outputs[r.Descriptor] = r.Outputs
	}
	want := map[string]map[string]any{
		"classified.main": {"host": "db.internal", "password": appsvc.RedactedValue, "undeclared": appsvc.RedactedValue, "config": appsvc.RedactedValue, "endpoints": appsvc.RedactedValue},
		"unknown.main":    {"host": appsvc.RedactedValue},
	}
	for descriptor, fields := range want {
		for name, value := range fields {
			if outputs[descriptor][name] != value {
				t.Fatalf("%s.%s = %v, want %v (all: %v)", descriptor, name, outputs[descriptor][name], value, outputs)
			}
		}
	}
}
