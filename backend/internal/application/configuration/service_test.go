package configuration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/internal/adapters/configmemory"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/domain/application"
	domain "orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/persistence"
)

func TestUC12DesiredRevisionRedactionAndIsolation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.json")
	st, err := store.NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveApplication(ctx, application.Application{Key: "app-1", ConfigurationProvider: "vault"}); err != nil {
		t.Fatal(err)
	}
	for _, env := range []string{"staging", "production"} {
		if err := st.SaveEnvironment(ctx, environment.Environment{ApplicationKey: "app-1", Key: env}); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewService(st, configmemory.New())
	secret := "do-not-store-this-password"
	view, err := svc.Put(ctx, "app-1", "staging", "API_TOKEN", domain.Secret, secret, 0)
	if err != nil {
		t.Fatal(err)
	}
	if view.Version != 1 || len(view.Keys) != 1 || view.Keys[0].Value != nil {
		t.Fatalf("secret leaked in view: %+v", view)
	}
	if _, err := svc.Put(ctx, "app-1", "staging", "API_TOKEN", domain.Variable, "plain", 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("want cross-type name conflict, got %v", err)
	}
	if _, err := svc.Put(ctx, "app-1", "staging", "LOG_LEVEL", domain.Variable, "debug", 0); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("want stale version conflict, got %v", err)
	}
	view, err = svc.Put(ctx, "app-1", "staging", "LOG_LEVEL", domain.Variable, "debug", 1)
	if err != nil || view.Version != 2 || view.Keys[1].Value == nil || *view.Keys[1].Value != "debug" {
		t.Fatalf("variable read failed: %+v, %v", view, err)
	}
	other, err := svc.List(ctx, "app-1", "production")
	if err != nil || len(other.Keys) != 0 {
		t.Fatalf("production changed: %+v, %v", other, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secret) || strings.Contains(string(data), "debug") {
		t.Fatal("raw configuration value reached state snapshot")
	}
	view, err = svc.Rename(ctx, "app-1", "staging", "API_TOKEN", "NEW_TOKEN", 2)
	if err != nil || view.Version != 3 {
		t.Fatalf("rename failed: %+v, %v", view, err)
	}
	view, err = svc.Delete(ctx, "app-1", "staging", "NEW_TOKEN", 3)
	if err != nil || view.Version != 4 {
		t.Fatalf("delete failed: %+v, %v", view, err)
	}
}
