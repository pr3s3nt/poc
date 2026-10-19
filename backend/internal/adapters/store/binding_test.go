package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/persistence/persistencetest"
)

func TestEnvironmentBindingContract(t *testing.T) {
	persistencetest.EnvironmentBinding(t, New())
}

// The JSON schema carries the SQL defaults explicitly: a new Environment is
// stored UNCONFIGURED / ENVIRONMENT, not through the Status()/Scope() helpers.
func TestNewEnvironmentIsStoredWithExplicitDefaults(t *testing.T) {
	st := New()
	env := environment.Environment{Key: "staging", ApplicationKey: "app", NamespaceIdentity: "app-app-staging"}
	if err := st.SaveEnvironment(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	raw := st.state.Environments[envKey("app", "staging")]
	if raw.RuntimeStatus != application.RuntimeUnconfigured || raw.InfrastructureScope != environment.ScopeEnvironment || raw.ConnectionKey != "" {
		t.Fatalf("stored row = %+v", raw)
	}
}

func TestEnvironmentOperationsContract(t *testing.T) {
	persistencetest.EnvironmentOperations(t, New())
}

// ADR-017: a snapshot whose Environment still carries a deployment binding is a retired baseline; the loader never converts it.
func TestEnvironmentWithARetiredDeploymentBindingRequiresReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	first, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	persistencetest.EnvironmentBinding(t, first)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewWithSnapshot(path); err == nil || !strings.Contains(err.Error(), "retired deployment binding") {
		t.Fatalf("a bound Environment was loaded: %v", err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatal("the loader rewrote the snapshot")
	}
}
