package kubernetes

import (
	"context"
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
