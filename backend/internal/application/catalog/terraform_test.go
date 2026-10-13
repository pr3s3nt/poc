package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/adapters/terraform"
	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/application/catalog"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/planning"
	"orchestrator/internal/ports/moduleregistry"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

const cacheURL = "https://git.example.com/acme/modules.git"

var cacheCommit = strings.Repeat("a", 40)

const cacheModule = `terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0"
    }
  }
}

variable "cache_name" {
  type = string
}

variable "memory" {
  type    = number
  default = 1
}

variable "region" {
  type = string
}

variable "tags" {
  type    = map(string)
  default = {}
}

variable "master_password" {
  type      = string
  sensitive = true
}

resource "aws_elasticache_replication_group" "r" {
  replication_group_id = var.cache_name
}

output "host" {
  value = aws_elasticache_replication_group.r.primary_endpoint_address
}

output "port" {
  value = 6379
}

output "password" {
  value     = var.master_password
  sensitive = true
}

output "tls" {
  value = true
}

output "internal_arn" {
  value = aws_elasticache_replication_group.r.arn
}
`

// cacheSnapshot serves one fixture revision from memory.
type cacheSnapshot struct{ files map[string]string }

func (c cacheSnapshot) Close() {}

func (c cacheSnapshot) Tree(dir string) (registry.Tree, error) {
	out := registry.Tree{}
	for p, content := range c.files {
		if strings.HasPrefix(p, dir+"/") {
			out[strings.TrimPrefix(p, dir+"/")] = []byte(content)
		}
	}
	return out, nil
}

type cacheGit struct{ files map[string]string }

func (g cacheGit) Open(_ context.Context, source registry.GitSource, commit string) (registry.Snapshot, error) {
	if source.URL != cacheURL || commit != cacheCommit {
		return nil, registry.ErrFetch
	}
	return cacheSnapshot{files: g.files}, nil
}

type terraformCatalog struct {
	st   persistence.Store
	svc  *catalog.Service
	reg  *registry.Service
	opts seed.Options
	ref  tfmodule.SourceRef
}

// newTerraformCatalog seeds a catalog with a "cache" Resource Type and a
// registry allowing one Git module that implements it (no auth token input:
// it is executor-owned through sensitivity, never authored).
func newTerraformCatalog(t *testing.T, typ resource.Type) terraformCatalog {
	t.Helper()
	return newTerraformCatalogOn(t, store.New(), typ)
}

func newTerraformCatalogOn(t *testing.T, st persistence.Store, typ resource.Type) terraformCatalog {
	t.Helper()
	ctx := context.Background()
	opts := seedtest.Defaults()
	if err := seedtest.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"modules/cache/main.tf": cacheModule}
	release, err := registry.DefaultRelease()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	cache, err := registry.OpenCache(dir)
	if err != nil {
		t.Fatal(err)
	}
	policy := registry.Policy{Sources: []registry.GitSource{{ID: "acme", Kind: tfmodule.SourceKindGit, URL: cacheURL, Modules: []registry.GitModule{{ID: "cache", Subdirectory: "modules/cache", ResourceTypes: []string{typ.Key}}}}}}
	reg, err := registry.New(registry.Config{Release: release, Policy: policy, Packaged: terraform.PackagedModules(), Git: cacheGit{files: files}, Cache: cache, Store: st})
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	svc := catalog.NewService(st, reg)
	if _, err := svc.RegisterResourceType(ctx, opts.OrganizationKey, typ); err != nil {
		t.Fatal(err)
	}
	tree, _ := cacheSnapshot{files: files}.Tree("modules/cache")
	ref := tfmodule.SourceRef{SourceID: "acme", ModuleID: "cache", Revision: cacheCommit, Subdirectory: "modules/cache", ContentSHA256: tree.Digest()}
	return terraformCatalog{st: st, svc: svc, reg: reg, opts: opts, ref: ref}
}

func cacheType() resource.Type {
	return resource.Type{
		Key:    "cache",
		Inputs: []resource.InputField{{Name: "size", Type: "number"}},
		Outputs: []resource.OutputField{
			{Name: "host", Type: "string", Required: true},
			{Name: "port", Type: "number", Required: true},
			{Name: "password", Type: "string", Required: true, Secret: true},
			{Name: "tls", Type: "bool", Required: true},
		},
	}
}

