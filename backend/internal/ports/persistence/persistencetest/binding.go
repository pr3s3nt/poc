package persistencetest

import (
	"context"
	"errors"
	"sync"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

// BindingFixture identifies what EnvironmentBinding created so adapter tests
// can reopen the store and prove the bindings survived a restart.
type BindingFixture struct {
	OrganizationKey string
	ApplicationKey  string
	Staging         string
	Production      string
}

func bind(app, env, connection string, profile application.ExecutionProfile, region string, status application.RuntimeStatus, version int64) persistence.EnvironmentBinding {
	return persistence.EnvironmentBinding{
		ApplicationKey: app, EnvironmentKey: env, ConnectionKey: connection, Profile: profile, Region: region,
		RuntimeStatus: status, Scope: environment.ScopeEnvironment, ExpectedVersion: version,
	}
}

// EnvironmentBinding is the adapter-neutral contract of the set-once
// Environment execution binding (ADR-011): a new Application is unbound,
// BindEnvironment is atomic and set-once, Save cannot overwrite it and
// runtime status updates stay legal.
func EnvironmentBinding(t *testing.T, st persistence.Store) BindingFixture {
	t.Helper()
	ctx := context.Background()
	org := "bindorg"
	if err := st.SaveOrganization(ctx, application.Organization{ID: ids.New(), Key: org, Name: "Bind", DefaultConnectionKey: "lab"}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []application.Connection{
		{ID: ids.New(), Key: "lab", Name: "Lab", OrganizationKey: org, Kind: application.ConnectionKubernetes, AuthenticationType: application.AuthHostContext, Status: application.ConnectionReady, Config: map[string]any{}, SecretRef: "host-kube-context://lab", Verification: map[string]any{}},
		{ID: ids.New(), Key: "cloud", Name: "Cloud", OrganizationKey: org, Kind: application.ConnectionAWS, AuthenticationType: application.AuthAWSAccessKey, Status: application.ConnectionReady, Config: map[string]any{"region": "eu-west-1"}, SecretRef: "secret://connections/cloud", Verification: map[string]any{}},
	} {
		if err := st.SaveConnection(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	appID := ids.New()
	app := application.Application{ID: appID, Key: appID, OrganizationKey: org, Name: "Bound", Subdomain: "bound-" + appID[:8], Version: 1}
	if err := st.SaveApplication(ctx, app); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"staging", "production"} {
		env := environment.Environment{ID: ids.New(), Key: key, ApplicationID: appID, ApplicationKey: appID, Name: key, Type: key, NamespaceIdentity: "app-" + appID + "-" + key, Version: 1}
		if err := st.SaveEnvironment(ctx, env); err != nil {
			t.Fatal(err)
		}
	}

	// New Application and Environments are unbound; the legacy columns stay empty.
	storedApp, err := st.GetApplication(ctx, appID)
	if err != nil || storedApp.ConnectionKey != "" || storedApp.Profile != "" || storedApp.Region != "" || storedApp.RuntimeStatus != application.RuntimeUnconfigured {
		t.Fatalf("new application must be unbound: %+v %v", storedApp, err)
	}
	staging, err := st.GetEnvironment(ctx, appID, "staging")
	if err != nil || staging.Configured() || staging.Status() != application.RuntimeUnconfigured || staging.Version != 1 {
		t.Fatalf("new environment must be UNCONFIGURED: %+v %v", staging, err)
	}
	// A save carrying a target never binds (insert path and update path).
	hostile := staging
	hostile.ConnectionKey, hostile.Profile, hostile.RuntimeStatus = "lab", application.ProfileInternalK8s, application.RuntimeReady
	if err := st.SaveEnvironment(ctx, hostile); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.GetEnvironment(ctx, appID, "staging"); got.Configured() {
		t.Fatalf("save bound the environment: %+v", got)
	}

	// Rejections do not mutate.
	if _, err := st.BindEnvironment(ctx, bind(appID, "staging", "lab", application.ProfileInternalK8s, "", application.RuntimeReady, 9)); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("stale version: %v", err)
	}
	if _, err := st.BindEnvironment(ctx, bind(appID, "staging", "missing", application.ProfileInternalK8s, "", application.RuntimeReady, 1)); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("missing connection: %v", err)
	}
	if _, err := st.BindEnvironment(ctx, bind(appID, "nope", "lab", application.ProfileInternalK8s, "", application.RuntimeReady, 1)); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("missing environment: %v", err)
	}
	if got, _ := st.GetEnvironment(ctx, appID, "staging"); got.Configured() || got.Version != 1 {
		t.Fatalf("rejected binds mutated: %+v", got)
	}

	// Set once, version +1, only that Environment.
	bound, err := st.BindEnvironment(ctx, bind(appID, "staging", "lab", application.ProfileInternalK8s, "", application.RuntimeReady, 1))
	if err != nil || bound.ConnectionKey != "lab" || bound.Profile != application.ProfileInternalK8s || bound.Version != 2 || bound.InfrastructureScope != environment.ScopeEnvironment {
		t.Fatalf("bind = %+v %v", bound, err)
	}
	if other, _ := st.GetEnvironment(ctx, appID, "production"); other.Configured() || other.Version != 1 {
		t.Fatalf("other environment changed: %+v", other)
	}
	// Repeat (same key, other key, stale version) is always already-configured.
	for _, key := range []string{"lab", "cloud"} {
		if _, err := st.BindEnvironment(ctx, bind(appID, "staging", key, application.ProfileInternalK8s, "", application.RuntimeReady, 2)); !errors.Is(err, persistence.ErrBindingConfigured) {
			t.Fatalf("repeat %s: %v", key, err)
		}
	}
	if _, err := st.BindEnvironment(ctx, bind(appID, "staging", "cloud", application.ProfileAWSEKS, "eu-west-1", application.RuntimePending, 1)); !errors.Is(err, persistence.ErrBindingConfigured) {
		t.Fatalf("stale repeat: %v", err)
	}

	// Save with a stale unset copy and with a hostile target cannot overwrite.
	for _, copyOf := range []environment.Environment{staging, func() environment.Environment {
		e := bound
		e.ConnectionKey, e.Profile, e.Region, e.InfrastructureScope = "cloud", application.ProfileAWSEKS, "eu-west-1", environment.ScopeLegacyApplication
		return e
	}()} {
		if err := st.SaveEnvironment(ctx, copyOf); err != nil {
			t.Fatal(err)
		}
		got, _ := st.GetEnvironment(ctx, appID, "staging")
		if got.ConnectionKey != "lab" || got.Profile != application.ProfileInternalK8s || got.Region != "" || got.InfrastructureScope != environment.ScopeEnvironment || got.RuntimeStatus != application.RuntimeReady {
			t.Fatalf("save overwrote the binding: %+v", got)
		}
	}
	// SaveApplication cannot attach a legacy target either.
	app.ConnectionKey, app.Profile, app.Region, app.RuntimeStatus = "cloud", application.ProfileAWSEKS, "eu-west-1", application.RuntimeReady
	if err := st.SaveApplication(ctx, app); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.GetApplication(ctx, appID); got.ConnectionKey != "" || got.Profile != "" || got.Region != "" {
		t.Fatalf("save attached a legacy application target: %+v", got)
	}

	// Concurrent binds of the unset production Environment: exactly one wins.
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < cap(results); i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key, profile, region, status := "lab", application.ExecutionProfile(application.ProfileInternalK8s), "", application.RuntimeReady
			if i%2 == 1 {
				key, profile, region, status = "cloud", application.ProfileAWSEKS, "eu-west-1", application.RuntimePending
			}
			_, err := st.BindEnvironment(ctx, bind(appID, "production", key, profile, region, status, 1))
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, persistence.ErrBindingConfigured), errors.Is(err, persistence.ErrVersionConflict):
		default:
			t.Fatalf("concurrent bind: %v", err)
		}
	}
	production, _ := st.GetEnvironment(ctx, appID, "production")
	if wins != 1 || !production.Configured() || production.Version != 2 {
		t.Fatalf("wins=%d production=%+v", wins, production)
	}

	// Runtime status stays writable for a configured AWS Environment only.
	if err := st.UpdateRuntimeStatus(ctx, appID, "production", application.RuntimeReady); err != nil {
		t.Fatal(err)
	}
	if production.Profile == application.ProfileAWSEKS {
		if got, _ := st.GetEnvironment(ctx, appID, "production"); got.RuntimeStatus != application.RuntimeReady || got.ConnectionKey != production.ConnectionKey || got.Version != 2 {
			t.Fatalf("runtime update changed more: %+v", got)
		}
	}
	if err := st.UpdateRuntimeStatus(ctx, appID, "production", application.RuntimeUnconfigured); err == nil {
		t.Fatal("runtime status must not return to UNCONFIGURED")
	}
	return BindingFixture{OrganizationKey: org, ApplicationKey: appID, Staging: "lab", Production: production.ConnectionKey}
}

// AssertBindingReloaded checks the bindings after the adapter was reopened.
func AssertBindingReloaded(t *testing.T, st persistence.Store, fx BindingFixture) {
	t.Helper()
	ctx := context.Background()
	staging, err := st.GetEnvironment(ctx, fx.ApplicationKey, "staging")
	if err != nil || staging.ConnectionKey != fx.Staging || staging.Version != 2 || staging.Profile != application.ProfileInternalK8s {
		t.Fatalf("staging after reopen: %+v %v", staging, err)
	}
	production, err := st.GetEnvironment(ctx, fx.ApplicationKey, "production")
	if err != nil || production.ConnectionKey != fx.Production || production.Version != 2 {
		t.Fatalf("production after reopen: %+v %v", production, err)
	}
	app, err := st.GetApplication(ctx, fx.ApplicationKey)
	if err != nil || app.ConnectionKey != "" || app.RuntimeStatus != application.RuntimeUnconfigured {
		t.Fatalf("application after reopen must stay unbound: %+v %v", app, err)
	}
}
