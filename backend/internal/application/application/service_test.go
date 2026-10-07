package application

import (
	"context"
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

func TestCreateApplication_UsesOrganizationDefaultTarget(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t,
		appdomain.Connection{ID: "c1", Key: "default", OrganizationKey: "acme", Kind: appdomain.ConnectionAWS, Status: appdomain.ConnectionReady, Config: map[string]any{"region": "us-east-1"}},
		appdomain.Connection{ID: "c2", Key: "default", OrganizationKey: "globex", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionReady},
	)
	svc := NewService(st)
	aws, err := svc.Create(ctx, CreateCommand{OrganizationKey: "acme", Name: "Payment", Subdomain: "Payment"})
	if err != nil {
		t.Fatal(err)
	}
	app := aws.Application
	if app.OrganizationKey != "acme" || app.Profile != appdomain.ProfileAWSEKS || app.ConnectionKey != "default" || app.Region != "us-east-1" || app.RuntimeStatus != appdomain.RuntimePending {
		t.Fatalf("aws-eks application = %+v", app)
	}
	if !uuidPattern.MatchString(app.ID) || app.Key != app.ID || app.Subdomain != "payment" {
		t.Fatalf("generated identity/subdomain = %q %q %q", app.ID, app.Key, app.Subdomain)
	}
	internal, err := svc.Create(ctx, CreateCommand{OrganizationKey: "globex", Name: "Payment", Subdomain: "globex-payment"})
	if err != nil {
		t.Fatal(err)
	}
	if internal.Application.Profile != appdomain.ProfileInternalK8s || internal.Application.RuntimeStatus != appdomain.RuntimeReady || internal.Application.ID == app.ID {
		t.Fatalf("internal-k8s application = %+v", internal.Application)
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

func TestCreateApplication_RequiresReadyDefaultTargetAndPersistsNothing(t *testing.T) {
	ctx := context.Background()
	for _, conn := range []appdomain.Connection{
		{ID: "c1", Key: "default", OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionVerifying},
		{ID: "c1", Key: "default", OrganizationKey: "acme", Kind: appdomain.ConnectionAWS, Status: appdomain.ConnectionReady},
	} {
		st := newTestStore(t, conn)
		if _, err := NewService(st).Create(ctx, CreateCommand{OrganizationKey: "acme", Name: "Catalog", Subdomain: "catalog"}); !errors.Is(err, ErrTargetNotReady) {
			t.Fatalf("%s/%s: err = %v", conn.Kind, conn.Status, err)
		}
		if apps, _ := st.ListApplications(ctx); len(apps) != 0 {
			t.Fatalf("failed create must persist nothing: %d applications", len(apps))
		}
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

func ptr(s string) *string { return &s }

func TestCreateApplication_ExplicitConnectionSelectionBindsAllEnvironments(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t,
		appdomain.Connection{ID: "c1", Key: "default", OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionReady},
		appdomain.Connection{ID: "c2", Key: "lab", Name: "Lab", OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionReady},
		appdomain.Connection{ID: "c3", Key: "cloud", OrganizationKey: "acme", Kind: appdomain.ConnectionAWS, Status: appdomain.ConnectionReady, Config: map[string]any{"region": "eu-west-1"}},
		appdomain.Connection{ID: "c4", Key: "foreign", OrganizationKey: "globex", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionReady},
	)
	svc := NewService(st)
	legacy, err := svc.Create(ctx, CreateCommand{OrganizationKey: "acme", Name: "Legacy", Subdomain: "legacy"})
	if err != nil || legacy.Application.ConnectionKey != "default" {
		t.Fatalf("omission must keep the default: %+v %v", legacy.Application, err)
	}
	lab, err := svc.Create(ctx, CreateCommand{OrganizationKey: "acme", Name: "Lab", Subdomain: "lab", ConnectionKey: ptr(" lab ")})
	if err != nil || lab.Application.ConnectionKey != "lab" || lab.Application.Profile != appdomain.ProfileInternalK8s || len(lab.Environments) != 2 {
		t.Fatalf("explicit lab: %+v %v", lab, err)
	}
	stored, err := st.GetApplication(ctx, lab.Application.Key)
	if err != nil || stored.ConnectionKey != "lab" {
		t.Fatalf("persisted binding: %+v %v", stored, err)
	}
	cloud, err := svc.Create(ctx, CreateCommand{OrganizationKey: "acme", Name: "Cloud", Subdomain: "cloud", ConnectionKey: ptr("cloud")})
	if err != nil || cloud.Application.Profile != appdomain.ProfileAWSEKS || cloud.Application.Region != "eu-west-1" || cloud.Application.RuntimeStatus != appdomain.RuntimePending {
		t.Fatalf("aws derivation: %+v %v", cloud.Application, err)
	}
	// Changing the default later does not move a stored binding.
	if err := st.SaveOrganization(ctx, appdomain.Organization{ID: "org-acme", Key: "acme", Name: "Acme", DefaultConnectionKey: "lab"}); err != nil {
		t.Fatal(err)
	}
	if again, _ := st.GetApplication(ctx, legacy.Application.Key); again.ConnectionKey != "default" {
		t.Fatalf("default change moved binding: %+v", again)
	}
}

func TestCreateApplication_RejectsUnavailableOrBlankConnectionWithoutFallback(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t,
		appdomain.Connection{ID: "c1", Key: "default", OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionReady},
		appdomain.Connection{ID: "c2", Key: "verifying", OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionVerifying},
		appdomain.Connection{ID: "c3", Key: "rejected", OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionRejected},
		appdomain.Connection{ID: "c4", Key: "gcp", OrganizationKey: "acme", Kind: appdomain.ConnectionKind("GCP"), Status: appdomain.ConnectionReady},
		appdomain.Connection{ID: "c5", Key: "noregion", OrganizationKey: "acme", Kind: appdomain.ConnectionAWS, Status: appdomain.ConnectionReady},
		appdomain.Connection{ID: "c6", Key: "foreign", OrganizationKey: "globex", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionReady},
	)
	svc := NewService(st)
	for _, key := range []string{"verifying", "rejected", "gcp", "noregion", "foreign", "missing"} {
		_, err := svc.Create(ctx, CreateCommand{OrganizationKey: "acme", Name: "App " + key, Subdomain: "app-" + key, ConnectionKey: ptr(key)})
		var field *FieldError
		if !errors.Is(err, ErrTargetNotReady) || !errors.As(err, &field) || field.Field != "connectionKey" || strings.Contains(err.Error(), key) {
			t.Fatalf("%s: %v", key, err)
		}
	}
	for _, key := range []string{"", "  "} {
		_, err := svc.Create(ctx, CreateCommand{OrganizationKey: "acme", Name: "Blank", Subdomain: "blank", ConnectionKey: ptr(key)})
		var field *FieldError
		if !errors.Is(err, ErrInvalid) || !errors.As(err, &field) || field.Field != "connectionKey" {
			t.Fatalf("blank %q: %v", key, err)
		}
	}
	apps, _ := st.ListApplications(ctx)
	if len(apps) != 0 {
		t.Fatalf("rejected requests created applications: %v", apps)
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
