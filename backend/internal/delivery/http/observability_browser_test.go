package http

import (
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/authentication"
	appquery "orchestrator/internal/application/deployment"
	obs "orchestrator/internal/application/observability"
	"orchestrator/internal/application/workloadconfig"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/platform/password"
	"orchestrator/internal/ports/persistence/persistencetest"
	"orchestrator/internal/seed"
)

// Always verifies the Console's authenticated history response. Set the script
// environment variable to additionally exercise the built Console in Chromium;
// the browser uses this real HTTP server, not intercepted telemetry responses.
func TestObservabilityConsoleHistory(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	f := persistencetest.Telemetry(t, st)
	hash, err := password.Hash(seed.FixedTestPassword)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveUserAccount(ctx, identity.UserAccount{ID: "console-observer", Username: "console-observer", OrganizationKey: f.Scope.OrganizationKey, PasswordHash: hash, Role: identity.RoleDeveloper, Status: identity.AccountActive}); err != nil {
		t.Fatal(err)
	}
	// This represents an existing deployed workload; telemetry itself is the
	// explicitly synthetic received-history fixture shared by the adapter tests.
	if err := st.UpsertWorkloadInstance(ctx, deployment.WorkloadInstance{ID: "console-workload", EnvironmentKey: f.Scope.ApplicationKey + "/staging", WorkloadID: "backend", Status: deployment.InstanceReady, ObservedAt: f.Now}); err != nil {
		t.Fatal(err)
	}
	ui, err := filepath.Abs("../../../../frontend/dist")
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(Config{Store: st, Authentication: authentication.NewService(st), Observability: obs.New(st, nil), Workloads: workloadconfig.NewService(st), Queries: appquery.NewQueryService(st), UIDir: ui})
	ts := httptest.NewServer(server)
	defer ts.Close()
	client := bundleClient(t, ts.URL, "console-observer")
	response, err := client.Get(ts.URL + "/api/v1/applications/" + f.Scope.ApplicationKey + "/environments/staging/observability/overview")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("history HTTP %d", response.StatusCode)
	}
	if script := os.Getenv("ORCHESTRATOR_OBSERVABILITY_BROWSER_SCRIPT"); script != "" {
		cmd := exec.Command("node", script)
		cmd.Env = append(os.Environ(), "OBSERVABILITY_BASE_URL="+ts.URL, "OBSERVABILITY_APP="+f.Scope.ApplicationKey, "OBSERVABILITY_TEST_USERNAME=console-observer", "OBSERVABILITY_TEST_PASSWORD="+seed.FixedTestPassword)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("Console browser: %v\n%s", err, output)
		} else {
			t.Log(string(output))
		}
	}
}
