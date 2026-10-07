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

func previewScore(name string) map[string]any {
	return map[string]any{
		"apiVersion": "score.dev/v1b1",
		"metadata":   map[string]any{"name": name},
		"containers": map[string]any{"main": map[string]any{"image": "example.invalid/" + name + ":v1", "variables": map[string]any{"PORT": "8080"}}},
		"service":    map[string]any{"ports": map[string]any{"http": map[string]any{"port": 8080}}},
	}
}

func TestUC05ScorePreviewHTTP(t *testing.T) {
	server, _, app := newServerApp(t, "")
	client := authenticatedClient(t, server.URL)
	created := createConfiguredApplication(t, client, server.URL, "Preview Test", "preview-test", "internal-cluster")
	key := created["application"].(map[string]any)["key"].(string)
	base := server.URL + "/api/v1/applications/" + key + "/environments/staging"
	endpoint := base + "/score-preview"
	valid := map[string]any{"workloadId": "api", "action": "deploy", "runId": "run-http", "scoreAfter": previewScore("api")}
	validJSON := toJSON(t, valid)
	_, draftsBefore := requestJSON(t, client, http.MethodGet, base+"/workloads", nil)

	t.Run("success is explicit, read-only and has no token", func(t *testing.T) {
		resp, body := call(t, client, http.MethodPost, endpoint, validJSON)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("preview = %d %s", resp.StatusCode, body)
		}
		view := decode(t, body)
		want := []string{"applicationKey", "environmentKey", "baseSetId", "baseVersion", "runId", "workloadId", "action", "planHash", "delta", "candidateSet", "graph", "matches", "batches", "classification"}
		if len(view) != len(want) {
			t.Fatalf("response keys = %v", view)
		}
		for _, k := range want {
			if _, ok := view[k]; !ok {
				t.Fatalf("missing %s", k)
			}
		}
		if view["applicationKey"] != key || view["environmentKey"] != "staging" || view["action"] != "deploy" || view["runId"] != "run-http" {
			t.Fatalf("scope/echo wrong: %v", view)
		}
		added := view["delta"].(map[string]any)["modules"].(map[string]any)["add"].(map[string]any)
		if _, ok := added["api"]; !ok {
			t.Fatalf("delta does not add api: %v", view["delta"])
		}
		if _, drafts := requestJSON(t, client, http.MethodGet, base+"/workloads", nil); toJSON(t, drafts) != toJSON(t, draftsBefore) {
			t.Fatal("preview saved a draft")
		}
		// The pending-change Preview is a separate flow and stays empty.
		if status, pending := requestJSON(t, client, http.MethodPost, base+"/preview", map[string]any{}); status != http.StatusOK || len(pending["changes"].([]any)) != 0 {
			t.Fatalf("pending preview affected: %d %v", status, pending)
		}
		deployments, _ := app.Store.ListDeployments(context.Background(), key, "staging")
		if len(deployments) != 0 {
			t.Fatal("preview created a Deployment")
		}
	})

	t.Run("strict request", func(t *testing.T) {
		for name, body := range map[string]string{
			"unknown field":         strings.Replace(validJSON, `"runId"`, `"organizationKey":"globex","runId"`, 1),
			"trailing document":     validJSON + ` {}`,
			"trailing garbage":      validJSON + ` x`,
			"malformed":             `{"workloadId":`,
			"empty":                 ``,
			"null before":           strings.Replace(validJSON, `"runId"`, `"scoreBefore":null,"runId"`, 1),
			"array after":           `{"workloadId":"api","action":"deploy","runId":"r","scoreAfter":[]}`,
			"string after":          `{"workloadId":"api","action":"deploy","runId":"r","scoreAfter":"api"}`,
			"wrong type":            `{"workloadId":7,"action":"deploy","runId":"r","scoreAfter":{}}`,
			"uppercase action":      strings.Replace(validJSON, `"deploy"`, `"DEPLOY"`, 1),
			"blank workload":        strings.Replace(validJSON, `"workloadId":"api"`, `"workloadId":"  "`, 1),
			"blank run":             strings.Replace(validJSON, `"run-http"`, `" "`, 1),
			"missing run":           `{"workloadId":"api","action":"deploy","scoreAfter":` + toJSON(t, previewScore("api")) + `}`,
			"update without before": strings.Replace(validJSON, `"deploy"`, `"update"`, 1),
			"name mismatch":         strings.Replace(validJSON, `"workloadId":"api"`, `"workloadId":"web"`, 1),
			"invalid Score":         `{"workloadId":"api","action":"deploy","runId":"r","scoreAfter":{"apiVersion":"score.dev/v1b1","metadata":{"name":"api"}}}`,
			"remove absent":         `{"workloadId":"api","action":"remove","runId":"r","scoreBefore":` + toJSON(t, previewScore("api")) + `}`,
		} {
			resp, out := call(t, client, http.MethodPost, endpoint, body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("%s = %d %s", name, resp.StatusCode, out)
				continue
			}
			if msg, _ := decode(t, out)["error"].(string); msg == "" {
				t.Errorf("%s has no actionable message", name)
			}
		}
		huge := `{"workloadId":"api","action":"deploy","runId":"r","scoreAfter":{"pad":"` + strings.Repeat("x", 3<<20) + `"}}`
		if resp, _ := call(t, client, http.MethodPost, endpoint, huge); resp.StatusCode != http.StatusRequestEntityTooLarge {
			t.Errorf("oversized body = %d", resp.StatusCode)
		}
		if resp, _ := call(t, client, http.MethodPost, endpoint, validJSON+strings.Repeat(" ", 3<<20)); resp.StatusCode != http.StatusRequestEntityTooLarge {
			t.Errorf("oversized trailing whitespace = %d", resp.StatusCode)
		}
	})

	t.Run("scope", func(t *testing.T) {
		if resp, _ := call(t, newJarClient(t), http.MethodPost, endpoint, validJSON); resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("anonymous = %d", resp.StatusCode)
		}
		for _, url := range []string{
			server.URL + "/api/v1/applications/missing/environments/staging/score-preview",
			server.URL + "/api/v1/applications/" + key + "/environments/dev/score-preview",
		} {
			if resp, body := call(t, client, http.MethodPost, url, validJSON); resp.StatusCode != http.StatusNotFound {
				t.Fatalf("%s = %d %s", url, resp.StatusCode, body)
			}
		}
		ctx := context.Background()
		hash, err := password.Hash("test-password")
		if err != nil {
			t.Fatal(err)
		}
		if err := app.Store.SaveOrganization(ctx, appdomain.Organization{ID: "org-globex", Key: "globex", Name: "Globex"}); err != nil {
			t.Fatal(err)
		}
		if err := app.Store.SaveUserAccount(ctx, identity.UserAccount{ID: "user-globex-preview", OrganizationKey: "globex", Username: "globex-preview", PasswordHash: hash, Role: identity.RoleDeveloper, Status: identity.AccountActive}); err != nil {
			t.Fatal(err)
		}
		foreign := authenticatedClientAs(t, server.URL, "globex-preview")
		resp, body := call(t, foreign, http.MethodPost, endpoint, validJSON)
		if resp.StatusCode != http.StatusNotFound || strings.Contains(body, "candidateSet") {
			t.Fatalf("foreign organization = %d %s", resp.StatusCode, body)
		}
	})

	t.Run("not ready connection is a safe 400", func(t *testing.T) {
		ctx := context.Background()
		a, _ := app.Store.GetApplication(ctx, key)
		e, _ := app.Store.GetEnvironment(ctx, key, "staging")
		conn, _ := app.Store.GetConnection(ctx, a.OrganizationKey, e.ConnectionKey)
		saved := conn
		conn.Status = appdomain.ConnectionVerifying
		if err := app.Store.SaveConnection(ctx, conn); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = app.Store.SaveConnection(ctx, saved) }()
		resp, body := call(t, client, http.MethodPost, endpoint, validJSON)
		var out map[string]string
		_ = json.Unmarshal([]byte(body), &out)
		if resp.StatusCode != http.StatusBadRequest || strings.Contains(out["error"], conn.Key) || !strings.Contains(out["error"], "not READY") {
			t.Fatalf("not ready = %d %s", resp.StatusCode, body)
		}
	})
}
