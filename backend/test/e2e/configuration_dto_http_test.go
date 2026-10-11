package e2e

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"orchestrator/internal/adapters/configmemory"
	"orchestrator/internal/bootstrap"
	configport "orchestrator/internal/ports/configuration"
)

// lazySpyRegistry counts store resolutions. The inner registry is attached
// after bootstrap because it needs the application's persistence store.
type lazySpyRegistry struct {
	configport.Registry
	mu    sync.Mutex
	calls int
}

func (r *lazySpyRegistry) Provider(ctx context.Context, org, key string) (configport.Provider, error) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	return r.Registry.Provider(ctx, org, key)
}

func (r *lazySpyRegistry) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

var forbiddenConfigPayload = []string{"valueRef", "kv2://", "legacy", "Legacy", "credentialRef", "very-secret-bytes"}

func assertPublicConfig(t *testing.T, label string, payload any) {
	t.Helper()
	text := toJSON(t, payload)
	for _, forbidden := range forbiddenConfigPayload {
		if strings.Contains(text, forbidden) {
			t.Fatalf("%s leaks %q: %s", label, forbidden, text)
		}
	}
}

func TestConfigurationDTOIsRedactedAndVariableCRUDNeverResolvesRegistry(t *testing.T) {
	spy := &lazySpyRegistry{}
	server, _, app := newServerAppWith(t, "", func(o *bootstrap.Options) { o.StoreRegistryOverride = spy })
	spy.Registry = configmemory.NewRegistry(app.Store)
	developer := authenticatedClient(t, server.URL)
	storeKey := registerSecretStore(t, server.URL, "DTO Vault")
	otherKey := registerSecretStore(t, server.URL, "DTO Other")
	created := createConfiguredApplication(t, developer, server.URL, "DTO Test", "dto-test", "internal-cluster")
	appKey := created["application"].(map[string]any)["key"].(string)
	selectSecretStore(t, developer, server.URL, appKey, "staging", storeKey)
	base := server.URL + "/api/v1/applications/" + appKey + "/environments/staging"
	keys := base + "/configuration/keys/"

	status, view := requestJSON(t, developer, http.MethodPut, keys+"API_TOKEN", map[string]any{"kind": "SECRET", "value": "very-secret-bytes", "version": 0})
	if status != http.StatusOK {
		t.Fatalf("secret put: %d %v", status, view)
	}
	assertPublicConfig(t, "secret put", view)

	// Variable operations with a Secret sibling never reach the registry.
	before := spy.count()
	version := view["version"]
	status, view = requestJSON(t, developer, http.MethodPut, keys+"MODE", map[string]any{"kind": "VARIABLE", "value": "blue", "version": version})
	if status != http.StatusOK {
		t.Fatalf("variable put: %d %v", status, view)
	}
	status, view = requestJSON(t, developer, http.MethodPut, keys+"MODE", map[string]any{"kind": "VARIABLE", "value": "green", "version": view["version"]})
	if status != http.StatusOK {
		t.Fatalf("variable update: %d %v", status, view)
	}
	status, view = requestJSON(t, developer, http.MethodPatch, keys+"MODE", map[string]any{"newName": "MODE2", "version": view["version"]})
	if status != http.StatusOK {
		t.Fatalf("variable rename: %d %v", status, view)
	}
	if status, got := requestJSON(t, developer, http.MethodGet, base+"/configuration", nil); status != http.StatusOK {
		t.Fatalf("get: %d %v", status, got)
	} else {
		assertPublicConfig(t, "get", got)
	}
	status, view = requestJSON(t, developer, http.MethodDelete, keys+"MODE2", map[string]any{"version": view["version"]})
	if status != http.StatusOK {
		t.Fatalf("variable delete: %d %v", status, view)
	}
	assertPublicConfig(t, "delete", view)
	if spy.count() != before {
		t.Fatalf("variable CRUD resolved a store %d time(s)", spy.count()-before)
	}
	for _, raw := range view["keys"].([]any) {
		if key := raw.(map[string]any); key["name"] == "API_TOKEN" && (key["value"] != nil || key["kind"] != "SECRET") {
			t.Fatalf("secret key view: %v", key)
		}
	}

	// A switch copies only the Secret; the old revision is re-read from the store.
	oldRevisionID := view["revisionId"].(string)
	oldRevision, err := app.Store.GetConfigurationRevision(context.Background(), oldRevisionID)
	if err != nil {
		t.Fatal(err)
	}
	_, envs := requestJSON(t, developer, http.MethodGet, server.URL+"/api/v1/applications/"+appKey, nil)
	var envVersion any
	for _, raw := range envs["application"].(map[string]any)["environments"].([]any) {
		if v := raw.(map[string]any); v["key"] == "staging" {
			envVersion = v["version"]
		}
	}
	status, out := requestJSON(t, developer, http.MethodPut, base+"/secret-store", map[string]any{"secretStoreKey": otherKey, "expectedVersion": envVersion, "expectedConfigVersion": view["version"]})
	if status != http.StatusOK || out["changed"] != true {
		t.Fatalf("switch: %d %v", status, out)
	}
	assertPublicConfig(t, "switch", out)
	again, err := app.Store.GetConfigurationRevision(context.Background(), oldRevisionID)
	if err != nil {
		t.Fatal(err)
	}
	for name, entry := range oldRevision.Entries {
		if again.Entries[name] != entry {
			t.Fatalf("historical revision entry %s changed after switch: %+v -> %+v", name, entry, again.Entries[name])
		}
	}
	if again.Entries["API_TOKEN"].StoreKey != storeKey {
		t.Fatalf("historical secret lost its owning store: %+v", again.Entries["API_TOKEN"])
	}
}

