package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/authentication"
	driftapp "orchestrator/internal/application/drift"
	"orchestrator/internal/application/envops"
	domain "orchestrator/internal/domain/drift"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformstate"
	"orchestrator/internal/seed/seedtest"
)

type emptyDriftInspector struct{ reads int }

func (f *emptyDriftInspector) ListInstances(context.Context, string) ([]tfresource.Instance, error) {
	return nil, nil
}
func (f *emptyDriftInspector) GetInstance(context.Context, string) (tfresource.Instance, error) {
	return tfresource.Instance{}, persistence.ErrNotFound
}
func (f *emptyDriftInspector) LatestExecutionBaseline(context.Context, string, string) (terraformstate.ExecutionBaseline, error) {
	return terraformstate.ExecutionBaseline{}, persistence.ErrNotFound
}
func (f *emptyDriftInspector) InspectInstance(context.Context, string, string) (domain.Report, error) {
	f.reads++
	return domain.Report{}, nil
}

func TestInfrastructureScansHTTPIsScopedReadOnlyOnGETAndPlatformOnlyOnPOST(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	opts := seedtest.Defaults()
	if err := seedtest.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveEnvironment(ctx, environment.Environment{ApplicationKey: opts.ApplicationKey, Key: "staging", NamespaceIdentity: "scan-staging"}); err != nil {
		t.Fatal(err)
	}
	env, err := st.GetEnvironment(ctx, opts.ApplicationKey, "staging")
	if err != nil {
		t.Fatal(err)
	}
	inspector := &emptyDriftInspector{}
	service := &driftapp.Service{Store: st, Instances: inspector, Inspector: inspector, Operations: envops.NewManager(st)}
	server := httptest.NewServer(NewServer(Config{Authentication: authentication.NewService(st), Store: st, SeedOptions: opts, Drift: service}))
	defer server.Close()
	developer := bundleClient(t, server.URL, "developer")
	engineer := bundleClient(t, server.URL, "platform-engineer")
	endpoint := server.URL + "/api/v1/applications/" + opts.ApplicationKey + "/environments/staging/drift-scans"
	body := fmt.Sprintf(`{"requestId":%q,"expectedEnvironmentVersion":%d}`, ids.New(), env.Version)
	post := func(client *http.Client, body string) int {
		t.Helper()
		resp, err := client.Post(endpoint, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}
	if code := post(developer, body); code != 403 {
		t.Fatal("developer scan mutation admitted", code)
	}
	if code := post(engineer, body); code != 202 {
		t.Fatal("platform scan rejected", code)
	}
	response, err := developer.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var view struct{ Scans []domain.Scan }
	if response.StatusCode != 200 || json.NewDecoder(response.Body).Decode(&view) != nil || len(view.Scans) != 1 {
		t.Fatal("stored scan projection unavailable")
	}
	if inspector.reads != 0 {
		t.Fatal("queue or GET invoked provider")
	}
	if view.Scans[0].Status != domain.Queued {
		t.Fatal("queue fabricated clean observation")
	}
	service.ProcessQueued(ctx)
	response2, err := developer.Get(endpoint + "/" + view.Scans[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	defer response2.Body.Close()
	if response2.StatusCode != 200 {
		t.Fatal(response2.StatusCode)
	}
	var detail struct{ Scan domain.Scan }
	if json.NewDecoder(response2.Body).Decode(&detail) != nil || detail.Scan.Result != domain.NotApplicable {
		t.Fatal("no eligible resources were shown clean")
	}
	if code := post(engineer, body); code != 202 {
		t.Fatal("idempotent request rejected", code)
	}
	foreign, err := developer.Get(server.URL + "/api/v1/applications/absent/environments/staging/drift-scans/" + view.Scans[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	foreign.Body.Close()
	if foreign.StatusCode != 404 {
		t.Fatal("foreign scope exposed", foreign.StatusCode)
	}
}
