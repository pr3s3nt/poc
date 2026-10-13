package catalog_test

import (
	"context"
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
	opts := seed.Defaults()
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
			Key: key, ResourceTypeKey: "k8s-cluster", DriverType: resource.DriverExistingCluster, ConnectionKey: opts.ConnectionKey,
			DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{
				"name": "${context.connection.cluster}", "kubeContext": "${context.connection.context}"}}},
			Criteria: []resource.Criterion{{Class: "internal"}},
		}
	}
	t.Fatalf("no seeded definition %s", key)
	return resource.Definition{}
}

// TestRegisterResourceDefinition_RejectsConnectionAndProfileMisuse covers
// UC-03 MS-04/BR-08 negative paths: nothing is stored for any of them.
func TestRegisterResourceDefinition_RejectsConnectionAndProfileMisuse(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, withRegistry)
	if err := st.SaveConnection(ctx, application.Connection{Name: "pending-cluster", AuthenticationType: application.AuthKubeconfig, Key: "pending-cluster", OrganizationKey: opts.OrganizationKey, Kind: application.ConnectionKubernetes, Status: application.ConnectionVerifying}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveOrganization(ctx, application.Organization{Key: "globex", Name: "Globex"}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveConnection(ctx, application.Connection{Name: "globex-cluster", AuthenticationType: application.AuthKubeconfig, Key: "globex-cluster", OrganizationKey: "globex", Kind: application.ConnectionKubernetes, Status: application.ConnectionReady}); err != nil {
		t.Fatal(err)
	}
	aurora := seededDefinition(t, opts, "postgres-aws-aurora")
	internal := seededDefinition(t, opts, "postgres-internal-statefulset")
	cases := map[string]func(*resource.Definition){
		"terraform with cluster connection":  func(d *resource.Definition) { *d = aurora; d.ConnectionKey = opts.ConnectionKey },
		"terraform with AWS account":         func(d *resource.Definition) { *d = aurora; d.ConnectionKey = opts.CloudConnectionKey },
		"connection not READY":               func(d *resource.Definition) { *d = internal; d.ConnectionKey = "pending-cluster" },
		"connection of another Organization": func(d *resource.Definition) { *d = internal; d.ConnectionKey = "globex-cluster" },
		"unknown connection":                 func(d *resource.Definition) { *d = internal; d.ConnectionKey = "missing" },
		"kubernetes definition for aws-eks":  func(d *resource.Definition) { *d = internal; d.ExecutionProfile = "aws-eks" },
		"empty criteria list":                func(d *resource.Definition) { *d = internal; d.Criteria = []resource.Criterion{} },
		"terraform for internal-k8s profile": func(d *resource.Definition) { *d = aurora; d.ExecutionProfile = "internal-k8s" },
		"unsupported driver/type pair":       func(d *resource.Definition) { *d = internal; d.ResourceTypeKey = "vpc" },
	}
	before, _ := st.ListResourceDefinitions(ctx, opts.OrganizationKey)
	for name, mutate := range cases {
		var def resource.Definition
		mutate(&def)
		// Keys satisfy UC-03 BR-10 so each case fails for its own reason.
		def.Key = "negative-" + strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(name, " ", "-"), "/", "-"))
		if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def); !errors.Is(err, catalog.ErrInvalid) {
			t.Errorf("%s: want ErrInvalid, got %v", name, err)
		}
	}
	after, _ := st.ListResourceDefinitions(ctx, opts.OrganizationKey)
	if len(after) != len(before) {
		t.Fatalf("rejected definitions were stored: %d -> %d", len(before), len(after))
	}
}

