package e2e

import (
	"encoding/json"
	"net/http"
	"orchestrator/internal/seed/seedtest"
	"strings"
	"testing"

	"orchestrator/internal/seed"
)

// TestTerraformVPCEKSRegistrationRejectsConnectionHTTP covers UC-03 BR-08 (T16):
// Terraform VPC/EKS register without a connection; any nonempty key is a safe
// 400 and nothing is inserted.
func TestTerraformVPCEKSRegistrationRejectsConnectionHTTP(t *testing.T) {
	server, _ := newServer(t, "")
	pe := authenticatedClientAs(t, server.URL, "platform-engineer")
	url := server.URL + "/api/v1/resource-definitions"
	count := func() int {
		status, out := requestJSON(t, pe, http.MethodGet, url, nil)
		if status != http.StatusOK {
			t.Fatalf("list: %d %v", status, out)
		}
		return len(out["resourceDefinitions"].([]any))
	}
	for _, seeded := range seed.ResourceDefinitions(seedtest.Defaults()) {
		if seeded.Key != "vpc-aws" && seeded.Key != "cluster-aws-eks" {
			continue
		}
		raw, err := json.Marshal(seeded)
		if err != nil {
			t.Fatal(err)
		}
		body := map[string]any{}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		body["key"], body["executionProfile"] = "registered-"+seeded.Key, "aws-eks"
		before := count()
		for _, key := range []string{"aws-account", "missing-account"} {
			body["connectionKey"] = key
			status, out := requestJSON(t, pe, http.MethodPost, url, body)
			if status != http.StatusBadRequest || out["code"] != "INVALID_REQUEST" || out["field"] != "definition.connectionKey" || strings.Contains(toJSON(t, out), key) {
				t.Fatalf("%s with %q: %d %v", seeded.Key, key, status, out)
			}
		}
		if got := count(); got != before {
			t.Fatalf("%s: rejected registration inserted a definition (%d -> %d)", seeded.Key, before, got)
		}
		delete(body, "connectionKey")
		if status, out := requestJSON(t, pe, http.MethodPost, url, body); status != http.StatusCreated {
			t.Fatalf("%s without a key: %d %v", seeded.Key, status, out)
		}
		if got := count(); got != before+1 {
			t.Fatalf("%s: definitions %d -> %d", seeded.Key, before, got)
		}
	}
}
