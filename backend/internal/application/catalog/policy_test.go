package catalog_test

import (
	"context"
	"errors"
	"orchestrator/internal/planning"
	"strings"
	"testing"

	"orchestrator/internal/adapters/terraform"
	"orchestrator/internal/application/catalog"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/seed"
)

// sentinel marks submitted values that must never appear in an error.
const sentinel = "sentinel-value-7f3a"

// TestRegisterResourceType_PublicIDPolicy covers UC-02 BR-05/BR-06: invalid
// and reserved IDs are rejected without normalization and nothing is stored.
func TestRegisterResourceType_PublicIDPolicy(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, nil)
	before, _ := st.ListResourceTypes(ctx, opts.OrganizationKey)
	outputs := []resource.OutputField{{Name: "host", Type: "string"}}
	for _, key := range []string{"", " cache", "cache ", "Cache", "cache_v2", "cache.v2", "cache/v2", "cáche", "cache\n", "environment", "service"} {
		_, err := svc.RegisterResourceType(ctx, opts.OrganizationKey, resource.Type{Key: key, Outputs: outputs})
		if !errors.Is(err, catalog.ErrInvalid) {
			t.Errorf("type ID %q: want ErrInvalid, got %v", key, err)
		}
	}
	if after, _ := st.ListResourceTypes(ctx, opts.OrganizationKey); len(after) != len(before) {
		t.Fatalf("rejected types were stored: %d -> %d", len(before), len(after))
	}
	if _, err := svc.RegisterResourceType(ctx, opts.OrganizationKey, resource.Type{Key: "cache-7", Outputs: outputs}); err != nil {
		t.Fatalf("valid ID rejected: %v", err)
	}
	// Seeded entries stay protected by insert-only duplicate semantics.
	if _, err := svc.RegisterResourceType(ctx, opts.OrganizationKey, resource.Type{Key: "postgres", Outputs: outputs}); !errors.Is(err, catalog.ErrDuplicate) {
		t.Fatalf("seeded type duplicate: %v", err)
	}
}

// TestRegisterResourceDefinition_PublicIDPolicy covers UC-03 BR-10.
func TestRegisterResourceDefinition_PublicIDPolicy(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, terraform.NewInspector())
	base := registrable(t, opts, "postgres-internal-statefulset")
	before, _ := st.ListResourceDefinitions(ctx, opts.OrganizationKey)
	for _, key := range []string{"", " pg", "PG", "pg_fast", "pg.fast", "pg\t"} {
		def := base
		def.Key = key
		if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def); !errors.Is(err, catalog.ErrInvalid) {
			t.Errorf("definition ID %q: want ErrInvalid, got %v", key, err)
		}
	}
	if after, _ := st.ListResourceDefinitions(ctx, opts.OrganizationKey); len(after) != len(before) {
		t.Fatalf("rejected definitions were stored: %d -> %d", len(before), len(after))
	}
	// Definition IDs have their own namespace: a Type key is a valid ID.
	def := base
	def.Key = "postgres"
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def); err != nil {
		t.Fatalf("definition ID equal to a type key: %v", err)
	}
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def); !errors.Is(err, catalog.ErrDuplicate) {
		t.Fatalf("duplicate definition: %v", err)
	}
}

// TestRegisterResourceDefinition_SeededShapesStillRegister proves every seeded
// Definition, including its placeholders, passes the strict policy.
func TestRegisterResourceDefinition_SeededShapesStillRegister(t *testing.T) {
	ctx := context.Background()
	_, svc, opts := seededCatalog(t, terraform.NewInspector())
	for _, name := range []string{"namespace-kubernetes", "cluster-internal-registered", "cluster-aws-eks", "vpc-aws", "postgres-internal-statefulset", "postgres-aws-aurora"} {
		def := registrable(t, opts, name)
		def.Key = name + "-copy"
		if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def); err != nil {
			t.Errorf("seeded shape %s rejected: %v", name, err)
		}
	}
}

// registrable returns a seeded Definition with the Execution Profile public
// registration requires (seeds rely on criteria instead).
func registrable(t *testing.T, opts seed.Options, key string) resource.Definition {
	t.Helper()
	def := seededDefinition(t, opts, key)
	if def.ExecutionProfile == "" {
		switch def.DriverType {
		case resource.DriverTerraform:
			def.ExecutionProfile = "aws-eks"
		case resource.DriverExistingCluster:
			def.ExecutionProfile = "internal-k8s"
		}
	}
	return def
}

