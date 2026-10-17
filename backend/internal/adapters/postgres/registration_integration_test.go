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
	st, url := openFresh(t)
	if err := st.SaveOrganization(ctx, application.Organization{Key: "atomic", Name: "atomic"}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateResourceType(ctx, "atomic", resource.Type{Key: "cache", Outputs: []resource.OutputField{{Name: "host", Type: "string"}}}); err != nil {
		t.Fatal(err)
	}
	// Test-only fault injection runs as the administrator: no runtime login has DDL.
	adminExec(t, url, `CREATE FUNCTION test_fail_criterion() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN IF NEW.class = 'boom' THEN RAISE EXCEPTION 'injected criterion failure'; END IF; RETURN NEW; END $$;
CREATE TRIGGER test_fail_criterion BEFORE INSERT ON matching_criteria FOR EACH ROW EXECUTE FUNCTION test_fail_criterion();`)
	def := resource.Definition{Key: "cache-atomic", Name: "cache-atomic", Version: 1, ResourceTypeKey: "cache", DriverType: resource.DriverKubernetes,
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
			{Key: "cache-def-" + suffix, Name: "A", Version: 1, ResourceTypeKey: types[w].Key, DriverType: resource.DriverKubernetes, DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"who": "a"}}}, Criteria: []resource.Criterion{{Class: "a"}}},
			{Key: "cache-def-" + suffix, Name: "B", Version: 1, ResourceTypeKey: types[w].Key, DriverType: resource.DriverKubernetes, DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"who": "b"}}}, Criteria: []resource.Criterion{{Class: "b"}, {}}},
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
				if got.ResourceTypeKey != defs[w].ResourceTypeKey || got.DriverType != defs[w].DriverType || got.Name != defs[w].Name || got.Version != defs[w].Version || got.DriverAccount != defs[w].DriverAccount ||
					!reflect.DeepEqual(got.Criteria, defs[w].Criteria) || !reflect.DeepEqual(got.DriverInputs, defs[w].DriverInputs) {
					t.Fatalf("round %d definition: stored %+v, winner %+v", round, got, defs[w])
				}
			}
		}
		requireOne(t, "definition "+defs[w].Key, found)

		conns := [2]application.Connection{
			{Key: "cluster-" + suffix, Name: "cluster-" + suffix, OrganizationKey: "race", Kind: application.ConnectionKubernetes, AuthenticationType: application.AuthKubeconfig, Status: application.ConnectionReady, Config: map[string]any{"cluster": "a"}, SecretRef: "fake-credential://a", Verification: map[string]any{"endpoint": "https://a"}},
			{Key: "cluster-" + suffix, Name: "cluster-" + suffix, OrganizationKey: "race", Kind: application.ConnectionKubernetes, AuthenticationType: application.AuthKubeconfig, Status: application.ConnectionReady, Config: map[string]any{"cluster": "b"}, SecretRef: "fake-credential://b", Verification: map[string]any{"endpoint": "https://b"}},
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

// UC-03 V02-06 on real SQL: the CAS update replaces the Definition and its
// criteria in one transaction. A fault in the second criterion INSERT rolls the
// UPDATE, the DELETE and the first criterion back; the version is not bumped.
func TestPostgresUpdateDefinitionRollsBackWholeEditOnCriterionFailure(t *testing.T) {
	ctx := context.Background()
	st, url := openFresh(t)
	if err := st.SaveOrganization(ctx, application.Organization{Key: "upd", Name: "upd"}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateResourceType(ctx, "upd", resource.Type{Key: "cache", Outputs: []resource.OutputField{{Name: "host", Type: "string"}}}); err != nil {
		t.Fatal(err)
	}
	original := resource.Definition{Key: "cache-upd", Name: "Gốc", Version: 1, ResourceTypeKey: "cache", DriverType: resource.DriverKubernetes,
		DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"v": "1"}}}, Criteria: []resource.Criterion{{Class: "old-a"}, {Class: "old-b"}}}
	if err := st.CreateResourceDefinition(ctx, "upd", original); err != nil {
		t.Fatal(err)
	}
	adminExec(t, url, `CREATE FUNCTION test_fail_criterion() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN IF NEW.class = 'boom' THEN RAISE EXCEPTION 'injected criterion failure'; END IF; RETURN NEW; END $$;
CREATE TRIGGER test_fail_criterion BEFORE INSERT ON matching_criteria FOR EACH ROW EXECUTE FUNCTION test_fail_criterion();`)
	next := original
	next.Name = "Mới"
	next.DriverInputs = map[string]any{"values": map[string]any{"variables": map[string]any{"v": "2"}}}
	next.Criteria = []resource.Criterion{{Class: "new-a"}, {Class: "boom"}}
	if _, err := st.UpdateResourceDefinition(ctx, "upd", 1, next); err == nil {
		t.Fatal("update committed despite the injected criterion failure")
	}
	defs, _ := st.ListResourceDefinitions(ctx, "upd")
	if len(defs) != 1 || !reflect.DeepEqual(defs[0], original) {
		t.Fatalf("a failed edit changed the stored definition: %+v", defs)
	}
	// The same edit without the fault succeeds from the untouched version.
	next.Criteria = []resource.Criterion{{Class: "new-a"}, {Class: "new-b"}}
	saved, err := st.UpdateResourceDefinition(ctx, "upd", 1, next)
	if err != nil || saved.Version != 2 {
		t.Fatalf("retry: %+v %v", saved, err)
	}
}