// UC-03 BR-08 (T16): Terraform vpc/eks register with an empty ConnectionKey and
// without any Environment; every nonempty key is rejected before any insert.
func TestRegisterResourceDefinition_TerraformVPCAndEKSRequireEmptyConnection(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, withRegistry)
	if err := st.SaveOrganization(ctx, application.Organization{Key: "globex", Name: "Globex"}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveConnection(ctx, application.Connection{Name: "globex-aws", AuthenticationType: application.AuthAWSAccessKey, Key: "globex-aws", OrganizationKey: "globex", Kind: application.ConnectionAWS, Status: application.ConnectionReady}); err != nil {
		t.Fatal(err)
	}
	for _, seededKey := range []string{"vpc-aws", "cluster-aws-eks"} {
		base := seededDefinition(t, opts, seededKey)
		if base.ConnectionKey != "" {
			t.Fatalf("seeded %s must not carry a connection: %q", seededKey, base.ConnectionKey)
		}
		base.ExecutionProfile = "aws-eks"
		ok := base
		ok.Key = "registered-" + seededKey
		ok.Criteria = []resource.Criterion{{ApplicationID: "registered"}}
		if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, ok); err != nil {
			t.Fatalf("%s with an empty key: %v", seededKey, err)
		}
		for name, key := range map[string]string{
			"equal to an Environment target": opts.CloudConnectionKey,
			"nonexistent":                    "missing",
			"other Organization":             "globex-aws",
			"Kubernetes connection":          opts.ConnectionKey,
		} {
			bad := base
			bad.Key = "rejected-" + seededKey + "-" + strings.ToLower(strings.ReplaceAll(name, " ", "-"))
			bad.ConnectionKey = key
			before, _ := st.ListResourceDefinitions(ctx, opts.OrganizationKey)
			_, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, bad)
			if !errors.Is(err, catalog.ErrInvalid) || !strings.Contains(err.Error(), "leave connectionKey empty") || strings.Contains(err.Error(), key) {
				t.Fatalf("%s %s: err = %v", seededKey, name, err)
			}
			after, _ := st.ListResourceDefinitions(ctx, opts.OrganizationKey)
			if len(after) != len(before) {
				t.Fatalf("%s %s: definition was inserted", seededKey, name)
			}
		}
	}
}

// UC-03 BR-08 (v2): Aurora, like every AWS module, uses the Environment AWS
// Connection; an explicit account override is rejected before any insert.
func TestRegisterResourceDefinition_AuroraUsesEnvironmentConnection(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, withRegistry)
	if err := st.SaveConnection(ctx, application.Connection{Name: "aurora-account", AuthenticationType: application.AuthAWSAccessKey, Key: "aurora-account", OrganizationKey: opts.OrganizationKey, Kind: application.ConnectionAWS, Status: application.ConnectionReady}); err != nil {
		t.Fatal(err)
	}
	aurora := seededDefinition(t, opts, "postgres-aws-aurora")
	aurora.Criteria = []resource.Criterion{{ApplicationID: "registered"}}
	aurora.Key, aurora.ConnectionKey = "aurora-separate", "aurora-account"
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, aurora); !errors.Is(err, catalog.ErrInvalid) || !strings.Contains(err.Error(), "leave connectionKey empty") {
		t.Fatalf("explicit account must be rejected: %v", err)
	}
	aurora.Key, aurora.ConnectionKey = "aurora-environment", ""
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, aurora); err != nil {
		t.Fatalf("Environment-bound Aurora: %v", err)
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
	base.ExecutionProfile = "internal-k8s"
	with := func(mutate func(*resource.Definition)) resource.Definition {
		d := base
		mutate(&d)
		return d
	}
	cases := map[string]resource.Definition{
		"arbitrary key":      with(func(d *resource.Definition) { d.Key = "cluster-custom" }),
		"reserved key":       with(func(d *resource.Definition) { d.Key = planning.BuiltinClusterKey }),
		"valid connection":   with(func(d *resource.Definition) { d.Key = "cluster-valid"; d.ConnectionKey = opts.ConnectionKey }),
		"no connection":      with(func(d *resource.Definition) { d.Key = "cluster-noconn"; d.ConnectionKey = "" }),
		"unknown connection": with(func(d *resource.Definition) { d.Key = "cluster-unknown"; d.ConnectionKey = "missing" }),
		"AWS connection":     with(func(d *resource.Definition) { d.Key = "cluster-aws"; d.ConnectionKey = opts.CloudConnectionKey }),
		"no profile":         with(func(d *resource.Definition) { d.Key = "cluster-noprofile"; d.ExecutionProfile = "" }),
		"other profile":      with(func(d *resource.Definition) { d.Key = "cluster-other"; d.ExecutionProfile = "aws-eks" }),
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