func withVariables(def resource.Definition, set map[string]any) resource.Definition {
	values := map[string]any{}
	for k, v := range def.DriverValues() {
		values[k] = v
	}
	variables := map[string]any{}
	for k, v := range def.Variables() {
		variables[k] = v
	}
	for k, v := range set {
		if v == deleteMarker {
			delete(variables, k)
			continue
		}
		variables[k] = v
	}
	values["variables"] = variables
	def.DriverInputs = map[string]any{"values": values}
	return def
}

type marker struct{}

var deleteMarker any = marker{}

// TestRegisterResourceDefinition_StrictDriverInputs covers UC-03 BR-11–BR-14
// negative paths: each is ErrInvalid, nothing is stored and no submitted
// value is echoed.
func TestRegisterResourceDefinition_StrictDriverInputs(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, terraform.NewInspector())
	pg := registrable(t, opts, "postgres-internal-statefulset")
	ns := registrable(t, opts, "namespace-kubernetes")
	existing := registrable(t, opts, "cluster-internal-registered")
	aurora := registrable(t, opts, "postgres-aws-aurora")
	vpcID := "${resources['vpc.default#applications.@app'].outputs.id}"
	raw := func(def resource.Definition, inputs map[string]any) resource.Definition {
		def.DriverInputs = inputs
		return def
	}
	cases := map[string]resource.Definition{
		// BR-11 structure.
		"missing driverInputs":        raw(pg, nil),
		"unknown root field":          raw(pg, map[string]any{"values": map[string]any{"variables": map[string]any{}}, "extra": sentinel}),
		"secret_refs":                 raw(pg, map[string]any{"values": map[string]any{"variables": map[string]any{}}, "secret_refs": map[string]any{"password": sentinel}}),
		"values null":                 raw(pg, map[string]any{"values": nil}),
		"values array":                raw(pg, map[string]any{"values": []any{sentinel}}),
		"unknown values field":        raw(pg, map[string]any{"values": map[string]any{"variables": map[string]any{}, "inputs": sentinel}}),
		"variables missing":           raw(pg, map[string]any{"values": map[string]any{}}),
		"variables null":              raw(pg, map[string]any{"values": map[string]any{"variables": nil}}),
		"variables array":             raw(pg, map[string]any{"values": map[string]any{"variables": []any{sentinel}}}),
		"kubernetes source":           raw(pg, map[string]any{"values": map[string]any{"variables": map[string]any{}, "source": map[string]any{"module": "aurora"}}}),
		"existing-cluster source":     raw(existing, map[string]any{"values": map[string]any{"variables": map[string]any{}, "source": map[string]any{}}}),
		"terraform source missing":    raw(aurora, map[string]any{"values": map[string]any{"variables": aurora.Variables()}}),
		"terraform source null":       raw(aurora, map[string]any{"values": map[string]any{"variables": aurora.Variables(), "source": nil}}),
		"terraform source url":        raw(aurora, map[string]any{"values": map[string]any{"variables": aurora.Variables(), "source": map[string]any{"module": "aurora", "url": sentinel}}}),
		"terraform module not string": raw(aurora, map[string]any{"values": map[string]any{"variables": aurora.Variables(), "source": map[string]any{"module": 7}}}),
		"terraform module empty":      raw(aurora, map[string]any{"values": map[string]any{"variables": aurora.Variables(), "source": map[string]any{"module": ""}}}),
		// BR-12 Kubernetes/existing-cluster contracts.
		"postgres password":          withVariables(pg, map[string]any{"password": sentinel}),
		"postgres unknown variable":  withVariables(pg, map[string]any{"replicas": 2}),
		"postgres image number":      withVariables(pg, map[string]any{"image": 5}),
		"postgres storage null":      withVariables(pg, map[string]any{"storage": nil}),
		"postgres namespace array":   withVariables(pg, map[string]any{"namespace": []any{sentinel}}),
		"namespace name missing":     withVariables(ns, map[string]any{"name": deleteMarker}),
		"namespace name number":      withVariables(ns, map[string]any{"name": 12}),
		"namespace unknown variable": withVariables(ns, map[string]any{"labels": map[string]any{"a": sentinel}}),
		"existing endpoint":          withVariables(existing, map[string]any{"endpoint": sentinel}),
		"existing kubeContext bool":  withVariables(existing, map[string]any{"kubeContext": true}),
		// BR-12/BR-14 Terraform module contract.
		"terraform master_password":    withVariables(aurora, map[string]any{"master_password": sentinel}),
		"terraform undeclared":         withVariables(aurora, map[string]any{"instance_class": sentinel}),
		"terraform number as string":   withVariables(aurora, map[string]any{"min_capacity": sentinel}),
		"terraform number interpolate": withVariables(aurora, map[string]any{"min_capacity": "${context.app.id}-" + sentinel}),
		"terraform number escaped":     withVariables(aurora, map[string]any{"min_capacity": "$${context.app.id}"}),
		"terraform string as number":   withVariables(aurora, map[string]any{"engine_version": 16}),
		"terraform list literal":       withVariables(aurora, map[string]any{"subnet_ids": sentinel}),
		"terraform list element type":  withVariables(aurora, map[string]any{"subnet_ids": []any{"subnet-a", 2}}),
		"terraform list element null":  withVariables(aurora, map[string]any{"subnet_ids": []any{"subnet-a", nil}}),
		"terraform map element type":   withVariables(aurora, map[string]any{"tags": map[string]any{"team": 1}}),
		"terraform map as list":        withVariables(aurora, map[string]any{"tags": []any{sentinel}}),
		"terraform null":               withVariables(aurora, map[string]any{"vpc_cidr": nil}),
		// BR-13 malformed placeholders.
		"unterminated placeholder":     withVariables(pg, map[string]any{"image": "${context.app.id-" + sentinel}),
		"unsupported placeholder":      withVariables(pg, map[string]any{"image": "${" + sentinel + "}"}),
		"empty placeholder":            withVariables(pg, map[string]any{"image": "${}"}),
		"empty context path":           withVariables(pg, map[string]any{"image": "${context.}"}),
		"malformed resource reference": withVariables(pg, map[string]any{"namespace": "${resources['k8s-namespace." + sentinel + "}"}),
		"malformed list element":       withVariables(aurora, map[string]any{"subnet_ids": []any{vpcID, "${context." + sentinel}}),
		"nested context placeholder":   withVariables(pg, map[string]any{"image": "${context.${" + sentinel + "}}"}),
		"nested escaped in context":    withVariables(pg, map[string]any{"image": "${context.$${" + sentinel + "}}"}),
		"nested interpolated":          withVariables(pg, map[string]any{"image": "db-${context.app.${" + sentinel + "}}-x"}),
		"nested in resource reference": withVariables(pg, map[string]any{"namespace": "${resources['k8s-namespace.${" + sentinel + "}'].outputs.name}"}),
		"nested number placeholder":    withVariables(aurora, map[string]any{"min_capacity": "${context.${" + sentinel + "}}"}),
		"nested list element":          withVariables(aurora, map[string]any{"subnet_ids": []any{vpcID, "${context.${" + sentinel + "}}"}}),
		"nested map element":           withVariables(aurora, map[string]any{"tags": map[string]any{"team": "${context.${" + sentinel + "}}"}}),
	}
	before, _ := st.ListResourceDefinitions(ctx, opts.OrganizationKey)
	for name, def := range cases {
		def.Key = "strict-" + strings.ToLower(strings.NewReplacer(" ", "-", "_", "-").Replace(name))
		_, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def)
		if !errors.Is(err, catalog.ErrInvalid) {
			t.Errorf("%s: want ErrInvalid, got %v", name, err)
			continue
		}
		t.Logf("%s: %v", name, err)
		if !strings.Contains(err.Error(), "driverInputs") {
			t.Errorf("%s: rejected for an unrelated reason: %v", name, err)
		}
		if strings.Contains(err.Error(), sentinel) {
			t.Errorf("%s: error echoes the submitted value: %v", name, err)
		}
	}
	if after, _ := st.ListResourceDefinitions(ctx, opts.OrganizationKey); len(after) != len(before) {
		t.Fatalf("rejected definitions were stored: %d -> %d", len(before), len(after))
	}
}

