package store

import (
	"context"
	"path/filepath"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/persistence/persistencetest"
)

func TestEnvironmentBindingContract(t *testing.T) {
	persistencetest.EnvironmentBinding(t, New())
}

func TestEnvironmentBindingSurvivesJSONRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	first, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	fixture := persistencetest.EnvironmentBinding(t, first)
	reopened, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	persistencetest.AssertBindingReloaded(t, reopened, fixture)
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
