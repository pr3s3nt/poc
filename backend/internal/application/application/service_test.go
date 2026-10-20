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
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func newTestStore(t *testing.T, conns ...appdomain.Connection) *store.Store {
	t.Helper()
	ctx := context.Background()
	st := store.New()
	for _, org := range []appdomain.Organization{
		{ID: "org-acme", Key: "acme", Name: "Acme"},
		{ID: "org-globex", Key: "globex", Name: "Globex"},
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
		if env.Version != 1 || env.InfrastructureScope != environment.ScopeEnvironment {
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
