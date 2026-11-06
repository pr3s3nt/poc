package observability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/adapters/credentialmemory"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/monitoring"
	"orchestrator/internal/domain/telemetry"
	"orchestrator/internal/ports/execution"
	obs "orchestrator/internal/ports/observability"
	"orchestrator/internal/ports/persistence"
)

type durableCredentialFixture struct{ *credentialmemory.Store }

func (durableCredentialFixture) Durable() bool { return true }

func monitoringFixture(t *testing.T) (*Monitoring, *store.Store, telemetry.Scope) {
	t.Helper()
	ctx := context.Background()
	st := store.New()
	if e := st.SaveOrganization(ctx, application.Organization{ID: "org-id", Key: "acme", Name: "Acme"}); e != nil {
		t.Fatal(e)
	}
	if e := st.SaveApplication(ctx, application.Application{ID: "app-id", Key: "shop", OrganizationKey: "acme", Name: "Shop"}); e != nil {
		t.Fatal(e)
	}
	if e := st.SaveEnvironment(ctx, environment.Environment{ID: "env-id", ApplicationKey: "shop", Key: "staging", Name: "Staging", NamespaceIdentity: "shop-stage"}); e != nil {
		t.Fatal(e)
	}
	return NewMonitoring(New(st, nil), st, durableCredentialFixture{credentialmemory.New()}), st, telemetry.Scope{OrganizationKey: "acme", ApplicationKey: "shop", EnvironmentKey: "staging"}
}
func connectionFixture(url string) monitoring.Connection {
	return monitoring.Connection{Name: "Test source", ClusterUID: "cluster-one", PrometheusURL: url, AlertmanagerURL: url, LokiURL: url, Rules: monitoring.DevRules(), RuleLabels: map[string]string{"release": "metrics"}}
}
func monitoringProvider(t *testing.T, headers bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/query"):
			if headers && r.Header.Get("Authorization") != "Bearer prometheus-secret" {
				t.Error("wrong scoped Prometheus credential")
			}
			if strings.Contains(r.URL.Query().Get("query"), "count by") {
				fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
				return
			}
			metric := map[string]string{"cluster_uid": "cluster-one"}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "vector", "result": []any{map[string]any{"metric": metric, "value": []any{time.Now().Unix(), "1"}}}}})
		case r.URL.Path == "/api/v2/status":
			if headers && r.Header.Get("Authorization") != "Bearer alertmanager-secret" {
				t.Error("wrong scoped Alertmanager credential")
			}
			fmt.Fprint(w, `{"versionInfo":{"version":"test"}}`)
		case r.URL.Path == "/loki/api/v1/label/cluster_uid/values":
			if headers && r.Header.Get("Authorization") != "Bearer loki-secret" {
				t.Error("wrong scoped Loki credential")
			}
			fmt.Fprint(w, `{"status":"success","data":["cluster-one"]}`)
		default:
			http.NotFound(w, r)
		}
	}))
}
func TestMonitoringSourceCredentialsStayScopedWhenEndpointsMatch(t *testing.T) {
	m, _, _ := monitoringFixture(t)
	server := monitoringProvider(t, true)
	defer server.Close()
	c, e := m.SaveWithTokens(context.Background(), "acme", connectionFixture(server.URL), 0, "", map[string]string{"prometheus": "prometheus-secret", "alertmanager": "alertmanager-secret", "loki": "loki-secret"})
	if e != nil {
		t.Fatal(e)
	}
	facts, e := m.Test(context.Background(), "acme", c.ID)
	if e != nil {
		t.Fatal(e)
	}
	if !facts.Connectivity || !facts.Cluster || facts.Metrics || facts.ConfigSync != "UNVERIFIED" {
		t.Fatalf("wrong independent coverage facts: %+v", facts)
	}
}
func TestMonitoringTestDoesNotInvalidatePreDeployBinding(t *testing.T) {
	ctx := context.Background()
	m, st, scope := monitoringFixture(t)
	server := monitoringProvider(t, false)
	defer server.Close()
	c, e := m.Save(ctx, "acme", connectionFixture(server.URL), 0, "")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = m.Test(ctx, "acme", c.ID); e != nil {
		t.Fatal(e)
	}
	b, e := m.BindSources(ctx, scope, c.ID, 0, "prometheus", "loki")
	if e != nil {
		t.Fatal("predeploy bind", e)
	}
	if _, e = m.Test(ctx, "acme", c.ID); e != nil {
		t.Fatal(e)
	}
	updated, e := st.GetMonitoringConnection(ctx, "acme", c.ID)
	if e != nil {
		t.Fatal(e)
	}
	if updated.Revision != c.Revision || b.ConnectionRevision != updated.Revision {
		t.Fatal("test mutated configuration revision")
	}
	view, e := m.Read(ctx, scope, Params{})
	if e != nil || view.State != "NOT_DEPLOYED" {
		t.Fatalf("predeploy view: %+v %v", view, e)
	}
	if e = m.Delete(ctx, "acme", c.ID, c.Revision); !errors.Is(e, monitoring.ErrInUse) {
		t.Fatal("referenced connection deletion was accepted", e)
	}
	if e = m.Unbind(ctx, scope, b.Revision); e != nil {
		t.Fatal(e)
	}
	if e = m.Delete(ctx, "acme", c.ID, c.Revision); e != nil {
		t.Fatal(e)
	}
}
func TestMonitoringVerificationRejectsConcurrentConfigRevision(t *testing.T) {
	ctx := context.Background()
	m, st, _ := monitoringFixture(t)
	c, e := m.Save(ctx, "acme", connectionFixture("http://127.0.0.1:9999"), 0, "")
	if e != nil {
		t.Fatal(e)
	}
	c.Name = "Changed source name"
	next, e := m.Save(ctx, "acme", c, c.Revision, "")
	if e != nil {
		t.Fatal(e)
	}
	c.Verification = monitoring.Verification{Revision: c.Revision}
	e = st.Transact(ctx, func(ctx context.Context) error { return st.VerifyMonitoringConnection(ctx, c, c.Revision) })
	if !errors.Is(e, persistence.ErrVersionConflict) {
		t.Fatal("stale verification overwrote changed config", e)
	}
	actual, e := st.GetMonitoringConnection(ctx, "acme", c.ID)
	if e != nil || actual.Revision != next.Revision || actual.VerifiedAt != nil {
		t.Fatal("changed config corrupted")
	}
}
func TestMonitoringConfigurationCloneAndReservedLabels(t *testing.T) {
	ctx := context.Background()
	m, st, _ := monitoringFixture(t)
	input := connectionFixture("http://127.0.0.1:9999")
	input.RuleLabels["orchestrator.io/application"] = "foreign"
	if _, e := m.Save(ctx, "acme", input, 0, ""); !errors.Is(e, monitoring.ErrInvalid) {
		t.Fatal("accepted identity override", e)
	}
	delete(input.RuleLabels, "orchestrator.io/application")
	c, e := m.Save(ctx, "acme", input, 0, "")
	if e != nil {
		t.Fatal(e)
	}
	c.RuleLabels["release"] = "changed-outside-repo"
	loaded, e := st.GetMonitoringConnection(ctx, "acme", c.ID)
	if e != nil || loaded.RuleLabels["release"] != "metrics" {
		t.Fatal("repository returned shared mutable config")
	}
	if _, e = st.GetMonitoringConnection(ctx, "foreign", c.ID); !errors.Is(e, persistence.ErrNotFound) {
		t.Fatal("cross-organization read", e)
	}
}