func (c terraformCatalog) definition(key string) resource.Definition {
	return resource.Definition{
		Key: key, ResourceTypeKey: "cache", ExecutionProfile: "aws-eks", DriverType: resource.DriverTerraform,
		Criteria: []resource.Criterion{{Class: key}},
		DriverInputs: map[string]any{"values": map[string]any{
			"source":            c.ref.Map(),
			"variables":         map[string]any{"cache_name": "${context.infra.resourceName}"},
			"input_mapping":     map[string]any{"memory": map[string]any{"param": "size"}},
			"output_mapping":    map[string]any{"host": "host", "port": "port", "password": "password", "tls": "tls"},
			"provider_bindings": map[string]any{"aws": map[string]any{"target": "environment"}},
		}},
	}
}

func setValue(def resource.Definition, field string, value any) resource.Definition {
	values := map[string]any{}
	for k, v := range def.DriverValues() {
		values[k] = v
	}
	if value == nil {
		delete(values, field)
	} else {
		values[field] = value
	}
	def.DriverInputs = map[string]any{"values": values}
	return def
}

func TestTerraformRegistrationVerifiesPinAndMappings(t *testing.T) {
	ctx := context.Background()
	c := newTerraformCatalog(t, cacheType())
	org := c.opts.OrganizationKey
	if _, err := c.svc.RegisterResourceDefinition(ctx, org, c.definition("cache-ok")); err != nil {
		t.Fatalf("valid definition: %v", err)
	}
	stored, err := c.st.GetModuleBundle(ctx, c.ref)
	if err != nil || stored.Ref != c.ref || !stored.ServesType("cache") {
		t.Fatalf("bundle not recorded with the definition: %+v %v", stored, err)
	}
	if _, ok := stored.Contract.Inputs["master_password"]; !ok || !stored.Contract.Inputs["master_password"].Sensitive {
		t.Fatalf("sensitive input not recorded: %+v", stored.Contract.Inputs)
	}

	base := c.definition("cache-bad")
	pin := func(mutate func(m map[string]any)) any {
		m := c.ref.Map()
		mutate(m)
		return m
	}
	cases := map[string]resource.Definition{
		"unknown type input":           setValue(base, "input_mapping", map[string]any{"memory": map[string]any{"param": "nope"}}),
		"type input unmapped":          setValue(base, "input_mapping", map[string]any{}),
		"same param twice":             setValue(base, "input_mapping", map[string]any{"memory": map[string]any{"param": "size"}, "cache_name": map[string]any{"param": "size"}}),
		"param type mismatch":          setValue(setValue(base, "variables", map[string]any{"memory": 1}), "input_mapping", map[string]any{"cache_name": map[string]any{"param": "size"}}),
		"map executor-owned region":    setValue(base, "input_mapping", map[string]any{"memory": map[string]any{"param": "size"}, "region": map[string]any{"param": "size"}}),
		"map sensitive input":          setValue(base, "input_mapping", map[string]any{"master_password": map[string]any{"param": "size"}}),
		"set executor-owned tags":      setValue(base, "variables", map[string]any{"cache_name": "x", "tags": map[string]any{"a": "b"}}),
		"set region":                   setValue(base, "variables", map[string]any{"cache_name": "x", "region": "eu-west-1"}),
		"set sensitive input":          setValue(base, "variables", map[string]any{"cache_name": "x", "master_password": "s3cr3t"}),
		"unknown variable":             setValue(base, "variables", map[string]any{"cache_name": "x", "nope": 1}),
		"literal type":                 setValue(base, "variables", map[string]any{"cache_name": 3}),
		"required variable missing":    setValue(base, "variables", map[string]any{}),
		"unknown module output":        setValue(base, "output_mapping", map[string]any{"host": "host", "port": "port", "password": "password", "tls": "tls", "nope": "host"}),
		"unknown type output":          setValue(base, "output_mapping", map[string]any{"host": "host", "port": "port", "password": "password", "tls": "tls", "internal_arn": "arn"}),
		"two outputs one target":       setValue(base, "output_mapping", map[string]any{"host": "host", "internal_arn": "host", "port": "port", "password": "password", "tls": "tls"}),
		"required output unmapped":     setValue(base, "output_mapping", map[string]any{"host": "host", "port": "port", "tls": "tls"}),
		"output type mismatch":         setValue(base, "output_mapping", map[string]any{"host": "host", "port": "tls", "password": "password", "tls": "port"}),
		"binding missing":              setValue(base, "provider_bindings", map[string]any{}),
		"binding unknown provider":     setValue(base, "provider_bindings", map[string]any{"aws": map[string]any{"target": "environment"}, "kubernetes": map[string]any{"target": "environment"}}),
		"aws bound to resource":        setValue(base, "provider_bindings", map[string]any{"aws": map[string]any{"target": "resource", "resource_ref": "k8s-cluster.eks#x"}}),
		"aws region override":          setValue(base, "provider_bindings", map[string]any{"aws": map[string]any{"target": "environment", "region": "eu-west-1"}}),
		"legacy module name":           setValue(base, "source", map[string]any{"module": "cache"}),
		"source url":                   setValue(base, "source", pin(func(m map[string]any) { m["url"] = cacheURL })),
		"wrong digest":                 setValue(base, "source", pin(func(m map[string]any) { m["content_sha256"] = strings.Repeat("9", 64) })),
		"branch revision":              setValue(base, "source", pin(func(m map[string]any) { m["revision"] = "main" })),
		"unallowlisted source":         setValue(base, "source", pin(func(m map[string]any) { m["source_id"] = "evil" })),
		"unknown driver input field":   setValue(base, "provisioners", []any{"local-exec"}),
		"arbitrary output expression":  setValue(base, "output_mapping", map[string]any{"host": "${upper(host)}"}),
		"module for another type":      func() resource.Definition { d := base; d.ResourceTypeKey = "postgres"; return d }(),
		"connection override":          func() resource.Definition { d := base; d.ConnectionKey = c.opts.CloudConnectionKey; return d }(),
		"aws module on internal-k8s":   func() resource.Definition { d := base; d.ExecutionProfile = "internal-k8s"; return d }(),
		"aws module without a profile": func() resource.Definition { d := base; d.ExecutionProfile = ""; return d }(),
	}
	before, _ := c.st.ListResourceDefinitions(ctx, org)
	for name, def := range cases {
		def.Key = "cache-" + strings.ReplaceAll(strings.ReplaceAll(name, " ", "-"), "_", "-")
		_, err := c.svc.RegisterResourceDefinition(ctx, org, def)
		if !errors.Is(err, catalog.ErrInvalid) {
			t.Errorf("%s: want ErrInvalid, got %v", name, err)
			continue
		}
		for _, leak := range []string{"s3cr3t", cacheURL, "eu-west-1"} {
			if strings.Contains(err.Error(), leak) {
				t.Errorf("%s: error echoes %q", name, leak)
			}
		}
	}
	after, _ := c.st.ListResourceDefinitions(ctx, org)
	if len(after) != len(before) {
		t.Fatalf("rejected definitions were stored: %d -> %d", len(before), len(after))
	}
}

