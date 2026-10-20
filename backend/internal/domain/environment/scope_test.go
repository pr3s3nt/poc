package environment

import (
	"errors"
	"strings"
	"testing"
)

func TestOnlyEnvironmentScopeIsValid(t *testing.T) {
	if !ScopeEnvironment.Valid() {
		t.Fatal("ENVIRONMENT must be valid")
	}
	for _, bad := range []InfrastructureScope{"", "LEGACY_APPLICATION", "APPLICATION", "environment", "ENVIRONMENT "} {
		if bad.Valid() {
			t.Fatalf("scope %q must be invalid", bad)
		}
	}
}

func TestEnvironmentRejectsExplicitInvalidScopeAndNeverNormalizesIt(t *testing.T) {
	base := Environment{Key: "dev", ApplicationKey: "app", NamespaceIdentity: "app-dev"}
	if err := base.Validate(); err != nil || base.Scope() != ScopeEnvironment || !base.InfrastructureScopeValid() {
		t.Fatalf("unset scope: %v %q", err, base.Scope())
	}
	for _, bad := range []InfrastructureScope{"LEGACY_APPLICATION", "Env-Scope"} {
		env := base
		env.InfrastructureScope = bad
		err := env.Validate()
		if !errors.Is(err, ErrInvalidInfrastructureScope) || strings.Contains(err.Error(), string(bad)) {
			t.Fatalf("scope %q: %v", bad, err)
		}
		if env.InfrastructureScopeValid() || env.Scope() != bad || env.Binding().Scope != bad {
			t.Fatalf("scope %q was normalized: %q", bad, env.Scope())
		}
	}
}
