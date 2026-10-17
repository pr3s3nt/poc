package persistencetest

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/persistence"
)

// RegistrationFixture names what Registration created, for reopen checks.
type RegistrationFixture struct {
	Organization string
	Type         resource.Type
	Definition   resource.Definition
	Connection   application.Connection
	// Versioned is the V01 account after one compare-and-set update.
	Versioned application.Connection
	// Edited is a Definition after one V02 compare-and-set update.
	Edited resource.Definition
}

// Registration checks the insert-only registration contract (UC-02/03/04):
// a duplicate never changes the stored record (criteria included), a missing
// Organization or Resource Type fails instead of a silent no-op, and keys are
// scoped per Organization.
func Registration(t *testing.T, st persistence.Store) RegistrationFixture {
	t.Helper()
	ctx := context.Background()
	for _, org := range []string{"reg-acme", "reg-globex"} {
		if err := st.SaveOrganization(ctx, application.Organization{Key: org, Name: org}); err != nil {
			t.Fatal(err)
		}
	}
	typ := resource.Type{Key: "cache", Inputs: []resource.InputField{{Name: "size", Type: "number", Required: true}}, Outputs: []resource.OutputField{{Name: "host", Type: "string", Required: true}, {Name: "password", Type: "string", Secret: true}}}
	if err := st.CreateResourceType(ctx, "reg-acme", typ); err != nil {
		t.Fatalf("create type: %v", err)
	}
	changed := typ
	changed.Inputs = nil
	if err := st.CreateResourceType(ctx, "reg-acme", changed); !errors.Is(err, persistence.ErrDuplicate) {
		t.Fatalf("duplicate type: %v", err)
	}
	if err := st.CreateResourceType(ctx, "reg-missing", typ); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("type in missing Organization: %v", err)
	}
	if err := st.CreateResourceType(ctx, "reg-globex", changed); err != nil {
		t.Fatalf("same key in another Organization: %v", err)
	}
	assertType(t, st, "reg-acme", typ)
	assertType(t, st, "reg-globex", changed)

	def := resource.Definition{
		Key: "cache-fast", Name: "Cache nhanh", Version: 1, ResourceTypeKey: "cache", DriverType: resource.DriverKubernetes,
		DriverInputs:   map[string]any{"values": map[string]any{"variables": map[string]any{"tier": "fast"}}},
		Criteria:       []resource.Criterion{{Class: "fast"}, {EnvironmentType: "development", ApplicationID: "a", EnvironmentID: "e", ResourceID: "r"}, {}},
		Provision:      map[string]resource.ProvisionRule{"cache.replica": {IsDependent: true, Params: map[string]any{"size": float64(1)}}},
		SourceFingerpr: "sha256:registration-contract",
	}
	if err := st.CreateResourceDefinition(ctx, "reg-acme", def); err != nil {
		t.Fatalf("create definition: %v", err)
	}
	loser := def
	loser.Criteria = []resource.Criterion{{EnvironmentType: "production"}}
	loser.DriverInputs = map[string]any{"values": map[string]any{"variables": map[string]any{"tier": "slow"}}}
	if err := st.CreateResourceDefinition(ctx, "reg-acme", loser); !errors.Is(err, persistence.ErrDuplicate) {
		t.Fatalf("duplicate definition: %v", err)
	}
	missingType := def
	missingType.Key, missingType.ResourceTypeKey = "orphan", "not-registered"
	if err := st.CreateResourceDefinition(ctx, "reg-acme", missingType); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("definition with missing type: %v", err)
	}
	if err := st.CreateResourceDefinition(ctx, "reg-missing", def); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("definition in missing Organization: %v", err)
	}
	assertDefinition(t, st, "reg-acme", def)

	conn := application.Connection{Key: "reg-cluster", Name: "Registered Cluster", OrganizationKey: "reg-acme", Kind: application.ConnectionKubernetes, AuthenticationType: application.AuthKubeconfig, Status: application.ConnectionReady,
		Config: map[string]any{"cluster": "c1", "kubeContext": "ctx-1"}, SecretRef: "fake-credential://ctx-1", Verification: map[string]any{"verified": true}}
	if err := st.CreateConnection(ctx, conn); err != nil {
		t.Fatalf("create connection: %v", err)
	}
	foreign := conn
	foreign.ID, foreign.Key, foreign.OrganizationKey = "", "reg-foreign-cluster", "reg-globex"
	if err := st.CreateConnection(ctx, foreign); err != nil {
		t.Fatalf("create foreign connection: %v", err)
	}
	// An explicit connection reference must resolve in the same Organization;
	// a missing or foreign key fails and inserts neither row nor criteria.
	for name, key := range map[string]string{"missing connection": "reg-missing-cluster", "foreign connection": "reg-foreign-cluster"} {
		bad := def
		bad.Key, bad.DriverAccount = "cache-"+strings.ReplaceAll(name, " ", "-"), key
		if err := st.CreateResourceDefinition(ctx, "reg-acme", bad); !errors.Is(err, persistence.ErrNotFound) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	withConnection := def
	withConnection.Key, withConnection.DriverAccount = "cache-on-cluster", "reg-cluster"
	if err := st.CreateResourceDefinition(ctx, "reg-acme", withConnection); err != nil {
		t.Fatalf("definition with same-Organization connection: %v", err)
	}
	defs, _ := st.ListResourceDefinitions(ctx, "reg-acme")
	keys := map[string]string{}
	for _, d := range defs {
		keys[d.Key] = d.DriverAccount
	}
	if len(keys) != 2 || keys["cache-on-cluster"] != "reg-cluster" || keys["cache-fast"] != "" {
		t.Fatalf("definitions after rejected references: %v", keys)
	}

	other := conn
	other.ID, other.Config, other.SecretRef = "", map[string]any{"cluster": "c2", "kubeContext": "ctx-2"}, "fake-credential://ctx-2"
	if err := st.CreateConnection(ctx, other); !errors.Is(err, persistence.ErrDuplicate) {
		t.Fatalf("duplicate connection: %v", err)
	}
	missing := conn
	missing.ID, missing.OrganizationKey = "", "reg-missing"
	if err := st.CreateConnection(ctx, missing); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("connection in missing Organization: %v", err)
	}
	assertConnection(t, st, conn)
	stored, _ := st.GetConnection(ctx, "reg-acme", "reg-cluster")
	if stored.Name != "Registered Cluster" || stored.AuthenticationType != application.AuthKubeconfig {
		t.Fatalf("connection explicit metadata: name %q, authentication %q", stored.Name, stored.AuthenticationType)
	}
	connectionAuthentication(t, st)
	versioned := connectionVersioning(t, st)
	edited := definitionVersioning(t, st, typ)
	return RegistrationFixture{Organization: "reg-acme", Type: typ, Definition: def, Connection: conn, Versioned: versioned, Edited: edited}
}

