package workloadconfig

import (
	"context"
	"errors"
	"strings"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
)

const paramSentinel = "sentinel-param-value-91c2"

func paramsWorkloadService(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	ctx := context.Background()
	svc, st := testWorkloadService(t)
	if err := st.SaveOrganization(ctx, application.Organization{Key: "acme", Name: "Acme"}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveApplication(ctx, application.Application{Key: "app", OrganizationKey: "acme", ConfigurationProvider: "vault"}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveResourceType(ctx, "acme", resource.Type{
		Key: "postgres",
		Inputs: []resource.InputField{
			{Name: "database", Type: "string", Required: true},
			{Name: "size", Type: "number"},
			{Name: "ha", Type: "bool"},
			{Name: "extra", Type: "any"},
		},
		Outputs: []resource.OutputField{{Name: "host", Type: "string", Required: true}},
	}); err != nil {
		t.Fatal(err)
	}
	return svc, st
}

// unboundScore declares one resource no container binds.
func unboundScore(resources map[string]any) map[string]any {
	return testScore("api", map[string]string{}, resources)
}

// TestSaveAndImportValidateUnboundResourceParams covers UC-16 BR-14: every
// declared non-virtual resource is checked, Save and ValidateImport agree,
// the error names the path without the value and nothing is mutated.
func TestSaveAndImportValidateUnboundResourceParams(t *testing.T) {
	ctx := context.Background()
	svc, st := paramsWorkloadService(t)
	db := func(params map[string]any) map[string]any {
		return map[string]any{"db": map[string]any{"type": "postgres", "params": params}}
	}
	cases := map[string]struct {
		resources map[string]any
		path      string
	}{
		"unknown type":     {map[string]any{"db": map[string]any{"type": "cache-" + paramSentinel}}, "resources.db.type"},
		"undeclared param": {db(map[string]any{"database": "shop", "owner": paramSentinel}), "resources.db.params.owner"},
		"missing required": {db(map[string]any{"size": 2}), "resources.db.params.database"},
		"missing params":   {map[string]any{"db": map[string]any{"type": "postgres"}}, "resources.db.params.database"},
		"null required":    {db(map[string]any{"database": nil}), "resources.db.params.database"},
		"null any":         {db(map[string]any{"database": "shop", "extra": nil}), "resources.db.params.extra"},
		"string as number": {db(map[string]any{"database": "shop", "size": paramSentinel}), "resources.db.params.size"},
		"number as string": {db(map[string]any{"database": 5}), "resources.db.params.database"},
		"string as bool":   {db(map[string]any{"database": "shop", "ha": "true"}), "resources.db.params.ha"},
		"object as string": {db(map[string]any{"database": map[string]any{"name": paramSentinel}}), "resources.db.params.database"},
		"second resource":  {map[string]any{"a": map[string]any{"type": "postgres", "params": map[string]any{"database": "x"}}, "b": map[string]any{"type": "postgres", "params": map[string]any{"size": 1}}}, "resources.b.params.database"},
	}
	for name, tc := range cases {
		raw := unboundScore(tc.resources)
		_, saveErr := svc.Save(ctx, "app", "staging", "api", raw, 0)
		importErr := svc.ValidateImport(ctx, "app", "staging", raw)
		for label, err := range map[string]error{"save": saveErr, "import": importErr} {
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("%s %s: want ErrInvalid, got %v", name, label, err)
				continue
			}
			if !strings.Contains(err.Error(), tc.path) || strings.Contains(err.Error(), paramSentinel) {
				t.Errorf("%s %s: want path %s without value, got %v", name, label, tc.path, err)
			}
		}
		if saveErr != nil && importErr != nil && saveErr.Error() != importErr.Error() {
			t.Errorf("%s: save/import parity: %v vs %v", name, saveErr, importErr)
		}
	}
	env, err := st.GetEnvironment(ctx, "app", "staging")
	if err != nil {
		t.Fatal(err)
	}
	drafts, err := st.ListWorkloadDrafts(ctx, "app", "staging")
	if err != nil {
		t.Fatal(err)
	}
	if env.DraftVersion != 0 || len(drafts) != 0 || env.CurrentDeploymentSetID != "set-staging" {
		t.Fatalf("rejected input mutated state: version=%d drafts=%d set=%s", env.DraftVersion, len(drafts), env.CurrentDeploymentSetID)
	}

	valid := unboundScore(map[string]any{
		"db":  map[string]any{"type": "postgres", "params": map[string]any{"database": "shop", "size": 2, "ha": true, "extra": []any{"x"}}},
		"env": map[string]any{"type": "environment"},
	})
	if err := svc.ValidateImport(ctx, "app", "staging", valid); err != nil {
		t.Fatalf("valid import: %v", err)
	}
	view, err := svc.Save(ctx, "app", "staging", "api", valid, 0)
	if err != nil || view.DraftVersion != 1 {
		t.Fatalf("valid save: %+v %v", view, err)
	}
}

type failingTypesStore struct{ *store.Store }

func (failingTypesStore) ListResourceTypes(context.Context, string) ([]resource.Type, error) {
	return nil, errors.New("pq: connection refused password=" + paramSentinel)
}

// TestResourceParamValidationStoreFailureIsInternal keeps a catalog read
// failure out of the ErrInvalid validation path.
func TestResourceParamValidationStoreFailureIsInternal(t *testing.T) {
	ctx := context.Background()
	_, st := paramsWorkloadService(t)
	svc := NewService(failingTypesStore{st})
	raw := unboundScore(map[string]any{"db": map[string]any{"type": "postgres", "params": map[string]any{"database": "shop"}}})
	if _, err := svc.Save(ctx, "app", "staging", "api", raw, 0); err == nil || errors.Is(err, ErrInvalid) {
		t.Fatalf("save store failure must be internal: %v", err)
	}
	if err := svc.ValidateImport(ctx, "app", "staging", raw); err == nil || errors.Is(err, ErrInvalid) {
		t.Fatalf("import store failure must be internal: %v", err)
	}
	if env, _ := st.GetEnvironment(ctx, "app", "staging"); env.DraftVersion != 0 {
		t.Fatalf("store failure advanced draft version to %d", env.DraftVersion)
	}
}