// TestRegisterResourceDefinition_PlaceholderAwareTyping covers UC-03 BR-13
// accepted shapes: complete placeholders defer typing, interpolation and
// escapes are strings, literal collection elements are typed.
func TestRegisterResourceDefinition_PlaceholderAwareTyping(t *testing.T) {
	ctx := context.Background()
	_, svc, opts := seededCatalog(t, terraform.NewInspector())
	pg := registrable(t, opts, "postgres-internal-statefulset")
	aurora := registrable(t, opts, "postgres-aws-aurora")
	vpcID := "${resources['vpc.default#applications.@app'].outputs.id}"
	cases := map[string]resource.Definition{
		"full placeholder number":       withVariables(aurora, map[string]any{"min_capacity": "${context.app.id}"}),
		"interpolated string":           withVariables(aurora, map[string]any{"database": "${context.app.id}-db"}),
		"escaped placeholder in string": withVariables(aurora, map[string]any{"username": "user-$${literal}"}),
		"adjacent placeholders":         withVariables(aurora, map[string]any{"database": "${context.app.id}${context.env.id}"}),
		"escaped next to placeholder":   withVariables(aurora, map[string]any{"username": "$${literal}${context.app.id}"}),
		"escaped full string":           withVariables(aurora, map[string]any{"engine_version": "$${context.app.id}"}),
		"full resource reference":       withVariables(aurora, map[string]any{"vpc_cidr": vpcID}),
		"list mixed elements":           withVariables(aurora, map[string]any{"subnet_ids": []any{"subnet-a", vpcID}}),
		"map of strings":                withVariables(aurora, map[string]any{"tags": map[string]any{"team": "data", "app": "${context.app.id}"}}),
		"optional default override":     withVariables(aurora, map[string]any{"seconds_until_auto_pause": 600}),
		"postgres options omitted":      withVariables(pg, map[string]any{"image": deleteMarker, "storage": deleteMarker, "namespace": deleteMarker}),
		"postgres all options":          withVariables(pg, map[string]any{"database": "app", "username": "app"}),
	}
	for name, def := range cases {
		def.Key = "typed-" + strings.ReplaceAll(name, " ", "-")
		if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	existing := registrable(t, opts, "cluster-internal-registered")
	existing = withVariables(existing, map[string]any{"name": deleteMarker, "kubeContext": deleteMarker})
	existing.Key = "existing-defaults"
	existing.Criteria = []resource.Criterion{{Class: "defaults"}}
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, existing); err != nil {
		t.Errorf("existing-cluster without variables: %v", err)
	}
}