func TestPostgresUpdateDefinitionCompareAndSetRaceAndImmutability(t *testing.T) {
	ctx := context.Background()
	st, _ := openFresh(t)
	if err := st.SaveOrganization(ctx, application.Organization{Key: "cas", Name: "cas"}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"cache", "other"} {
		if err := st.CreateResourceType(ctx, "cas", resource.Type{Key: key, Outputs: []resource.OutputField{{Name: "host", Type: "string"}}}); err != nil {
			t.Fatal(err)
		}
	}
	def := resource.Definition{Key: "cache-cas", Name: "Gốc", Version: 1, ResourceTypeKey: "cache", DriverType: resource.DriverKubernetes,
		DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{}}}, Criteria: []resource.Criterion{{}}}
	if err := st.CreateResourceDefinition(ctx, "cas", def); err != nil {
		t.Fatal(err)
	}
	const writers = 6
	errs := make([]error, writers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			next := def
			next.Name = fmt.Sprintf("writer-%d", i)
			_, errs[i] = st.UpdateResourceDefinition(ctx, "cas", 1, next)
		}(i)
	}
	close(start)
	wg.Wait()
	wins := 0
	for _, err := range errs {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, persistence.ErrVersionConflict):
		default:
			t.Fatalf("unexpected: %v", err)
		}
	}
	defs, _ := st.ListResourceDefinitions(ctx, "cas")
	if wins != 1 || len(defs) != 1 || defs[0].Version != 2 {
		t.Fatalf("wins=%d stored=%+v", wins, defs)
	}
	// Immutable Resource Type and missing Definition.
	moved := defs[0]
	moved.ResourceTypeKey = "other"
	if _, err := st.UpdateResourceDefinition(ctx, "cas", 2, moved); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("resource type change: %v", err)
	}
	ghost := def
	ghost.Key = "ghost"
	if _, err := st.UpdateResourceDefinition(ctx, "cas", 1, ghost); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("missing definition: %v", err)
	}
	// The database refuses a blank or control-character Name and a non-positive version.
	for _, bad := range []string{"UPDATE resource_definitions SET definition_name='  '", "UPDATE resource_definitions SET definition_name=E'a\\nb'", "UPDATE resource_definitions SET version=0"} {
		if _, err := st.pool.Exec(ctx, bad); err == nil {
			t.Fatalf("SQL accepted %q", bad)
		}
	}
}
