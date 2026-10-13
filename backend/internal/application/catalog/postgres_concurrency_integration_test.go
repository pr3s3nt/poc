package catalog_test

import (
	"context"
	"errors"
	"orchestrator/internal/adapters/postgres/postgrestest"
	"reflect"
	"sync"
	"testing"
	"time"

	"orchestrator/internal/adapters/credentialmemory"
	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/adapters/terraform"
	"orchestrator/internal/application/catalog"
	"orchestrator/internal/application/connection"
	ct "orchestrator/internal/application/connection/connectiontest"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

// barrierStore holds every Create* call until both racers reach it, so both
// registration services have already passed their list prechecks and only
// the insert-only repository can decide the winner.
type barrierStore struct {
	persistence.Store
	mu      sync.Mutex
	arrived int
	release chan struct{}
}

func newBarrier(st persistence.Store) *barrierStore {
	return &barrierStore{Store: st, release: make(chan struct{})}
}

var errBarrierTimeout = errors.New("test barrier: the second racer never reached Create")

// wait blocks until both racers arrived. A timeout is an error that stops the
// insert, so the test cannot pass without both prechecks having run.
func (b *barrierStore) wait() error {
	b.mu.Lock()
	b.arrived++
	if b.arrived == 2 {
		close(b.release)
	}
	b.mu.Unlock()
	select {
	case <-b.release:
		return nil
	case <-time.After(10 * time.Second):
		return errBarrierTimeout
	}
}

func (b *barrierStore) arrivals() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.arrived
}

func (b *barrierStore) CreateResourceType(ctx context.Context, org string, t resource.Type) error {
	if err := b.wait(); err != nil {
		return err
	}
	return b.Store.CreateResourceType(ctx, org, t)
}
func (b *barrierStore) CreateResourceDefinition(ctx context.Context, org string, d resource.Definition) error {
	if err := b.wait(); err != nil {
		return err
	}
	return b.Store.CreateResourceDefinition(ctx, org, d)
}
func (b *barrierStore) CreateConnection(ctx context.Context, c application.Connection) error {
	if err := b.wait(); err != nil {
		return err
	}
	return b.Store.CreateConnection(ctx, c)
}

func requireBothArrived(t *testing.T, label string, b *barrierStore) {
	t.Helper()
	if n := b.arrivals(); n != 2 {
		t.Fatalf("%s: %d racer(s) reached Create after the precheck, want 2", label, n)
	}
}

func raceServices(t *testing.T, label string, calls [2]func() error, duplicate error) int {
	t.Helper()
	var errs [2]error
	var wg sync.WaitGroup
	for i := range calls {
		wg.Add(1)
		go func(i int) { defer wg.Done(); errs[i] = calls[i]() }(i)
	}
	wg.Wait()
	switch {
	case errs[0] == nil && errors.Is(errs[1], duplicate):
		return 0
	case errs[1] == nil && errors.Is(errs[0], duplicate):
		return 1
	}
	t.Fatalf("%s: want one success and one duplicate, got %v", label, errs)
	return -1
}

type readyCluster struct{}

func (readyCluster) VerifyKubeconfig(context.Context, connection.SelectedKubeconfig) (connection.KubernetesVerification, error) {
	return connection.KubernetesVerification{Endpoint: "https://cluster.example"}, nil
}

