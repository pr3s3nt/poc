package provisioning_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"orchestrator/internal/adapters/fake"
	"orchestrator/internal/adapters/secrets"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/clock"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/provisioning"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/seed"
)

func TestEnsureBuiltinCluster_IdempotentAndFailsClosedOnCollision(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	opts := seed.Defaults()
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := provisioning.EnsureBuiltinCluster(ctx, st, opts.OrganizationKey); err != nil {
			t.Fatalf("admission %d: %v", i, err)
		}
	}
	defs, _ := st.ListResourceDefinitions(ctx, opts.OrganizationKey)
	count := 0
	for _, d := range defs {
		if d.Key == planning.BuiltinClusterKey {
			count++
			if !planning.IsBuiltinClusterDefinition(d) {
				t.Fatalf("stored record differs: %+v", d)
			}
		}
	}
	if count != 1 {
		t.Fatalf("stored %d records", count)
	}

	// A foreign record under the reserved key is never overwritten.
	foreign := planning.BuiltinClusterDefinition()
	foreign.Criteria = []resource.Criterion{{ApplicationID: "mine"}}
	other := store.New()
	if err := seed.Apply(ctx, other, opts); err != nil {
		t.Fatal(err)
	}
	if err := other.SaveResourceDefinition(ctx, opts.OrganizationKey, foreign); err != nil {
		t.Fatal(err)
	}
	if err := provisioning.EnsureBuiltinCluster(ctx, other, opts.OrganizationKey); !errors.Is(err, provisioning.ErrBuiltinCollision) {
		t.Fatalf("collision = %v", err)
	}
	defs, _ = other.ListResourceDefinitions(ctx, opts.OrganizationKey)
	for _, d := range defs {
		if d.Key == planning.BuiltinClusterKey && len(d.Criteria) != 1 || d.Key == planning.BuiltinClusterKey && d.Criteria[0].ApplicationID != "mine" {
			t.Fatalf("foreign record overwritten: %+v", d)
		}
	}
}

func builtinRequest(opts seed.Options, connectionKey string, match func(*planning.Match)) provisioning.Request {
	descriptor := "k8s-cluster.internal#connections." + connectionKey
	env := environment.Environment{Key: "staging", ApplicationKey: "a", Profile: application.ProfileInternalK8s, ConnectionKey: connectionKey}
	m := planning.Match{Descriptor: descriptor, DefinitionKey: planning.BuiltinClusterKey, DriverType: resource.DriverExistingCluster, ConnectionKey: connectionKey, Binding: planning.BindingEnvironmentConnection}
	if match != nil {
		match(&m)
	}
	return provisioning.Request{
		DeploymentID: "dep-1", RunID: "run-1",
		Context: planning.Context{OrganizationKey: opts.OrganizationKey, App: application.Application{Key: "a", OrganizationKey: opts.OrganizationKey}, Env: env},
		Plan: &planning.Plan{
			Graph:   planning.Graph{Nodes: []planning.Node{{Descriptor: descriptor, Kind: planning.NodeResource, ResourceType: "k8s-cluster", Class: "internal", Scope: resource.Scope{Type: resource.ScopeApplication, ID: "a"}}}},
			Matches: map[string]planning.Match{descriptor: m},
			Batches: [][]string{{descriptor}},
		},
		Types: map[string]resource.Type{"k8s-cluster": {Key: "k8s-cluster"}},
	}
}

