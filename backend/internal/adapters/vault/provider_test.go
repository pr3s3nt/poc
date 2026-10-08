package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProviderRoundTripAndScope(t *testing.T) {
	values := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Vault-Token") != "scoped" || !strings.HasPrefix(r.URL.Path, "/v1/kv/data/orchestrator/apps/app-1/envs/staging/values/") {
			t.Errorf("unexpected Vault request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.Method == http.MethodPost {
			var body struct {
				Data map[string]string `json:"data"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			values[r.URL.Path] = body.Data["value"]
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = fmt.Fprintf(w, `{"data":{"data":{"value":%q}}}`, values[r.URL.Path])
	}))
	defer server.Close()
	provider, err := New(server.URL, "scoped", "kv", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	ref, err := provider.WriteValue(context.Background(), "app-1", "staging", "s'ecret $HOME")
	if err != nil {
		t.Fatal(err)
	}
	value, err := provider.ReadValue(context.Background(), ref)
	if err != nil || value != "s'ecret $HOME" {
		t.Fatalf("round trip failed: %v", err)
	}
	if _, err := provider.ReadValue(context.Background(), "kv2://kv/orchestrator/apps/app-1/envs/production/values/../x"); err == nil {
		t.Fatal("invalid value reference was accepted")
	}
}

func TestPrepareWorkloadAccessUsesExactPathsAndServiceAccount(t *testing.T) {
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Vault-Token") != "scoped" {
			t.Error("missing backend token")
		}
		if !strings.HasPrefix(r.URL.Path, "/v1/sys/policies/acl/orch-") && !strings.HasPrefix(r.URL.Path, "/v1/auth/kubernetes/role/orch-") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		encoded, _ := json.Marshal(body)
		bodies = append(bodies, string(encoded))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	provider, err := New(server.URL, "scoped", "kv", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.SetAgentAddress("http://vault-uc12.vault.svc:8200"); err != nil {
		t.Fatal(err)
	}
	ref := "kv2://kv/orchestrator/apps/app-1/envs/staging/values/immutable"
	access, err := provider.PrepareWorkloadAccess(context.Background(), "app-1", "staging", "frontend", "app-staging", "revision-1", []string{ref, ref})
	if err != nil {
		t.Fatal(err)
	}
	if access.Role == "" || access.ServiceAccount == "" || len(bodies) != 2 {
		t.Fatalf("invalid access setup: %+v, %d requests", access, len(bodies))
	}
	if !strings.Contains(bodies[0], "kv/data/orchestrator/apps/app-1/envs/staging/values/immutable") || strings.Count(bodies[0], "capabilities") != 1 {
		t.Fatalf("policy is not exact: %s", bodies[0])
	}
	if !strings.Contains(bodies[1], access.ServiceAccount) || !strings.Contains(bodies[1], "app-staging") {
		t.Fatalf("role is not scoped: %s", bodies[1])
	}
	if _, err := provider.PrepareWorkloadAccess(context.Background(), "app-1", "staging", "frontend", "app-staging", "revision-1", []string{"kv2://kv/orchestrator/apps/app-2/envs/staging/values/immutable"}); err == nil {
		t.Fatal("cross-Application reference accepted")
	}
}

func TestPrepareWorkloadBundleWritesScopedImmutableData(t *testing.T) {
	var bundlePath string
	var bundleData map[string]string
	var policy string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Vault-Token") != "scoped" {
			t.Error("missing scoped token")
		}
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/values/source"):
			_, _ = w.Write([]byte(`{"data":{"data":{"value":"opaque-value"}}}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/values/deployment-1"):
			bundlePath = r.URL.Path
			var body struct {
				Options map[string]int    `json:"options"`
				Data    map[string]string `json:"data"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Options["cas"] != 0 {
				t.Error("bundle must be create-only")
			}
			bundleData = body.Data
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/sys/policies/acl/"):
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			policy = body["policy"]
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/auth/kubernetes/role/"):
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	provider, err := New(server.URL, "scoped", "kv", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.SetAgentAddress("http://vault-uc12.vault.svc:8200"); err != nil {
		t.Fatal(err)
	}
	ref := "kv2://kv/orchestrator/apps/app-1/envs/staging/values/source"
	if !provider.OwnsRef(ref, "app-1", "staging") || provider.OwnsRef("kv2://kv/orchestrator/apps/app-2/envs/staging/values/source", "app-1", "staging") {
		t.Fatal("ref ownership must be scoped to the Application Environment")
	}
	value, err := provider.ReadValue(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := provider.WriteBundle(context.Background(), "app-1", "staging", "frontend", "app-staging", "revision-1", "deployment-1", map[string]map[string]string{"main": {"API_TOKEN": value}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(bundlePath, "/values/deployment-1") || bundleData["main_API_TOKEN"] != "opaque-value" || len(bundleData) != 1 {
		t.Fatal("wrong bundle content or scope")
	}
	if !strings.Contains(policy, "kv/data/orchestrator/apps/app-1/envs/staging/values/deployment-1") || strings.Contains(policy, "/values/source") {
		t.Fatal("policy must read only bundle")
	}
	if bundle.Keys["main"]["API_TOKEN"] != "main_API_TOKEN" || bundle.SecretName == "" || bundle.Role == "" {
		t.Fatal("wrong bundle metadata")
	}
}
