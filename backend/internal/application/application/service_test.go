package application

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
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
			{Name: "default", AuthenticationType: appdomain.AuthKubeconfig, ID: "c1", Key: "default", OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionReady},
			{Name: "default", AuthenticationType: appdomain.AuthKubeconfig, ID: "c2", Key: "default", OrganizationKey: "globex", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionReady},
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
	st := newTestStore(t, appdomain.Connection{Name: "unrelated", AuthenticationType: appdomain.AuthKubeconfig, ID: "c0", Key: "unrelated", OrganizationKey: "globex", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionVerifying})
	result, err := NewService(st).Create(ctx, CreateCommand{OrganizationKey: "acme", Name: "Payment", Subdomain: "Payment"})
	if err != nil {
		t.Fatal(err)
	}
	app := result.Application
	if app.OrganizationKey != "acme" || app.Name != "Payment" {
		t.Fatalf("application identity = %+v", app)
	}
	if b, err := json.Marshal(app); err != nil || strings.Contains(string(b), "executionProfile") || strings.Contains(string(b), "connectionKey") || strings.Contains(string(b), "runtimeStatus") || strings.Contains(string(b), "region") || strings.Contains(string(b), "configurationProvider") {
		t.Fatalf("application JSON carries retired fields: %s %v", b, err)
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
		{Name: "default", AuthenticationType: appdomain.AuthKubeconfig, ID: "c1", Key: "default", OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionReady},
		{ID: "c2", Key: "lab", Name: "Lab", AuthenticationType: appdomain.AuthKubeconfig, OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionReady},
		{Name: "cloud", AuthenticationType: appdomain.AuthAWSAccessKey, ID: "c3", Key: "cloud", OrganizationKey: "acme", Kind: appdomain.ConnectionAWS, Status: appdomain.ConnectionReady, Config: map[string]any{"region": "eu-west-1"}},
		{Name: "cloud-us", AuthenticationType: appdomain.AuthAWSAccessKey, ID: "c4", Key: "cloud-us", OrganizationKey: "acme", Kind: appdomain.ConnectionAWS, Status: appdomain.ConnectionReady, Config: map[string]any{"region": "us-east-1"}},
		{Name: "foreign", AuthenticationType: appdomain.AuthKubeconfig, ID: "c5", Key: "foreign", OrganizationKey: "globex", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionReady},
		{Name: "verifying", AuthenticationType: appdomain.AuthKubeconfig, ID: "c6", Key: "verifying", OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionVerifying},
		{Name: "noregion", AuthenticationType: appdomain.AuthAWSAccessKey, ID: "c7", Key: "noregion", OrganizationKey: "acme", Kind: appdomain.ConnectionAWS, Status: appdomain.ConnectionReady},
	}
}

func TestSetConnection_KubernetesConnectionIsRetiredAndChangesNothing(t *testing.T) {
	ctx := context.Background()
	svc, st, created := newUnconfigured(t, choiceConnections()...)
	app := created.Application.Key
	env, _ := st.GetEnvironment(ctx, app, "staging")
	_, err := svc.SetConnection(ctx, setCommand(env, "acme", app, "staging", "lab"))
	if !errors.Is(err, ErrDeploymentConnectionRetired) {
		t.Fatalf("a Kubernetes connection must be retired as a deployment target: %v", err)
	}
	after, _ := st.GetEnvironment(ctx, app, "staging")
	if after.Configured() || after.Version != env.Version {
		t.Fatalf("the retired write changed staging: %+v", after)
	}
}

func TestSetConnection_StaleVersionIsDistinct(t *testing.T) {
	ctx := context.Background()
	svc, st, created := newUnconfigured(t, choiceConnections()...)
	app := created.Application.Key
	cmd := SetConnectionCommand{OrganizationKey: "acme", ApplicationKey: app, EnvironmentKey: "staging", ConnectionKey: "lab", ExpectedVersion: 7}
	if _, err := svc.SetConnection(ctx, cmd); !errors.Is(err, ErrStaleVersion) {
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
	for _, key := range []string{"foreign", "verifying", "noregion", "missing"} {
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

func TestListChoices_ReturnsOnlyReadySupportedOrganizationConnections(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t,
		appdomain.Connection{AuthenticationType: appdomain.AuthKubeconfig, ID: "c1", Key: "default", Name: "Default", OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionVerifying},
		appdomain.Connection{Name: "lab", AuthenticationType: appdomain.AuthKubeconfig, ID: "c2", Key: "lab", OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionReady, SecretRef: "secret"},
		appdomain.Connection{Name: "foreign", AuthenticationType: appdomain.AuthKubeconfig, ID: "c3", Key: "foreign", OrganizationKey: "globex", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionReady},
	)
	choices, err := NewService(st).ListChoices(ctx, "acme")
	if err != nil || len(choices.Connections) != 1 || choices.Connections[0].Key != "lab" || choices.Connections[0].Name != "lab" || choices.DefaultConnectionKey != "" {
		t.Fatalf("choices = %+v %v", choices, err)
	}
}

func TestSetConnection_EveryConnectionKindIsRetiredAndChangesNothing(t *testing.T) {
	ctx := context.Background()
	svc, st, created := newUnconfigured(t, choiceConnections()...)
	app := created.Application.Key
	env, _ := st.GetEnvironment(ctx, app, "staging")
	for _, key := range []string{"lab", "cloud", "cloud-us"} {
		if _, err := svc.SetConnection(ctx, setCommand(env, "acme", app, "staging", key)); !errors.Is(err, ErrDeploymentConnectionRetired) {
			t.Fatalf("%s: a deployment connection must be retired: %v", key, err)
		}
	}
	after, _ := st.GetEnvironment(ctx, app, "staging")
	if after.Configured() || after.Version != env.Version {
		t.Fatalf("the retired write changed staging: %+v", after)
	}
}
