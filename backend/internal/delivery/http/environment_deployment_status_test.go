package http

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	appsvc "orchestrator/internal/application/application"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/seed/seedtest"
)

func TestApplicationListProjectsStatusPerEnvironment(t *testing.T) {
	ts, st := newUsersServer(t)
	admin := signInClient(t, ts.URL, "admin", adminTestPassword)
	url := ts.URL + "/api/v1/applications"
	ctx := context.Background()
	created, err := appsvc.NewService(st).Create(ctx, appsvc.CreateCommand{
		OrganizationKey: seedtest.Defaults().OrganizationKey,
		Name:            "Issue98",
	})
	if err != nil {
		t.Fatal(err)
	}
	key := created.Application.Key
	staging, err := st.GetEnvironment(ctx, key, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveDeployment(ctx, deployment.Deployment{
		ID: "issue-98-run", ApplicationKey: key, EnvironmentKey: staging.Key,
		EnvironmentID: staging.ID, Status: deployment.StatusFailed, StartedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	code, body := request(t, admin, http.MethodGet, url, "")
	var listed struct{ Applications []applicationView }
	if code != http.StatusOK || json.Unmarshal([]byte(body), &listed) != nil {
		t.Fatalf("list applications: %d", code)
	}
	for _, app := range listed.Applications {
		if app.Key != key {
			continue
		}
		status := map[string]string{}
		for _, env := range app.Environments {
			status[env.Key] = env.DeploymentStatus
		}
		if status["staging"] != "FAILED" || status["production"] != "NOT_DEPLOYED" {
			t.Fatalf("deployment status: %v", status)
		}
		return
	}
	t.Fatal("created application not listed")
}
