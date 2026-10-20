package planning

import (
	"errors"
	"orchestrator/internal/domain/environment"
	"strings"
	"testing"
)

func TestPlanRejectsInvalidEnvironmentInfrastructureScope(t *testing.T) {
	for _, bad := range []environment.InfrastructureScope{"LEGACY_APPLICATION", "APPLICATION", "Env-Scope", "Prod"} {
		req := testRequest(t, "internal-k8s", "backend")
		req.Env.InfrastructureScope = bad
		plan, err := NewService(testBundles()).Plan(req)
		if plan != nil || !errors.Is(err, environment.ErrInvalidInfrastructureScope) || strings.Contains(err.Error(), string(bad)) {
			t.Fatalf("scope %q: plan=%v err=%v", bad, plan, err)
		}
	}
	req := testRequest(t, "internal-k8s", "backend")
	req.Env.InfrastructureScope = ""
	plan, err := NewService(testBundles()).Plan(req)
	if err != nil || plan.Target.Scope != environment.ScopeEnvironment {
		t.Fatalf("unset scope: plan=%v err=%v", plan, err)
	}
}

func scopedAWSRequest(t *testing.T, appKey, envKey string) Request {
	t.Helper()
	req := testRequest(t, "internal-k8s", "backend")
	req.App.Key = appKey
	req.Env.ApplicationKey = appKey
	req.Env.Key = envKey
	req.Env.NamespaceIdentity = "app-" + appKey + "-" + envKey
	return req
}
