package catalog_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/adapters/terraform"
	"orchestrator/internal/application/catalog"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/seed"
)

func seededCatalog(t *testing.T, inspector planning.ModuleInspector) (*store.Store, *catalog.Service, seed.Options) {
	t.Helper()
	ctx := context.Background()
	st := store.New()
	opts := seed.Defaults()
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	return st, catalog.NewService(st, inspector), opts
}

func seededDefinition(t *testing.T, opts seed.Options, key string) resource.Definition {
	t.Helper()
	for _, d := range seed.ResourceDefinitions(opts) {
		if d.Key == key {
			return d
		}
	}
	t.Fatalf("no seeded definition %s", key)
	return resource.Definition{}
}

// TestRegisterResourceDefinition_RejectsConnectionAndProfileMisuse covers
// UC-03 MS-04/BR-08 negative paths: nothing is stored for any of them.
func TestRegisterResourceDefinition_RejectsConnectionAndProfileMisuse(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, terraform.NewInspector())
	if err := st.SaveConnection(ctx, application.Connection{Key: "pending-cluster", OrganizationKey: opts.OrganizationKey, Kind: application.ConnectionKubernetes, Status: application.ConnectionVerifying}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveOrganization(ctx, application.Organization{Key: "globex", Name: "Globex"}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveConnection(ctx, application.Connection{Key: "globex-cluster", OrganizationKey: "globex", Kind: application.ConnectionKubernetes, Status: application.ConnectionReady}); err != nil {
		t.Fatal(err)
	}
	aurora := seededDefinition(t, opts, "postgres-aws-aurora")
	existing := seededDefinition(t, opts, "cluster-internal-registered")
	internal := seededDefinition(t, opts, "postgres-internal-statefulset")
	cases := map[string]func(*resource.Definition){
		"terraform with cluster connection":    func(d *resource.Definition) { *d = aurora; d.ConnectionKey = opts.ConnectionKey },
		"terraform without connection":         func(d *resource.Definition) { *d = aurora; d.ConnectionKey = "" },
		"existing-cluster with AWS connection": func(d *resource.Definition) { *d = existing; d.ConnectionKey = opts.CloudConnectionKey },
		"connection not READY":                 func(d *resource.Definition) { *d = internal; d.ConnectionKey = "pending-cluster" },
		"connection of another Organization":   func(d *resource.Definition) { *d = internal; d.ConnectionKey = "globex-cluster" },
		"unknown connection":                   func(d *resource.Definition) { *d = internal; d.ConnectionKey = "missing" },
		"kubernetes definition for aws-eks":    func(d *resource.Definition) { *d = internal; d.ExecutionProfile = "aws-eks" },
		"empty criteria list":                  func(d *resource.Definition) { *d = internal; d.Criteria = []resource.Criterion{} },
		"terraform for internal-k8s profile":   func(d *resource.Definition) { *d = aurora; d.ExecutionProfile = "internal-k8s" },
		"unsupported driver/type pair":         func(d *resource.Definition) { *d = internal; d.ResourceTypeKey = "vpc" },
	}
	before, _ := st.ListResourceDefinitions(ctx, opts.OrganizationKey)
	for name, mutate := range cases {
		var def resource.Definition
		mutate(&def)
		def.Key = "negative-" + strings.ReplaceAll(name, " ", "-")
		if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def); !errors.Is(err, catalog.ErrInvalid) {
			t.Errorf("%s: want ErrInvalid, got %v", name, err)
		}
	}
	after, _ := st.ListResourceDefinitions(ctx, opts.OrganizationKey)
	if len(after) != len(before) {
		t.Fatalf("rejected definitions were stored: %d -> %d", len(before), len(after))
	}
}

type failingInspector struct {
	planning.ModuleInspector
}

const inspectorSentinel = "sentinel-module-path-/home/secret"

func (failingInspector) Inspect(string) (planning.ModuleContract, error) {
	return planning.ModuleContract{}, errors.New("terraform: cannot read " + inspectorSentinel)
}

func TestRegisterResourceDefinition_InspectorFailureIsNotValidation(t *testing.T) {
	ctx := context.Background()
	_, svc, opts := seededCatalog(t, failingInspector{ModuleInspector: terraform.NewInspector()})
	def := seededDefinition(t, opts, "postgres-aws-aurora")
	def.Key = "aurora-inspect-fails"
	_, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def)
	if err == nil || errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("inspector failure must be internal, got %v", err)
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