// A module-sensitive output stays secret even when mapped to a Type output
// the Type does not mark secret; the planning contract carries it.
func TestTerraformOutputSecretClassificationIsMonotonic(t *testing.T) {
	ctx := context.Background()
	typ := cacheType()
	typ.Outputs[2].Secret = false // the Type alone would display "password"
	c := newTerraformCatalog(t, typ)
	def := c.definition("cache-monotonic")
	if _, err := c.svc.RegisterResourceDefinition(ctx, c.opts.OrganizationKey, def); err != nil {
		t.Fatal(err)
	}
	contract, err := planning.RegistryInspector{Metadata: c.reg}.Contract(def)
	if err != nil {
		t.Fatal(err)
	}
	if len(contract.SecretOutputs) != 1 || contract.SecretOutputs[0] != "password" {
		t.Fatalf("secret outputs %v", contract.SecretOutputs)
	}
	if contract.OutputMapping["internal_arn"] != "" {
		t.Fatal("an unmapped internal output was published")
	}
}

// driftingRegistrar returns, for an already persisted pin, a bundle whose
// verified contract differs (as a concurrent or drifted verification would).
type driftingRegistrar struct {
	moduleregistry.Registry
}

func (d driftingRegistrar) Verify(ctx context.Context, ref tfmodule.SourceRef) (tfmodule.Bundle, error) {
	b, err := d.Registry.Verify(ctx, ref)
	if err != nil {
		return b, err
	}
	b = b.Clone()
	b.ID = "00000000-0000-4000-8000-0000000000ff"
	b.Contract.Inputs["memory"] = tfmodule.Input{Type: "number", HasDefault: true, Default: float64(2)}
	return b, nil
}

