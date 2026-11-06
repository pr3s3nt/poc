package planning

import (
	"regexp"
	"strings"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
)

func TestIDPInfrastructureNamesStayReadableAndGenerationIsolated(t *testing.T) {
	key, err := application.NewKey("nangcap", "Order Management")
	if err != nil {
		t.Fatal(err)
	}
	c := Context{App: application.Application{Key: key}, Env: environment.Environment{Key: "production"}}
	if got := c.ResourceName(); got != "idp-nangcap-order-management-production-"+key[len(key)-6:] {
		t.Fatal(got)
	}
	key, err = application.NewKey("acme", strings.Repeat("a", 22))
	if err != nil {
		t.Fatal(err)
	}
	c.App.Key = key
	seen := map[string]bool{}
	for _, generation := range []int64{0, 1, 2, 9223372036854775807} {
		c.Env.TargetGeneration = generation
		got := c.ResourceName()
		if len(got) > 49 || !regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`).MatchString(got) || !strings.HasPrefix(got, "idp-acme-") || seen[got] || got != c.ResourceName() {
			t.Fatalf("unsafe or unstable generation %d: %s", generation, got)
		}
		seen[got] = true
	}
	// Persisted historical keys keep their exact original naming algorithm.
	c.App.Key, c.Env.Key, c.Env.TargetGeneration = "catalog", "staging", 0
	if got := c.ResourceName(); got != "orch-catalog-staging-c6b7df330a7635c9ef22afe8" {
		t.Fatalf("legacy naming changed: %s", got)
	}
}