// connectionAuthentication checks the UC-04 name/authentication type columns:
// an uploaded-kubeconfig record round-trips, and an incompatible or unknown
// authentication type is rejected without a row.
func connectionAuthentication(t *testing.T, st persistence.Store) {
	t.Helper()
	ctx := context.Background()
	uploaded := application.Connection{Key: "reg-upload", Name: "Uploaded Cluster", OrganizationKey: "reg-acme", Kind: application.ConnectionKubernetes,
		AuthenticationType: application.AuthKubeconfig, Status: application.ConnectionReady,
		Config:    map[string]any{"cluster": "c3", "kubeContext": "ctx-3", "endpoint": "https://c3.example"},
		SecretRef: "kv2://kv/orchestrator/connections/reg-acme/reg-upload/credentials/00000000-0000-4000-8000-000000000001", Verification: map[string]any{"verified": true}}
	if err := st.CreateConnection(ctx, uploaded); err != nil {
		t.Fatalf("create uploaded connection: %v", err)
	}
	got, err := st.GetConnection(ctx, "reg-acme", "reg-upload")
	if err != nil || got.Name != "Uploaded Cluster" || got.AuthenticationType != application.AuthKubeconfig || got.SecretRef != uploaded.SecretRef {
		t.Fatalf("uploaded connection = %+v, %v", got, err)
	}
	for name, bad := range map[string]application.Connection{
		"incompatible": {Key: "reg-bad-auth", Name: "Bad", OrganizationKey: "reg-acme", Kind: application.ConnectionKubernetes, AuthenticationType: application.AuthAWSAccessKey, Status: application.ConnectionReady, Config: map[string]any{}, SecretRef: "x", Verification: map[string]any{}},
		"host context": {Key: "reg-bad-host", Name: "Bad", OrganizationKey: "reg-acme", Kind: application.ConnectionKubernetes, AuthenticationType: "HOST_CONTEXT", Status: application.ConnectionReady, Config: map[string]any{}, SecretRef: "fake-credential://x", Verification: map[string]any{}},
		"unknown ref":  {Key: "reg-bad-ref", Name: "Bad", OrganizationKey: "reg-acme", Kind: application.ConnectionKubernetes, Status: application.ConnectionReady, Config: map[string]any{}, SecretRef: "https://elsewhere", Verification: map[string]any{}},
	} {
		if err := st.CreateConnection(ctx, bad); err == nil {
			t.Fatalf("%s authentication accepted", name)
		}
		if _, err := st.GetConnection(ctx, "reg-acme", bad.Key); !errors.Is(err, persistence.ErrNotFound) {
			t.Fatalf("%s left a row: %v", name, err)
		}
	}
}

