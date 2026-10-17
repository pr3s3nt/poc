package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/moduleregistry"
	"reflect"
	"strings"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/adapters/terraform/registry/registrytest"
	"orchestrator/internal/application/catalog"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

// registryFactory builds the module registry of a test catalog; nil means
// the catalog has none.
type registryFactory func(t *testing.T, st *store.Store) moduleregistry.Registry

// withRegistry is the real packaged-only registry prepared into st.
func withRegistry(t *testing.T, st *store.Store) moduleregistry.Registry {
	return registrytest.Open(t, st)
}

func seededCatalog(t *testing.T, modules registryFactory) (*store.Store, *catalog.Service, seed.Options) {
	t.Helper()
	ctx := context.Background()
	st := store.New()
	opts := seedtest.Defaults()
	opts.Region, opts.AccountID = "us-east-1", "000000000000"
	if err := seedtest.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	if modules == nil {
		return st, catalog.NewService(st), opts
	}
	return st, catalog.NewService(st, modules(t, st)), opts
}

func seededDefinition(t *testing.T, opts seed.Options, key string) resource.Definition {
	t.Helper()
	for _, d := range seed.ResourceDefinitions(opts) {
		if d.Key == key {
			return d
		}
	}
	if key == "cluster-internal-registered" {
		// Historical authored existing-cluster shape (no longer seeded, ADR-013);
		// public registration rejects it for every key.
		return resource.Definition{
			Key: key, Name: key, Version: 1, ResourceTypeKey: "k8s-cluster", DriverType: resource.DriverExistingCluster, DriverAccount: opts.ConnectionKey,
			DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{
				"name": "${context.connection.cluster}", "kubeContext": "${context.connection.context}"}}},
			Criteria: []resource.Criterion{{Class: "internal"}},
		}
	}
	t.Fatalf("no seeded definition %s", key)
	return resource.Definition{}
}

// UC-03 V02-03 negative paths: nothing is stored for any of them, and the
// failing field is the driver account or region, never an echoed value.
func TestRegisterResourceDefinition_RejectsAccountMisuse(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, withRegistry)
	if err := st.SaveConnection(ctx, application.Connection{Name: "pending-aws", AuthenticationType: application.AuthAWSAccessKey, Key: "pending-aws", OrganizationKey: opts.OrganizationKey, Kind: application.ConnectionAWS, Status: application.ConnectionVerifying, Version: 1}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveOrganization(ctx, application.Organization{Key: "globex", Name: "Globex"}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveConnection(ctx, application.Connection{Name: "globex-aws", AuthenticationType: application.AuthAWSAccessKey, Key: "globex-aws", OrganizationKey: "globex", Kind: application.ConnectionAWS, Status: application.ConnectionReady, Version: 1}); err != nil {
		t.Fatal(err)
	}
	aurora := seededDefinition(t, opts, "postgres-aws-aurora")
	vars := func(d *resource.Definition) map[string]any {
		return d.DriverInputs["values"].(map[string]any)["variables"].(map[string]any)
	}
	cases := map[string]struct {
		change func(*resource.Definition)
		field  string
	}{
		"no account":                      {func(d *resource.Definition) { d.DriverAccount = "" }, "definition.driverAccount"},
		"internal (cluster) account":      {func(d *resource.Definition) { d.DriverAccount = opts.ConnectionKey }, "definition.driverAccount"},
		"account not READY":               {func(d *resource.Definition) { d.DriverAccount = "pending-aws" }, "definition.driverAccount"},
		"account of another Organization": {func(d *resource.Definition) { d.DriverAccount = "globex-aws" }, "definition.driverAccount"},
		"unknown account":                 {func(d *resource.Definition) { d.DriverAccount = "missing" }, "definition.driverAccount"},
		"missing region":                  {func(d *resource.Definition) { delete(vars(d), "region") }, "definition.driverInputs.values.variables.region"},
		"placeholder region":              {func(d *resource.Definition) { vars(d)["region"] = "${context.env.region}" }, "definition.driverInputs.values.variables.region"},
		"malformed region":                {func(d *resource.Definition) { vars(d)["region"] = "mars-1" }, "definition.driverInputs.values.variables.region"},
		"environment binding": {func(d *resource.Definition) {
			d.DriverInputs["values"].(map[string]any)["provider_bindings"] = map[string]any{"aws": map[string]any{"target": "environment"}}
		}, "definition.driverInputs.values.provider_bindings"},
		"empty criteria list":   {func(d *resource.Definition) { d.Criteria = []resource.Criterion{} }, "definition.schema"},
		"blank name":            {func(d *resource.Definition) { d.Name = "   " }, "definition.name"},
		"unsupported type pair": {func(d *resource.Definition) { d.ResourceTypeKey = "vpc" }, ""},
	}
	before, _ := st.ListResourceDefinitions(ctx, opts.OrganizationKey)
	for name, tc := range cases {
		def := aurora
		def.DriverInputs = cloneInputs(aurora.DriverInputs)
		tc.change(&def)
		// Keys satisfy UC-03 BR-10 so each case fails for its own reason.
		def.Key = "negative-" + strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
				return r
			}
			return '-'
		}, strings.ToLower(name))
		_, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def)
		var fieldErr *catalog.FieldError
		if !errors.Is(err, catalog.ErrInvalid) {
			t.Errorf("%s: want ErrInvalid, got %v", name, err)
		} else if tc.field != "" && (!errors.As(err, &fieldErr) || fieldErr.Field != tc.field) {
			t.Errorf("%s: field = %v, want %s (err %v)", name, fieldErr, tc.field, err)
		}
	}
	after, _ := st.ListResourceDefinitions(ctx, opts.OrganizationKey)
	if len(after) != len(before) {
		t.Fatalf("rejected definitions were stored: %d -> %d", len(before), len(after))
	}
}

