package application

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"testing"

	"orchestrator/internal/adapters/store"
	appdomain "orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/persistence"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func newTestStore(t *testing.T, conns ...appdomain.Connection) *store.Store {
	t.Helper()
	ctx := context.Background()
	st := store.New()
	for _, org := range []appdomain.Organization{
		{ID: "org-acme", Key: "acme", Name: "Acme", DefaultConnectionKey: "default"},
		{ID: "org-globex", Key: "globex", Name: "Globex", DefaultConnectionKey: "default"},
	} {
		if err := st.SaveOrganization(ctx, org); err != nil {
			t.Fatal(err)
		}
	}
	if len(conns) == 0 {
		conns = []appdomain.Connection{
			{ID: "c1", Key: "default", OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionReady},
			{ID: "c2", Key: "default", OrganizationKey: "globex", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionReady},
		}
	}
	for _, conn := range conns {
		if err := st.SaveConnection(ctx, conn); err != nil {
			t.Fatal(err)
		}
	}
	return st
}

func TestCreateApplication_CreatesUnconfiguredEnvironmentsWithoutAnyConnection(t *testing.T) {
	ctx := context.Background()
	// No Connection exists at all: creation needs no default and binds nothing.
	st := newTestStore(t, appdomain.Connection{ID: "c0", Key: "unrelated", OrganizationKey: "globex", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionVerifying})
	result, err := NewService(st).Create(ctx, CreateCommand{OrganizationKey: "acme", Name: "Payment", Subdomain: "Payment"})
	if err != nil {
		t.Fatal(err)
	}
	app := result.Application
	if app.OrganizationKey != "acme" || app.Profile != "" || app.ConnectionKey != "" || app.Region != "" || app.RuntimeStatus != appdomain.RuntimeUnconfigured {
		t.Fatalf("application must carry no target: %+v", app)
	}
	if !uuidPattern.MatchString(app.ID) || app.Key != app.ID || app.Subdomain != "payment" {
		t.Fatalf("generated identity/subdomain = %q %q %q", app.ID, app.Key, app.Subdomain)
	}
	envs, _ := st.ListEnvironments(ctx, app.Key)
	if len(envs) != 2 {
		t.Fatalf("environments = %+v", envs)
	}
	for _, env := range envs {
		if env.Configured() || env.Status() != appdomain.RuntimeUnconfigured || env.Profile != "" || env.Region != "" || env.Version != 1 {
			t.Fatalf("environment %s must be UNCONFIGURED at version 1: %+v", env.Key, env)
		}
	}
}

func TestCreateApplication_CreatesStagingAndProductionWithEmptyDeploymentSets(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	result, err := NewService(st).Create(ctx, CreateCommand{OrganizationKey: "acme", Name: "  Catalog  ", Subdomain: " catalog "})
	if err != nil {
		t.Fatal(err)
	}
	if result.Application.Name != "Catalog" {
		t.Fatalf("name = %q", result.Application.Name)
	}
	envs, err := st.ListEnvironments(ctx, result.Application.Key)
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 2 || len(result.Environments) != 2 {
		t.Fatalf("want exactly two environments, got %d", len(envs))
	}
	namespaces := map[string]bool{}
	keys := map[string]bool{}
	for _, env := range envs {
		keys[env.Key] = true
		namespaces[env.NamespaceIdentity] = true
		if env.ApplicationKey != result.Application.Key || env.CurrentDeploymentSetID == "" {
			t.Fatalf("environment = %+v", env)
		}
		set, err := st.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
		if err != nil {
			t.Fatal(err)
		}
		if len(set.Document.Modules) != 0 || len(set.Document.Shared) != 0 || set.EnvironmentID != env.ID {
			t.Fatalf("deployment set must be empty: %+v", set)
		}
		if drafts, err := st.ListWorkloadDrafts(ctx, result.Application.Key, env.Key); err != nil || len(drafts) != 0 {
			t.Fatalf("no workload may exist: %v %v", drafts, err)
		}
	}
	if !keys["staging"] || !keys["production"] || len(namespaces) != 2 {
		t.Fatalf("environments = %v namespaces = %v", keys, namespaces)
	}
	if deployments, err := st.ListDeployments(ctx, result.Application.Key, ""); err != nil || len(deployments) != 0 {
		t.Fatalf("create must not deploy: %v %v", deployments, err)
	}
}