// AssertRegistration re-reads a RegistrationFixture, e.g. after reopen.
func AssertRegistration(t *testing.T, st persistence.Store, f RegistrationFixture) {
	t.Helper()
	assertType(t, st, f.Organization, f.Type)
	assertDefinition(t, st, f.Organization, f.Definition)
	assertConnection(t, st, f.Connection)
	assertDefinition(t, st, f.Organization, f.Edited)
	got, err := st.GetConnection(context.Background(), f.Organization, f.Versioned.Key)
	if err != nil || got.Version != f.Versioned.Version || got.Name != f.Versioned.Name || got.SecretRef != f.Versioned.SecretRef ||
		got.Status != f.Versioned.Status || !reflect.DeepEqual(got.Config["endpoint"], f.Versioned.Config["endpoint"]) {
		t.Fatalf("versioned connection after reopen = %+v, %v; want %+v", got, err, f.Versioned)
	}
}

func assertType(t *testing.T, st persistence.Store, org string, want resource.Type) {
	t.Helper()
	types, err := st.ListResourceTypes(context.Background(), org)
	if err != nil {
		t.Fatal(err)
	}
	for _, got := range types {
		if got.Key == want.Key {
			if !reflect.DeepEqual(normalizeType(got), normalizeType(want)) {
				t.Fatalf("type %s/%s = %+v, want %+v", org, want.Key, got, want)
			}
			return
		}
	}
	t.Fatalf("type %s/%s missing", org, want.Key)
}

// normalizeType treats nil and empty field lists alike, as JSON columns do.
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
	if d.DriverInputs == nil {
		d.DriverInputs = map[string]any{}
	}
	return d
}

func assertDefinition(t *testing.T, st persistence.Store, org string, want resource.Definition) {
	t.Helper()
	defs, err := st.ListResourceDefinitions(context.Background(), org)
	if err != nil {
		t.Fatal(err)
	}
	for _, got := range defs {
		if got.Key == want.Key {
			if !reflect.DeepEqual(normalizeDefinition(got), normalizeDefinition(want)) {
				t.Fatalf("definition %s = %+v, want %+v", want.Key, got, want)
			}
			return
		}
	}
	t.Fatalf("definition %s missing", want.Key)
}

