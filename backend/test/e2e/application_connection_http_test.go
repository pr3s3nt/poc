package e2e

import (
	"context"
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

func TestUC01RetiredApplicationChoicesAreAbsentForEveryRole(t *testing.T) {
	app, server := buildOnboardingApp(t, "test", "")
	ctx := context.Background()
	saveConnection(t, app.Store, appdomain.Connection{ID: "c-lab", Key: "lab", Name: "Lab", OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, AuthenticationType: appdomain.AuthKubeconfig, Status: appdomain.ConnectionReady, SecretRef: "memory://secret-ref-value", Config: map[string]any{"kubeContext": "lab", "token": "do-not-leak"}, Verification: map[string]any{"serverVersion": "v1"}})
	saveConnection(t, app.Store, appdomain.Connection{Name: "pending", AuthenticationType: appdomain.AuthKubeconfig, ID: "c-pending", Key: "pending", OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionVerifying})
	saveConnection(t, app.Store, appdomain.Connection{Name: "rejected", AuthenticationType: appdomain.AuthKubeconfig, ID: "c-rejected", Key: "rejected", OrganizationKey: "acme", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionRejected})
	saveConnection(t, app.Store, appdomain.Connection{Name: "noregion", AuthenticationType: appdomain.AuthAWSAccessKey, ID: "c-noregion", Key: "noregion", OrganizationKey: "acme", Kind: appdomain.ConnectionAWS, Status: appdomain.ConnectionReady})
	if err := app.Store.SaveOrganization(ctx, appdomain.Organization{ID: "org-globex", Key: "globex", Name: "Globex"}); err != nil {
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

	for _, client := range []*http.Client{http.DefaultClient, authenticatedClient(t, server.URL), authenticatedClientAs(t, server.URL, "platform-engineer"), authenticatedClientAs(t, server.URL, "globex-developer")} {
		resp, body := call(t, client, http.MethodGet, server.URL+"/api/v1/application-connections", "")
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("retired application choice route = %d", resp.StatusCode)
		}
		for _, leak := range []string{"secret-ref-value", "do-not-leak", "globex-cluster", "internal-cluster", "memory://", "defaultConnectionKey"} {
			if strings.Contains(body, leak) {
				t.Fatalf("retired route leaks %q", leak)
			}
		}
	}
	// Management stays available only to Platform Engineer/Admin; current safe
	// Driver Account DTO/organization isolation is covered by driver_accounts_http_test.
	if resp, _ := call(t, authenticatedClient(t, server.URL), http.MethodGet, server.URL+"/api/v1/driver-accounts", ""); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("developer management = %d", resp.StatusCode)
	}
	if resp, _ := call(t, authenticatedClientAs(t, server.URL, "platform-engineer"), http.MethodGet, server.URL+"/api/v1/driver-accounts", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("platform management = %d", resp.StatusCode)
	}

}

func TestUC01RetiredApplicationChoicesNeverInferADefault(t *testing.T) {
	app, server := buildOnboardingApp(t, "test", "")
	ctx := context.Background()
	if err := app.Store.SaveOrganization(ctx, appdomain.Organization{ID: "org-globex", Key: "globex", Name: "Globex"}); err != nil {
		t.Fatal(err)
	}
	saveConnection(t, app.Store, appdomain.Connection{Name: "globex-cluster", AuthenticationType: appdomain.AuthKubeconfig, ID: "c-globex", Key: "globex-cluster", OrganizationKey: "globex", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionVerifying})
	saveConnection(t, app.Store, appdomain.Connection{Name: "globex-two", AuthenticationType: appdomain.AuthKubeconfig, ID: "c-globex2", Key: "globex-two", OrganizationKey: "globex", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionReady})
	hash, _ := password.Hash("test-password")
	if err := app.Store.SaveUserAccount(ctx, identity.UserAccount{ID: "user-globex", OrganizationKey: "globex", Username: "globex-developer", PasswordHash: hash, Role: identity.RoleDeveloper, Status: identity.AccountActive}); err != nil {
		t.Fatal(err)
	}
	response, body := call(t, authenticatedClientAs(t, server.URL, "globex-developer"), http.MethodGet, server.URL+"/api/v1/application-connections", "")
	if response.StatusCode != http.StatusNotFound || strings.Contains(body, "globex-two") || strings.Contains(body, "defaultConnectionKey") {
		t.Fatalf("retired route inferred default: %d %s", response.StatusCode, body)
	}
}