func TestCreateApplication_RejectsInvalidOrDuplicateSubdomain(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	svc := NewService(st)
	if _, err := svc.Create(ctx, CreateCommand{OrganizationKey: "acme", Name: "Catalog", Subdomain: "catalog"}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, org, appName, subdomain, field string
		want                                 error
	}{
		{"empty name", "acme", " ", "other", "name", ErrInvalid},
		{"invalid label", "acme", "Other", "bad_label", "subdomain", ErrInvalid},
		{"leading hyphen", "acme", "Other", "-other", "subdomain", ErrInvalid},
		{"too long", "acme", "Other", strings.Repeat("a", 64), "subdomain", ErrInvalid},
		{"duplicate name in organization", "acme", "catalog", "other", "name", ErrDuplicate},
		{"subdomain used by another organization", "globex", "Catalog", "CATALOG", "subdomain", ErrDuplicate},
	} {
		_, err := svc.Create(ctx, CreateCommand{OrganizationKey: tc.org, Name: tc.appName, Subdomain: tc.subdomain})
		var fieldErr *FieldError
		if !errors.Is(err, tc.want) || !errors.As(err, &fieldErr) || fieldErr.Field != tc.field {
			t.Fatalf("%s: err = %v", tc.name, err)
		}
	}
	if _, err := svc.Create(ctx, CreateCommand{OrganizationKey: "globex", Name: "Catalog", Subdomain: strings.Repeat("a", 63)}); err != nil {
		t.Fatalf("same name in another organization and a 63-character label are valid: %v", err)
	}
	apps, _ := st.ListApplications(ctx)
	if len(apps) != 2 {
		t.Fatalf("rejected commands must not persist, got %d applications", len(apps))
	}
}

// failingEnvironmentStore fails after Application and staging are written.
type failingEnvironmentStore struct{ *store.Store }

func (s failingEnvironmentStore) SaveEnvironment(ctx context.Context, env environment.Environment) error {
	if env.Key == "production" {
		return errors.New("disk full")
	}
	return s.Store.SaveEnvironment(ctx, env)
}

func TestCreateApplication_RollsBackWhenAnyEnvironmentFails(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	result, err := NewService(failingEnvironmentStore{st}).Create(ctx, CreateCommand{OrganizationKey: "acme", Name: "Catalog", Subdomain: "catalog"})
	if err == nil {
		t.Fatalf("create must fail, got %+v", result)
	}
	if apps, _ := st.ListApplications(ctx); len(apps) != 0 {
		t.Fatalf("Application must roll back: %+v", apps)
	}
	if _, err := NewService(st).Create(ctx, CreateCommand{OrganizationKey: "acme", Name: "Catalog", Subdomain: "catalog"}); err != nil {
		t.Fatalf("rolled-back Name/Subdomain must stay available: %v", err)
	}
}

func setCommand(app environment.Environment, org, appKey, env, connection string) SetConnectionCommand {
	return SetConnectionCommand{OrganizationKey: org, ApplicationKey: appKey, EnvironmentKey: env, ConnectionKey: connection, ExpectedVersion: app.Version}
}

func newUnconfigured(t *testing.T, conns ...appdomain.Connection) (*Service, *store.Store, Result) {
	t.Helper()
	st := newTestStore(t, conns...)
	svc := NewService(st)
	result, err := svc.Create(context.Background(), CreateCommand{OrganizationKey: "acme", Name: "Payment", Subdomain: "payment"})
	if err != nil {
		t.Fatal(err)
	}
	return svc, st, result
}

func choiceConnections() []appdomain.Connection {
	return []appdomain.Connection{
		{ID: "c1", Key: "default", OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionReady},
		{ID: "c2", Key: "lab", Name: "Lab", OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionReady},
		{ID: "c3", Key: "cloud", OrganizationKey: "acme", Kind: appdomain.ConnectionAWS, Status: appdomain.ConnectionReady, Config: map[string]any{"region": "eu-west-1"}},
		{ID: "c4", Key: "cloud-us", OrganizationKey: "acme", Kind: appdomain.ConnectionAWS, Status: appdomain.ConnectionReady, Config: map[string]any{"region": "us-east-1"}},
		{ID: "c5", Key: "foreign", OrganizationKey: "globex", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionReady},
		{ID: "c6", Key: "verifying", OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionVerifying},
		{ID: "c7", Key: "noregion", OrganizationKey: "acme", Kind: appdomain.ConnectionAWS, Status: appdomain.ConnectionReady},
		{ID: "c8", Key: "gcp", OrganizationKey: "acme", Kind: appdomain.ConnectionKind("GCP"), Status: appdomain.ConnectionReady},
	}
}

