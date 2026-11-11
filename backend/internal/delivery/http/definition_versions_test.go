package http

import (
	"context"
	"encoding/json"
	"net/http"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"strings"
	"testing"

	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed/seedtest"
)

func TestRevisionAPIsSeparateProposedAndEnvironmentUpgrade(t *testing.T) {
	_, ts, st := bundleTestServer(t, nil)
	opts := seedtest.Defaults()
	opts.EnvironmentKey = "staging"
	if err := st.SaveEnvironment(context.Background(), environment.Environment{Key: "staging", ApplicationKey: opts.ApplicationKey, NamespaceIdentity: "versions-staging", Version: 1}); err != nil {
		t.Fatal(err)
	}
	pe := bundleClient(t, ts.URL, "platform-engineer")
	developer := bundleClient(t, ts.URL, "developer")
	base := ts.URL + "/api/v1/resource-definitions"
	body := `{"key":"version-api","name":"Version one","resourceType":"k8s-cluster","driverType":"existing-cluster","driverAccount":"` + opts.ConnectionKey + `","driverInputs":{"values":{"variables":{}}},"criteria":[{"class":"version-api"}]}`
	if status, out := request(t, pe, http.MethodPost, base, body); status != 201 {
		t.Fatal(status, out)
	}
	var history struct {
		History persistence.DefinitionHistory `json:"history"`
	}
	status, out := request(t, pe, http.MethodGet, base+"/version-api/versions", "")
	if status != 200 || json.Unmarshal([]byte(out), &history) != nil {
		t.Fatal(status, out)
	}
	if len(history.History.Versions) != 1 {
		t.Fatal(out)
	}
	first := history.History.ActiveID
	// A running legacy Environment (deployed before versioning) is pinned by
	// authoring; one that never deployed successfully is not (issue #68).
	if err := st.UpsertWorkloadInstance(context.Background(), deployment.WorkloadInstance{EnvironmentKey: opts.ApplicationKey + "/staging", WorkloadID: "legacy", LastDeploymentID: "pre-versioning"}); err != nil {
		t.Fatal(err)
	}
	edit := strings.TrimSuffix(strings.Replace(body, "Version one", "Version two", 1), "}") + `,"version":1}`
	if status, out := request(t, pe, http.MethodPut, base+"/version-api", edit); status != 200 {
		t.Fatal(status, out)
	}
	status, out = request(t, pe, http.MethodGet, base+"/version-api/versions", "")
	if status != 200 || json.Unmarshal([]byte(out), &history) != nil {
		t.Fatal(status, out)
	}
	if history.History.ActiveID != first || len(history.History.Versions) != 2 {
		t.Fatal("editor automatically promoted", out)
	}
	// Legacy frozen configuration still counts as pinned Usage even without an ID.
	status, out = request(t, pe, http.MethodGet, base+"/workload-template/versions", "")
	if status != 200 || !strings.Contains(out, `"environment":"staging"`) {
		t.Fatal("legacy frozen Usage omitted", status, out)
	}
	second := history.History.Versions[1].ID
	for _, user := range []*http.Client{developer, bundleClient(t, ts.URL, "other-admin")} {
		status, _ := request(t, user, http.MethodGet, base+"/version-api/versions", "")
		if status != 403 && status != 404 {
			t.Fatal("unauthorized history", status)
		}
	}
	envURL := ts.URL + "/api/v1/applications/" + opts.ApplicationKey + "/environments/" + opts.EnvironmentKey + "/definition-versions"
	status, out = request(t, developer, http.MethodGet, envURL, "")
	if status != 200 {
		t.Fatal(status, out)
	}
	if strings.Contains(out, "driverInputs") || strings.Contains(out, "driverAccount") {
		t.Fatal("Environment metadata exposed platform configuration")
	}
	var env struct {
		Revision           int64 `json:"revision"`
		EnvironmentVersion int64 `json:"environmentVersion"`
		Pending            bool  `json:"pending"`
		Definitions        []struct {
			Key     string `json:"key"`
			Desired string `json:"desiredRevisionId"`
		} `json:"definitions"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatal(err)
	}
	for _, d := range env.Definitions {
		if d.Key == "version-api" && d.Desired != first {
			t.Fatal("Environment changed before selection", out)
		}
	}
	payload, _ := json.Marshal(map[string]any{"definitionKey": "version-api", "revisionId": second, "expectedRevision": env.Revision, "expectedEnvironmentVersion": env.EnvironmentVersion})
	if status, out := request(t, developer, http.MethodPut, envURL, string(payload)); status != 200 {
		t.Fatal(status, out)
	}
	if status, _ := request(t, developer, http.MethodPut, envURL, string(payload)); status != 409 {
		t.Fatal("stale selection accepted", status)
	}
	status, out = request(t, developer, http.MethodGet, envURL, "")
	json.Unmarshal([]byte(out), &env)
	if status != 200 || !env.Pending {
		t.Fatal("selection claimed applied", status, out)
	}
	if status, out := request(t, pe, http.MethodPost, base+"/version-api/versions/"+second+"/promote", `{"expectedGeneration":2}`); status != 200 {
		t.Fatal(status, out)
	}
	if status, _ := request(t, pe, http.MethodPost, base+"/version-api/versions/"+second+"/archive", `{"expectedGeneration":3}`); status != 400 {
		t.Fatal("active revision archived", status)
	}
}