func TestTerraformRegistrationRejectsDriftAgainstStoredBundle(t *testing.T) {
	ctx := context.Background()
	c := newTerraformCatalog(t, cacheType())
	org := c.opts.OrganizationKey
	if _, err := c.svc.RegisterResourceDefinition(ctx, org, c.definition("cache-first")); err != nil {
		t.Fatal(err)
	}
	drifting := catalog.NewService(c.st, driftingRegistrar{c.reg})
	if _, err := drifting.RegisterResourceDefinition(ctx, org, c.definition("cache-drift")); !errors.Is(err, catalog.ErrInvalid) || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("conflicting contract for a stored pin: %v", err)
	}
	defs, _ := c.st.ListResourceDefinitions(ctx, org)
	for _, d := range defs {
		if d.Key == "cache-drift" {
			t.Fatal("the drifted definition was stored")
		}
	}
	stored, _ := c.st.GetModuleBundle(ctx, c.ref)
	if stored.Contract.Inputs["memory"].Default != float64(1) {
		t.Fatal("the stored bundle changed")
	}
}

// noFetch fails the test when capability metadata uses a registry operation
// that may fetch, inspect or verify for execution.
type noFetch struct {
	moduleregistry.Registry
	t *testing.T
}

func (n noFetch) Verify(context.Context, tfmodule.SourceRef) (tfmodule.Bundle, error) {
	n.t.Fatal("capability metadata called the registrar")
	return tfmodule.Bundle{}, nil
}
func (n noFetch) VerifyForExecution(context.Context, tfmodule.SourceRef) (tfmodule.VerifiedArtifact, error) {
	n.t.Fatal("capability metadata called execution verification")
	return tfmodule.VerifiedArtifact{}, nil
}

// Capabilities publish the verified Git bundle with its safe pin and
// provider pins, from metadata only, without URL, cache path, lock hashes or
// executor-owned/sensitive controls.
func TestTerraformCapabilitiesPublishVerifiedBundleSafely(t *testing.T) {
	ctx := context.Background()
	c := newTerraformCatalog(t, cacheType())
	if _, err := c.svc.RegisterResourceDefinition(ctx, c.opts.OrganizationKey, c.definition("cache-caps")); err != nil {
		t.Fatal(err)
	}
	caps, err := catalog.NewService(c.st, noFetch{Registry: c.reg, t: t}).ListDefinitionCapabilities(ctx, c.opts.OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	var found *catalog.Capability
	for i := range caps {
		if caps[i].ResourceType == "cache" {
			found = &caps[i]
		}
	}
	if found == nil || found.Source == nil || found.Source.ContentSHA256 != c.ref.ContentSHA256 || found.Source.Revision != cacheCommit || found.TerraformVersion != "1.9.8" {
		t.Fatalf("capability %+v", found)
	}
	for _, v := range found.Variables {
		if v.Name == "master_password" || v.Name == "region" || v.Name == "tags" {
			t.Fatalf("exposes %s as an authoring control", v.Name)
		}
	}
	raw, _ := json.Marshal(caps)
	for _, leak := range []string{cacheURL, "git.example.com", "bundles/", "h1:", "zh:", os.TempDir()} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("capabilities leak %q", leak)
		}
	}
}