func TestSetConnection_SetsEachEnvironmentIndependentlyAndOnce(t *testing.T) {
	ctx := context.Background()
	svc, st, created := newUnconfigured(t, choiceConnections()...)
	app := created.Application.Key
	staging, _ := st.GetEnvironment(ctx, app, "staging")
	bound, err := svc.SetConnection(ctx, setCommand(staging, "acme", app, "staging", " lab "))
	if err != nil {
		t.Fatal(err)
	}
	if bound.ConnectionKey != "lab" || bound.Profile != appdomain.ProfileInternalK8s || bound.RuntimeStatus != appdomain.RuntimeReady || bound.InfrastructureScope != environment.ScopeEnvironment || bound.Version != staging.Version+1 {
		t.Fatalf("staging binding = %+v", bound)
	}
	// Production is still unset and independent of staging.
	production, _ := st.GetEnvironment(ctx, app, "production")
	if production.Configured() || production.Version != 1 {
		t.Fatalf("production must stay unset: %+v", production)
	}
	// A different kind and region is allowed for the other Environment.
	cloud, err := svc.SetConnection(ctx, setCommand(production, "acme", app, "production", "cloud"))
	if err != nil || cloud.Profile != appdomain.ProfileAWSEKS || cloud.Region != "eu-west-1" || cloud.RuntimeStatus != appdomain.RuntimePending || cloud.InfrastructureScope != environment.ScopeEnvironment {
		t.Fatalf("production binding = %+v %v", cloud, err)
	}
	// Application itself never receives the projected target.
	stored, _ := st.GetApplication(ctx, app)
	if stored.ConnectionKey != "" || stored.Profile != "" || stored.Region != "" || stored.RuntimeStatus != appdomain.RuntimeUnconfigured {
		t.Fatalf("application leaked target: %+v", stored)
	}
	// Same key, another key and an unset-style retry are all already configured.
	for _, key := range []string{"lab", "default", "cloud"} {
		fresh, _ := st.GetEnvironment(ctx, app, "staging")
		if _, err := svc.SetConnection(ctx, setCommand(fresh, "acme", app, "staging", key)); !errors.Is(err, ErrAlreadyConfigured) {
			t.Fatalf("repeat %s: %v", key, err)
		}
		// Even a stale version reports already configured, not stale.
		stale := setCommand(fresh, "acme", app, "staging", key)
		stale.ExpectedVersion = 1
		if _, err := svc.SetConnection(ctx, stale); !errors.Is(err, ErrAlreadyConfigured) {
			t.Fatalf("stale repeat %s: %v", key, err)
		}
	}
	after, _ := st.GetEnvironment(ctx, app, "staging")
	if after.ConnectionKey != "lab" || after.Version != bound.Version {
		t.Fatalf("rejected repeats mutated staging: %+v", after)
	}
}

func TestSetConnection_StaleVersionIsDistinctFromAlreadyConfigured(t *testing.T) {
	ctx := context.Background()
	svc, st, created := newUnconfigured(t, choiceConnections()...)
	app := created.Application.Key
	cmd := SetConnectionCommand{OrganizationKey: "acme", ApplicationKey: app, EnvironmentKey: "staging", ConnectionKey: "lab", ExpectedVersion: 7}
	if _, err := svc.SetConnection(ctx, cmd); !errors.Is(err, ErrStaleVersion) || errors.Is(err, ErrAlreadyConfigured) {
		t.Fatalf("stale expected version: %v", err)
	}
	env, _ := st.GetEnvironment(ctx, app, "staging")
	if env.Configured() {
		t.Fatalf("stale request mutated: %+v", env)
	}
}

func TestSetConnection_RejectsInvalidAndUnavailableWithoutMutation(t *testing.T) {
	ctx := context.Background()
	svc, st, created := newUnconfigured(t, choiceConnections()...)
	app := created.Application.Key
	env, _ := st.GetEnvironment(ctx, app, "staging")
	for _, key := range []string{"foreign", "verifying", "noregion", "gcp", "missing"} {
		_, err := svc.SetConnection(ctx, setCommand(env, "acme", app, "staging", key))
		var field *FieldError
		if !errors.Is(err, ErrTargetNotReady) || !errors.As(err, &field) || field.Field != "connectionKey" || strings.Contains(err.Error(), key) {
			t.Fatalf("%s: %v", key, err)
		}
	}
	for _, key := range []string{"", "  "} {
		_, err := svc.SetConnection(ctx, setCommand(env, "acme", app, "staging", key))
		var field *FieldError
		if !errors.Is(err, ErrInvalid) || !errors.As(err, &field) || field.Field != "connectionKey" {
			t.Fatalf("blank %q: %v", key, err)
		}
	}
	zero := setCommand(env, "acme", app, "staging", "lab")
	zero.ExpectedVersion = 0
	if _, err := svc.SetConnection(ctx, zero); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing expected version: %v", err)
	}
	// Scope: foreign Organization session, missing Application and Environment are 404.
	for _, cmd := range []SetConnectionCommand{
		setCommand(env, "globex", app, "staging", "lab"),
		setCommand(env, "acme", "missing", "staging", "lab"),
		setCommand(env, "acme", app, "missing", "lab"),
	} {
		if _, err := svc.SetConnection(ctx, cmd); !errors.Is(err, persistence.ErrNotFound) {
			t.Fatalf("scope %+v: %v", cmd, err)
		}
	}
	after, _ := st.GetEnvironment(ctx, app, "staging")
	if after.Configured() || after.Version != env.Version {
		t.Fatalf("rejected sets mutated: %+v", after)
	}
}

