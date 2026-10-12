package store

import (
	"context"
	"errors"
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

func TestEnvironmentOperationsContract(t *testing.T) {
	persistencetest.EnvironmentOperations(t, New())
}

func TestEnvironmentOperationsSurviveJSONRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	first, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	fixture := persistencetest.EnvironmentOperations(t, first)
	reopened, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	persistencetest.AssertOperationsReloaded(t, reopened, fixture)
}

// A snapshot with an explicit non-ENVIRONMENT scope is refused on startup,
// configured or not, before the unconfigured defaults could erase it. The
// error never quotes the stored value and the file is not rewritten.
func TestSnapshotWithRetiredInfrastructureScopeIsRejected(t *testing.T) {
	for name, binding := range map[string]string{
		"configured":   `"connectionKey":"lab","executionProfile":"internal-k8s","runtimeStatus":"READY",`,
		"unconfigured": ``,
	} {
		path := filepath.Join(t.TempDir(), "state.json")
		raw := `{"environments":{"app/dev":{"key":"dev","applicationKey":"app","namespaceIdentity":"app-dev",` + binding + `"infrastructureScope":"LEGACY_APPLICATION"}}}`
		if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		st, err := NewWithSnapshot(path)
		if err == nil || st != nil || !errors.Is(err, environment.ErrInvalidInfrastructureScope) || strings.Contains(err.Error(), "LEGACY_APPLICATION") {
			t.Fatalf("%s: store=%v err=%v", name, st, err)
		}
		if after, _ := os.ReadFile(path); string(after) != raw {
			t.Fatalf("%s: snapshot was rewritten", name)
		}
	}
}
