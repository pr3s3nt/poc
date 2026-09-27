package score

import (
	"testing"

	"orchestrator/internal/domain/resource"
)

func types() map[string]resource.Type {
	return map[string]resource.Type{
		"postgres": {
			Key: "postgres",
			Inputs: []resource.InputField{
				{Name: "database", Type: "string", Required: true},
				{Name: "username", Type: "string", Required: true},
			},
			Outputs: []resource.OutputField{
				{Name: "host", Type: "string", Required: true},
				{Name: "password", Type: "string", Required: true, Secret: true},
			},
		},
		"redis": {
			Key:     "redis",
			Outputs: []resource.OutputField{{Name: "host", Type: "string"}},
		},
	}
}

func base() map[string]any {
	return map[string]any{
		"apiVersion": "score.dev/v1b1",
		"metadata":   map[string]any{"name": "backend"},
		"containers": map[string]any{
			"main": map[string]any{"image": "backend:dev", "variables": map[string]any{"PGHOST": "${resources.db.host}"}},
		},
		"resources": map[string]any{
			"db": map[string]any{
				"type":   "postgres",
				"id":     "acceptance-db",
				"params": map[string]any{"database": "acceptance", "username": "app"},
			},
		},
	}
}

func fragment(t *testing.T, doc map[string]any) *Fragment {
	t.Helper()
	parsed, err := FromMap(doc)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	f, err := parsed.Fragment(types())
	if err != nil {
		t.Fatalf("fragment: %v", err)
	}
	return f
}

func TestFragmentHoistsSharedResourceAndRewritesPlaceholder(t *testing.T) {
	f := fragment(t, base())
	if f.Module.Profile == "" {
		t.Fatal("the module must carry a profile")
	}
	entry, ok := f.Shared["acceptance-db"]
	if !ok {
		t.Fatalf("shared entry is missing: %v", f.Shared)
	}
	if entry.Class != "default" || entry.Params["database"] != "acceptance" {
		t.Fatalf("unexpected shared entry: %#v", entry)
	}
	if len(f.Module.Externals) != 0 {
		t.Fatalf("a shared resource must not become a private external: %v", f.Module.Externals)
	}
	if got := f.Module.Spec.Containers["main"].Variables["PGHOST"]; got != "${shared.acceptance-db.host}" {
		t.Fatalf("placeholder was not rewritten: %q", got)
	}
}

func TestFragmentKeepsPrivateResourceUnderExternals(t *testing.T) {
	doc := base()
	doc["resources"] = map[string]any{"cache": map[string]any{"type": "redis"}}
	doc["containers"] = map[string]any{
		"main": map[string]any{"image": "backend:dev", "variables": map[string]any{"CACHE": "${resources.cache.host}"}},
	}
	f := fragment(t, doc)
	if _, ok := f.Module.Externals["cache"]; !ok {
		t.Fatalf("private resource is missing: %v", f.Module.Externals)
	}
	if len(f.Shared) != 0 {
		t.Fatalf("a private resource must not be hoisted: %v", f.Shared)
	}
	if got := f.Module.Spec.Containers["main"].Variables["CACHE"]; got != "${externals.cache.host}" {
		t.Fatalf("placeholder was not rewritten: %q", got)
	}
}

func TestFragmentRejectsParamsOutsideTheInputContract(t *testing.T) {
	doc := base()
	resources := doc["resources"].(map[string]any)
	db := resources["db"].(map[string]any)
	db["params"] = map[string]any{"database": "acceptance", "username": "app", "storage": "20Gi"}
	parsed, err := FromMap(doc)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := parsed.Fragment(types()); err == nil {
		t.Fatal("expected an input contract error")
	}
}

func TestFragmentRejectsUnknownOutput(t *testing.T) {
	doc := base()
	doc["containers"] = map[string]any{
		"main": map[string]any{"image": "backend:dev", "variables": map[string]any{"X": "${resources.db.not_in_contract}"}},
	}
	parsed, err := FromMap(doc)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := parsed.Fragment(types()); err == nil {
		t.Fatal("expected an output contract error")
	}
}

func TestFragmentRejectsUnknownResourceType(t *testing.T) {
	doc := base()
	doc["resources"] = map[string]any{"db": map[string]any{"type": "mysql", "id": "x"}}
	doc["containers"] = map[string]any{"main": map[string]any{"image": "backend:dev"}}
	parsed, err := FromMap(doc)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := parsed.Fragment(types()); err == nil {
		t.Fatal("expected an unknown resource type error")
	}
}

func TestParseRejectsUnknownApiVersionAndFields(t *testing.T) {
	doc := base()
	doc["apiVersion"] = "score.dev/v1"
	if _, err := FromMap(doc); err == nil {
		t.Fatal("expected an apiVersion error")
	}
	doc = base()
	doc["unexpected"] = true
	if _, err := FromMap(doc); err == nil {
		t.Fatal("expected an unknown field error")
	}
}

func TestRewriteRejectsUndeclaredResource(t *testing.T) {
	doc := base()
	doc["resources"] = map[string]any{}
	parsed, err := FromMap(doc)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := parsed.Fragment(types()); err == nil {
		t.Fatal("expected an undeclared resource error")
	}
}

func TestFragmentRewritesVirtualConfigurationAndServiceWithoutProvisioning(t *testing.T) {
	doc := base()
	doc["resources"] = map[string]any{
		"env":     map[string]any{"type": "environment"},
		"backend": map[string]any{"type": "service", "params": map[string]any{"workload": "backend", "port": "http"}},
	}
	doc["containers"] = map[string]any{"main": map[string]any{"image": "frontend:dev", "variables": map[string]any{
		"TOKEN":       "${resources.env.API_TOKEN}",
		"BACKEND_URL": "${resources.backend.url}",
	}}}
	parsed, err := FromMap(doc)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := parsed.Fragment(types())
	if err != nil {
		t.Fatal(err)
	}
	if len(fragment.Module.Externals) != 0 || len(fragment.Shared) != 0 {
		t.Fatalf("virtual resources were provisioned: %+v", fragment)
	}
	vars := fragment.Module.Spec.Containers["main"].Variables
	if vars["TOKEN"] != "${context.uc12.API_TOKEN}" || vars["BACKEND_URL"] != "${context.service.backend.http}" {
		t.Fatalf("unexpected virtual references: %+v", vars)
	}
}
