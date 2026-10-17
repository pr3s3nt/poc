package persistencetest

import (
	"context"
	"errors"
	"fmt"
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
	// Versioned is the account record after the V01 CAS sequence.
	Versioned application.Connection
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
	return RegistrationFixture{Organization: "reg-acme", Type: typ, Definition: def, Connection: conn, Versioned: versioned}
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

// connectionVersioning checks the V01 optimistic version contract: new records
// start at 1, UpdateConnection is a CAS that never touches identity, kind or
// authentication, a stale or foreign request changes nothing and exactly one
// of several racing updates wins.
func connectionVersioning(t *testing.T, st persistence.Store) application.Connection {
	t.Helper()
	ctx := context.Background()
	acct := application.Connection{Key: "reg-account", Name: "Account", OrganizationKey: "reg-acme", Kind: application.ConnectionAWS,
		AuthenticationType: application.AuthAWSAccessKey, Status: application.ConnectionReady,
		Config:    map[string]any{"accountId": "123456789012", "region": "us-east-1", "allowedRegions": []string{"us-east-1"}},
		SecretRef: "kv2://kv/orchestrator/connections/reg-acme/reg-account/credentials/00000000-0000-4000-8000-0000000000a1", Verification: map[string]any{"verified": true}}
	if err := st.CreateConnection(ctx, acct); err != nil {
		t.Fatalf("create account: %v", err)
	}
	created, err := st.GetConnection(ctx, "reg-acme", "reg-account")
	if err != nil || created.Version != 1 || created.ID == "" {
		t.Fatalf("a new record must start at version 1: %+v %v", created, err)
	}
	update := persistence.ConnectionUpdate{OrganizationKey: "reg-acme", Key: "reg-account", ExpectedVersion: 1, Name: "Renamed",
		Config: map[string]any{"accountId": "123456789012", "region": "us-east-1", "verificationRegions": []string{"eu-west-1"}}, SecretRef: "kv2://kv/orchestrator/connections/reg-acme/reg-account/credentials/00000000-0000-4000-8000-0000000000a2",
		Status: application.ConnectionReady, Verification: map[string]any{"verified": true, "verifiedAt": "2026-10-17T08:00:00Z"}}
	updated, err := st.UpdateConnection(ctx, update)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Version != 2 || updated.Name != "Renamed" || updated.SecretRef != update.SecretRef || updated.ID != created.ID ||
		updated.Kind != created.Kind || updated.AuthenticationType != created.AuthenticationType || updated.Verification["verifiedAt"] != "2026-10-17T08:00:00Z" {
		t.Fatalf("updated = %+v", updated)
	}
	stale := update
	stale.Name, stale.SecretRef = "Stale", "kv2://kv/orchestrator/connections/reg-acme/reg-account/credentials/00000000-0000-4000-8000-0000000000a3"
	if _, err := st.UpdateConnection(ctx, stale); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("stale update: %v", err)
	}
	for name, bad := range map[string]persistence.ConnectionUpdate{
		"version zero":   {OrganizationKey: "reg-acme", Key: "reg-account", ExpectedVersion: 0, Name: "Zero", Config: map[string]any{}, Verification: map[string]any{}, Status: application.ConnectionReady},
		"future version": {OrganizationKey: "reg-acme", Key: "reg-account", ExpectedVersion: 9, Name: "Future", Config: map[string]any{}, Verification: map[string]any{}, Status: application.ConnectionReady},
	} {
		if _, err := st.UpdateConnection(ctx, bad); !errors.Is(err, persistence.ErrVersionConflict) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	for name, miss := range map[string]persistence.ConnectionUpdate{
		"missing key":     {OrganizationKey: "reg-acme", Key: "reg-none", ExpectedVersion: 1, Name: "x", Config: map[string]any{}, Verification: map[string]any{}, Status: application.ConnectionReady},
		"foreign record":  {OrganizationKey: "reg-acme", Key: "reg-foreign-cluster", ExpectedVersion: 1, Name: "x", Config: map[string]any{}, Verification: map[string]any{}, Status: application.ConnectionReady},
		"no organization": {OrganizationKey: "reg-missing", Key: "reg-account", ExpectedVersion: 1, Name: "x", Config: map[string]any{}, Verification: map[string]any{}, Status: application.ConnectionReady},
	} {
		if _, err := st.UpdateConnection(ctx, miss); !errors.Is(err, persistence.ErrNotFound) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	foreign, err := st.GetConnection(ctx, "reg-globex", "reg-foreign-cluster")
	if err != nil || foreign.Version != 1 || foreign.Name != "Registered Cluster" {
		t.Fatalf("the foreign Organization record changed: %+v %v", foreign, err)
	}
	// The seed upsert never rewinds the version and bumps it only on change.
	seed := updated
	seed.ID, seed.Version = "", 0
	if err := st.SaveConnection(ctx, seed); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.GetConnection(ctx, "reg-acme", "reg-account"); got.Version != 2 || got.ID != created.ID {
		t.Fatalf("an unchanged seed must keep version and ID: %+v", got)
	}
	// Racing updates at the same version: exactly one wins.
	const racers = 8
	results := make(chan error, racers)
	for i := 0; i < racers; i++ {
		go func() {
			next := update
			next.ExpectedVersion, next.Name = 2, fmt.Sprintf("Racer %d", i)
			_, err := st.UpdateConnection(ctx, next)
			results <- err
		}()
	}
	wins, losses := 0, 0
	for i := 0; i < racers; i++ {
		switch err := <-results; {
		case err == nil:
			wins++
		case errors.Is(err, persistence.ErrVersionConflict):
			losses++
		default:
			t.Fatalf("racing update: %v", err)
		}
	}
	if wins != 1 || losses != racers-1 {
		t.Fatalf("racing updates: %d wins, %d conflicts", wins, losses)
	}
	final, err := st.GetConnection(ctx, "reg-acme", "reg-account")
	if err != nil || final.Version != 3 {
		t.Fatalf("final version = %+v %v", final, err)
	}
	return final
}

// AssertRegistration re-reads a RegistrationFixture, e.g. after reopen.
func AssertRegistration(t *testing.T, st persistence.Store, f RegistrationFixture) {
	t.Helper()
	if got, err := st.GetConnection(context.Background(), f.Organization, f.Versioned.Key); err != nil || got.Version != f.Versioned.Version || got.Name != f.Versioned.Name || got.SecretRef != f.Versioned.SecretRef {
		t.Fatalf("versioned connection after reopen = %+v, %v; want %+v", got, err, f.Versioned)
	}
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
		got.AuthenticationType == "" || got.Name == "" ||
		!reflect.DeepEqual(got.Config, want.Config) || got.Status != want.Status || !reflect.DeepEqual(got.Verification, want.Verification) {
		t.Fatalf("connection = %+v, want %+v", got, want)
	}
}