func assertConnection(t *testing.T, st persistence.Store, want application.Connection) {
	t.Helper()
	got, err := st.GetConnection(context.Background(), want.OrganizationKey, want.Key)
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != want.Key || got.OrganizationKey != want.OrganizationKey || got.Kind != want.Kind || got.SecretRef != want.SecretRef ||
		got.AuthenticationType == "" || got.Name == "" ||
		!reflect.DeepEqual(got.Config, want.Config) || got.Status != want.Status || !reflect.DeepEqual(got.Verification, want.Verification) {
		t.Fatalf("connection = %+v, want %+v", got, want)
	}
}

// connectionVersioning checks the V01 account compare-and-set: new records
// start at version 1, an update with the current version replaces name/config/
// reference/status/verification and increments the version once, a stale
// version leaves the winner untouched, a foreign or missing key is not found,
// and concurrent writers of one version have exactly one winner.
func connectionVersioning(t *testing.T, st persistence.Store) application.Connection {
	t.Helper()
	ctx := context.Background()
	base := application.Connection{Key: "reg-versioned", Name: "Versioned", OrganizationKey: "reg-acme", Kind: application.ConnectionKubernetes,
		AuthenticationType: application.AuthKubeconfig, Status: application.ConnectionReady,
		Config:    map[string]any{"cluster": "v", "kubeContext": "v-ctx", "endpoint": "https://v.example"},
		SecretRef: "kv2://kv/orchestrator/connections/reg-acme/reg-versioned/credentials/00000000-0000-4000-8000-0000000000a1", Verification: map[string]any{"verified": true}}
	if err := st.CreateConnection(ctx, base); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetConnection(ctx, "reg-acme", base.Key)
	if err != nil || got.Version != 1 {
		t.Fatalf("new connection version = %d, %v", got.Version, err)
	}
	next := got
	next.Name, next.Status = "Renamed", application.ConnectionReady
	next.SecretRef = "kv2://kv/orchestrator/connections/reg-acme/reg-versioned/credentials/00000000-0000-4000-8000-0000000000a2"
	next.Verification = map[string]any{"verified": true, "verifiedAt": "2026-10-17T00:00:00Z"}
	next.Kind, next.AuthenticationType, next.Key = application.ConnectionAWS, application.AuthAWSAccessKey, "other-key"
	updated, err := st.UpdateConnection(ctx, "reg-acme", base.Key, 1, next)
	if err == nil && (updated.Kind != application.ConnectionKubernetes || updated.Key != base.Key) {
		t.Fatalf("identity changed: %+v", updated)
	}
	if err != nil {
		// Adapters may also reject a record whose kind and authentication disagree.
		next.Kind, next.AuthenticationType, next.Key = base.Kind, base.AuthenticationType, base.Key
		if updated, err = st.UpdateConnection(ctx, "reg-acme", base.Key, 1, next); err != nil {
			t.Fatalf("update: %v", err)
		}
	}
	if updated.Version != 2 || updated.Name != "Renamed" || updated.SecretRef != next.SecretRef || updated.ID != got.ID {
		t.Fatalf("updated = %+v", updated)
	}
	if _, err := st.UpdateConnection(ctx, "reg-acme", base.Key, 1, next); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("stale update: %v", err)
	}
	if _, err := st.UpdateConnection(ctx, "reg-globex", base.Key, 2, next); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("foreign organization update: %v", err)
	}
	if _, err := st.UpdateConnection(ctx, "reg-acme", "reg-nothing", 1, next); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("missing update: %v", err)
	}
	const racers = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins, stale := 0, 0
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			candidate := updated
			candidate.Name = fmt.Sprintf("Racer %d", i)
			_, err := st.UpdateConnection(ctx, "reg-acme", base.Key, 2, candidate)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				wins++
			case errors.Is(err, persistence.ErrVersionConflict):
				stale++
			default:
				t.Errorf("racer %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	if wins != 1 || stale != racers-1 {
		t.Fatalf("racers: %d wins, %d stale", wins, stale)
	}
	final, err := st.GetConnection(ctx, "reg-acme", base.Key)
	if err != nil || final.Version != 3 {
		t.Fatalf("final = %+v, %v", final, err)
	}
	return final
}

