package store

import (
	"context"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/persistence/persistencetest"
	"testing"
)

func TestNewEnvironmentHasNoExecutionTarget(t *testing.T) {
	st := New()
	env := environment.Environment{Key: "staging", ApplicationKey: "app", NamespaceIdentity: "app-app-staging"}
	if err := st.SaveEnvironment(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	raw := st.state.Environments[envKey("app", "staging")]
	if raw.SecretStoreKey != "" || raw.InfrastructureScope != environment.ScopeEnvironment {
		t.Fatalf("stored Environment = %+v", raw)
	}
}

func TestEnvironmentOperationsContract(t *testing.T) { persistencetest.EnvironmentOperations(t, New()) }
