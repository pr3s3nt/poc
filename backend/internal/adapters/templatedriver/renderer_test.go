package templatedriver

import (
	"encoding/json"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/execution"
	"testing"
)

func installed(t *testing.T) *Renderer { t.Helper(); return New() }
func request(r *Renderer) execution.RenderRequest {
	replicas := 3
	return execution.RenderRequest{WorkloadID: "backend", Namespace: "test-dev", Selection: resource.RenderingSelection{DefinitionKey: "render", DefinitionHash: "pin", DriverType: resource.DriverTemplate}, Module: environment.Module{Profile: environment.ModuleProfile, Spec: environment.ModuleSpec{Replicas: &replicas, Containers: map[string]environment.Container{"main": {Image: "backend:v1", Command: []string{"sh", "-c"}, Args: []string{"echo ${KEEP_LITERAL}"}, ReadinessProbe: &environment.Probe{Path: "/health", Port: 8080}, Resources: &environment.ContainerResourceRequirements{Limits: &environment.ComputeResources{CPU: "100m", Memory: "64Mi"}}}}, Service: &environment.Service{Ports: map[string]environment.Port{"http": {Port: 8080}}}}}, PlainEnv: map[string]map[string]string{"main": {"PGHOST": "database.internal", "TEMPLATE": "{{ fail \"should stay literal\" }}", "PRICE": "${literal}"}}, SecretEnv: map[string]map[string]string{"main": {"PGPASSWORD": "database-secret-sentinel"}}, ConfigSecretName: "backend-rev-1", ConfigSecretKeys: map[string]map[string]string{"main": {"API_KEY": "main_API_KEY"}}, ImagePullSecret: "registry-pull", Labels: map[string]string{"orchestrator.io/application": "shop"}}
}
func objects(ms []execution.Manifest) []byte {
	out := []any{}
	for _, m := range ms {
		out = append(out, m.Object)
	}
	raw, _ := json.Marshal(out)
	return raw
}
