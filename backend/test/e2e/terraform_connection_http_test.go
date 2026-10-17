package e2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"orchestrator/internal/seed"
)

// definitionBody is the strict public authoring document of a seeded
// Definition: the server-owned version and fingerprint are not part of it.
func definitionBody(t *testing.T, seeded any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(seeded)
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]any{}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	delete(body, "version")
	delete(body, "sourceFingerprint")
	return body
}

// TestTerraformAWSRegistrationAccountRulesHTTP covers UC-03 V02-03: AWS
// Terraform Definitions (VPC, EKS, Aurora) register with a READY AWS Driver
// Account and a literal region, never with an Environment; any other account is
// a safe 400 at definition.driverAccount and nothing is inserted.
func TestTerraformAWSRegistrationAccountRulesHTTP(t *testing.T) {
	server, opts := newServer(t, "")
	pe := authenticatedClientAs(t, server.URL, "platform-engineer")
	url := server.URL + "/api/v1/resource-definitions"
	count := func() int {
		status, out := requestJSON(t, pe, http.MethodGet, url, nil)
		if status != http.StatusOK {
			t.Fatalf("list: %d %v", status, out)
		}
		return len(out["resourceDefinitions"].([]any))
	}
	for _, seeded := range seed.ResourceDefinitions(opts) {
		if seeded.Key != "vpc-aws" && seeded.Key != "cluster-aws-eks" && seeded.Key != "postgres-aws-aurora" {
			continue
		}
		body := definitionBody(t, seeded)
		body["key"] = "registered-" + seeded.Key
		before := count()
		for _, account := range []string{"", "missing-account", opts.ConnectionKey} {
			if account == "" {
				delete(body, "driverAccount")
			} else {
				body["driverAccount"] = account
			}
			status, out := requestJSON(t, pe, http.MethodPost, url, body)
			if status != http.StatusBadRequest || out["code"] != "INVALID_REQUEST" || out["field"] != "definition.driverAccount" || (account != "" && strings.Contains(toJSON(t, out), account)) {
				t.Fatalf("%s with account %q: %d %v", seeded.Key, account, status, out)
			}
		}
		if got := count(); got != before {
			t.Fatalf("%s: rejected registration inserted a definition (%d -> %d)", seeded.Key, before, got)
		}
		body["driverAccount"] = opts.CloudConnectionKey
		status, out := requestJSON(t, pe, http.MethodPost, url, body)
		if status != http.StatusCreated || out["version"] != float64(1) || out["driverAccount"] != opts.CloudConnectionKey || out["name"] == "" {
			t.Fatalf("%s with its account: %d %v", seeded.Key, status, out)
		}
		if got := count(); got != before+1 {
			t.Fatalf("%s: definitions %d -> %d", seeded.Key, before, got)
		}
	}
}

// Retired and server-owned authoring fields are rejected by the strict DTO.
func TestDefinitionAuthoringRejectsRetiredAndServerOwnedFieldsHTTP(t *testing.T) {
	server, opts := newServer(t, "")
	pe := authenticatedClientAs(t, server.URL, "platform-engineer")
	url := server.URL + "/api/v1/resource-definitions"
	var base map[string]any
	for _, seeded := range seed.ResourceDefinitions(opts) {
		if seeded.Key == "vpc-aws" {
			base = definitionBody(t, seeded)
		}
	}
	for field, value := range map[string]any{"executionProfile": "aws-eks", "connectionKey": "aws-account", "version": 3, "sourceFingerprint": "sha256:x", "regions": []string{"us-east-1"}} {
		body := map[string]any{}
		for k, v := range base {
			body[k] = v
		}
		body["key"] = "strict-" + strings.ToLower(field)
		body[field] = value
		status, out := requestJSON(t, pe, http.MethodPost, url, body)
		if status != http.StatusBadRequest || strings.Contains(toJSON(t, out), field) {
			t.Fatalf("retired field %s: %d %v", field, status, out)
		}
	}
	// The same bodies are rejected by the edit route.
	bad := map[string]any{}
	for k, v := range base {
		bad[k] = v
	}
	bad["version"], bad["executionProfile"] = 1, "aws-eks"
	if status, _ := requestJSON(t, pe, http.MethodPut, url+"/vpc-aws", bad); status != http.StatusBadRequest {
		t.Fatalf("PUT with a retired field: %d", status)
	}
}

