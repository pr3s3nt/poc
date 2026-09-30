package persistencetest

import (
	"context"
	"errors"
	"reflect"
	"strings"
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
		Key: "cache-fast", ResourceTypeKey: "cache", ExecutionProfile: "internal-k8s", DriverType: resource.DriverKubernetes,
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

	conn := application.Connection{Key: "reg-cluster", OrganizationKey: "reg-acme", Kind: application.ConnectionKubernetes, Status: application.ConnectionReady,
		Config: map[string]any{"cluster": "c1", "kubeContext": "ctx-1"}, SecretRef: "host-kube-context://ctx-1", Verification: map[string]any{"verified": true}}
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
		bad.Key, bad.ConnectionKey = "cache-"+strings.ReplaceAll(name, " ", "-"), key
		if err := st.CreateResourceDefinition(ctx, "reg-acme", bad); !errors.Is(err, persistence.ErrNotFound) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	withConnection := def
	withConnection.Key, withConnection.ConnectionKey = "cache-on-cluster", "reg-cluster"
	if err := st.CreateResourceDefinition(ctx, "reg-acme", withConnection); err != nil {
		t.Fatalf("definition with same-Organization connection: %v", err)
	}
	defs, _ := st.ListResourceDefinitions(ctx, "reg-acme")
	keys := map[string]string{}
	for _, d := range defs {
		keys[d.Key] = d.ConnectionKey
	}
	if len(keys) != 2 || keys["cache-on-cluster"] != "reg-cluster" || keys["cache-fast"] != "" {
		t.Fatalf("definitions after rejected references: %v", keys)
	}

	other := conn
	other.ID, other.Config, other.SecretRef = "", map[string]any{"cluster": "c2", "kubeContext": "ctx-2"}, "host-kube-context://ctx-2"
	if err := st.CreateConnection(ctx, other); !errors.Is(err, persistence.ErrDuplicate) {
		t.Fatalf("duplicate connection: %v", err)
	}
	missing := conn
	missing.ID, missing.OrganizationKey = "", "reg-missing"
	if err := st.CreateConnection(ctx, missing); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("connection in missing Organization: %v", err)
	}
	assertConnection(t, st, conn)
	return RegistrationFixture{Organization: "reg-acme", Type: typ, Definition: def, Connection: conn}
}

// AssertRegistration re-reads a RegistrationFixture, e.g. after reopen.
func AssertRegistration(t *testing.T, st persistence.Store, f RegistrationFixture) {
	t.Helper()
	assertType(t, st, f.Organization, f.Type)
	assertDefinition(t, st, f.Organization, f.Definition)
	assertConnection(t, st, f.Connection)
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
		!reflect.DeepEqual(got.Config, want.Config) || got.Status != want.Status || !reflect.DeepEqual(got.Verification, want.Verification) {
		t.Fatalf("connection = %+v, want %+v", got, want)
	}
}