// cloneInputs deep-copies driver inputs so a mutating case never leaks into the
// next one.
func cloneInputs(in map[string]any) map[string]any {
	raw, _ := json.Marshal(in)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}

// UC-03 V02-03: AWS Terraform Definitions (VPC, EKS, Aurora, ElastiCache) all
// register with their own READY AWS account and a literal region, without any
// Environment; Kubernetes-bound modules take no account.
func TestRegisterResourceDefinition_AWSDefinitionsTakeTheirOwnAccountAndRegion(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, withRegistry)
	if err := st.SaveConnection(ctx, application.Connection{Name: "second", AuthenticationType: application.AuthAWSAccessKey, Key: "second-aws", OrganizationKey: opts.OrganizationKey, Kind: application.ConnectionAWS, Status: application.ConnectionReady, Version: 1}); err != nil {
		t.Fatal(err)
	}
	for _, seededKey := range []string{"vpc-aws", "cluster-aws-eks", "postgres-aws-aurora"} {
		base := seededDefinition(t, opts, seededKey)
		if base.DriverAccount != opts.CloudConnectionKey {
			t.Fatalf("seeded %s account = %q", seededKey, base.DriverAccount)
		}
		for i, account := range []string{opts.CloudConnectionKey, "second-aws"} {
			def := base
			def.DriverInputs = cloneInputs(base.DriverInputs)
			def.DriverInputs["values"].(map[string]any)["variables"].(map[string]any)["region"] = []string{"us-east-1", "eu-west-1"}[i]
			def.Key = "registered-" + seededKey + "-" + strings.ToLower(account)
			def.DriverAccount = account
			def.Criteria = []resource.Criterion{{ApplicationID: def.Key}}
			created, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def)
			if err != nil {
				t.Fatalf("%s with %s: %v", seededKey, account, err)
			}
			if created.Version != 1 || created.DriverAccount != account {
				t.Fatalf("created = %+v", created)
			}
		}
	}
	// A Kubernetes-bound module takes no account (cluster access is inherited).
	kube := moduleShape(t, "namespace")
	kube.Key, kube.DriverAccount = "namespace-with-account", opts.ConnectionKey
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, kube); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("an account on a Kubernetes-bound module: %v", err)
	}
}

type failingRegistry struct {
	moduleregistry.Registry
}

const inspectorSentinel = "sentinel-module-path-/home/secret"

func (failingRegistry) Verify(context.Context, tfmodule.SourceRef) (tfmodule.Bundle, error) {
	return tfmodule.Bundle{}, errors.New("registry: cannot read " + inspectorSentinel)
}

