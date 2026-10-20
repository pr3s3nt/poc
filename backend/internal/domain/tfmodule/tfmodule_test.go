package tfmodule

import (
	"strings"
	"testing"
)

func validRef() SourceRef {
	return SourceRef{SourceID: "acme", ModuleID: "redis", Revision: strings.Repeat("a", 40), Subdirectory: "modules/redis", ContentSHA256: strings.Repeat("b", 64)}
}

func TestParseSourceRefIsExact(t *testing.T) {
	good := validRef().Map()
	if ref, err := ParseSourceRef(good); err != nil || ref != validRef() {
		t.Fatalf("valid pin: %+v %v", ref, err)
	}
	cases := map[string]func(map[string]any){
		"url field":           func(m map[string]any) { m["url"] = "https://user:token@example.com/x.git" },
		"module name field":   func(m map[string]any) { m["module"] = "vpc" },
		"missing digest":      func(m map[string]any) { delete(m, "content_sha256") },
		"non-string revision": func(m map[string]any) { m["revision"] = 1 },
		"uppercase digest":    func(m map[string]any) { m["content_sha256"] = strings.Repeat("B", 64) },
		"short digest":        func(m map[string]any) { m["content_sha256"] = "abc" },
		"traversal path":      func(m map[string]any) { m["subdirectory"] = "../outside" },
		"absolute path":       func(m map[string]any) { m["subdirectory"] = "/etc" },
		"empty path":          func(m map[string]any) { m["subdirectory"] = "" },
		"dot segment":         func(m map[string]any) { m["subdirectory"] = "a/./b" },
		"bad source id":       func(m map[string]any) { m["source_id"] = "Acme Repo" },
		"revision with space": func(m map[string]any) { m["revision"] = "main branch" },
	}
	for name, mutate := range cases {
		m := validRef().Map()
		mutate(m)
		_, err := ParseSourceRef(m)
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		if strings.Contains(err.Error(), "token") || strings.Contains(err.Error(), "/etc") {
			t.Errorf("%s: error echoes the value: %v", name, err)
		}
	}
	if _, err := ParseSourceRef("vpc"); err == nil {
		t.Fatal("a plain module name must be rejected")
	}
}

func TestFullCommitRejectsMutableRevisions(t *testing.T) {
	for _, rev := range []string{"main", "v1.2.3", "HEAD", "abc1234", strings.Repeat("A", 40), strings.Repeat("a", 39)} {
		if FullCommit(rev) {
			t.Errorf("%q accepted as a full commit", rev)
		}
	}
	for _, rev := range []string{strings.Repeat("a", 40), strings.Repeat("0", 64)} {
		if !FullCommit(rev) {
			t.Errorf("%q rejected", rev)
		}
	}
}

func driverValues() map[string]any {
	return map[string]any{
		"source":            validRef().Map(),
		"variables":         map[string]any{"name": "x"},
		"input_mapping":     map[string]any{"db_name": map[string]any{"param": "database"}},
		"output_mapping":    map[string]any{"endpoint": "host"},
		"provider_bindings": map[string]any{"aws": map[string]any{"target": "account"}, "kubernetes": map[string]any{"target": "resource", "resource_ref": "k8s-cluster.eks#x"}},
	}
}

func TestParseDriverInputsStrictShape(t *testing.T) {
	parsed, err := ParseDriverInputs(driverValues())
	if err != nil {
		t.Fatal(err)
	}
	if parsed.InputMapping["db_name"] != "database" || parsed.OutputMapping["endpoint"] != "host" || parsed.ProviderBindings["kubernetes"].ResourceRef != "k8s-cluster.eks#x" {
		t.Fatalf("parsed %+v", parsed)
	}
	cases := map[string]func(map[string]any){
		"unknown field":      func(v map[string]any) { v["provisioners"] = []any{} },
		"secret refs":        func(v map[string]any) { v["secret_refs"] = map[string]any{} },
		"no bindings":        func(v map[string]any) { delete(v, "provider_bindings") },
		"null variables":     func(v map[string]any) { v["variables"] = nil },
		"mapping expression": func(v map[string]any) { v["input_mapping"] = map[string]any{"db_name": "${var.x}"} },
		"mapping extra key": func(v map[string]any) {
			v["input_mapping"] = map[string]any{"db_name": map[string]any{"param": "x", "expr": "y"}}
		},
		"output mapping object": func(v map[string]any) { v["output_mapping"] = map[string]any{"endpoint": map[string]any{"to": "host"}} },
		"region override": func(v map[string]any) {
			v["provider_bindings"] = map[string]any{"aws": map[string]any{"target": "account", "region": "eu-west-1"}}
		},
		"credential override": func(v map[string]any) {
			v["provider_bindings"] = map[string]any{"aws": map[string]any{"target": "account", "access_key": "AKIA"}}
		},
		"retired environment target": func(v map[string]any) {
			v["provider_bindings"] = map[string]any{"aws": map[string]any{"target": "environment"}}
		},
		"connection target": func(v map[string]any) {
			v["provider_bindings"] = map[string]any{"aws": map[string]any{"target": "connection"}}
		},
		"resource without ref": func(v map[string]any) {
			v["provider_bindings"] = map[string]any{"kubernetes": map[string]any{"target": "resource"}}
		},
		"environment with ref": func(v map[string]any) {
			v["provider_bindings"] = map[string]any{"aws": map[string]any{"target": "environment", "resource_ref": "x"}}
		},
		"source with url":          func(v map[string]any) { v["source"].(map[string]any)["url"] = "https://example.com/x.git" },
		"legacy module name":       func(v map[string]any) { v["source"] = map[string]any{"module": "vpc"} },
		"non-string binding value": func(v map[string]any) { v["provider_bindings"] = map[string]any{"aws": map[string]any{"target": true}} },
	}
	for name, mutate := range cases {
		v := driverValues()
		mutate(v)
		if _, err := ParseDriverInputs(v); err == nil {
			t.Errorf("%s: accepted", name)
		} else if strings.Contains(err.Error(), "AKIA") || strings.Contains(err.Error(), "eu-west-1") {
			t.Errorf("%s: error echoes a value: %v", name, err)
		}
	}
}

