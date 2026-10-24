package deploymentreview

import (
	"context"
	"strings"
	"testing"

	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/execution"
)

type pinnedReader struct {
	called int
	files  map[string][]byte
}

func (r *pinnedReader) ReadPinned(_ context.Context, pin tfmodule.SourceRef) (map[string][]byte, error) {
	r.called++
	if pin.ContentSHA256 != "pinned-digest" {
		panic("wrong pin")
	}
	return r.files, nil
}

func TestProjectHCLRedactsNestedLiteralsCommentsAndOutputs(t *testing.T) {
	secret := "literal-credential-should-never-appear"
	input := `# ` + secret + `
variable "password" {
  default = { nested = "` + secret + `" }
}
resource "aws_db_instance" "db" {
  password = "` + secret + `"
}
output "password" {
  value = "` + secret + `"
  sensitive = true
}
`
	projection, paths, ok := ProjectHCL([]byte(input))
	if !ok || len(paths) < 3 {
		t.Fatalf("projection unavailable: %v %v", ok, paths)
	}
	if strings.Contains(projection, secret) || strings.Contains(projection, "#") {
		t.Fatalf("secret/comment escaped projection: %s", projection)
	}
	for _, part := range []string{"variable \"password\"", "resource \"aws_db_instance\" \"db\"", "output \"password\"", "<redacted>"} {
		if !strings.Contains(projection, part) {
			t.Fatalf("missing structural source %q: %s", part, projection)
		}
	}
}

func TestProjectHCLRejectsMalformedOrOversizedContent(t *testing.T) {
	for _, input := range [][]byte{[]byte("variable {"), []byte{0, 1}, []byte(strings.Repeat("x", MaxSideBytes+1))} {
		if _, _, ok := ProjectHCL(input); ok {
			t.Fatal("unsafe content shown")
		}
	}
}

func TestProjectManifestsNeverCopiesSecretOrLiteralEnv(t *testing.T) {
	secret := "literal-credential-should-never-appear"
	object := map[string]any{"spec": map[string]any{"replicas": 2, "template": map[string]any{"metadata": map[string]any{"annotations": map[string]any{"token": secret}}, "spec": map[string]any{"containers": []any{map[string]any{"name": "api", "image": "example/api:v1", "env": []any{map[string]any{"name": "PASSWORD", "value": secret}, map[string]any{"name": "DB_PASSWORD", "valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": "db-secret", "key": "password"}}}}}}}}}}
	projected := ProjectManifests([]execution.Manifest{{APIVersion: "apps/v1", Kind: "Deployment", Name: "api", Namespace: "test", Object: object}, {APIVersion: "v1", Kind: "Secret", Name: "api-env", Secret: true, Object: map[string]any{"stringData": map[string]any{"PASSWORD": secret}}}})
	if len(projected) != 1 {
		t.Fatalf("unexpected objects: %v", projected)
	}
	text := projected["Deployment/api"].Text
	if strings.Contains(text, secret) || strings.Contains(text, "annotations") || strings.Contains(text, "stringData") {
		t.Fatalf("secret escaped projection: %s", text)
	}
	if !strings.Contains(text, "db-secret") || !strings.Contains(text, "<redacted>") {
		t.Fatalf("safe ref or redaction missing: %s", text)
	}
}

func TestProjectManifestsDropsUnknownNestedFields(t *testing.T) {
	secret := "nested-credential-should-never-appear"
	object := map[string]any{"spec": map[string]any{
		"selector": map[string]any{"matchLabels": map[string]any{"app.kubernetes.io/name": "api"}, "extra": secret},
		"template": map[string]any{"spec": map[string]any{"containers": []any{map[string]any{
			"name": "api", "image": "example/api:v1", "ports": []any{map[string]any{"containerPort": 8080, "unknown": secret}},
			"resources":      map[string]any{"requests": map[string]any{"cpu": "10m", "unknown": secret}},
			"readinessProbe": map[string]any{"httpGet": map[string]any{"path": "/health", "unknown": secret}},
		}}}},
	}}
	got := ProjectManifests([]execution.Manifest{{APIVersion: "apps/v1", Kind: "Deployment", Name: "api", Namespace: "test", Object: object}})["Deployment/api"]
	if got.Availability != "PARTIAL" || strings.Contains(got.Text, secret) || !strings.Contains(got.Text, "containerPort") {
		t.Fatalf("nested projection was unsafe or empty: %#v", got)
	}
}

// ADR-021 RB-C06: accepted Git policy fields are reviewable; platform
// delivery and credential-named annotations stay out of the projection.
func TestProjectManifestsShowsAcceptedPolicyFields(t *testing.T) {
	m := execution.Manifest{APIVersion: "apps/v1", Kind: "Deployment", Name: "api", Namespace: "test", Object: map[string]any{
		"metadata": map[string]any{"labels": map[string]any{"example.com/team": "payments"}},
		"spec": map[string]any{"replicas": 1, "revisionHistoryLimit": 3, "strategy": map[string]any{"type": "RollingUpdate"}, "template": map[string]any{
			"metadata": map[string]any{"annotations": map[string]any{"example.com/render-policy": "b", "vault.hashicorp.com/agent-inject-template-app-env": "{{ with secret }}", "example.com/api-token": "tok-sentinel"}},
			"spec": map[string]any{"securityContext": map[string]any{"seccompProfile": map[string]any{"type": "RuntimeDefault"}}, "nodeSelector": map[string]any{"kubernetes.io/os": "linux"},
				"containers": []any{map[string]any{"name": "api", "image": "example/api:v1", "securityContext": map[string]any{"allowPrivilegeEscalation": false}}}}}}}}
	side := ProjectManifests([]execution.Manifest{m})["Deployment/api"]
	for _, want := range []string{"example.com/team: payments", "example.com/render-policy: b", "RuntimeDefault", "allowPrivilegeEscalation: false", "revisionHistoryLimit: 3", "RollingUpdate", "kubernetes.io/os: linux"} {
		if !strings.Contains(side.Text, want) {
			t.Fatalf("projection misses %q:\n%s", want, side.Text)
		}
	}
	for _, leak := range []string{"vault.hashicorp.com", "with secret", "tok-sentinel"} {
		if strings.Contains(side.Text, leak) {
			t.Fatalf("projection leaks %q:\n%s", leak, side.Text)
		}
	}
}