// An unexpected registry failure is a platform error: never a validation
// result and never echoing paths.
func TestRegisterResourceDefinition_InspectorFailureIsNotValidation(t *testing.T) {
	ctx := context.Background()
	_, svc, opts := seededCatalog(t, func(t *testing.T, st *store.Store) moduleregistry.Registry {
		return failingRegistry{registrytest.Open(t, st)}
	})
	def := seededDefinition(t, opts, "postgres-aws-aurora")
	def.Key = "aurora-inspect-fails"
	_, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def)
	if err == nil || errors.Is(err, catalog.ErrInvalid) || strings.Contains(err.Error(), inspectorSentinel) {
		t.Fatalf("registry failure must be a safe internal error, got %v", err)
	}
}

func TestRegisterResourceType_DuplicateKeepsFirstContract(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, nil)
	first := resource.Type{Key: "cache", Inputs: []resource.InputField{{Name: "size", Type: "number"}}, Outputs: []resource.OutputField{{Name: "host", Type: "string"}}}
	if _, err := svc.RegisterResourceType(ctx, opts.OrganizationKey, first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.Inputs = nil
	if _, err := svc.RegisterResourceType(ctx, opts.OrganizationKey, second); !errors.Is(err, catalog.ErrDuplicate) {
		t.Fatalf("duplicate: %v", err)
	}
	types, _ := st.ListResourceTypes(ctx, opts.OrganizationKey)
	for _, typ := range types {
		if typ.Key == "cache" && len(typ.Inputs) != 1 {
			t.Fatal("duplicate registration replaced the first contract")
		}
	}
}

// ADR-013/UC-03 BR-08 (T17): public registration rejects driver existing-cluster
// for every key and shape, and the reserved system key for every driver. Nothing
// is inserted or overwritten.
func TestRegisterResourceDefinition_RejectsExistingClusterDriver(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, withRegistry)
	base := seededDefinition(t, opts, "cluster-internal-registered")
	with := func(mutate func(*resource.Definition)) resource.Definition {
		d := base
		mutate(&d)
		return d
	}
	cases := map[string]resource.Definition{
		"arbitrary key":      with(func(d *resource.Definition) { d.Key = "cluster-custom" }),
		"reserved key":       with(func(d *resource.Definition) { d.Key = planning.BuiltinClusterKey }),
		"valid connection":   with(func(d *resource.Definition) { d.Key = "cluster-valid"; d.DriverAccount = opts.ConnectionKey }),
		"no connection":      with(func(d *resource.Definition) { d.Key = "cluster-noconn"; d.DriverAccount = "" }),
		"unknown connection": with(func(d *resource.Definition) { d.Key = "cluster-unknown"; d.DriverAccount = "missing" }),
		"AWS connection":     with(func(d *resource.Definition) { d.Key = "cluster-aws"; d.DriverAccount = opts.CloudConnectionKey }),
		"no criteria":        with(func(d *resource.Definition) { d.Key = "cluster-nocrit"; d.Criteria = nil }),
		"invalid key":        with(func(d *resource.Definition) { d.Key = "Not A Key" }),
	}
	before, _ := st.ListResourceDefinitions(ctx, opts.OrganizationKey)
	for name, def := range cases {
		if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def); !errors.Is(err, catalog.ErrInvalid) {
			t.Errorf("%s: want ErrInvalid, got %v", name, err)
		}
	}
	// Reserved key stays protected for every other driver.
	pg := seededDefinition(t, opts, "postgres-internal-statefulset")
	pg.Key = planning.BuiltinClusterKey
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, pg); !errors.Is(err, catalog.ErrInvalid) {
		t.Errorf("reserved key with kubernetes driver: want ErrInvalid, got %v", err)
	}
	after, _ := st.ListResourceDefinitions(ctx, opts.OrganizationKey)
	if len(after) != len(before) {
		t.Fatalf("rejected definitions were stored: %d -> %d", len(before), len(after))
	}
	for i := range before {
		if !reflect.DeepEqual(before[i], after[i]) {
			t.Fatalf("existing definition %s changed", before[i].Key)
		}
	}
}
