package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/persistence"
)

// A JSON snapshot written before ADR-011 carries the target on the
// Application only. Loading it locks each unset Environment with that target
// as LEGACY_APPLICATION, while a new unbound Application stays UNCONFIGURED;
// the conversion is deterministic on every restart and never touches the
// Organization default, identity, namespace or resource state.
func TestJSONLegacyApplicationBindingMigratesOnLoadAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.json")
	legacy := `{
  "organizations": {"acme": {"id":"o","key":"acme","name":"Acme","defaultConnectionKey":"default-cluster"}},
  "applications": {
    "old-k8s": {"id":"a1","key":"old-k8s","organizationKey":"acme","name":"Old","subdomain":"old","executionProfile":"internal-k8s","connectionKey":"lab","runtimeStatus":"READY","version":1},
    "old-aws": {"id":"a2","key":"old-aws","organizationKey":"acme","name":"OldAws","subdomain":"oldaws","executionProfile":"aws-eks","connectionKey":"cloud","region":"eu-west-1","runtimeStatus":"PENDING","version":1},
    "fresh":   {"id":"a3","key":"fresh","organizationKey":"acme","name":"Fresh","subdomain":"fresh","runtimeStatus":"UNCONFIGURED","version":1}
  },
  "environments": {
    "old-k8s/staging": {"id":"e1","key":"staging","applicationId":"a1","applicationKey":"old-k8s","name":"Staging","environmentType":"staging","namespaceIdentity":"app-old-k8s-staging","currentDeploymentSetId":"","version":4},
    "old-k8s/production": {"id":"e2","key":"production","applicationId":"a1","applicationKey":"old-k8s","name":"Production","environmentType":"production","namespaceIdentity":"app-old-k8s-production","currentDeploymentSetId":"","version":1},
    "old-aws/staging": {"id":"e3","key":"staging","applicationId":"a2","applicationKey":"old-aws","name":"Staging","environmentType":"staging","namespaceIdentity":"app-old-aws-staging","currentDeploymentSetId":"","version":1},
    "fresh/staging": {"id":"e4","key":"staging","applicationId":"a3","applicationKey":"fresh","name":"Staging","environmentType":"staging","namespaceIdentity":"app-fresh-staging","currentDeploymentSetId":"","version":1}
  }
}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	for restart := 0; restart < 3; restart++ {
		st, err := NewWithSnapshot(path)
		if err != nil {
			t.Fatal(err)
		}
		get := func(app, env string) environment.Environment {
			t.Helper()
			e, err := st.GetEnvironment(ctx, app, env)
			if err != nil {
				t.Fatal(err)
			}
			return e
		}
		for _, key := range []string{"staging", "production"} {
			e := get("old-k8s", key)
			if e.ConnectionKey != "lab" || e.Profile != application.ProfileInternalK8s || e.RuntimeStatus != application.RuntimeReady || e.InfrastructureScope != environment.ScopeLegacyApplication {
				t.Fatalf("restart %d: legacy k8s %s = %+v", restart, key, e)
			}
		}
		if e := get("old-k8s", "staging"); e.Version != 4 || e.NamespaceIdentity != "app-old-k8s-staging" {
			t.Fatalf("migration changed version/namespace: %+v", e)
		}
		aws := get("old-aws", "staging")
		if aws.ConnectionKey != "cloud" || aws.Profile != application.ProfileAWSEKS || aws.Region != "eu-west-1" || aws.RuntimeStatus != application.RuntimePending || aws.InfrastructureScope != environment.ScopeLegacyApplication {
			t.Fatalf("restart %d: legacy aws = %+v", restart, aws)
		}
		fresh := get("fresh", "staging")
		if raw := st.state.Environments[envKey("fresh", "staging")]; raw.RuntimeStatus != application.RuntimeUnconfigured || raw.InfrastructureScope != environment.ScopeEnvironment {
			t.Fatalf("restart %d: loaded row must carry explicit defaults: %+v", restart, raw)
		}
		if fresh.Configured() || fresh.Status() != application.RuntimeUnconfigured || fresh.InfrastructureScope != "" && fresh.InfrastructureScope != environment.ScopeEnvironment {
			t.Fatalf("restart %d: a new Application must stay UNCONFIGURED: %+v", restart, fresh)
		}
		// A locked legacy binding is immutable for the product too.
		if _, err := st.BindEnvironment(ctx, persistence.EnvironmentBinding{ApplicationKey: "old-k8s", EnvironmentKey: "staging", ConnectionKey: "lab", Profile: application.ProfileInternalK8s, RuntimeStatus: application.RuntimeReady, Scope: environment.ScopeEnvironment, ExpectedVersion: get("old-k8s", "staging").Version}); err == nil {
			t.Fatal("legacy binding must be locked")
		}
		org, _ := st.GetOrganization(ctx, "acme")
		if org.DefaultConnectionKey != "default-cluster" {
			t.Fatalf("default changed: %+v", org)
		}
		// Persist so the next loop iteration reads what a real restart reads.
		if err := st.SaveApplication(ctx, application.Application{Key: "fresh", OrganizationKey: "acme", Name: "Fresh", Subdomain: "fresh", Version: 1}); err != nil {
			t.Fatal(err)
		}
	}
}
