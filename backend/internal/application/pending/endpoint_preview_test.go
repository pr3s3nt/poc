package pending_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"orchestrator/internal/application/pending"
	"orchestrator/internal/domain/endpoint"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed/seedtest"
)

func TestPreviewShowsEndpointAndDeployReservesIt(t *testing.T) {
	ctx := context.Background()
	app, workloads, svc, opts := fixture(t)
	if err := app.Store.SaveResourceType(ctx, opts.OrganizationKey, resource.Type{Key: "endpoint",
		Inputs:  []resource.InputField{{Name: "name", Type: "string", Required: true}, {Name: "port", Type: "string", Required: true}, {Name: "path", Type: "string"}},
		Outputs: []resource.OutputField{{Name: "hostname", Type: "string"}, {Name: "url", Type: "string"}}}); err != nil {
		t.Fatal(err)
	}
	if err := app.Store.SaveResourceDefinition(ctx, opts.OrganizationKey, resource.Definition{Key: "public-endpoint", Name: "Public endpoint", Version: 1,
		ResourceTypeKey: "endpoint", DriverType: resource.DriverEndpoint, Criteria: []resource.Criterion{{ApplicationID: opts.ApplicationKey}},
		DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"rootDomain": "apps.example.com", "scheme": "https", "gatewayName": "public", "gatewayNamespace": "gateway-system"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := seedtest.PublishCatalog(ctx, app.Store, opts); err != nil {
		t.Fatal(err)
	}
	initial := scoreFor("backend", "example.invalid/backend:v1", false)
	deploy(t, app, opts, "backend", initial)
	withEndpoint := scoreFor("backend", "example.invalid/backend:v1", false)
	withEndpoint["resources"] = map[string]any{"public": map[string]any{"type": "endpoint", "params": map[string]any{"name": "shop", "port": "http"}}}
	if _, err := workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "backend", withEndpoint, 0); err != nil {
		t.Fatal(err)
	}

	// Another owner holding the hostname makes Preview fail with a fixed reason.
	repo := app.Store.(persistence.EndpointRepository)
	env, err := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	other := endpoint.Owner{Organization: opts.OrganizationKey, Application: opts.ApplicationKey, Environment: opts.EnvironmentKey, Generation: env.TargetGeneration, Workload: "other"}
	if err := repo.ReserveEndpoint(ctx, "shop.apps.example.com", other); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey); !errors.Is(err, pending.ErrInvalid) || !strings.Contains(err.Error(), "already reserved") {
		t.Fatalf("taken hostname not reported: %v", err)
	}
	if err := repo.RetainEndpoints(ctx, other, nil); err != nil {
		t.Fatal(err)
	}

	result, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Changes) != 1 || result.Changes[0].Endpoint == nil || result.Changes[0].Endpoint.URL != "https://shop.apps.example.com/" {
		t.Fatalf("endpoint missing from Preview: %+v", result.Changes)
	}
	// Preview never writes a reservation.
	if err := repo.CheckEndpoint(ctx, "shop.apps.example.com", other); err != nil {
		t.Fatalf("preview reserved the hostname: %v", err)
	}
	if _, err := svc.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", result.Token); err != nil {
		t.Fatal(err)
	}
	if err := repo.CheckEndpoint(ctx, "shop.apps.example.com", other); !errors.Is(err, persistence.ErrHostnameTaken) {
		t.Fatalf("deploy did not reserve the hostname: %v", err)
	}
}
