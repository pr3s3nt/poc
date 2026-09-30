package postgres

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/persistence/persistencetest"
)

func TestPostgresRegistrationContractSurvivesReopen(t *testing.T) {
	first, url := openFresh(t)
	fixture := persistencetest.Registration(t, first)
	first.Close()
	reopened, err := Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	persistencetest.AssertRegistration(t, reopened, fixture)
}

// TestPostgresCreateDefinitionRollsBackOnCriterionFailure injects a fault in
// the second criterion INSERT (test-only trigger in this fresh database) and
// proves the Definition row and its first criterion are rolled back too.
func TestPostgresCreateDefinitionRollsBackOnCriterionFailure(t *testing.T) {
	ctx := context.Background()
	st, _ := openFresh(t)
	if err := st.SaveOrganization(ctx, application.Organization{Key: "atomic", Name: "atomic"}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateResourceType(ctx, "atomic", resource.Type{Key: "cache", Outputs: []resource.OutputField{{Name: "host", Type: "string"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.pool.Exec(ctx, `CREATE FUNCTION test_fail_criterion() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN IF NEW.class = 'boom' THEN RAISE EXCEPTION 'injected criterion failure'; END IF; RETURN NEW; END $$;
CREATE TRIGGER test_fail_criterion BEFORE INSERT ON matching_criteria FOR EACH ROW EXECUTE FUNCTION test_fail_criterion();`); err != nil {
		t.Fatal(err)
	}
	def := resource.Definition{Key: "cache-atomic", ResourceTypeKey: "cache", DriverType: resource.DriverKubernetes,
		DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{}}},
		Criteria:     []resource.Criterion{{Class: "ok"}, {Class: "boom"}}}
	if err := st.CreateResourceDefinition(ctx, "atomic", def); err == nil {
		t.Fatal("definition created despite the injected criterion failure")
	}
	var definitions, criteria int
	if err := st.pool.QueryRow(ctx, `SELECT count(*) FROM resource_definitions WHERE definition_key=$1`, def.Key).Scan(&definitions); err != nil {
		t.Fatal(err)
	}
	if err := st.pool.QueryRow(ctx, `SELECT count(*) FROM matching_criteria`).Scan(&criteria); err != nil {
		t.Fatal(err)
	}
	if definitions != 0 || criteria != 0 {
		t.Fatalf("partial create persisted: %d definitions, %d criteria", definitions, criteria)
	}
	// The key is still free: a valid registration succeeds afterwards.
	def.Criteria = []resource.Criterion{{Class: "ok"}}
	if err := st.CreateResourceDefinition(ctx, "atomic", def); err != nil {
		t.Fatalf("retry after rollback: %v", err)
	}
}

// race runs two inserts at once and returns the index of the only success;
// the other must be ErrDuplicate.
func race(t *testing.T, label string, inserts [2]func() error) int {
	t.Helper()
	var errs [2]error
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range inserts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = inserts[i]()
		}(i)
	}
	close(start)
	wg.Wait()
	switch {
	case errs[0] == nil && errors.Is(errs[1], persistence.ErrDuplicate):
		return 0
	case errs[1] == nil && errors.Is(errs[0], persistence.ErrDuplicate):
		return 1
	}
	t.Fatalf("%s: want one success and one ErrDuplicate, got %v", label, errs)
	return -1
}

// TestPostgresConcurrentRegistrationHasOneWinner races two inserts of the
// same key for each registration record; the stored record is the complete
// winner (criteria included) and exists exactly once.
func TestPostgresConcurrentRegistrationHasOneWinner(t *testing.T) {
	ctx := context.Background()
	st, _ := openFresh(t)
	if err := st.SaveOrganization(ctx, application.Organization{Key: "race", Name: "race"}); err != nil {
		t.Fatal(err)
	}
	// One round: the deterministic service-level barrier test forces the
	// precheck race; this checks the repository insert guard itself.
	for round := 0; round < 1; round++ {
		suffix := fmt.Sprint(round)

		types := [2]resource.Type{
			{Key: "cache-" + suffix, Outputs: []resource.OutputField{{Name: "host", Type: "string"}}},
			{Key: "cache-" + suffix, Inputs: []resource.InputField{{Name: "size", Type: "number", Required: true}}, Outputs: []resource.OutputField{{Name: "host", Type: "string", Required: true}}},
		}
		w := race(t, "type", [2]func() error{
			func() error { return st.CreateResourceType(ctx, "race", types[0]) },
			func() error { return st.CreateResourceType(ctx, "race", types[1]) },
		})
		listed, _ := st.ListResourceTypes(ctx, "race")
		requireOne(t, "type "+types[w].Key, len(filterTypes(listed, types[w].Key)))
		storedType, wantType := filterTypes(listed, types[w].Key)[0], types[w]
		if storedType.Inputs == nil {
			storedType.Inputs = []resource.InputField{}
		}
		if wantType.Inputs == nil {
			wantType.Inputs = []resource.InputField{}
		}
		if !reflect.DeepEqual(storedType, wantType) {
			t.Fatalf("round %d type: stored %+v, winner %+v", round, storedType, wantType)
		}

		defs := [2]resource.Definition{
			{Key: "cache-def-" + suffix, ResourceTypeKey: types[w].Key, DriverType: resource.DriverKubernetes, DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"who": "a"}}}, Criteria: []resource.Criterion{{Class: "a"}}},
			{Key: "cache-def-" + suffix, ResourceTypeKey: types[w].Key, DriverType: resource.DriverKubernetes, DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"who": "b"}}}, Criteria: []resource.Criterion{{Class: "b"}, {}}},
		}
		w = race(t, "definition", [2]func() error{
			func() error { return st.CreateResourceDefinition(ctx, "race", defs[0]) },
			func() error { return st.CreateResourceDefinition(ctx, "race", defs[1]) },
		})
		all, _ := st.ListResourceDefinitions(ctx, "race")
		found := 0
		for _, got := range all {
			if got.Key == defs[w].Key {
				found++
				if got.ResourceTypeKey != defs[w].ResourceTypeKey || got.DriverType != defs[w].DriverType || got.ExecutionProfile != defs[w].ExecutionProfile || got.ConnectionKey != defs[w].ConnectionKey ||
					!reflect.DeepEqual(got.Criteria, defs[w].Criteria) || !reflect.DeepEqual(got.DriverInputs, defs[w].DriverInputs) {
					t.Fatalf("round %d definition: stored %+v, winner %+v", round, got, defs[w])
				}
			}
		}
		requireOne(t, "definition "+defs[w].Key, found)

		conns := [2]application.Connection{
			{Key: "cluster-" + suffix, OrganizationKey: "race", Kind: application.ConnectionKubernetes, Status: application.ConnectionReady, Config: map[string]any{"cluster": "a"}, SecretRef: "host-kube-context://a", Verification: map[string]any{"endpoint": "https://a"}},
			{Key: "cluster-" + suffix, OrganizationKey: "race", Kind: application.ConnectionKubernetes, Status: application.ConnectionReady, Config: map[string]any{"cluster": "b"}, SecretRef: "host-kube-context://b", Verification: map[string]any{"endpoint": "https://b"}},
		}
		w = race(t, "connection", [2]func() error{
			func() error { return st.CreateConnection(ctx, conns[0]) },
			func() error { return st.CreateConnection(ctx, conns[1]) },
		})
		got, err := st.GetConnection(ctx, "race", conns[w].Key)
		if err != nil || !reflect.DeepEqual(got.Config, conns[w].Config) || got.SecretRef != conns[w].SecretRef || !reflect.DeepEqual(got.Verification, conns[w].Verification) {
			t.Fatalf("round %d connection: stored %+v (%v), winner %+v", round, got, err, conns[w])
		}
	}
}

func filterTypes(types []resource.Type, key string) []resource.Type {
	var out []resource.Type
	for _, typ := range types {
		if typ.Key == key {
			out = append(out, typ)
		}
	}
	return out
}

func requireOne(t *testing.T, what string, n int) {
	t.Helper()
	if n != 1 {
		t.Fatalf("%s stored %d times, want exactly once", what, n)
	}
}