func TestSetConnection_ConcurrentRequestsHaveOneWinner(t *testing.T) {
	ctx := context.Background()
	svc, st, created := newUnconfigured(t, choiceConnections()...)
	app := created.Application.Key
	env, _ := st.GetEnvironment(ctx, app, "staging")
	keys := []string{"default", "lab", "cloud", "cloud-us"}
	results := make(chan error, len(keys))
	var wg sync.WaitGroup
	for _, key := range keys {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.SetConnection(ctx, setCommand(env, "acme", app, "staging", key))
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	wins, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, ErrAlreadyConfigured), errors.Is(err, ErrStaleVersion):
			conflicts++
		default:
			t.Fatalf("unexpected: %v", err)
		}
	}
	if wins != 1 || conflicts != len(keys)-1 {
		t.Fatalf("wins=%d conflicts=%d", wins, conflicts)
	}
}

func TestSaveEnvironmentCannotOverwriteConfiguredBinding(t *testing.T) {
	ctx := context.Background()
	svc, st, created := newUnconfigured(t, choiceConnections()...)
	app := created.Application.Key
	env, _ := st.GetEnvironment(ctx, app, "staging")
	bound, err := svc.SetConnection(ctx, setCommand(env, "acme", app, "staging", "lab"))
	if err != nil {
		t.Fatal(err)
	}
	// A stale unset copy, and a copy claiming another target, change nothing.
	if err := st.SaveEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	hostile := bound
	hostile.ConnectionKey, hostile.Profile, hostile.Region, hostile.InfrastructureScope = "cloud", appdomain.ProfileAWSEKS, "eu-west-1", environment.ScopeLegacyApplication
	if err := st.SaveEnvironment(ctx, hostile); err != nil {
		t.Fatal(err)
	}
	after, _ := st.GetEnvironment(ctx, app, "staging")
	if after.ConnectionKey != "lab" || after.Profile != appdomain.ProfileInternalK8s || after.Region != "" || after.InfrastructureScope != environment.ScopeEnvironment || after.RuntimeStatus != appdomain.RuntimeReady {
		t.Fatalf("save overwrote binding: %+v", after)
	}
	// A new Environment saved with a binding stays unconfigured too.
	other := environment.Environment{Key: "qa", ApplicationKey: app, NamespaceIdentity: "app-qa", ConnectionKey: "lab", Profile: appdomain.ProfileInternalK8s, RuntimeStatus: appdomain.RuntimeReady}
	if err := st.SaveEnvironment(ctx, other); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.GetEnvironment(ctx, app, "qa"); got.Configured() {
		t.Fatalf("insert wrote a binding: %+v", got)
	}
}

func TestSetConnection_DoesNotConsultOrChangeOrganizationDefault(t *testing.T) {
	ctx := context.Background()
	svc, st, created := newUnconfigured(t, choiceConnections()...)
	app := created.Application.Key
	env, _ := st.GetEnvironment(ctx, app, "staging")
	if _, err := svc.SetConnection(ctx, setCommand(env, "acme", app, "staging", "lab")); err != nil {
		t.Fatal(err)
	}
	org, _ := st.GetOrganization(ctx, "acme")
	if org.DefaultConnectionKey != "default" {
		t.Fatalf("default changed: %+v", org)
	}
	production, _ := st.GetEnvironment(ctx, app, "production")
	if production.Configured() {
		t.Fatalf("the default must never configure an Environment: %+v", production)
	}
}

func TestListChoices_ReturnsOnlyReadySupportedOrganizationConnections(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t,
		appdomain.Connection{ID: "c1", Key: "default", Name: "Default", OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionVerifying},
		appdomain.Connection{ID: "c2", Key: "lab", OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionReady, SecretRef: "secret"},
		appdomain.Connection{ID: "c3", Key: "foreign", OrganizationKey: "globex", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionReady},
	)
	choices, err := NewService(st).ListChoices(ctx, "acme")
	if err != nil || len(choices.Connections) != 1 || choices.Connections[0].Key != "lab" || choices.Connections[0].Name != "lab" || choices.DefaultConnectionKey != "" {
		t.Fatalf("choices = %+v %v", choices, err)
	}
}
