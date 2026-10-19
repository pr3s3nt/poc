package e2e

import (
	"context"
	"encoding/json"
	"net/http"
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
	saveConnection(t, app.Store, appdomain.Connection{Name: "pending", AuthenticationType: appdomain.AuthKubeconfig, ID: "c-pending", Key: "pending", OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionVerifying})
	saveConnection(t, app.Store, appdomain.Connection{Name: "rejected", AuthenticationType: appdomain.AuthKubeconfig, ID: "c-rejected", Key: "rejected", OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionRejected})
	saveConnection(t, app.Store, appdomain.Connection{Name: "noregion", AuthenticationType: appdomain.AuthAWSAccessKey, ID: "c-noregion", Key: "noregion", OrganizationKey: "acme", Kind: appdomain.ConnectionAWS, Status: appdomain.ConnectionReady})
	if err := app.Store.SaveOrganization(ctx, appdomain.Organization{ID: "org-globex", Key: "globex", Name: "Globex", DefaultConnectionKey: "globex-cluster"}); err != nil {
		t.Fatal(err)
	}
	saveConnection(t, app.Store, appdomain.Connection{Name: "globex-cluster", AuthenticationType: appdomain.AuthKubeconfig, ID: "c-globex", Key: "globex-cluster", OrganizationKey: "globex", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionReady})
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
	saveConnection(t, app.Store, appdomain.Connection{Name: "globex-cluster", AuthenticationType: appdomain.AuthKubeconfig, ID: "c-globex", Key: "globex-cluster", OrganizationKey: "globex", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionVerifying})
	saveConnection(t, app.Store, appdomain.Connection{Name: "globex-two", AuthenticationType: appdomain.AuthKubeconfig, ID: "c-globex2", Key: "globex-two", OrganizationKey: "globex", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionReady})
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
