package seed_test

import (
	"context"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/seed"
)

func TestCatalogNoneSeedsOnlyTheOwnedNamespaceIdentity(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	o := seed.Defaults()
	o.Profile, o.Catalog = seed.ProfileAcceptanceV2, seed.CatalogNone
	o.NamespaceIdentity = "orch-v2-test-run1"
	if err := seed.Apply(ctx, st, o); err != nil {
		t.Fatal(err)
	}
	if defs, _ := st.ListResourceDefinitions(ctx, o.OrganizationKey); len(defs) != 0 {
		t.Fatalf("catalog-free seed registered %d Definitions", len(defs))
	}
	if types, _ := st.ListResourceTypes(ctx, o.OrganizationKey); len(types) != 0 {
		t.Fatalf("catalog-free seed registered %d Types", len(types))
	}
	if conns, _ := st.ListConnections(ctx, o.OrganizationKey); len(conns) != 0 {
		t.Fatalf("catalog-free seed registered %d Connections", len(conns))
	}
	env, err := st.GetEnvironment(ctx, o.ApplicationKey, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if env.NamespaceIdentity != "orch-v2-test-run1" || env.Configured() {
		t.Fatalf("environment %+v must carry the exact identity and stay unbound", env)
	}
	prod, err := st.GetEnvironment(ctx, o.ApplicationKey, "production")
	if err != nil || prod.NamespaceIdentity == env.NamespaceIdentity || prod.Configured() {
		t.Fatalf("production %+v %v", prod, err)
	}
	if _, err := st.GetApplication(ctx, o.CloudApplicationKey); err == nil {
		t.Fatal("catalog-free seed created the cloud Application")
	}
	if org, _ := st.GetOrganization(ctx, o.OrganizationKey); org.DefaultConnectionKey != "" {
		t.Fatalf("default connection marker %q", org.DefaultConnectionKey)
	}
	// Idempotent.
	if err := seed.Apply(ctx, st, o); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogModeCombinationsAreRefused(t *testing.T) {
	for name, o := range map[string]seed.Options{
		"none in production":           {Profile: "production", Catalog: seed.CatalogNone},
		"acceptance-v2 with a catalog": {Profile: seed.ProfileAcceptanceV2, Catalog: seed.CatalogTerraformK8s},
		"acceptance-v2 legacy":         {Profile: seed.ProfileAcceptanceV2},
		"unknown":                      {Profile: "local", Catalog: "other"},
	} {
		if err := o.Validate(); err == nil {
			t.Errorf("%s was accepted", name)
		}
		d := seed.Defaults()
		d.Profile, d.Catalog = o.Profile, o.Catalog
		if err := seed.Apply(context.Background(), store.New(), d); err == nil {
			t.Errorf("%s: Apply accepted it", name)
		}
	}
}

// The terraform-k8s catalog replaces exactly the legacy Kubernetes-driver
// namespace and PostgreSQL; every remaining Kubernetes-driver Definition is
// absent and the in-cluster ones bind the Kubernetes provider to the
// Environment target.
func TestTerraformK8sCatalogHasNoLegacyKubernetesDriver(t *testing.T) {
	o := seed.Defaults()
	o.Catalog = seed.CatalogTerraformK8s
	keys := map[string]resource.Definition{}
	for _, d := range seed.ResourceDefinitions(o) {
		if d.DriverType == resource.DriverKubernetes {
			t.Errorf("legacy Kubernetes driver Definition %q in the terraform-k8s catalog", d.Key)
		}
		keys[d.Key] = d
	}
	for _, want := range []string{"namespace-k8s-module", "postgres-k8s-module", "redis-k8s-module", "cluster-aws-eks", "vpc-aws", "postgres-aws-aurora"} {
		if _, ok := keys[want]; !ok {
			t.Errorf("missing Definition %q", want)
		}
	}
	for _, key := range []string{"namespace-k8s-module", "postgres-k8s-module", "redis-k8s-module"} {
		values := keys[key].DriverValues()
		bindings, _ := values["provider_bindings"].(map[string]any)
		k8s, _ := bindings["kubernetes"].(map[string]any)
		if k8s["target"] != "environment" {
			t.Errorf("%s binds the Kubernetes provider to %v", key, k8s)
		}
		src, _ := values["source"].(map[string]any)
		if src["contentSha256"] == nil && src["content_sha256"] == nil && len(src) == 0 {
			t.Errorf("%s has no verified source pin", key)
		}
	}
	types := map[string]bool{}
	for _, typ := range seed.ResourceTypesFor(o) {
		types[typ.Key] = true
	}
	if !types["redis"] {
		t.Error("terraform-k8s catalog lacks the redis Type")
	}
	legacy := seed.Defaults()
	var found bool
	for _, d := range seed.ResourceDefinitions(legacy) {
		found = found || (d.Key == "namespace-kubernetes" && d.DriverType == resource.DriverKubernetes)
	}
	if !found {
		t.Error("legacy catalog compatibility was lost")
	}
}
