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

func TestUC01CreateApplicationConnectionSelection(t *testing.T) {
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
	create := func(body string) (int, map[string]any) {
		t.Helper()
		resp, out := call(t, client, http.MethodPost, server.URL+"/api/v1/applications", body)
		return resp.StatusCode, decode(t, out)
	}

	// Omission keeps the Organization default.
	status, out := create(`{"name":"Legacy","subdomain":"legacy"}`)
	view := out["application"].(map[string]any)
	if status != http.StatusCreated || view["connectionKey"] != "internal-cluster" || view["executionProfile"] != "internal-k8s" {
		t.Fatalf("default create = %d %v", status, out)
	}

	// Explicit non-default internal target.
	status, out = create(`{"name":"Lab App","subdomain":"lab-app","connectionKey":"lab"}`)
	view = out["application"].(map[string]any)
	labKey := view["key"].(string)
	if status != http.StatusCreated || view["connectionKey"] != "lab" || view["executionProfile"] != "internal-k8s" || len(view["environments"].([]any)) != 2 {
		t.Fatalf("lab create = %d %v", status, out)
	}
	stored, err := app.Store.GetApplication(ctx, labKey)
	if err != nil || stored.ConnectionKey != "lab" || stored.Profile != appdomain.ProfileInternalK8s {
		t.Fatalf("persisted binding = %+v %v", stored, err)
	}

	// AWS derives profile, region and PENDING status from the Connection.
	status, out = create(`{"name":"Cloud App","subdomain":"cloud-app","connectionKey":"aws-account"}`)
	view = out["application"].(map[string]any)
	if status != http.StatusCreated || view["executionProfile"] != "aws-eks" || view["region"] != "us-east-1" || view["runtimeStatus"] != "PENDING" || view["connectionKey"] != "aws-account" {
		t.Fatalf("aws create = %d %v", status, out)
	}

	// Strict body: explicit blank/null/non-string is a field 400, never a fallback.
	for _, body := range []string{
		`{"name":"N1","subdomain":"n1","connectionKey":""}`,
		`{"name":"N2","subdomain":"n2","connectionKey":"   "}`,
		`{"name":"N3","subdomain":"n3","connectionKey":null}`,
		`{"name":"N3b","subdomain":"n3b","connectionKey" :   null  }`,
		`{"name":"N4","subdomain":"n4","connectionKey":7}`,
		`{"name":"N5","subdomain":"n5","connectionKey":["lab"]}`,
	} {
		status, out := create(body)
		if status != http.StatusBadRequest || out["field"] != "connectionKey" {
			t.Fatalf("%s = %d %v", body, status, out)
		}
	}
	// Unavailable choices share one safe 422; no fallback to the default.
	messages := map[string]bool{}
	for _, key := range []string{"missing", "foreign", "pending", "odd", "noregion"} {
		status, out := create(`{"name":"U-` + key + `","subdomain":"u-` + key + `","connectionKey":"` + key + `"}`)
		if status != http.StatusUnprocessableEntity || out["field"] != "connectionKey" {
			t.Fatalf("%s = %d %v", key, status, out)
		}
		messages[out["error"].(string)] = true
		if strings.Contains(out["error"].(string), key) {
			t.Fatalf("error echoes %q: %v", key, out)
		}
	}
	if len(messages) != 1 {
		t.Fatalf("422 messages differ by cause: %v", messages)
	}
	_, body := call(t, client, http.MethodGet, server.URL+"/api/v1/applications", "")
	if n := len(decode(t, body)["applications"].([]any)); n != 5 { // two seeded fixtures plus the three created above
		t.Fatalf("rejected requests created Applications: %d (%s)", n, body)
	}
	// Other trusted fields stay rejected.
	for _, body := range []string{`{"name":"E","subdomain":"e","executionProfile":"aws-eks"}`, `{"name":"E","subdomain":"e","region":"x"}`} {
		if status, _ := create(body); status != http.StatusBadRequest {
			t.Fatalf("%s = %d", body, status)
		}
	}

	// GET and list expose the same safe binding.
	_, body = call(t, client, http.MethodGet, server.URL+"/api/v1/applications/"+labKey, "")
	if decode(t, body)["application"].(map[string]any)["connectionKey"] != "lab" || strings.Contains(body, "memory://") {
		t.Fatalf("get = %s", body)
	}
	_, body = call(t, client, http.MethodGet, server.URL+"/api/v1/applications", "")
	if !strings.Contains(body, `"connectionKey":"lab"`) {
		t.Fatalf("list = %s", body)
	}

	// A later default change does not move any stored binding, and bindings
	// survive a backend restart.
	if err := app.Store.SaveOrganization(ctx, appdomain.Organization{ID: "org-acme", Key: "acme", Name: "Acme", DefaultConnectionKey: "lab"}); err != nil {
		t.Fatal(err)
	}
	server.Close()
	_, restarted := buildOnboardingApp(t, "test", statePath)
	_, body = call(t, authenticatedClient(t, restarted.URL), http.MethodGet, restarted.URL+"/api/v1/applications/"+labKey, "")
	if decode(t, body)["application"].(map[string]any)["connectionKey"] != "lab" {
		t.Fatalf("binding lost across restart: %s", body)
	}
	_, body = call(t, authenticatedClient(t, restarted.URL), http.MethodGet, restarted.URL+"/api/v1/applications", "")
	bindings := map[string]string{}
	for _, raw := range decode(t, body)["applications"].([]any) {
		view := raw.(map[string]any)
		bindings[view["name"].(string)] = view["connectionKey"].(string)
	}
	if bindings["Legacy"] != "internal-cluster" || bindings["Lab App"] != "lab" || bindings["Cloud App"] != "aws-account" {
		t.Fatalf("default change moved stored bindings: %v", bindings)
	}
	_, body = call(t, authenticatedClient(t, restarted.URL), http.MethodGet, restarted.URL+"/api/v1/application-connections", "")
	if decode(t, body)["defaultConnectionKey"] != "lab" {
		t.Fatalf("new default must apply to later creates only: %s", body)
	}
}