type desiredClusterFixture struct{ uid string }

func (f desiredClusterFixture) Collect(context.Context, obs.Source, telemetry.State) (telemetry.Batch, error) {
	return telemetry.Batch{}, nil
}
func (f desiredClusterFixture) MonitoringCluster(context.Context, execution.Target) (string, error) {
	return f.uid, nil
}
func TestMonitoringFirstDeploymentVerifiesPhysicalCluster(t *testing.T) {
	ctx := context.Background()
	m, st, scope := monitoringFixture(t)
	server := monitoringProvider(t, false)
	defer server.Close()
	c, e := m.Save(ctx, "acme", connectionFixture(server.URL), 0, "")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = m.Test(ctx, "acme", c.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = m.Bind(ctx, scope, c.ID, 0); e != nil {
		t.Fatal(e)
	}
	env, e := st.GetEnvironment(ctx, "shop", "staging")
	if e != nil {
		t.Fatal(e)
	}
	m.Native.reader = desiredClusterFixture{"cluster-one"}
	target := execution.Target{Organization: "acme", Connection: "scoped-account", Namespace: "shop-stage"}
	manifests := []execution.Manifest{{APIVersion: "apps/v1", Kind: "Deployment", Name: "web", Namespace: "shop-stage"}}
	planned, e := m.PlanRules(ctx, "acme", "shop", env, target, "web", manifests)
	if e != nil || len(planned) != 2 || planned[1].Kind != "PrometheusRule" {
		t.Fatalf("first deploy rules: %+v %v", planned, e)
	}
	second, e := m.PlanRules(ctx, "acme", "shop", env, target, "web", manifests)
	if e != nil || second[1].Name != planned[1].Name {
		t.Fatal("unstable desired rule identity", e)
	}
	m.Native.reader = desiredClusterFixture{"foreign-cluster"}
	if _, e = m.PlanRules(ctx, "acme", "shop", env, target, "web", manifests); !errors.Is(e, monitoring.ErrInvalid) {
		t.Fatal("foreign physical destination accepted", e)
	}
}

func TestMonitoringUnchangedConfigAndBindingAreNoops(t *testing.T) {
	ctx := context.Background()
	m, st, scope := monitoringFixture(t)
	server := monitoringProvider(t, false)
	defer server.Close()
	c, e := m.Save(ctx, "acme", connectionFixture(server.URL), 0, "")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = m.Test(ctx, "acme", c.ID); e != nil {
		t.Fatal(e)
	}
	c, e = st.GetMonitoringConnection(ctx, "acme", c.ID)
	if e != nil {
		t.Fatal(e)
	}
	same, e := m.Save(ctx, "acme", c, c.Revision, "")
	if e != nil || same.Revision != c.Revision || same.VerifiedAt == nil {
		t.Fatal("same config invalidated verification", e)
	}
	first, e := m.Bind(ctx, scope, c.ID, 0)
	if e != nil {
		t.Fatal(e)
	}
	second, e := m.Bind(ctx, scope, c.ID, first.Revision)
	if e != nil || second.Revision != first.Revision {
		t.Fatal("same binding bumped revision", e)
	}
}

type failingDeleteCredentials struct {
	durableCredentialFixture
	fail bool
}

func (f *failingDeleteCredentials) Delete(ctx context.Context, org, key, ref string) error {
	if f.fail {
		return errors.New("provider unavailable")
	}
	return f.Store.Delete(ctx, org, key, ref)
}
func TestMonitoringCredentialRotationAndDeletionAreRecoverable(t *testing.T) {
	ctx := context.Background()
	m, st, scope := monitoringFixture(t)
	creds := &failingDeleteCredentials{durableCredentialFixture: durableCredentialFixture{credentialmemory.New()}}
	m.Credentials = creds
	c, e := m.SaveWithTokens(ctx, "acme", connectionFixture("https://metrics.example.test"), 0, "", map[string]string{"prometheus": "old-token"})
	if e != nil {
		t.Fatal(e)
	}
	_, old := c.Credential("prometheus")
	creds.fail = true
	c, e = m.SaveWithTokens(ctx, "acme", c, c.Revision, "", map[string]string{"prometheus": "new-token"})
	if e != nil {
		t.Fatal(e)
	}
	if len(c.RetiredCredentials) != 1 || c.RetiredCredentials[0].Reference != old {
		t.Fatal("lost cleanup receipt")
	}
	if _, e = m.client(ctx, c, "prometheus"); e != nil {
		t.Fatal("new token unavailable", e)
	}
	creds.fail = false
	rev := c.Revision
	c, e = m.Save(ctx, "acme", c, c.Revision, "")
	if e != nil || c.Revision != rev || len(c.RetiredCredentials) != 0 {
		t.Fatal("retry changed config or lost receipt", e)
	}
	if _, e = creds.Get(ctx, "acme", c.Key+"-prometheus", old); e == nil {
		t.Fatal("superseded credential retained")
	}
	changed := c
	changed.PrometheusURL = "https://new.example.test"
	if _, e = m.Save(ctx, "acme", changed, c.Revision, ""); !errors.Is(e, monitoring.ErrCredentialChange) {
		t.Fatal("endpoint reused credential", e)
	}
	creds.fail = true
	if e = m.Delete(ctx, "acme", c.ID, c.Revision); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
	saved, e := st.GetMonitoringConnection(ctx, "acme", c.ID)
	if e != nil || !saved.Deleting {
		t.Fatal("failed deletion lost tombstone", e)
	}
	if _, e = m.Save(ctx, "acme", c, c.Revision, ""); !errors.Is(e, monitoring.ErrInUse) {
		t.Fatal("reactivated deleting connection", e)
	}
	if _, e = m.Test(ctx, "acme", c.ID); !errors.Is(e, monitoring.ErrInUse) {
		t.Fatal("tested deleting connection", e)
	}
	if _, e = m.BindSources(ctx, scope, c.ID, 0, "native", "native"); !errors.Is(e, monitoring.ErrInUse) {
		t.Fatal("bound deleting connection", e)
	}
	creds.fail = false
	if e = m.Delete(ctx, "acme", c.ID, c.Revision); e != nil {
		t.Fatal(e)
	}
	if _, e = st.GetMonitoringConnection(ctx, "acme", c.ID); !errors.Is(e, persistence.ErrNotFound) {
		t.Fatal("retry did not finish deletion", e)
	}
}
func TestMonitoringEndpointChangeRequiresExplicitCredentialClear(t *testing.T) {
	ctx := context.Background()
	m, _, _ := monitoringFixture(t)
	c, e := m.SaveWithTokens(ctx, "acme", connectionFixture("https://old.example.test"), 0, "", map[string]string{"prometheus": "token"})
	if e != nil {
		t.Fatal(e)
	}
	c.PrometheusURL = "https://new.example.test"
	c, e = m.SaveWithTokens(ctx, "acme", c, c.Revision, "", map[string]string{"prometheus": ""})
	if e != nil {
		t.Fatal(e)
	}
	if _, ref := c.Credential("prometheus"); ref != "" {
		t.Fatal("explicit clear failed")
	}
}
