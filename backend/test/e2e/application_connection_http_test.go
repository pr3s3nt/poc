package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	appdomain "orchestrator/internal/domain/application"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/platform/password"
)

// UC-01 connection selection over HTTP: safe choice list, strict create body
// and persisted binding. Approved contract: UC-01 BR-07/BR-08, OC-01.

func saveConnection(t *testing.T, store interface {
	SaveConnection(context.Context, appdomain.Connection) error
}, conn appdomain.Connection) {
	t.Helper()
	if err := store.SaveConnection(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
}

func TestUC01ApplicationConnectionChoicesAreSafeAndOrganizationScoped(t *testing.T) {
	app, server := buildOnboardingApp(t, "test", "")
	ctx := context.Background()
	saveConnection(t, app.Store, appdomain.Connection{ID: "c-lab", Key: "lab", Name: "Lab", OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, AuthenticationType: appdomain.AuthKubeconfig, Status: appdomain.ConnectionReady, SecretRef: "memory://secret-ref-value", Config: map[string]any{"kubeContext": "lab", "token": "do-not-leak"}, Verification: map[string]any{"serverVersion": "v1"}})
	saveConnection(t, app.Store, appdomain.Connection{ID: "c-pending", Key: "pending", OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionVerifying})
	saveConnection(t, app.Store, appdomain.Connection{ID: "c-rejected", Key: "rejected", OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionRejected})
	saveConnection(t, app.Store, appdomain.Connection{ID: "c-odd", Key: "odd", OrganizationKey: "acme", Kind: appdomain.ConnectionKind("GCP"), Status: appdomain.ConnectionReady})
	saveConnection(t, app.Store, appdomain.Connection{ID: "c-noregion", Key: "noregion", OrganizationKey: "acme", Kind: appdomain.ConnectionAWS, Status: appdomain.ConnectionReady})
	if err := app.Store.SaveOrganization(ctx, appdomain.Organization{ID: "org-globex", Key: "globex", Name: "Globex", DefaultConnectionKey: "globex-cluster"}); err != nil {
		t.Fatal(err)
	}
	saveConnection(t, app.Store, appdomain.Connection{ID: "c-globex", Key: "globex-cluster", OrganizationKey: "globex", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionReady})
	hash, err := password.Hash("test-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Store.SaveUserAccount(ctx, identity.UserAccount{ID: "user-globex", OrganizationKey: "globex", Username: "globex-developer", PasswordHash: hash, Role: identity.RoleDeveloper, Status: identity.AccountActive}); err != nil {
		t.Fatal(err)
	}

	if resp, _ := call(t, http.DefaultClient, http.MethodGet, server.URL+"/api/v1/application-connections", ""); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous list = %d", resp.StatusCode)
	}
	for _, user := range []string{"developer", "platform-engineer"} {
		resp, body := call(t, authenticatedClientAs(t, server.URL, user), http.MethodGet, server.URL+"/api/v1/application-connections", "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s list = %d %s", user, resp.StatusCode, body)
		}
		var out struct {
			Connections []map[string]any `json:"connections"`
			Default     string           `json:"defaultConnectionKey"`
		}
		if err := json.Unmarshal([]byte(body), &out); err != nil {
			t.Fatal(err)
		}
		keys := []string{}
		for _, c := range out.Connections {
			keys = append(keys, c["key"].(string))
			if len(c) != 4 || c["name"] == "" || c["status"] != "READY" {
				t.Fatalf("choice is not the minimal READY shape: %v", c)
			}
		}
		if strings.Join(keys, ",") != "aws-account,internal-cluster,lab" || out.Default != "internal-cluster" {
			t.Fatalf("%s choices = %v default=%q", user, keys, out.Default)
		}
		for _, leak := range []string{"secret-ref-value", "do-not-leak", "memory://", "kubeContext", "authenticationType", "verification", "serverVersion", "secretRef"} {
			if strings.Contains(body, leak) {
				t.Fatalf("list leaks %q: %s", leak, body)
			}
		}
	}
	// Only the session Organization is listed.
	_, body := call(t, authenticatedClientAs(t, server.URL, "globex-developer"), http.MethodGet, server.URL+"/api/v1/application-connections", "")
	if strings.Contains(body, "internal-cluster") || strings.Contains(body, "lab") || !strings.Contains(body, `"globex-cluster"`) {
		t.Fatalf("globex choices = %s", body)
	}
	// UC-04 management list stays platform-only.
	if resp, _ := call(t, authenticatedClient(t, server.URL), http.MethodGet, server.URL+"/api/v1/connections", ""); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("developer management list = %d", resp.StatusCode)
	}
	if resp, _ := call(t, authenticatedClientAs(t, server.URL, "platform-engineer"), http.MethodGet, server.URL+"/api/v1/connections", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("platform management list = %d", resp.StatusCode)
	}
}

