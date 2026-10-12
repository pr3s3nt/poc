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
	for _, raw := range out["capabilities"].([]any) {
		capability := raw.(map[string]any)
		for _, rawVariable := range capability["variables"].([]any) {
			variable := rawVariable.(map[string]any)
			if preset, ok := variable["preset"].(string); ok {
				presets[capability["resourceType"].(string)+"/"+capability["driverType"].(string)+"/"+variable["name"].(string)] = preset
			}
		}
	}
	for key, want := range map[string]string{
		"k8s-namespace/kubernetes/name": "${context.env.namespace}",
		"vpc/terraform/name":            "${context.infra.resourceName}",
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
	if status := register(map[string]any{
		"key": "ns-t31", "resourceType": "k8s-namespace", "executionProfile": "internal-k8s", "driverType": "kubernetes",
		"driverInputs": map[string]any{"values": map[string]any{"variables": map[string]any{"name": presets["k8s-namespace/kubernetes/name"]}}},
		"criteria":     []any{map[string]any{"class": "t31"}},
	}); status != http.StatusCreated {
		t.Fatalf("preset namespace definition: %d", status)
	}
	if status := register(map[string]any{
		"key": "vpc-t31", "resourceType": "vpc", "executionProfile": "aws-eks", "driverType": "terraform",
		"driverInputs": map[string]any{"values": map[string]any{"source": map[string]any{"module": "vpc"}, "variables": map[string]any{"name": presets["vpc/terraform/name"]}}},
		"criteria":     []any{map[string]any{"class": "t31"}},
	}); status != http.StatusCreated {
		t.Fatalf("defaults-omitted vpc definition: %d", status)
	}
	if status := register(map[string]any{
		"key": "vpc-t31-secret", "resourceType": "vpc", "executionProfile": "aws-eks", "driverType": "terraform",
		"driverInputs": map[string]any{"values": map[string]any{"source": map[string]any{"module": "vpc"}, "variables": map[string]any{"name": "x", "master_password": "p"}}},
		"criteria":     []any{map[string]any{"class": "t31"}},
	}); status != http.StatusBadRequest {
		t.Fatalf("master_password must be rejected: %d", status)
	}
}
