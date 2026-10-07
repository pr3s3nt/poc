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

// TestUC07PendingDraftHTTPContract covers the UC-07 mapping: strict bodies
// with a required version, session scope, and 409 on stale version or token.
func TestUC07PendingDraftHTTPContract(t *testing.T) {
	server, _, app := newServerApp(t, "")
	client := authenticatedClient(t, server.URL)
	created := createConfiguredApplication(t, client, server.URL, "Update Remove", "update-remove", "internal-cluster")
	key := created["application"].(map[string]any)["key"].(string)
	base := server.URL + "/api/v1/applications/" + key + "/environments/staging"
	draft := previewScore("api")
	delete(draft["containers"].(map[string]any)["main"].(map[string]any), "variables") // UC-16 forbids literals
	api := toJSON(t, draft)

	for name, tc := range map[string]struct{ method, path, body string }{
		"save without version":   {http.MethodPut, "/workloads/api", `{"score":` + api + `}`},
		"save null version":      {http.MethodPut, "/workloads/api", `{"version":null,"score":` + api + `}`},
		"save negative version":  {http.MethodPut, "/workloads/api", `{"version":-1,"score":` + api + `}`},
		"save unknown field":     {http.MethodPut, "/workloads/api", `{"version":0,"score":` + api + `,"force":true}`},
		"save trailing":          {http.MethodPut, "/workloads/api", `{"version":0,"score":` + api + `} {}`},
		"save string version":    {http.MethodPut, "/workloads/api", `{"version":"0","score":` + api + `}`},
		"delete without version": {http.MethodDelete, "/workloads/api", `{}`},
		"undo without version":   {http.MethodPost, "/workloads/api/undo", `{}`},
		"preview unknown field":  {http.MethodPost, "/preview", `{"token":"x"}`},
		"preview null root":      {http.MethodPost, "/preview", `null`},
		"preview array root":     {http.MethodPost, "/preview", `[]`},
		"delete null root":       {http.MethodDelete, "/workloads/api", `null`},
		"deploy missing token":   {http.MethodPost, "/deploy", `{}`},
		"deploy unknown field":   {http.MethodPost, "/deploy", `{"token":"x","force":true}`},
	} {
		if resp, body := call(t, client, tc.method, base+tc.path, tc.body); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s = %d %s", name, resp.StatusCode, body)
		}
	}

	for name, tc := range map[string]struct{ path, body string }{
		"preview oversized trailing": {"/preview", `{}` + strings.Repeat(" ", 2<<10)},
		"delete oversized trailing":  {"/workloads/api", `{"version":0}` + strings.Repeat(" ", 2<<10)},
	} {
		method := http.MethodPost
		if strings.HasPrefix(tc.path, "/workloads") {
			method = http.MethodDelete
		}
		if resp, body := call(t, client, method, base+tc.path, tc.body); resp.StatusCode != http.StatusRequestEntityTooLarge {
			t.Errorf("%s = %d %s", name, resp.StatusCode, body)
		}
	}

	// Valid pending add, then a stale version and stale token are conflicts.
	if resp, body := call(t, client, http.MethodPut, base+"/workloads/api", `{"version":0,"score":`+api+`}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("save = %d %s", resp.StatusCode, body)
	}
	if resp, _ := call(t, client, http.MethodDelete, base+"/workloads/api", `{"version":0}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("stale delete = %d", resp.StatusCode)
	}
	if resp, _ := call(t, client, http.MethodPost, base+"/workloads/api/undo", `{"version":0}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("stale undo = %d", resp.StatusCode)
	}
	_, preview := requestJSON(t, client, http.MethodPost, base+"/preview", map[string]any{})
	token := preview["token"].(string)
	if resp, _ := call(t, client, http.MethodPut, base+"/workloads/api", `{"version":1,"score":`+strings.Replace(api, ":v1", ":v2", 1)+`}`); resp.StatusCode != http.StatusOK {
		t.Fatal("second save failed")
	}
	resp, body := call(t, client, http.MethodPost, base+"/deploy", `{"token":"`+token+`"}`)
	if resp.StatusCode != http.StatusConflict || !strings.Contains(body, "preview changes again") {
		t.Fatalf("stale token = %d %s", resp.StatusCode, body)
	}

	// Another Organization cannot see or mutate these drafts.
	ctx := context.Background()
	hash, err := password.Hash("test-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Store.SaveOrganization(ctx, appdomain.Organization{ID: "org-globex-uc07", Key: "globex", Name: "Globex"}); err != nil {
		t.Fatal(err)
	}
	if err := app.Store.SaveUserAccount(ctx, identity.UserAccount{ID: "user-globex-uc07", OrganizationKey: "globex", Username: "globex-uc07", PasswordHash: hash, Role: identity.RoleDeveloper, Status: identity.AccountActive}); err != nil {
		t.Fatal(err)
	}
	foreign := authenticatedClientAs(t, server.URL, "globex-uc07")
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/workloads", ""},
		{http.MethodPut, "/workloads/api", `{"version":2,"score":` + api + `}`},
		{http.MethodDelete, "/workloads/api", `{"version":2}`},
		{http.MethodPost, "/workloads/api/undo", `{"version":2}`},
		{http.MethodPost, "/preview", `{}`},
		{http.MethodPost, "/deploy", `{"token":"x"}`},
	} {
		if resp, _ := call(t, foreign, tc.method, base+tc.path, tc.body); resp.StatusCode != http.StatusNotFound {
			t.Errorf("foreign %s %s = %d", tc.method, tc.path, resp.StatusCode)
		}
	}
	if resp, _ := call(t, newJarClient(t), http.MethodDelete, base+"/workloads/api", `{"version":2}`); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous delete = %d", resp.StatusCode)
	}
}