// TestPostgresRegistrationServicesRaceOnSameKey: both service prechecks pass,
// the database constraint picks one winner, and the stored record is the
// complete winner.
func TestPostgresRegistrationServicesRaceOnSameKey(t *testing.T) {
	ctx := context.Background()
	st, err := postgres.Open(ctx, postgrestest.Fresh(t).Console)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	opts := seed.Defaults()
	if err := seedtest.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	org := opts.OrganizationKey

	types := [2]resource.Type{
		{Key: "cache", Outputs: []resource.OutputField{{Name: "host", Type: "string"}}},
		{Key: "cache", Inputs: []resource.InputField{{Name: "size", Type: "number"}}, Outputs: []resource.OutputField{{Name: "host", Type: "string"}}},
	}
	typeBarrier := newBarrier(st)
	svc := catalog.NewService(typeBarrier, terraform.NewInspector())
	w := raceServices(t, "type", [2]func() error{
		func() error { _, err := svc.RegisterResourceType(ctx, org, types[0]); return err },
		func() error { _, err := svc.RegisterResourceType(ctx, org, types[1]); return err },
	}, catalog.ErrDuplicate)
	requireBothArrived(t, "type", typeBarrier)
	listed, _ := st.ListResourceTypes(ctx, org)
	var storedTypes []resource.Type
	for _, got := range listed {
		if got.Key == "cache" {
			storedTypes = append(storedTypes, got)
		}
	}
	if len(storedTypes) != 1 || !reflect.DeepEqual(normalizeType(storedTypes[0]), normalizeType(types[w])) {
		t.Fatalf("type: stored %+v, winner %+v", storedTypes, types[w])
	}

	base := seededDefinition(t, opts, "postgres-internal-statefulset")
	defs := [2]resource.Definition{base, base}
	defs[0].Key, defs[0].Criteria = "postgres-race", []resource.Criterion{{Class: "a"}}
	defs[1].Key, defs[1].Criteria = "postgres-race", []resource.Criterion{{Class: "b"}, {}}
	defs[1].DriverInputs = map[string]any{"values": map[string]any{"variables": map[string]any{"image": "postgres:17-alpine", "storage": "4Gi", "namespace": "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"}}}
	defBarrier := newBarrier(st)
	svc = catalog.NewService(defBarrier, terraform.NewInspector())
	w = raceServices(t, "definition", [2]func() error{
		func() error { _, err := svc.RegisterResourceDefinition(ctx, org, defs[0]); return err },
		func() error { _, err := svc.RegisterResourceDefinition(ctx, org, defs[1]); return err },
	}, catalog.ErrDuplicate)
	requireBothArrived(t, "definition", defBarrier)
	all, _ := st.ListResourceDefinitions(ctx, org)
	var storedDefs []resource.Definition
	for _, got := range all {
		if got.Key == "postgres-race" {
			storedDefs = append(storedDefs, got)
		}
	}
	if len(storedDefs) != 1 || !reflect.DeepEqual(normalizeDefinition(storedDefs[0]), normalizeDefinition(defs[w])) {
		t.Fatalf("definition: stored %+v, winner %+v", storedDefs, defs[w])
	}

	// Two concurrent uploads of the same display name must both succeed with
	// distinct generated keys: the insert-only unique constraint is the final
	// guard and nothing existing is overwritten.
	connBarrier := newBarrier(st)
	conns := connection.NewService(connBarrier)
	conns.SetKubeconfigRegistration(readyCluster{}, credentialmemory.New())
	document := ct.Single("race", "https://cluster.example", "race-token-0123456789")
	var wg sync.WaitGroup
	results := make([]application.Connection, 2)
	errs := make([]error, 2)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = conns.RegisterKubeconfig(ctx, org, connection.RegisterKubeconfigCommand{Name: "Race cluster", Kubeconfig: document, Context: "race"})
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("racing upload %d: %v", i, err)
		}
	}
	// The loser retries with the next free key, so it reaches Create again.
	if connBarrier.arrivals() < 2 {
		t.Fatalf("connection: %d racer(s) reached Create, want at least 2", connBarrier.arrivals())
	}
	if results[0].Key == results[1].Key {
		t.Fatalf("racing uploads share key %q", results[0].Key)
	}
	for _, created := range results {
		got, err := st.GetConnection(ctx, org, created.Key)
		if err != nil || got.Kind != application.ConnectionKubernetes || got.AuthenticationType != application.AuthKubeconfig || got.Status != application.ConnectionReady || got.SecretRef == "" {
			t.Fatalf("connection %q: stored %+v (%v)", created.Key, got, err)
		}
	}
}

// normalizeType/normalizeDefinition treat nil and empty JSON collections
// alike; every other field is compared.
func normalizeType(t resource.Type) resource.Type {
	if t.Inputs == nil {
		t.Inputs = []resource.InputField{}
	}
	if t.Outputs == nil {
		t.Outputs = []resource.OutputField{}
	}
	return t
}

func normalizeDefinition(d resource.Definition) resource.Definition {
	if d.Provision == nil {
		d.Provision = map[string]resource.ProvisionRule{}
	}
	return d
}
