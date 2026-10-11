package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
)

// A snapshot whose Application still carries a retired execution target or
// provider is rejected with a reset-required error. Its target is never
// converted to an Environment binding and never silently dropped (T26).
func TestJSONRetiredApplicationTargetRequiresReset(t *testing.T) {
	cases := map[string]string{
		"profile":    `"executionProfile":"internal-k8s"`,
		"connection": `"connectionKey":"lab"`,
		"region":     `"region":"eu-west-1"`,
		"runtime":    `"runtimeStatus":"READY"`,
		"provider":   `"configurationProvider":"vault"`,
		"numeric":    `"region":5`,
		"object":     `"connectionKey":{"k":"lab"}`,
		"array":      `"executionProfile":["internal-k8s"]`,
		"bool":       `"configurationProvider":true`,
		"bad status": `"runtimeStatus":7`,
	}
	for name, field := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			doc := `{"applications":{"old":{"id":"a1","key":"old","organizationKey":"acme","name":"Old","subdomain":"old",` + field + `,"version":1}}}`
			if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := NewWithSnapshot(path)
			if err == nil || !strings.Contains(err.Error(), "reset") || strings.Contains(err.Error(), `"old"`) || strings.Contains(err.Error(), "internal-k8s") || strings.Contains(err.Error(), "eu-west-1") || strings.Contains(err.Error(), "vault") {
				t.Fatalf("want safe reset-required error, got %v", err)
			}
		})
	}
}

// Absent, null and empty retired fields carry no target and load normally.
func TestJSONEmptyRetiredApplicationFieldsLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	doc := `{"applications":{"old":{"id":"a1","key":"old","organizationKey":"acme","name":"Old","subdomain":"old","executionProfile":"","connectionKey":null,"region":"","runtimeStatus":"","configurationProvider":null,"version":1}}}`
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWithSnapshot(path); err != nil {
		t.Fatal(err)
	}
}

// An identity-only Application loads even when it carries the former
// UNCONFIGURED status marker; its Environments stay unconfigured and
// independently bound Environments keep their own binding across restarts.
func TestJSONIdentityOnlyApplicationReopensWithoutTargetFallback(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.json")
	doc := `{
  "organizations": {"acme": {"id":"o","key":"acme","name":"Acme","defaultConnectionKey":"default-cluster"}},
  "applications": {
    "fresh": {"id":"a3","key":"fresh","organizationKey":"acme","name":"Fresh","subdomain":"fresh","runtimeStatus":"UNCONFIGURED","version":1}
  },
  "environments": {
    "fresh/staging": {"id":"e4","key":"staging","applicationId":"a3","applicationKey":"fresh","name":"Staging","environmentType":"staging","namespaceIdentity":"app-fresh-staging","version":1},
    "fresh/production": {"id":"e5","key":"production","applicationId":"a3","applicationKey":"fresh","name":"Production","environmentType":"production","namespaceIdentity":"app-fresh-production","connectionKey":"lab","executionProfile":"internal-k8s","runtimeStatus":"READY","infrastructureScope":"ENVIRONMENT","version":2}
  }
}`
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	for restart := 0; restart < 3; restart++ {
		st, err := NewWithSnapshot(path)
		if err != nil {
			t.Fatal(err)
		}
		staging, err := st.GetEnvironment(ctx, "fresh", "staging")
		if err != nil {
			t.Fatal(err)
		}
		if staging.Configured() || staging.ConnectionKey != "" || staging.Status() != application.RuntimeUnconfigured {
			t.Fatalf("restart %d: staging must stay unconfigured: %+v", restart, staging)
		}
		if raw := st.state.Environments[envKey("fresh", "staging")]; raw.InfrastructureScope != environment.ScopeEnvironment {
			t.Fatalf("restart %d: explicit defaults missing: %+v", restart, raw)
		}
		prod, err := st.GetEnvironment(ctx, "fresh", "production")
		if err != nil {
			t.Fatal(err)
		}
		if prod.ConnectionKey != "lab" || prod.Profile != application.ProfileInternalK8s || prod.Version != 2 {
			t.Fatalf("restart %d: production binding changed: %+v", restart, prod)
		}
		if err := st.SaveApplication(ctx, application.Application{Key: "fresh", OrganizationKey: "acme", Name: "Fresh", Subdomain: "fresh", Version: 1}); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		app := raw[strings.Index(string(raw), `"applications"`):]
		app = app[:strings.Index(string(app), `"environments"`)]
		for _, old := range []string{"executionProfile", "connectionKey", "region", "runtimeStatus", "configurationProvider"} {
			if strings.Contains(string(app), old) {
				t.Fatalf("restart %d: saved Application carries %s: %s", restart, old, app)
			}
		}
	}
}
