package scorek8s

import (
	"context"
	"encoding/json"
	"orchestrator/internal/adapters/kubernetes"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/execution"
	"os/exec"
	"strings"
	"testing"
)

func installed(t *testing.T) *Renderer {
	t.Helper()
	path, err := exec.LookPath("score-k8s")
	if err != nil {
		t.Skip("local score-k8s 0.15.0 unavailable")
	}
	r, err := New(context.Background(), path, kubernetes.NewRenderer())
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func request(r *Renderer) execution.RenderRequest {
	replicas := 3
	return execution.RenderRequest{WorkloadID: "backend", Namespace: "test-dev", Selection: resource.RenderingSelection{DefinitionKey: "render", DefinitionHash: "pin", DriverType: resource.DriverScoreK8s, Bundle: r.bundle}, Module: environment.Module{Profile: environment.ModuleProfile, Spec: environment.ModuleSpec{Replicas: &replicas, Containers: map[string]environment.Container{"main": {Image: "backend:v1", Command: []string{"sh", "-c"}, Args: []string{"echo ${KEEP_LITERAL}"}, ReadinessProbe: &environment.Probe{Path: "/health", Port: 8080}, Resources: &environment.ContainerResourceRequirements{Limits: &environment.ComputeResources{CPU: "100m", Memory: "64Mi"}}}}, Service: &environment.Service{Ports: map[string]environment.Port{"http": {Port: 8080}}}}}, PlainEnv: map[string]map[string]string{"main": {"PGHOST": "database.internal", "TEMPLATE": "{{ fail \"should stay literal\" }}", "PRICE": "${literal}"}}, SecretEnv: map[string]map[string]string{"main": {"PGPASSWORD": "database-secret-sentinel"}}, ConfigSecretName: "backend-rev-1", ConfigSecretKeys: map[string]map[string]string{"main": {"API_KEY": "main_API_KEY"}}, ImagePullSecret: "registry-pull", Labels: map[string]string{"orchestrator.io/application": "shop"}}
}
func TestRealCLIProtectsSecretsAndPreservesProductSemantics(t *testing.T) {
	r := installed(t)
	req := request(r)
	manifests, err := r.Render(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.Render(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range manifests {
		if m.Kind != "Secret" {
			raw, _ := json.Marshal(m.Object)
			if strings.Contains(string(raw), "database-secret-sentinel") {
				t.Fatal("raw secret leaked into workload")
			}
		}
	}
	a := objects(manifests)
	b := objects(second)
	if string(a) != string(b) {
		t.Fatal("unstable output in fresh workspaces")
	}
	if len(manifests) != 3 {
		t.Fatalf("expected Secret/Deployment/Service, got %d", len(manifests))
	}
	policy, _ := kubernetes.NewRenderer().Render(context.Background(), req)
	doc, patches, _, err := normalize(req, policy)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal([]any{doc, patches})
	if strings.Contains(string(raw), "database-secret-sentinel") {
		t.Fatal("raw secret leaked into workspace input")
	}
}
func objects(ms []execution.Manifest) []byte {
	out := []any{}
	for _, m := range ms {
		out = append(out, m.Object)
	}
	raw, _ := json.Marshal(out)
	return raw
}
func TestRealCLIPreservesAgentAndMultipleContainers(t *testing.T) {
	r := installed(t)
	req := request(r)
	req.ConfigSecretName = ""
	req.ConfigSecretKeys = nil
	req.Vault = &execution.VaultInjection{Address: "http://vault:8200", Role: "backend", ServiceAccount: "backend-vault", Bindings: map[string]map[string]string{"main": {"API_KEY": "kv2://kv/app/revision/key"}}}
	req.Module.Spec.Containers["sidecar"] = environment.Container{Image: "busybox:1", Args: []string{"sleep", "3600"}}
	if _, err := r.Render(context.Background(), req); err != nil {
		t.Fatal(err)
	}
}
func TestProtectedManifestRejectsNamespaceAndSecretRefPatch(t *testing.T) {
	r := &Renderer{}
	req := request(r)
	policy, _ := kubernetes.NewRenderer().Render(context.Background(), req)
	var generated []execution.Manifest
	for _, m := range policy {
		if m.Kind == "Deployment" {
			generated = append(generated, m)
		}
	}
	generated[0].Object["metadata"].(map[string]any)["namespace"] = "wrong"
	fresh, _ := kubernetes.NewRenderer().Render(context.Background(), req)
	if validateObjects(generated, fresh) == nil {
		t.Fatal("namespace patch accepted")
	}
	generated = fresh[1:2]
	pod := generated[0].Object["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)
	env := pod["containers"].([]any)[0].(map[string]any)["env"].([]any)
	for _, item := range env {
		e := item.(map[string]any)
		if from, ok := e["valueFrom"].(map[string]any); ok {
			from["secretKeyRef"].(map[string]any)["name"] = "foreign-secret"
		}
	}
	fresh, _ = kubernetes.NewRenderer().Render(context.Background(), req)
	if validateObjects(generated, fresh) == nil {
		t.Fatal("foreign Secret accepted")
	}
}

func TestNormalizationExcludesRawSecretsWithoutCLI(t *testing.T) {
	req := request(&Renderer{})
	policy, err := kubernetes.NewRenderer().Render(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	doc, patches, _, err := normalize(req, policy)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal([]any{doc, patches})
	if strings.Contains(string(raw), "database-secret-sentinel") || !strings.Contains(string(raw), "backend-env") || !strings.Contains(string(raw), "main_PGPASSWORD") {
		t.Fatal("workspace Secret bridge is unsafe")
	}
	malformed := execution.Manifest{Kind: "Deployment", Object: map[string]any{"metadata": policy[1].Object["metadata"], "spec": map[string]any{}}}
	if validateObjects([]execution.Manifest{malformed}, policy) == nil {
		t.Fatal("malformed generated Deployment accepted")
	}
}
