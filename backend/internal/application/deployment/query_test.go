package deployment_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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
	"orchestrator/internal/seed/seedtest"
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
		{ID: "old", ReviewRequestID: "11111111-1111-4111-8111-111111111111", OrganizationKey: "org-a", ApplicationKey: "app-a", EnvironmentKey: "staging", Status: domain.StatusPlanning, StartedAt: now.Add(-time.Minute)},
		{ID: "new", ReviewRequestID: "22222222-2222-4222-8222-222222222222", OrganizationKey: "org-a", ApplicationKey: "app-a", EnvironmentKey: "staging", Status: domain.StatusFailed, StartedAt: now},
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
	correlated, err := queries.ListDeployments(ctx, appsvc.ListDeploymentsQuery{OrganizationKey: "org-a", ApplicationKey: "app-a", EnvironmentKey: "staging", ReviewRequestID: "11111111-1111-4111-8111-111111111111"})
	if err != nil || len(correlated) != 1 || correlated[0].ID != "old" {
		t.Fatalf("request correlation mixed executions: %#v %v", correlated, err)
	}
	if _, err := queries.ListDeployments(ctx, appsvc.ListDeploymentsQuery{ReviewRequestID: "not-a-uuid"}); !errors.Is(err, appsvc.ErrInvalidReviewRequestID) {
		t.Fatalf("malformed correlation accepted: %v", err)
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
	opts := seedtest.Defaults()
	if err := seedtest.Apply(ctx, st, opts); err != nil {
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

type attemptView struct {
	persistence.Store
	attempts []domain.ResourceAttempt
	err      error
}

func (a *attemptView) ListDeploymentAttempts(context.Context, string) ([]domain.ResourceAttempt, error) {
	return a.attempts, a.err
}

type attemptStore struct {
	*countingStore
	attempts []domain.ResourceAttempt
	err      error
}

func (a *attemptStore) ReadSnapshot(ctx context.Context, fn func(context.Context, persistence.Store) error) error {
	return a.countingStore.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error {
		return fn(ctx, &attemptView{Store: view, attempts: a.attempts, err: a.err})
	})
}

// The deployment view carries the safe persisted-attempt projection under
// `resourceProgress`, always as an array, scoped like the rest of the view; a
// failed read is an error, never an empty progress list.
func TestGetDeploymentCarriesSafeResourceProgress(t *testing.T) {
	ctx := context.Background()
	base, opts, id := queryFixture(t)
	scope := appsvc.GetDeploymentQuery{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, DeploymentID: id}

	view, err := appsvc.NewQueryService(base).GetDeployment(ctx, scope)
	if err != nil || view.ResourceProgress == nil || len(view.ResourceProgress) != 0 {
		t.Fatalf("no attempts must be an empty array: %#v %v", view.ResourceProgress, err)
	}

	attempts := []domain.ResourceAttempt{{Descriptor: "redis.in-cluster#cache", ScopeType: "SHARED", Number: 1, Action: "UPDATE", Ownership: "PREEXISTING", Stage: "APPLYING", Status: "RUNNING", ModuleID: "redis-k8s", ModuleRevision: "r1", StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}}
	view, err = appsvc.NewQueryService(&attemptStore{countingStore: base, attempts: attempts}).GetDeployment(ctx, scope)
	if err != nil || len(view.ResourceProgress) != 1 || view.ResourceProgress[0].Stage != "APPLYING" {
		t.Fatalf("progress = %#v %v", view.ResourceProgress, err)
	}

	// Scope still gates the whole view, including the progress.
	foreign := scope
	foreign.OrganizationKey = "another-org"
	if _, err := appsvc.NewQueryService(&attemptStore{countingStore: base, attempts: attempts}).GetDeployment(ctx, foreign); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("foreign organization = %v", err)
	}

	outage := errors.New("attempt read outage")
	if _, err := appsvc.NewQueryService(&attemptStore{countingStore: base, err: outage}).GetDeployment(ctx, scope); !errors.Is(err, outage) {
		t.Fatalf("a failed progress read must fail the view: %v", err)
	}
}

// The view projects through an allowlist: hostile persisted stage/status/error
// text never reaches the response, while known codes are preserved.
func TestGetDeploymentProjectsAttemptsThroughTheAllowlist(t *testing.T) {
	ctx := context.Background()
	base, opts, id := queryFixture(t)
	scope := appsvc.GetDeploymentQuery{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, DeploymentID: id}
	hostile := domain.ResourceAttempt{Descriptor: "redis.in-cluster#shared.cache", ScopeType: "SHARED", Number: 1, Action: "CREATE", Ownership: "PREEXISTING", Stage: "AKIAPRIVATE /var/private/state", Status: "token=PRIVATE", SafeErrorCode: "AUTH vault://PRIVATE", ModuleID: "redis-k8s", ModuleRevision: "r1"}
	known := hostile
	known.Number, known.Stage, known.Status, known.SafeErrorCode = 2, "READY", "SUCCEEDED", ""
	view, err := appsvc.NewQueryService(&attemptStore{countingStore: base, attempts: []domain.ResourceAttempt{hostile, known}}).GetDeployment(ctx, scope)
	if err != nil || len(view.ResourceProgress) != 2 {
		t.Fatalf("%v %v", view, err)
	}
	raw, _ := json.Marshal(view.ResourceProgress)
	for _, secret := range []string{"AKIA", "PRIVATE", "/var/", "vault://", "token="} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("view leaks %q: %s", secret, raw)
		}
	}
	if p := view.ResourceProgress[0]; p.Stage != "UNKNOWN" || p.Status != "UNKNOWN" || p.SafeErrorCode != "UNKNOWN_ERROR" {
		t.Fatalf("hostile row %+v", p)
	}
	if p := view.ResourceProgress[1]; p.Stage != "READY" || p.Status != "SUCCEEDED" || p.SafeErrorCode != "" {
		t.Fatalf("known row %+v", p)
	}
}
