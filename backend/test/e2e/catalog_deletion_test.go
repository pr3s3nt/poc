package e2e

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/persistence"
)

func TestCatalogDeletionRequiresScopedRoleAndNoReferences(t *testing.T) {
	f := newRCFixture(t)
	ctx := context.Background()
	const typeKey, defKey = "unused-type", "unused-def"
	if err := f.app.Store.CreateResourceType(ctx, f.orgKey, resource.Type{Key: typeKey}); err != nil {
		t.Fatal(err)
	}
	def := resource.Definition{
		Key: defKey, Name: "Unused definition", Version: 1,
		ResourceTypeKey: typeKey, DriverType: resource.DriverKubernetes, Criteria: []resource.Criterion{{}},
	}
	if err := f.app.Store.CreateResourceDefinition(ctx, f.orgKey, def); err != nil {
		t.Fatal(err)
	}

	typeURL := f.server.URL + "/api/v1/resource-types/" + typeKey
	defURL := f.server.URL + "/api/v1/resource-definitions/" + defKey
	if status, raw, _ := f.do(f.dev, http.MethodDelete, defURL, nil); status != http.StatusForbidden {
		t.Fatalf("developer deletion = %d %s", status, raw)
	}
	if status, raw, _ := f.do(f.foreign, http.MethodDelete, defURL, nil); status != http.StatusNotFound && status != http.StatusForbidden {
		t.Fatalf("foreign deletion = %d %s", status, raw)
	}
	status, raw, body := f.do(f.pe, http.MethodDelete, typeURL, nil)
	refs, _ := body["references"].([]any)
	if status != http.StatusConflict || body["code"] != "CATALOG_DELETION_BLOCKED" || len(refs) != 1 || refs[0] != "definition/"+defKey {
		t.Fatalf("referenced Type deletion = %d %s", status, raw)
	}

	before, err := f.app.Store.GetEnvironment(ctx, f.appKey, "staging")
	if err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{defURL, typeURL} {
		if status, raw, _ := f.do(f.pe, http.MethodDelete, endpoint, nil); status != http.StatusNoContent {
			t.Fatalf("unused entry deletion = %d %s", status, raw)
		}
		if status, raw, _ := f.do(f.pe, http.MethodDelete, endpoint, nil); status != http.StatusNotFound {
			t.Fatalf("repeated deletion = %d %s", status, raw)
		}
	}
	after, err := f.app.Store.GetEnvironment(ctx, f.appKey, "staging")
	if err != nil || after.Version != before.Version+2 || after.DraftVersion != before.DraftVersion+2 {
		t.Fatalf("catalog deletion did not invalidate stale writes: before=%+v after=%+v err=%v", before, after, err)
	}
	if status, _, _ := f.do(f.pe, http.MethodDelete, f.server.URL+"/api/v1/resource-types/k8s-cluster", nil); status != http.StatusConflict {
		t.Fatalf("built-in Type deletion = %d", status)
	}
}

func TestCatalogDeletionSerializesWithDefinitionRegistration(t *testing.T) {
	f := newRCFixture(t)
	ctx := context.Background()
	const typeKey = "type-race"
	if err := f.app.Store.CreateResourceType(ctx, f.orgKey, resource.Type{Key: typeKey}); err != nil {
		t.Fatal(err)
	}
	def := resource.Definition{
		Key: "def-race", Name: "Concurrent definition", Version: 1,
		ResourceTypeKey: typeKey, DriverType: resource.DriverKubernetes, Criteria: []resource.Criterion{{}},
	}
	start := make(chan struct{})
	registered := make(chan error, 1)
	go func() {
		<-start
		registered <- f.app.Store.CreateResourceDefinition(ctx, f.orgKey, def)
	}()
	close(start)
	status, raw, _ := f.do(f.pe, http.MethodDelete, f.server.URL+"/api/v1/resource-types/"+typeKey, nil)
	err := <-registered
	switch status {
	case http.StatusNoContent:
		if !errors.Is(err, persistence.ErrNotFound) {
			t.Fatalf("deletion won; registration should reject the deleted Type, got %v", err)
		}
	case http.StatusConflict:
		if err != nil || !strings.Contains(raw, "definition/def-race") {
			t.Fatalf("registration won; deletion should report its reference: %s %v", raw, err)
		}
	default:
		t.Fatalf("unexpected deletion response: %d %s", status, raw)
	}
}