func TestUC01ApplicationConnectionChoiceEmptyWhenDefaultNotReady(t *testing.T) {
	app, server := buildOnboardingApp(t, "test", "")
	ctx := context.Background()
	if err := app.Store.SaveOrganization(ctx, appdomain.Organization{ID: "org-globex", Key: "globex", Name: "Globex", DefaultConnectionKey: "globex-cluster"}); err != nil {
		t.Fatal(err)
	}
	saveConnection(t, app.Store, appdomain.Connection{ID: "c-globex", Key: "globex-cluster", OrganizationKey: "globex", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionVerifying})
	saveConnection(t, app.Store, appdomain.Connection{ID: "c-globex2", Key: "globex-two", OrganizationKey: "globex", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionReady})
	hash, _ := password.Hash("test-password")
	if err := app.Store.SaveUserAccount(ctx, identity.UserAccount{ID: "user-globex", OrganizationKey: "globex", Username: "globex-developer", PasswordHash: hash, Role: identity.RoleDeveloper, Status: identity.AccountActive}); err != nil {
		t.Fatal(err)
	}
	_, body := call(t, authenticatedClientAs(t, server.URL, "globex-developer"), http.MethodGet, server.URL+"/api/v1/application-connections", "")
	out := decode(t, body)
	if out["defaultConnectionKey"] != "" || len(out["connections"].([]any)) != 1 {
		t.Fatalf("default must be omitted when not eligible: %s", body)
	}
}

