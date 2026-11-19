package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"orchestrator/internal/application/workloadconfig"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/seed/seedtest"
	"strings"
	"testing"
)

func TestWorkloadScoreExportIsScopedAndPreservesAdvancedNativeDraft(t *testing.T) {
	handler, server, st := bundleTestServer(t, nil)
	handler.workloads = workloadconfig.NewService(st)
	opts := seedtest.Defaults()
	ctx := context.Background()
	if err := st.SaveEnvironment(ctx, environment.Environment{Key: "staging", ApplicationKey: opts.ApplicationKey, NamespaceIdentity: "fixture-staging", Version: 1}); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/applications/" + opts.ApplicationKey + "/environments/staging/workloads/"
	body := `{"workload":{"apiVersion":"orchestrator.dev/workload/v1","metadata":{"name":"web"},"replicas":0,"containers":{"main":{"image":"example/web:1","securityContext":{"readOnlyRootFilesystem":false}}},"workloadExecution":{"version":1,"kind":"Deployment","automountServiceAccountToken":false}}}`
	client := bundleClient(t, server.URL, "developer")
	response, err := client.Post(server.URL+path+"export", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("export %d %s", response.StatusCode, raw)
	}
	var doc struct {
		Score      string `json:"score"`
		Extensions string `json:"extensions"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.Score, "score.dev/v1b1") || strings.Contains(doc.Score, "securityContext") || !strings.Contains(doc.Extensions, "readOnlyRootFilesystem: false") || !strings.Contains(doc.Extensions, "replicas: 0") {
		t.Fatal("export lost explicit runtime intent")
	}
	drafts, err := st.ListWorkloadDrafts(ctx, opts.ApplicationKey, "staging")
	if err != nil || len(drafts) > 0 {
		t.Fatal("export wrote a draft")
	}
	foreign := bundleClient(t, server.URL, "other-admin")
	r, err := foreign.Post(server.URL+path+"export", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode == 200 {
		t.Fatal("foreign org exported workload")
	}
	for _, body := range []string{
		`{"content":"apiVersion: score.dev/v1b1\nmetadata: {name: web}\ncontainers: {main: {image: example/web:1}}","extensions":"apiVersion: humanitec.org/v1b1\nspec: {pod: {nodeSelector: {pool: other}}}"}`,
		`{"content":"name: web\nimage: example/web:1","extensions":"apiVersion: humanitec.org/v1b1"}`,
		`{"content":"name: web\nimage: example/web:1\n---\nname: second"}`,
	} {
		r, err := client.Post(server.URL+path+"parse", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != http.StatusBadRequest {
			t.Fatalf("unsupported import status %d", r.StatusCode)
		}
	}
}