// TestRegisterResourceDefinition_NamespaceNameFromTypeContract covers the
// UC-03 BR-12 exception: the Type input contract may supply the name.
func TestRegisterResourceDefinition_NamespaceNameFromTypeContract(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, terraform.NewInspector())
	ns := withVariables(registrable(t, opts, "namespace-kubernetes"), map[string]any{"name": deleteMarker})
	ns.Key = "namespace-from-params"
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, ns); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("namespace without name: %v", err)
	}
	saveNamespaceType := func(inputs ...resource.InputField) {
		t.Helper()
		if err := st.SaveResourceType(ctx, opts.OrganizationKey, resource.Type{
			Key:     "k8s-namespace",
			Inputs:  inputs,
			Outputs: []resource.OutputField{{Name: "name", Type: "string", Required: true}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	rejected := map[string]resource.InputField{
		"optional string":  {Name: "name", Type: "string"},
		"required number":  {Name: "name", Type: "number", Required: true},
		"required bool":    {Name: "name", Type: "bool", Required: true},
		"required any":     {Name: "name", Type: "any", Required: true},
		"required untyped": {Name: "name", Required: true},
		"optional number":  {Name: "name", Type: "number"},
	}
	for name, input := range rejected {
		saveNamespaceType(input)
		def := ns
		def.Key = "namespace-" + strings.ReplaceAll(name, " ", "-")
		if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def); !errors.Is(err, catalog.ErrInvalid) {
			t.Errorf("%s name input: want ErrInvalid, got %v", name, err)
		}
	}
	saveNamespaceType(resource.InputField{Name: "name", Type: "string", Required: true})
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, ns); err != nil {
		t.Fatalf("namespace name supplied by Type contract: %v", err)
	}
}

// ADR-013: the builtin cluster key is reserved for the system.
func TestRegisterResourceDefinition_RejectsReservedBuiltinKey(t *testing.T) {
	ctx := context.Background()
	_, svc, opts := seededCatalog(t, terraform.NewInspector())
	def := planning.BuiltinClusterDefinition()
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("reserved key = %v", err)
	}
}
