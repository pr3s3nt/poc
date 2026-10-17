package e2e

import (
	"net/http"
	"testing"
)

// TestT31DefinitionCapabilitiesGuideRegistrationHTTP checks that the read-only
// capability metadata (OC-04d) matches what registration accepts: a definition
// built from presets and omitting defaults registers, executor-owned and
// credential variables stay rejected or hidden, and the read mutates nothing.
func TestT31DefinitionCapabilitiesGuideRegistrationHTTP(t *testing.T) {
	server, _ := newServer(t, "", approvedCluster{})
	if status, _ := requestJSON(t, &http.Client{}, http.MethodGet, server.URL+"/api/v1/resource-definition-capabilities", nil); status != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", status)
	}
	dev := authenticatedClientAs(t, server.URL, "developer")
	if status, _ := requestJSON(t, dev, http.MethodGet, server.URL+"/api/v1/resource-definition-capabilities", nil); status != http.StatusForbidden {
		t.Fatalf("developer: %d", status)
	}
	pe := authenticatedClientAs(t, server.URL, "platform-engineer")
	count := func() int {
		_, out := requestJSON(t, pe, http.MethodGet, server.URL+"/api/v1/resource-definitions", nil)
		return len(out["resourceDefinitions"].([]any))
	}
	before := count()
	status, out := requestJSON(t, pe, http.MethodGet, server.URL+"/api/v1/resource-definition-capabilities", nil)
	if status != http.StatusOK {
		t.Fatalf("capabilities: %d %v", status, out)
	}
	presets := map[string]string{}
	var vpcSource map[string]any
	for _, raw := range out["capabilities"].([]any) {
		capability := raw.(map[string]any)
		if capability["resourceType"] == "vpc" && capability["driverType"] == "terraform" {
			pin := capability["source"].(map[string]any)
			// The published pin is what registration takes as driverInputs.values.source.
			vpcSource = map[string]any{"source_id": pin["sourceId"], "module_id": pin["moduleId"], "revision": pin["revision"], "subdirectory": pin["subdirectory"], "content_sha256": pin["contentSha256"]}
		}
		for _, rawVariable := range capability["variables"].([]any) {
			variable := rawVariable.(map[string]any)
			if preset, ok := variable["preset"].(string); ok {
				presets[capability["resourceType"].(string)+"/"+capability["driverType"].(string)+"/"+variable["name"].(string)] = preset
			}
		}
	}
	for key, want := range map[string]string{
		"k8s-namespace/terraform/name": "${context.env.namespace}",
		"vpc/terraform/name":           "${context.infra.resourceName}",
	} {
		if presets[key] != want {
			t.Fatalf("preset %s = %q, want %q", key, presets[key], want)
		}
	}
	if count() != before {
		t.Fatal("metadata GET changed Definitions")
	}
	register := func(body map[string]any) int {
		status, _ := requestJSON(t, pe, http.MethodPost, server.URL+"/api/v1/resource-definitions", body)
		return status
	}
	nsDef := k8sModuleDefinition("ns-t31", "k8s-namespace", "k8s-namespace", map[string]any{"name": presets["k8s-namespace/terraform/name"]}, nil, []string{"name"}, []any{map[string]any{"class": "t31"}})
	if status := register(nsDef); status != http.StatusCreated {
		t.Fatalf("preset namespace definition: %d", status)
	}
	// The retired static Kubernetes driver is not registrable (ADR-014).
	legacy := map[string]any{
		"key": "ns-t31-static", "name": "Static", "resourceType": "k8s-namespace", "driverType": "kubernetes",
		"driverInputs": map[string]any{"values": map[string]any{"variables": map[string]any{"name": "x"}}},
		"criteria":     []any{map[string]any{"class": "t31"}},
	}
	if status := register(legacy); status != http.StatusBadRequest {
		t.Fatalf("static kubernetes driver: %d, want 400", status)
	}
	if status := register(map[string]any{
		"key": "vpc-t31", "name": "VPC t31", "resourceType": "vpc", "driverType": "terraform", "driverAccount": "aws-account",
		"driverInputs": map[string]any{"values": map[string]any{"source": vpcSource, "variables": map[string]any{"name": presets["vpc/terraform/name"], "region": "us-east-1"},
			"output_mapping": map[string]any{"id": "id", "cidr": "cidr", "subnetIds": "subnetIds", "privateSubnetIds": "privateSubnetIds"}, "provider_bindings": map[string]any{"aws": map[string]any{"target": "account"}}}},
		"criteria": []any{map[string]any{"class": "t31"}},
	}); status != http.StatusCreated {
		t.Fatalf("defaults-omitted vpc definition: %d", status)
	}
	if status := register(map[string]any{
		"key": "vpc-t31-legacy", "name": "Legacy", "resourceType": "vpc", "driverType": "terraform", "driverAccount": "aws-account",
		"driverInputs": map[string]any{"values": map[string]any{"source": map[string]any{"module": "vpc"}, "variables": map[string]any{"name": "x"}}},
		"criteria":     []any{map[string]any{"class": "t31"}},
	}); status != http.StatusBadRequest {
		t.Fatalf("an unpinned module name must be rejected: %d", status)
	}
	if status := register(map[string]any{
		"key": "vpc-t31-secret", "name": "Secret", "resourceType": "vpc", "driverType": "terraform", "driverAccount": "aws-account",
		"driverInputs": map[string]any{"values": map[string]any{"source": vpcSource, "variables": map[string]any{"name": "x", "master_password": "p"},
			"output_mapping": map[string]any{"id": "id"}, "provider_bindings": map[string]any{"aws": map[string]any{"target": "account"}}}},
		"criteria": []any{map[string]any{"class": "t31"}},
	}); status != http.StatusBadRequest {
		t.Fatalf("master_password must be rejected: %d", status)
	}
}