// UC-03 V02-06 over HTTP: PUT replaces the whole editable configuration with a
// version CAS, keeps ID and Resource Type, and a stale or foreign request never
// overwrites the winner.
func TestDefinitionEditIsAVersionCompareAndSetHTTP(t *testing.T) {
	server, opts := newServer(t, "")
	pe := authenticatedClientAs(t, server.URL, "platform-engineer")
	dev := authenticatedClient(t, server.URL)
	url := server.URL + "/api/v1/resource-definitions"
	var base map[string]any
	for _, seeded := range seed.ResourceDefinitions(opts) {
		if seeded.Key == "vpc-aws" {
			base = definitionBody(t, seeded)
		}
	}
	base["key"], base["name"] = "vpc-edit", "VPC để sửa"
	base["criteria"] = []any{map[string]any{"app_id": "edit-app"}}
	if status, out := requestJSON(t, pe, http.MethodPost, url, base); status != http.StatusCreated {
		t.Fatalf("create: %d %v", status, out)
	}
	edit := func(mutate func(map[string]any), version any) (int, map[string]any) {
		body := map[string]any{}
		for k, v := range base {
			body[k] = v
		}
		mutate(body)
		body["version"] = version
		return requestJSON(t, pe, http.MethodPut, url+"/vpc-edit", body)
	}
	status, out := edit(func(b map[string]any) {
		b["name"] = "VPC đã sửa"
		b["criteria"] = []any{map[string]any{"app_id": "edit-app", "class": "fast"}, map[string]any{}}
	}, 1)
	if status != http.StatusOK || out["version"] != float64(2) || out["name"] != "VPC đã sửa" || len(out["criteria"].([]any)) != 2 {
		t.Fatalf("edit: %d %v", status, out)
	}
	if status, out := edit(func(b map[string]any) { b["name"] = "Bản thua" }, 1); status != http.StatusConflict || out["code"] != "STALE_VERSION" {
		t.Fatalf("stale edit: %d %v", status, out)
	}
	if status, out := edit(func(b map[string]any) { b["resourceType"] = "k8s-cluster" }, 2); status != http.StatusBadRequest {
		t.Fatalf("resource type change: %d %v", status, out)
	}
	if status, out := edit(func(b map[string]any) { b["driverAccount"] = "missing-account" }, 2); status != http.StatusBadRequest || out["field"] != "definition.driverAccount" {
		t.Fatalf("unknown account: %d %v", status, out)
	}
	if status, out := edit(func(b map[string]any) { b["name"] = "  " }, 2); status != http.StatusBadRequest || out["field"] != "definition.name" {
		t.Fatalf("blank name: %d %v", status, out)
	}
	if status, _ := requestJSON(t, pe, http.MethodPut, url+"/vpc-missing", func() map[string]any {
		b := map[string]any{}
		for k, v := range base {
			b[k] = v
		}
		b["key"], b["version"] = "vpc-missing", 1
		return b
	}()); status != http.StatusNotFound {
		t.Fatalf("missing definition: %d", status)
	}
	if status, _ := requestJSON(t, dev, http.MethodPut, url+"/vpc-edit", func() map[string]any {
		b := map[string]any{}
		for k, v := range base {
			b[k] = v
		}
		b["version"] = 2
		return b
	}()); status != http.StatusForbidden {
		t.Fatalf("developer edit: %d", status)
	}
	listStatus, listed := requestJSON(t, pe, http.MethodGet, url, nil)
	if listStatus != http.StatusOK {
		t.Fatalf("list: %d", listStatus)
	}
	found := false
	for _, item := range listed["resourceDefinitions"].([]any) {
		def := item.(map[string]any)
		if def["key"] == "vpc-edit" {
			found = true
			if def["version"] != float64(2) || def["name"] != "VPC đã sửa" || def["driverAccount"] != opts.CloudConnectionKey {
				t.Fatalf("stored = %v", def)
			}
			if strings.Contains(toJSON(t, def), "secret://") {
				t.Fatalf("list leaks a credential reference: %v", def)
			}
		}
	}
	if !found {
		t.Fatal("edited definition missing from the list")
	}
}
