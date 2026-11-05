package deployment_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/domain/endpoint"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

func registerEndpoint(t *testing.T, app *bootstrap.App, opts seed.Options) {
	organizationKey, applicationKey := opts.OrganizationKey, opts.ApplicationKey
	t.Helper()
	ctx := context.Background()
	if err := app.Store.SaveResourceType(ctx, organizationKey, resource.Type{Key: "endpoint",
		Inputs:  []resource.InputField{{Name: "name", Type: "string", Required: true}, {Name: "port", Type: "string", Required: true}, {Name: "path", Type: "string"}},
		Outputs: []resource.OutputField{{Name: "hostname", Type: "string"}, {Name: "url", Type: "string"}}}); err != nil {
		t.Fatal(err)
	}
	if err := app.Store.SaveResourceDefinition(ctx, organizationKey, resource.Definition{Key: "public-endpoint", Name: "Public endpoint", Version: 1,
		ResourceTypeKey: "endpoint", DriverType: resource.DriverEndpoint, Criteria: []resource.Criterion{{ApplicationID: applicationKey}},
		DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"rootDomain": "apps.example.com", "scheme": "https", "gatewayName": "public", "gatewayNamespace": "gateway-system"}}}}); err != nil {
		t.Fatal(err)
	}
	// Tests changing Definitions publish a new fixture release to select them.
	if err := seedtest.PublishCatalog(ctx, app.Store, opts); err != nil {
		t.Fatal(err)
	}
}

func withEndpoint(workload map[string]any, name string) map[string]any {
	out := map[string]any{}
	for k, v := range workload {
		out[k] = v
	}
	resources := map[string]any{}
	if existing, ok := workload["resources"].(map[string]any); ok {
		for k, v := range existing {
			resources[k] = v
		}
	}
	resources["public"] = map[string]any{"type": "endpoint", "params": map[string]any{"name": name, "port": "http"}}
	out["resources"] = resources
	return out
}

func TestEndpointDeployReservesRoutesAndReleases(t *testing.T) {
	ctx := context.Background()
	app, opts := newApp(t)
	registerEndpoint(t, app, opts)
	deployAll(t, app, opts)
	repo, ok := app.Store.(persistence.EndpointRepository)
	if !ok {
		t.Fatal("store has no endpoint repository")
	}
	base := seed.AcceptanceWorkloads(opts)["frontend"]
	shop := withEndpoint(base, "shop")
	cmd := appsvc.DeployCommand{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, WorkloadID: "frontend", WorkloadBefore: base, WorkloadAfter: shop, Actor: "test"}
	if _, err := app.Deployments.DeployWorkload(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	env, err := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	owner := endpoint.Owner{Organization: opts.OrganizationKey, Application: opts.ApplicationKey, Environment: opts.EnvironmentKey, Generation: env.TargetGeneration, Workload: "frontend"}
	foreign := owner
	foreign.Workload = "backend"
	if err := repo.CheckEndpoint(ctx, "shop.apps.example.com", foreign); !errors.Is(err, persistence.ErrHostnameTaken) {
		t.Fatalf("hostname not reserved for frontend: %v", err)
	}
	last := app.FakeDeploy.Applied[len(app.FakeDeploy.Applied)-1]
	var route map[string]any
	for _, m := range last.Manifests {
		if m.Kind == "HTTPRoute" {
			route = m.Object
		}
	}
	if route == nil {
		t.Fatalf("HTTPRoute not delivered with workload manifests: %v", app.FakeDeploy.AppliedNames())
	}
	if hosts := route["spec"].(map[string]any)["hostnames"].([]any); len(hosts) != 1 || hosts[0] != "shop.apps.example.com" {
		t.Fatalf("route hostnames %v", hosts)
	}

	// Rename: the new name is claimed and the old one released after success.
	store := withEndpoint(base, "store")
	cmd.WorkloadBefore, cmd.WorkloadAfter = shop, store
	if _, err := app.Deployments.DeployWorkload(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	if err := repo.CheckEndpoint(ctx, "shop.apps.example.com", foreign); err != nil {
		t.Fatalf("old hostname still held after rename: %v", err)
	}
	if err := repo.CheckEndpoint(ctx, "store.apps.example.com", foreign); !errors.Is(err, persistence.ErrHostnameTaken) {
		t.Fatalf("new hostname not reserved: %v", err)
	}

	// Dropping the endpoint prunes its route and releases the hostname.
	cmd.WorkloadBefore, cmd.WorkloadAfter = store, base
	if _, err := app.Deployments.DeployWorkload(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	if err := repo.CheckEndpoint(ctx, "store.apps.example.com", foreign); err != nil {
		t.Fatalf("hostname still held after the endpoint was removed: %v", err)
	}
	pruned := false
	for _, ref := range app.FakeDeploy.RemovedObjects {
		pruned = pruned || strings.HasPrefix(ref, "HTTPRoute/endpoint-")
	}
	if !pruned {
		t.Fatalf("endpoint HTTPRoute not pruned: %v", app.FakeDeploy.RemovedObjects)
	}
}

func TestEndpointDeployRefusesHostnameHeldByAnotherOwner(t *testing.T) {
	ctx := context.Background()
	app, opts := newApp(t)
	registerEndpoint(t, app, opts)
	deployAll(t, app, opts)
	repo := app.Store.(persistence.EndpointRepository)
	env, err := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	other := endpoint.Owner{Organization: opts.OrganizationKey, Application: opts.ApplicationKey, Environment: opts.EnvironmentKey, Generation: env.TargetGeneration, Workload: "backend"}
	if err := repo.ReserveEndpoint(ctx, "shop.apps.example.com", other); err != nil {
		t.Fatal(err)
	}
	base := seed.AcceptanceWorkloads(opts)["frontend"]
	applied := len(app.FakeDeploy.Applied)
	_, err = app.Deployments.DeployWorkload(ctx, appsvc.DeployCommand{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, WorkloadID: "frontend", WorkloadBefore: base, WorkloadAfter: withEndpoint(base, "shop"), Actor: "test"})
	if err == nil {
		t.Fatal("deploy took a hostname reserved by another owner")
	}
	if len(app.FakeDeploy.Applied) != applied {
		t.Fatal("workload manifests were applied despite the reservation conflict")
	}
	if err := repo.CheckEndpoint(ctx, "shop.apps.example.com", other); err != nil {
		t.Fatalf("original owner lost its reservation: %v", err)
	}
}
