package e2e

import (
	"encoding/json"

	"orchestrator/internal/adapters/terraform/registry"
)

// packagedPin returns the public pin (source_id, module_id, revision,
// subdirectory, content_sha256) of a packaged registry module, exactly as the
// capability metadata publishes it.
func packagedPin(module string) map[string]any {
	refs, err := registry.PackagedRefs()
	if err != nil {
		panic(err)
	}
	ref, ok := refs[module]
	if !ok {
		panic("unknown packaged module " + module)
	}
	return ref.Map()
}

func mustJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

const namespaceOutput = "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"

// k8sModuleDefinition is the registrable shape of an in-cluster Kubernetes
// resource: a verified packaged module bound to its inherited cluster (ADR-014).
// Static kubernetes drivers are not registrable.
func k8sModuleDefinition(key, resourceType, module string, variables map[string]any, inputs, outputs []string, criteria []any) map[string]any {
	inputMapping := map[string]any{}
	for _, name := range inputs {
		inputMapping[name] = map[string]any{"param": name}
	}
	outputMapping := map[string]any{}
	for _, name := range outputs {
		outputMapping[name] = name
	}
	return map[string]any{
		"key": key, "name": "Định nghĩa " + key, "resourceType": resourceType, "driverType": "terraform",
		"driverInputs": map[string]any{"values": map[string]any{
			"source": packagedPin(module), "variables": variables, "input_mapping": inputMapping, "output_mapping": outputMapping,
			"provider_bindings": map[string]any{"kubernetes": map[string]any{"target": "resource", "resource_ref": "k8s-cluster.internal#environments.@app.@env"}},
		}},
		"criteria": criteria,
	}
}

var postgresOutputs = []string{"host", "port", "database", "username", "password"}

func postgresModuleDefinition(key string, variables map[string]any, criteria []any) map[string]any {
	return k8sModuleDefinition(key, "postgres", "postgres-k8s", variables, []string{"database", "username"}, postgresOutputs, criteria)
}

// postgresBody is a PostgreSQL module Definition request body. A Kubernetes
// module takes no Driver Account (its cluster access is inherited), so there is
// no connection parameter any more (UC-03 V02-03).
func postgresBody(key string, criteria []any) string {
	def := postgresModuleDefinition(key, map[string]any{"image": "postgres:17-alpine", "storage": "2Gi", "namespace": namespaceOutput}, criteria)
	return mustJSON(def)
}

// fastPostgres registers a PostgreSQL module for class "fast".
var fastPostgres = postgresBody("postgres-fast", []any{map[string]any{"class": "fast"}})