func TestUC01EnvironmentConnectionEditableOverHTTP(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	app, server := buildOnboardingApp(t, "test", statePath)
	ctx := context.Background()
	saveConnection(t, app.Store, appdomain.Connection{ID: "c-lab", Key: "lab", Name: "Lab", OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, AuthenticationType: appdomain.AuthKubeconfig, Status: appdomain.ConnectionReady, SecretRef: "memory://x"})
	saveConnection(t, app.Store, appdomain.Connection{ID: "c-pending", Key: "pending", OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionVerifying})
	saveConnection(t, app.Store, appdomain.Connection{ID: "c-odd", Key: "odd", OrganizationKey: "acme", Kind: appdomain.ConnectionKind("GCP"), Status: appdomain.ConnectionReady})
	saveConnection(t, app.Store, appdomain.Connection{ID: "c-noregion", Key: "noregion", OrganizationKey: "acme", Kind: appdomain.ConnectionAWS, Status: appdomain.ConnectionReady})
	if err := app.Store.SaveOrganization(ctx, appdomain.Organization{ID: "org-globex", Key: "globex", Name: "Globex", DefaultConnectionKey: "foreign"}); err != nil {
		t.Fatal(err)
	}
	saveConnection(t, app.Store, appdomain.Connection{ID: "c-foreign", Key: "foreign", OrganizationKey: "globex", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionReady})
	client := authenticatedClient(t, server.URL)
	request := func(method, path, body string) (int, map[string]any) {
		t.Helper()
		resp, out := call(t, client, method, server.URL+path, body)
		return resp.StatusCode, decode(t, out)
	}

	// Create takes only name and subdomain and starts both Environments unset,
	// without consulting the Organization default.
	status, out := request(http.MethodPost, "/api/v1/applications", `{"name":"Payments","subdomain":"payments"}`)
	view := out["application"].(map[string]any)
	appKey := view["key"].(string)
	if status != http.StatusCreated || len(view) != 4 {
		t.Fatalf("create = %d %v", status, out)
	}
	envs := map[string]map[string]any{}
	for _, raw := range view["environments"].([]any) {
		env := raw.(map[string]any)
		envs[env["key"].(string)] = env
		if env["configured"] != false || env["connectionKey"] != "" || env["runtimeStatus"] != "UNCONFIGURED" || env["executionProfile"] != "" || env["version"] != float64(1) {
			t.Fatalf("environment must be UNCONFIGURED: %v", env)
		}
	}
	for _, body := range []string{
		`{"name":"E","subdomain":"e","connectionKey":"lab"}`, `{"name":"E","subdomain":"e","connectionKey":null}`,
		`{"name":"E","subdomain":"e","executionProfile":"aws-eks"}`, `{"name":"E","subdomain":"e","region":"x"}`,
	} {
		if status, _ := request(http.MethodPost, "/api/v1/applications", body); status != http.StatusBadRequest {
			t.Fatalf("%s = %d", body, status)
		}
	}
	// Unset Preview and Deploy are a safe 422 before any side effect.
	for _, path := range []string{"/preview", "/deploy"} {
		body := `{}`
		if path == "/deploy" {
			body = `{"token":"x"}`
		}
		status, out := request(http.MethodPost, "/api/v1/applications/"+appKey+"/environments/staging"+path, body)
		if status != http.StatusUnprocessableEntity || out["field"] != "connectionKey" || out["code"] != "ENVIRONMENT_UNCONFIGURED" {
			t.Fatalf("unconfigured %s = %d %v", path, status, out)
		}
	}
	// Every delivery boundary answers the same safe 422: the direct deployment
	// API and the standalone Score preview too.
	score := `{"apiVersion":"score.dev/v1b1","metadata":{"name":"api"},"containers":{"main":{"image":"example.invalid/api:v1"}}}`
	for name, call := range map[string]struct{ path, body string }{
		"direct deployment": {"/api/v1/deployments", `{"applicationKey":"` + appKey + `","environmentKey":"staging","workloadId":"api","score":` + score + `}`},
		"score preview":     {"/api/v1/applications/" + appKey + "/environments/staging/score-preview", `{"workloadId":"api","action":"deploy","runId":"r1","scoreAfter":` + score + `}`},
	} {
		status, out := request(http.MethodPost, call.path, call.body)
		if status != http.StatusUnprocessableEntity || out["code"] != "ENVIRONMENT_UNCONFIGURED" || out["field"] != "connectionKey" {
			t.Fatalf("unconfigured %s = %d %v", name, status, out)
		}
	}
	if applied := len(app.FakeDeploy.Applied); applied != 0 {
		t.Fatalf("an unconfigured request applied %d manifests", applied)
	}
	set := func(env, key string, version any) (int, map[string]any) {
		body, _ := json.Marshal(map[string]any{"connectionKey": key, "expectedVersion": version})
		return request(http.MethodPut, "/api/v1/applications/"+appKey+"/environments/"+env+"/connection", string(body))
	}

	// Staging selects lab; production stays unset and picks AWS independently.
	status, out = set("staging", "lab", 1)
	staging := out["environment"].(map[string]any)
	if status != http.StatusOK || staging["connectionKey"] != "lab" || staging["connectionName"] != "Lab" || staging["connectionKind"] != "KUBERNETES" || staging["executionProfile"] != "internal-k8s" || staging["runtimeStatus"] != "READY" || staging["infrastructureScope"] != "ENVIRONMENT" || staging["version"] != float64(2) {
		t.Fatalf("set staging = %d %v", status, out)
	}
	status, out = request(http.MethodGet, "/api/v1/applications/"+appKey, "")
	for _, raw := range out["application"].(map[string]any)["environments"].([]any) {
		env := raw.(map[string]any)
		if env["key"] == "production" && (env["configured"] != false || env["version"] != float64(1)) {
			t.Fatalf("production changed by staging: %v", env)
		}
	}
	status, out = set("production", "aws-account", 1)
	production := out["environment"].(map[string]any)
	if status != http.StatusOK || production["executionProfile"] != "aws-eks" || production["region"] != "us-east-1" || production["runtimeStatus"] != "PENDING" || production["infrastructureScope"] != "ENVIRONMENT" {
		t.Fatalf("set production = %d %v", status, out)
	}
	// Application stays unbound.
	stored, err := app.Store.GetApplication(ctx, appKey)
	if err != nil || stored.ConnectionKey != "" || stored.Profile != "" || stored.Region != "" {
		t.Fatalf("application leaked target: %+v %v", stored, err)
	}

	// Same key at the current version is a no-op; a stale version is 409
	// STALE_VERSION; another key at the current version replaces the target
	// (no permanent lock, ADR-012).
	status, out = set("staging", "lab", 2)
	if status != http.StatusOK || out["environment"].(map[string]any)["version"] != float64(2) {
		t.Fatalf("same key no-op = %d %v", status, out)
	}
	for _, attempt := range []struct {
		key     string
		version any
	}{{"lab", 1}, {"internal-cluster", 1}, {"aws-account", 99}} {
		status, out := set("staging", attempt.key, attempt.version)
		if status != http.StatusConflict || out["code"] != "STALE_VERSION" {
			t.Fatalf("stale %v = %d %v", attempt, status, out)
		}
	}
	status, out = set("staging", "internal-cluster", 2)
	if edited := out["environment"].(map[string]any); status != http.StatusOK || edited["connectionKey"] != "internal-cluster" || edited["version"] != float64(3) {
		t.Fatalf("change connection = %d %v", status, out)
	}
	// Strict body and typed failures on a fresh unset Environment.
	status, out = request(http.MethodPost, "/api/v1/applications", `{"name":"Other","subdomain":"other"}`)
	appKey = out["application"].(map[string]any)["key"].(string)
	for _, body := range []string{
		`{"connectionKey":"","expectedVersion":1}`, `{"connectionKey":"  ","expectedVersion":1}`, `{"connectionKey":null,"expectedVersion":1}`,
		`{"connectionKey":7,"expectedVersion":1}`, `{"expectedVersion":1}`, `{"connectionKey":"lab"}`, `{"connectionKey":"lab","expectedVersion":0}`,
		`{"connectionKey":"lab","expectedVersion":"1"}`, `{"connectionKey":"lab","expectedVersion":1,"executionProfile":"aws-eks"}`,
	} {
		if status, out := request(http.MethodPut, "/api/v1/applications/"+appKey+"/environments/staging/connection", body); status != http.StatusBadRequest {
			t.Fatalf("%s = %d %v", body, status, out)
		}
	}
	messages := map[string]bool{}
	for _, key := range []string{"missing", "foreign", "pending", "odd", "noregion"} {
		status, out := set("staging", key, 1)
		if status != http.StatusUnprocessableEntity || out["field"] != "connectionKey" || strings.Contains(out["error"].(string), key) {
			t.Fatalf("%s = %d %v", key, status, out)
		}
		messages[out["error"].(string)] = true
	}
	if len(messages) != 1 {
		t.Fatalf("422 messages differ by cause: %v", messages)
	}
	status, out = set("staging", "lab", 5)
	if status != http.StatusConflict || out["code"] != "STALE_VERSION" {
		t.Fatalf("stale = %d %v", status, out)
	}
	if status, _ := request(http.MethodPut, "/api/v1/applications/missing/environments/staging/connection", `{"connectionKey":"lab","expectedVersion":1}`); status != http.StatusNotFound {
		t.Fatalf("missing application = %d", status)
	}
	if status, _ := request(http.MethodPut, "/api/v1/applications/"+appKey+"/environments/nope/connection", `{"connectionKey":"lab","expectedVersion":1}`); status != http.StatusNotFound {
		t.Fatalf("missing environment = %d", status)
	}
	if resp, _ := call(t, http.DefaultClient, http.MethodPut, server.URL+"/api/v1/applications/"+appKey+"/environments/staging/connection", `{"connectionKey":"lab","expectedVersion":1}`); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d", resp.StatusCode)
	}
	// Another Organization cannot see or set the Environment.
	hash, _ := password.Hash("test-password")
	if err := app.Store.SaveUserAccount(ctx, identity.UserAccount{ID: "user-globex", OrganizationKey: "globex", Username: "globex-developer", PasswordHash: hash, Role: identity.RoleDeveloper, Status: identity.AccountActive}); err != nil {
		t.Fatal(err)
	}
	foreign := authenticatedClientAs(t, server.URL, "globex-developer")
	if resp, _ := call(t, foreign, http.MethodPut, server.URL+"/api/v1/applications/"+appKey+"/environments/staging/connection", `{"connectionKey":"foreign","expectedVersion":1}`); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("foreign organization = %d", resp.StatusCode)
	}
	if env, _ := app.Store.GetEnvironment(ctx, appKey, "staging"); env.Configured() {
		t.Fatalf("rejected requests mutated: %+v", env)
	}

	// Changing the Organization default later moves nothing, and the bindings
	// survive a backend restart (JSON store).
	if err := app.Store.SaveOrganization(ctx, appdomain.Organization{ID: "org-acme", Key: "acme", Name: "Acme", DefaultConnectionKey: "lab"}); err != nil {
		t.Fatal(err)
	}
	server.Close()
	_, restarted := buildOnboardingApp(t, "test", statePath)
	_, body := call(t, authenticatedClient(t, restarted.URL), http.MethodGet, restarted.URL+"/api/v1/applications", "")
	found := map[string]string{}
	for _, raw := range decode(t, body)["applications"].([]any) {
		view := raw.(map[string]any)
		for _, e := range view["environments"].([]any) {
			env := e.(map[string]any)
			found[view["name"].(string)+"/"+env["key"].(string)] = env["connectionKey"].(string)
		}
	}
	if found["Payments/staging"] != "internal-cluster" || found["Payments/production"] != "aws-account" || found["Other/staging"] != "" || found["Other/production"] != "" {
		t.Fatalf("bindings after restart: %v", found)
	}
	// The seeded acceptance fixtures keep their historical LEGACY_APPLICATION binding.
	if found["Acceptance/dev"] == "" && !strings.Contains(body, `"infrastructureScope":"LEGACY_APPLICATION"`) {
		t.Fatalf("legacy seeded binding lost: %s", body)
	}
}