// definitionVersioning checks the UC-03 V02-06 Definition compare-and-set: new
// records start at the given version, an update replaces name, driver inputs,
// account and criteria and increments the version exactly once, a stale version
// leaves the winner untouched, the Resource Type is immutable, an account that
// is not in the Organization fails without writing, and concurrent writers of
// one version have exactly one winner. Every adapter must behave identically.
func definitionVersioning(t *testing.T, st persistence.Store, typ resource.Type) resource.Definition {
	t.Helper()
	ctx := context.Background()
	other := resource.Type{Key: "queue", Outputs: []resource.OutputField{{Name: "url", Type: "string"}}}
	if err := st.CreateResourceType(ctx, "reg-acme", other); err != nil {
		t.Fatal(err)
	}
	base := resource.Definition{Key: "cache-edited", Name: "Bản đầu", Version: 1, ResourceTypeKey: typ.Key, DriverType: resource.DriverTerraform,
		DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"v": "1"}}},
		Criteria:     []resource.Criterion{{Class: "one"}, {EnvironmentType: "dev"}}, SourceFingerpr: "sha256:one"}
	if err := st.CreateResourceDefinition(ctx, "reg-acme", base); err != nil {
		t.Fatalf("create: %v", err)
	}
	next := base
	next.Name, next.DriverAccount = "Bản sửa", "reg-cluster"
	next.DriverInputs = map[string]any{"values": map[string]any{"variables": map[string]any{"v": "2"}}}
	next.Criteria = []resource.Criterion{{ApplicationID: "app", Class: "two"}}
	next.Provision = map[string]resource.ProvisionRule{"cache.replica": {IsDependent: true}}
	next.SourceFingerpr = "sha256:two"
	updated, err := st.UpdateResourceDefinition(ctx, "reg-acme", 1, next)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	next.Version = 2
	if !reflect.DeepEqual(normalizeDefinition(updated), normalizeDefinition(next)) {
		t.Fatalf("returned %+v, want %+v", updated, next)
	}
	assertDefinition(t, st, "reg-acme", next)
	if _, err := st.UpdateResourceDefinition(ctx, "reg-acme", 1, next); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("stale update: %v", err)
	}
	moved := next
	moved.ResourceTypeKey = other.Key
	if _, err := st.UpdateResourceDefinition(ctx, "reg-acme", 2, moved); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("resource type change: %v", err)
	}
	ghost := next
	ghost.Key = "cache-ghost"
	if _, err := st.UpdateResourceDefinition(ctx, "reg-acme", 1, ghost); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("missing definition: %v", err)
	}
	if _, err := st.UpdateResourceDefinition(ctx, "reg-globex", 2, next); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("foreign organization: %v", err)
	}
	foreign := next
	foreign.DriverAccount = "reg-missing-cluster"
	if _, err := st.UpdateResourceDefinition(ctx, "reg-acme", 2, foreign); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("unknown account: %v", err)
	}
	blank := next
	blank.Name = "   "
	if _, err := st.UpdateResourceDefinition(ctx, "reg-acme", 2, blank); err == nil {
		t.Fatal("blank name accepted")
	}
	assertDefinition(t, st, "reg-acme", next) // every rejected edit left the winner untouched
	const racers = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins, stale := 0, 0
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			candidate := next
			candidate.Name = fmt.Sprintf("Racer %d", i)
			_, err := st.UpdateResourceDefinition(ctx, "reg-acme", 2, candidate)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				wins++
			case errors.Is(err, persistence.ErrVersionConflict):
				stale++
			default:
				t.Errorf("racer %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	if wins != 1 || stale != racers-1 {
		t.Fatalf("racers: %d wins, %d stale", wins, stale)
	}
	defs, _ := st.ListResourceDefinitions(ctx, "reg-acme")
	for _, d := range defs {
		if d.Key == base.Key {
			if d.Version != 3 {
				t.Fatalf("final version = %d", d.Version)
			}
			return d
		}
	}
	t.Fatal("edited definition missing")
	return resource.Definition{}
}
