package kubernetes

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/execution"
)

func renderBackend(t *testing.T) []execution.Manifest {
	t.Helper()
	replicas := 1
	module := environment.Module{
		Profile: environment.ModuleProfile,
		Spec: environment.ModuleSpec{
			Containers: map[string]environment.Container{
				"main": {
					Image:          "acceptance-backend:dev",
					ReadinessProbe: &environment.Probe{Path: "/readyz", Port: 8080},
				},
			},
			Service:  &environment.Service{Ports: map[string]environment.Port{"http": {Port: 8080, TargetPort: 8080}}},
			Replicas: &replicas,
		},
	}
	manifests, err := NewRenderer().Render(context.Background(), execution.RenderRequest{
		WorkloadID: "backend",
		Module:     module,
		Namespace:  "acceptance-dev",
		PlainEnv:   map[string]map[string]string{"main": {"PGHOST": "acceptance-db", "PGPORT": "5432"}},
		SecretEnv:  map[string]map[string]string{"main": {"PGPASSWORD": "s3cret"}},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return manifests
}

func TestRenderProducesSecretDeploymentAndService(t *testing.T) {
	manifests := renderBackend(t)
	kinds := map[string]bool{}
	for _, m := range manifests {
		kinds[m.Kind] = true
		if m.Namespace != "acceptance-dev" {
			t.Fatalf("manifest %s/%s is in namespace %q", m.Kind, m.Name, m.Namespace)
		}
	}
	for _, want := range []string{"Secret", "Deployment", "Service"} {
		if !kinds[want] {
			t.Fatalf("missing %s manifest: %v", want, kinds)
		}
	}
}

func TestRenderReferencesNamespaceHarborPullSecret(t *testing.T) {
	module := environment.Module{Profile: environment.ModuleProfile, Spec: environment.ModuleSpec{Containers: map[string]environment.Container{"main": {Image: "harbor.example/library/frontend:v1"}}}}
	manifests, err := NewRenderer().Render(context.Background(), execution.RenderRequest{WorkloadID: "frontend", Module: module, Namespace: "app-demo-staging", ImagePullSecret: "harbor-pull"})
	if err != nil {
		t.Fatal(err)
	}
	for _, manifest := range manifests {
		if manifest.Kind != "Deployment" {
			continue
		}
		pod := manifest.Object["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)
		secrets := pod["imagePullSecrets"].([]any)
		if len(secrets) != 1 || secrets[0].(map[string]any)["name"] != "harbor-pull" {
			t.Fatalf("unexpected pull secrets: %v", secrets)
		}
		return
	}
	t.Fatal("missing Deployment")
}

func TestRenderKeepsSecretValuesOutOfTheDeployment(t *testing.T) {
	manifests := renderBackend(t)
	for _, m := range manifests {
		if m.Kind != "Deployment" {
			continue
		}
		containers := m.Object["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)["containers"].([]any)
		env := containers[0].(map[string]any)["env"].([]any)
		var sawPlain, sawSecretRef bool
		for _, item := range env {
			entry := item.(map[string]any)
			switch entry["name"] {
			case "PGHOST":
				sawPlain = entry["value"] == "acceptance-db"
			case "PGPASSWORD":
				if entry["value"] != nil {
					t.Fatal("secret value must not be inlined in the Deployment")
				}
				sawSecretRef = entry["valueFrom"] != nil
			}
		}
		if !sawPlain || !sawSecretRef {
			t.Fatalf("unexpected env block: %v", env)
		}
	}
}

func TestRenderRejectsMissingNamespace(t *testing.T) {
	_, err := NewRenderer().Render(context.Background(), execution.RenderRequest{WorkloadID: "backend"})
	if err == nil {
		t.Fatal("expected an error when the namespace is missing")
	}
}

func TestRenderVaultAgentFileContainsOnlyReferencesAndSafeTemplate(t *testing.T) {
	module := environment.Module{Profile: environment.ModuleProfile, Spec: environment.ModuleSpec{Containers: map[string]environment.Container{"main": {Image: "example.invalid/test:1"}}}}
	manifests, err := NewRenderer().Render(context.Background(), execution.RenderRequest{
		WorkloadID: "frontend", Module: module, Namespace: "app-staging",
		Vault: &execution.VaultInjection{Address: "http://vault-uc12.vault.svc:8200", Role: "orch-role", ServiceAccount: "frontend-vault", Bindings: map[string]map[string]string{
			"main": {"API_TOKEN": "kv2://kv/orchestrator/apps/app/envs/staging/values/immutable-id"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var serviceAccount, deployment, kubeSecret bool
	for _, m := range manifests {
		if m.Kind == "ServiceAccount" {
			serviceAccount = true
		}
		if m.Kind == "Secret" {
			kubeSecret = true
		}
		if m.Kind != "Deployment" {
			continue
		}
		deployment = true
		pod := m.Object["spec"].(map[string]any)["template"].(map[string]any)
		annotations := pod["metadata"].(map[string]any)["annotations"].(map[string]any)
		template := annotations["vault.hashicorp.com/agent-inject-template-app-env"].(string)
		if !strings.Contains(template, "base64Encode") || !strings.Contains(template, "export API_TOKEN") || strings.Contains(template, "raw-secret") {
			t.Fatalf("unsafe Agent template: %s", template)
		}
		if pod["spec"].(map[string]any)["serviceAccountName"] != "frontend-vault" {
			t.Fatal("wrong ServiceAccount")
		}
		data, _ := json.Marshal(m.Object)
		if strings.Contains(string(data), "raw-secret") {
			t.Fatal("raw secret in Deployment")
		}
	}
	if !serviceAccount || !deployment || kubeSecret {
		t.Fatalf("unexpected manifests: serviceAccount=%v deployment=%v KubernetesSecret=%v", serviceAccount, deployment, kubeSecret)
	}
}

func TestRenderVSOUsesSecretKeyRefWithoutAgentOrValues(t *testing.T) {
	module := environment.Module{Profile: environment.ModuleProfile, Spec: environment.ModuleSpec{Containers: map[string]environment.Container{"main": {Image: "example.invalid/test:1"}}}}
	manifests, err := NewRenderer().Render(context.Background(), execution.RenderRequest{
		WorkloadID: "frontend", Module: module, Namespace: "app-staging",
		ConfigSecretName: "orch-revision", ConfigSecretKeys: map[string]map[string]string{"main": {"API_TOKEN": "main_API_TOKEN"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(manifests) != 1 || manifests[0].Kind != "Deployment" {
		t.Fatalf("unexpected manifests: %+v", manifests)
	}
	pod := manifests[0].Object["spec"].(map[string]any)["template"].(map[string]any)
	if _, ok := pod["metadata"].(map[string]any)["annotations"]; ok {
		t.Fatal("unexpected Agent annotations")
	}
	container := pod["spec"].(map[string]any)["containers"].([]any)[0].(map[string]any)
	env := container["env"].([]any)[0].(map[string]any)
	ref := env["valueFrom"].(map[string]any)["secretKeyRef"].(map[string]any)
	if env["name"] != "API_TOKEN" || ref["name"] != "orch-revision" || ref["key"] != "main_API_TOKEN" {
		t.Fatalf("wrong VSO reference: %+v", env)
	}
	if _, ok := env["value"]; ok {
		t.Fatal("value was inlined")
	}
	if _, err := NewRenderer().Render(context.Background(), execution.RenderRequest{WorkloadID: "frontend", Module: module, Namespace: "app-staging", ConfigSecretKeys: map[string]map[string]string{"main": {"API_TOKEN": "main_API_TOKEN"}}}); err == nil {
		t.Fatal("accepted missing Secret name")
	}
}