func TestOldPersistedVariableRefIsRejectedOverHTTPWithoutConversionOrStoreCalls(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state.json")
	withState := func(spy *lazySpyRegistry) func(*bootstrap.Options) {
		return func(o *bootstrap.Options) {
			o.StatePath = state
			if spy != nil {
				o.StoreRegistryOverride = spy
			}
		}
	}
	server, _, _ := newServerAppWith(t, "", withState(nil))
	developer := authenticatedClient(t, server.URL)
	created := createConfiguredApplication(t, developer, server.URL, "Old Ref", "old-ref", "internal-cluster")
	appKey := created["application"].(map[string]any)["key"].(string)
	base := server.URL + "/api/v1/applications/" + appKey + "/environments/staging"
	status, view := requestJSON(t, developer, http.MethodPut, base+"/configuration/keys/OLD", map[string]any{"kind": "VARIABLE", "value": "old-marker-value", "version": 0})
	if status != http.StatusOK {
		t.Fatalf("seed variable: %d %v", status, view)
	}
	server.Close()

	data, err := os.ReadFile(state)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	var replaced string
	for _, pair := range [][2]string{{`"value": "old-marker-value"`, `"valueRef": "kv2://kv/old/ref", "storeKey": "legacy-store"`}, {`"value":"old-marker-value"`, `"valueRef":"kv2://kv/old/ref","storeKey":"legacy-store"`}} {
		if strings.Contains(text, pair[0]) {
			replaced = strings.Replace(text, pair[0], pair[1], 1)
		}
	}
	if replaced == "" {
		t.Fatal("snapshot layout changed; cannot seed old-format variable")
	}
	if err := os.WriteFile(state, []byte(replaced), 0o600); err != nil {
		t.Fatal(err)
	}

	spy := &lazySpyRegistry{}
	server2, _, app2 := newServerAppWith(t, "", withState(spy))
	spy.Registry = configmemory.NewRegistry(app2.Store)
	developer = authenticatedClient(t, server2.URL)
	base = server2.URL + "/api/v1/applications/" + appKey + "/environments/staging"
	keys := base + "/configuration/keys/"
	assertSafe := func(label string, status int, out map[string]any) {
		t.Helper()
		if status != http.StatusInternalServerError || out["code"] != "INTERNAL_ERROR" {
			t.Fatalf("%s = %d %v", label, status, out)
		}
		text := toJSON(t, out)
		for _, forbidden := range []string{"kv2://", "legacy-store", "old-marker-value", "valueRef", "variable must carry"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s leaks %q: %s", label, forbidden, text)
			}
		}
	}
	s, o := requestJSON(t, developer, http.MethodGet, base+"/configuration", nil)
	assertSafe("get", s, o)
	s, o = requestJSON(t, developer, http.MethodPut, keys+"NEW", map[string]any{"kind": "VARIABLE", "value": "x", "version": 1})
	assertSafe("put", s, o)
	s, o = requestJSON(t, developer, http.MethodPatch, keys+"OLD", map[string]any{"newName": "OLD2", "version": 1})
	assertSafe("rename", s, o)
	s, o = requestJSON(t, developer, http.MethodDelete, keys+"OLD", map[string]any{"version": 1})
	assertSafe("delete", s, o)
	if spy.count() != 0 {
		t.Fatalf("old variable ref caused %d store resolution(s)", spy.count())
	}
	after, _ := os.ReadFile(state)
	if strings.Contains(string(after), "old-marker-value") || !strings.Contains(string(after), "kv2://kv/old/ref") {
		t.Fatal("rejected operations converted or rewrote the old revision")
	}
}