// A stale snapshot cannot bypass the execution-time Connection recheck.
func TestProvision_BuiltinClusterRechecksStoredConnection(t *testing.T) {
	ctx := context.Background()
	opts := seed.Defaults()
	cases := map[string]func(*application.Connection){
		"wrong kind":    func(c *application.Connection) { c.Kind = application.ConnectionAWS },
		"not ready":     func(c *application.Connection) { c.Status = application.ConnectionVerifying },
		"other org":     func(c *application.Connection) { c.OrganizationKey = "globex" },
		"missing":       nil,
		"ready kubectl": func(c *application.Connection) {},
	}
	for name, mutate := range cases {
		st := store.New()
		if err := seed.Apply(ctx, st, opts); err != nil {
			t.Fatal(err)
		}
		conn := application.Connection{ID: "lab", Key: "lab", OrganizationKey: opts.OrganizationKey, Kind: application.ConnectionKubernetes, Status: application.ConnectionReady, AuthenticationType: application.AuthHostContext}
		if mutate != nil {
			mutate(&conn)
			if err := st.SaveConnection(ctx, conn); err != nil {
				t.Fatal(err)
			}
		}
		registry := &countingRegistry{}
		service := provisioning.NewService(st, registry, secrets.NewMemory(), clock.System{})
		_, err := service.Provision(ctx, builtinRequest(opts, "lab", nil))
		if name == "ready kubectl" {
			if err == nil || strings.Contains(err.Error(), "unavailable") {
				t.Fatalf("%s: connection must be accepted before executor lookup: %v", name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), "unavailable") {
			t.Fatalf("%s: err = %v", name, err)
		}
		active, _ := st.ListActiveResources(ctx, opts.OrganizationKey)
		for _, a := range active {
			if a.DefinitionKey == planning.BuiltinClusterKey {
				t.Fatalf("%s: active resource written", name)
			}
		}
	}
}

func TestProvision_BuiltinClusterRejectsForgedMatch(t *testing.T) {
	ctx := context.Background()
	opts := seed.Defaults()
	forgeries := map[string]func(*planning.Match){
		"other connection": func(m *planning.Match) { m.ConnectionKey = "other" },
		"no binding":       func(m *planning.Match) { m.Binding = "" },
		"wrong driver":     func(m *planning.Match) { m.DriverType = resource.DriverKubernetes },
	}
	for name, forge := range forgeries {
		st := store.New()
		if err := seed.Apply(ctx, st, opts); err != nil {
			t.Fatal(err)
		}
		service := provisioning.NewService(st, &countingRegistry{}, secrets.NewMemory(), clock.System{})
		if _, err := service.Provision(ctx, builtinRequest(opts, "lab", forge)); err == nil {
			t.Fatalf("%s: forged match executed", name)
		}
		defs, _ := st.ListResourceDefinitions(ctx, opts.OrganizationKey)
		for _, d := range defs {
			if d.Key == planning.BuiltinClusterKey {
				t.Fatalf("%s: system definition admitted for a forged match", name)
			}
		}
	}
}

func TestEnsureBuiltinCluster_ConcurrentAdmissionStoresOneRecord(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	opts := seed.Defaults()
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() { errs <- provisioning.EnsureBuiltinCluster(ctx, st, opts.OrganizationKey) }()
	}
	for i := 0; i < 8; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	defs, _ := st.ListResourceDefinitions(ctx, opts.OrganizationKey)
	count := 0
	for _, d := range defs {
		if d.Key == planning.BuiltinClusterKey {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("stored %d records", count)
	}
}

// fkStore emulates the PostgreSQL definition foreign keys locally: progress and
// Active Resource rows are rejected unless their Definition already exists.
type fkStore struct {
	persistence.Store
	rejected int
}

func (f *fkStore) require(ctx context.Context, org, key string) error {
	defs, err := f.Store.ListResourceDefinitions(ctx, org)
	if err != nil {
		return err
	}
	for _, d := range defs {
		if d.Key == key {
			return nil
		}
	}
	f.rejected++
	return errors.New("foreign key violation: definition " + key)
}

func (f *fkStore) SaveDeploymentResource(ctx context.Context, r deployment.Resource) error {
	if err := f.require(ctx, "acme", r.DefinitionKey); err != nil {
		return err
	}
	return f.Store.SaveDeploymentResource(ctx, r)
}

func (f *fkStore) UpsertActiveResource(ctx context.Context, a resource.ActiveResource) (resource.ActiveResource, error) {
	if err := f.require(ctx, a.OrganizationKey, a.DefinitionKey); err != nil {
		return resource.ActiveResource{}, err
	}
	return f.Store.UpsertActiveResource(ctx, a)
}

type fakeRegistry struct{ exec execution.ResourceExecutor }

func (r fakeRegistry) Resolve(resource.DriverType) (execution.ResourceExecutor, error) {
	return r.exec, nil
}

// Definition admission precedes every progress and Active Resource write, so
// the persisted foreign keys hold without any PostgreSQL mutation here.
func TestProvision_BuiltinAdmissionPrecedesFKDependentWrites(t *testing.T) {
	ctx := context.Background()
	opts := seed.Defaults()
	opts.OrganizationKey = "acme"
	inner := store.New()
	if err := seed.Apply(ctx, inner, opts); err != nil {
		t.Fatal(err)
	}
	if err := inner.SaveConnection(ctx, application.Connection{ID: "lab", Key: "lab", OrganizationKey: "acme", Kind: application.ConnectionKubernetes, Status: application.ConnectionReady, AuthenticationType: application.AuthHostContext, Config: map[string]any{"cluster": "c", "kubeContext": "k"}}); err != nil {
		t.Fatal(err)
	}
	types := map[string]resource.Type{}
	for _, typ := range seed.ResourceTypes() {
		types[typ.Key] = typ
	}
	fk := &fkStore{Store: inner}
	service := provisioning.NewService(fk, fakeRegistry{fake.NewResourceExecutor()}, secrets.NewMemory(), clock.System{})
	req := builtinRequest(opts, "lab", nil)
	req.Types = types
	if _, err := service.Provision(ctx, req); err != nil {
		t.Fatalf("provision: %v", err)
	}
	if fk.rejected != 0 {
		t.Fatalf("%d writes preceded admission", fk.rejected)
	}
	active, _ := inner.ListActiveResources(ctx, "acme")
	if len(active) != 1 || active[0].DefinitionKey != planning.BuiltinClusterKey {
		t.Fatalf("active = %+v", active)
	}
}

func TestEnsureBuiltinCluster_ConcurrentCollisionPreservesContent(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	opts := seed.Defaults()
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	foreign := planning.BuiltinClusterDefinition()
	foreign.Criteria = []resource.Criterion{{ApplicationID: "mine"}}
	if err := st.SaveResourceDefinition(ctx, opts.OrganizationKey, foreign); err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() { errs <- provisioning.EnsureBuiltinCluster(ctx, st, opts.OrganizationKey) }()
	}
	for i := 0; i < 8; i++ {
		if err := <-errs; !errors.Is(err, provisioning.ErrBuiltinCollision) {
			t.Fatalf("collision = %v", err)
		}
	}
	defs, _ := st.ListResourceDefinitions(ctx, opts.OrganizationKey)
	for _, d := range defs {
		if d.Key == planning.BuiltinClusterKey && (len(d.Criteria) != 1 || d.Criteria[0].ApplicationID != "mine") {
			t.Fatalf("foreign content changed: %+v", d)
		}
	}
}
