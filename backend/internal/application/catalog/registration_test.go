package catalog_test

import (
	"context"
	"errors"
	"reflect"
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
	internal := seededDefinition(t, opts, "postgres-internal-statefulset")
	cases := map[string]func(*resource.Definition){
		"terraform with cluster connection":  func(d *resource.Definition) { *d = aurora; d.ConnectionKey = opts.ConnectionKey },
		"terraform without connection":       func(d *resource.Definition) { *d = aurora; d.ConnectionKey = "" },
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
	st, svc, opts := seededCatalog(t, terraform.NewInspector())
	if err := st.SaveOrganization(ctx, application.Organization{Key: "globex", Name: "Globex"}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveConnection(ctx, application.Connection{Key: "globex-aws", OrganizationKey: "globex", Kind: application.ConnectionAWS, Status: application.ConnectionReady}); err != nil {
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

// Aurora keeps requiring an explicit READY AWS account of the Organization.
func TestRegisterResourceDefinition_AuroraRequiresExplicitAWSAccount(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, terraform.NewInspector())
	if err := st.SaveConnection(ctx, application.Connection{Key: "aurora-account", OrganizationKey: opts.OrganizationKey, Kind: application.ConnectionAWS, Status: application.ConnectionReady}); err != nil {
		t.Fatal(err)
	}
	aurora := seededDefinition(t, opts, "postgres-aws-aurora")
	aurora.Key, aurora.ConnectionKey = "aurora-separate", "aurora-account"
	aurora.Criteria = []resource.Criterion{{ApplicationID: "registered"}}
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, aurora); err != nil {
		t.Fatalf("explicit account: %v", err)
	}
	aurora.Key, aurora.ConnectionKey = "aurora-missing", ""
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, aurora); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("Aurora without an account must be rejected: %v", err)
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

// ADR-013/UC-03 BR-08 (T17): public registration rejects driver existing-cluster
// for every key and shape, and the reserved system key for every driver. Nothing
// is inserted or overwritten.
func TestRegisterResourceDefinition_RejectsExistingClusterDriver(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, terraform.NewInspector())
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