// Secret classification is monotonic: a mapped output is secret when either
// side marks it; nothing downgrades a secret to a displayable value.
func TestEffectiveSecretIsMonotonic(t *testing.T) {
	for _, c := range []struct{ typ, module, want bool }{{false, false, false}, {true, false, true}, {false, true, true}, {true, true, true}} {
		if got := EffectiveSecret(c.typ, c.module); got != c.want {
			t.Errorf("EffectiveSecret(%v,%v)=%v", c.typ, c.module, got)
		}
	}
}

func validBundle() Bundle {
	return Bundle{
		ID: "b", Ref: validRef(), TerraformVersion: "1.9.8", TerraformBinarySHA256: strings.Repeat("c", 64), ProviderLockDigest: strings.Repeat("d", 64), Status: StatusVerified,
		Contract: Contract{
			SchemaVersion: ContractSchemaVersion, SourceKind: SourceKindGit, ResourceTypes: []string{"redis"},
			Inputs:      map[string]Input{"tags": {Type: "map(string)", HasDefault: true, Default: map[string]any{"a": "b"}}},
			Outputs:     map[string]Output{"host": {Type: "string"}},
			Transitive:  []TransitiveModule{{SourceID: "acme", Revision: strings.Repeat("e", 40), Path: "modules/net", ContentSHA256: strings.Repeat("f", 64)}},
			ModuleCalls: []ModuleCall{{Key: "net", Source: "git::https://git.example.com/acme.git//modules/net?ref=" + strings.Repeat("e", 40), Transitive: 0}},
		},
		Providers: map[string]ProviderRequirement{"aws": {Source: "registry.terraform.io/hashicorp/aws", Version: "5.80.0", Binding: BindingAWS, Hashes: []string{"h1:x"}}},
	}
}

func TestBundleValidateChecksPinsAndCalls(t *testing.T) {
	if err := validBundle().Validate(); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*Bundle){
		"revoked status":     func(b *Bundle) { b.Status = "REVOKED" },
		"floating terraform": func(b *Bundle) { b.TerraformVersion = "~> 1.9" },
		"bad transitive":     func(b *Bundle) { b.Contract.Transitive[0].ContentSHA256 = "x" },
		"call out of range":  func(b *Bundle) { b.Contract.ModuleCalls[0].Transitive = 3 },
		"unknown binding": func(b *Bundle) {
			b.Providers["aws"] = ProviderRequirement{Source: "s", Version: "1", Binding: "host", Hashes: []string{"h1:x"}}
		},
		"provider w/o hashes": func(b *Bundle) {
			b.Providers["aws"] = ProviderRequirement{Source: "s", Version: "1", Binding: BindingAWS}
		},
	}
	for name, mutate := range cases {
		b := validBundle().Clone()
		mutate(&b)
		if b.Validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestCloneIsolatesContractAndProviders(t *testing.T) {
	original := validBundle()
	copied := original.Clone()
	copied.Providers["aws"].Hashes[0] = "h1:tampered"
	copied.Contract.Inputs["tags"].Default.(map[string]any)["a"] = "tampered"
	copied.Contract.Transitive[0].ContentSHA256 = "tampered"
	copied.Contract.ResourceTypes[0] = "tampered"
	copied.Contract.Outputs["host"] = Output{Type: "number"}
	if !original.SameVerifiedContent(validBundle()) {
		t.Fatalf("mutating a clone changed the original: %+v", original)
	}
	if original.SameVerifiedContent(copied) {
		t.Fatal("drifted clone compares equal")
	}
}

func TestTypeCompatibility(t *testing.T) {
	if !InputCompatible("string", "string") || !InputCompatible("", "number") || !InputCompatible("number", "any") {
		t.Fatal("compatible input rejected")
	}
	if InputCompatible("string", "number") || InputCompatible("bool", "list(string)") {
		t.Fatal("incompatible input accepted")
	}
	if !OutputCompatible("string", "") || !OutputCompatible("any", "list(any)") {
		t.Fatal("deferred/any output rejected")
	}
	if OutputCompatible("string", "list(any)") || OutputCompatible("number", "string") {
		t.Fatal("incompatible output accepted")
	}
	if _, err := ParseType("object({a=string})"); err == nil {
		t.Fatal("object types are outside the supported subset")
	}
}
