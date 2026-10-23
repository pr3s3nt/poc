package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/seed"
)

// effectivenessDay is 10:00 on 2026-10-10 in the report timezone.
var effectivenessDay = time.Date(2026, 10, 10, 3, 0, 0, 0, time.UTC)

func saveRun(t *testing.T, st *store.Store, id, org, app, env, workload string, action deployment.Action, status deployment.Status, actor string, start, minutes int) {
	t.Helper()
	ctx := context.Background()
	started := effectivenessDay.Add(time.Duration(start) * time.Minute)
	d := deployment.Deployment{ID: id, OrganizationKey: org, ApplicationKey: app, EnvironmentKey: env, WorkloadID: workload, Action: action,
		ActorRef: actor, Status: deployment.StatusPlanning, StartedAt: started, FailureReason: "S3CRET provider output"}
	if err := st.SaveDeployment(ctx, d); err != nil {
		t.Fatal(err)
	}
	if status != deployment.StatusFailed {
		snapshot, err := deployment.NewDeploymentDeltaSnapshot("snap-"+id, id, deployment.DeltaDocument{}, deployment.DeltaSnapshotMetadata{ActorRef: actor, Action: action, WorkloadID: workload}, started)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.SaveDeltaSnapshot(ctx, snapshot); err != nil {
			t.Fatal(err)
		}
		d.DeltaSnapshotID = snapshot.ID
	}
	d.Status = status
	if status == deployment.StatusSucceeded || status == deployment.StatusFailed {
		done := started.Add(time.Duration(minutes) * time.Minute)
		d.FinishedAt = &done
	}
	if err := st.SaveDeployment(ctx, d); err != nil {
		t.Fatal(err)
	}
}

func effectivenessServer(t *testing.T) (string, *store.Store) {
	t.Helper()
	s, ts, st := bundleTestServer(t, nil)
	s.effectiveness.SetClock(func() time.Time { return effectivenessDay.Add(4 * time.Hour) })
	ctx := context.Background()
	for _, app := range []application.Application{
		{ID: "a-shop", Key: "shop", OrganizationKey: "acme", Name: "Shop", Subdomain: "shop", Version: 1},
		{ID: "a-foreign", Key: "foreign", OrganizationKey: "other-org", Name: "Foreign", Subdomain: "foreign", Version: 1},
	} {
		if err := st.SaveApplication(ctx, app); err != nil {
			t.Fatal(err)
		}
	}
	// shop/api on production: two failures then a success (one item), by two people.
	saveRun(t, st, "d1", "acme", "shop", "production", "api", deployment.ActionUpdate, deployment.StatusFailed, seed.FixedPlatformEngineerAccountID, 0, 5)
	saveRun(t, st, "d2", "acme", "shop", "production", "api", deployment.ActionUpdate, deployment.StatusFailed, "developer", 10, 5)
	saveRun(t, st, "d3", "acme", "shop", "production", "api", deployment.ActionUpdate, deployment.StatusSucceeded, seed.FixedPlatformEngineerAccountID, 20, 20)
	// shop/web staging by CI; acceptance production by the developer.
	saveRun(t, st, "d4", "acme", "shop", "staging", "web", deployment.ActionDeploy, deployment.StatusSucceeded, "ci-pipeline", 50, 10)
	saveRun(t, st, "d5", "acme", "acceptance", "production", "backend", deployment.ActionDeploy, deployment.StatusSucceeded, seed.FixedDeveloperAccountID, 60, 60)
	saveRun(t, st, "d6", "acme", "acceptance", "production", "worker", deployment.ActionDeploy, deployment.StatusDeploying, seed.FixedDeveloperAccountID, 70, 0)
	// Another Organization's history never enters acme.
	saveRun(t, st, "x1", "other-org", "foreign", "production", "api", deployment.ActionDeploy, deployment.StatusSucceeded, "other-admin", 0, 1)
	return ts.URL, st
}

func effectivenessGet(t *testing.T, c *http.Client, url string) (int, string) {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func effectivenessPost(t *testing.T, c *http.Client, url, body string) (int, string) {
	t.Helper()
	resp, err := c.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

type testMetric struct {
	ID             string             `json:"id"`
	Status         string             `json:"status"`
	Value          *float64           `json:"value"`
	Numerator      *float64           `json:"numerator"`
	Denominator    *float64           `json:"denominator"`
	Details        map[string]float64 `json:"details"`
	RequiredSource string             `json:"requiredSource"`
}

type testReport struct {
	Scope struct {
		Level, ApplicationKey, WorkloadID string
	} `json:"scope"`
	Period struct {
		From, To, Timezone string
		Days               int
	} `json:"period"`
	DataOrigin string       `json:"dataOrigin"`
	Metrics    []testMetric `json:"metrics"`
	Sources    []struct {
		ID        string `json:"id"`
		Available bool   `json:"available"`
	} `json:"sources"`
	Breakdown *struct {
		Level string `json:"level"`
		Rows  []struct {
			Key                   string `json:"key"`
			ProductionDeployments int    `json:"productionDeployments"`
			CompletedWork         int    `json:"completedWork"`
			OpenWork              int    `json:"openWork"`
			Attempts              int    `json:"attempts"`
			FailedAttempts        int    `json:"failedAttempts"`
			ActiveUsers           int    `json:"activeUsers"`
		} `json:"rows"`
	} `json:"breakdown"`
	Baseline *struct {
		Version   int64  `json:"version"`
		CreatedBy string `json:"createdBy"`
	} `json:"baseline"`
}

func (r testReport) metric(t *testing.T, id string) testMetric {
	t.Helper()
	for _, m := range r.Metrics {
		if m.ID == id {
			return m
		}
	}
	t.Fatalf("metric %s missing", id)
	return testMetric{}
}

func decodeReport(t *testing.T, body string) testReport {
	t.Helper()
	var r testReport
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("%v: %s", err, body)
	}
	return r
}

func TestEffectivenessRequiresPlatformRole(t *testing.T) {
	base, _ := effectivenessServer(t)
	developer := bundleClient(t, base, "developer")
	for _, path := range []string{"/api/v1/effectiveness/report", "/api/v1/effectiveness/records", "/api/v1/effectiveness/baselines"} {
		if code, _ := effectivenessGet(t, &http.Client{}, base+path); code != http.StatusUnauthorized {
			t.Fatalf("anonymous %s: %d", path, code)
		}
		if code, body := effectivenessGet(t, developer, base+path); code != http.StatusForbidden || strings.Contains(body, "shop") {
			t.Fatalf("developer %s: %d %s", path, code, body)
		}
	}
	if code, _ := effectivenessPost(t, developer, base+"/api/v1/effectiveness/baselines", `{}`); code != http.StatusForbidden {
		t.Fatalf("developer baseline write: %d", code)
	}
}

func TestEffectivenessOrganizationReport(t *testing.T) {
	base, _ := effectivenessServer(t)
	pe := bundleClient(t, base, "platform-engineer")
	code, body := effectivenessGet(t, pe, base+"/api/v1/effectiveness/report?from=2026-10-04&to=2026-10-10")
	if code != http.StatusOK {
		t.Fatalf("%d %s", code, body)
	}
	for _, leak := range []string{"S3CRET", "failureReason", "foreign", "other-admin"} {
		if strings.Contains(body, leak) {
			t.Fatalf("report leaks %q: %s", leak, body)
		}
	}
	r := decodeReport(t, body)
	if r.Scope.Level != "ORGANIZATION" || r.Period.Days != 7 || r.Period.Timezone != "Asia/Ho_Chi_Minh" || r.DataOrigin != "TEST_INSTALLATION" {
		t.Fatalf("scope/period/origin: %+v", r)
	}
	if freq := r.metric(t, "deployment_frequency"); *freq.Value != 2 || *freq.Denominator != 7 {
		t.Fatalf("frequency counts one retried item and one deploy: %+v", freq)
	}
	if users := r.metric(t, "active_users"); *users.Value != 2 || users.Details["automationRuns"] != 1 {
		t.Fatalf("users by ID and username are one person each; CI is automation: %+v", users)
	}
	attempts := r.metric(t, "execution_attempts")
	if *attempts.Value != 6 || attempts.Details["failedRuns"] != 2 || attempts.Details["runningRuns"] != 1 || *attempts.Numerator != 5 || *attempts.Denominator != 3 {
		t.Fatalf("attempts: %+v", attempts)
	}
	if work := r.metric(t, "completed_work"); *work.Value != 3 || work.Details["openWork"] != 1 {
		t.Fatalf("completed work: %+v", work)
	}
	if apps := r.metric(t, "active_applications"); *apps.Numerator != 2 || *apps.Denominator != 2 {
		t.Fatalf("active applications: %+v", apps)
	}
	if lead := r.metric(t, "change_lead_time"); lead.Status != "NOT_COLLECTED" || lead.Value != nil || lead.RequiredSource != "COMMIT_LINK" {
		t.Fatalf("lead time must stay not collected: %+v", lead)
	}
	if r.Breakdown == nil || r.Breakdown.Level != "APPLICATION" || len(r.Breakdown.Rows) != 2 {
		t.Fatalf("breakdown: %+v", r.Breakdown)
	}
	shop := r.Breakdown.Rows[1]
	if shop.Key != "shop" || shop.ProductionDeployments != 1 || shop.CompletedWork != 2 || shop.Attempts != 4 || shop.FailedAttempts != 2 || shop.ActiveUsers != 2 {
		t.Fatalf("shop row: %+v", shop)
	}
	for _, src := range r.Sources {
		if src.ID == "resource_cleanups" && src.Available {
			t.Fatal("the memory store has no cleanup history; it must be reported unavailable")
		}
	}
}

func TestEffectivenessDrillDownAndRecords(t *testing.T) {
	base, _ := effectivenessServer(t)
	pe := bundleClient(t, base, "platform-engineer")
	code, body := effectivenessGet(t, pe, base+"/api/v1/effectiveness/report?from=2026-10-04&to=2026-10-10&application=shop")
	if code != http.StatusOK {
		t.Fatalf("%d %s", code, body)
	}
	app := decodeReport(t, body)
	if app.Scope.Level != "APPLICATION" || app.Breakdown.Level != "WORKLOAD" || len(app.Breakdown.Rows) != 2 || app.Breakdown.Rows[0].Key != "api" {
		t.Fatalf("application scope: %s", body)
	}
	_, body = effectivenessGet(t, pe, base+"/api/v1/effectiveness/report?from=2026-10-04&to=2026-10-10&application=shop&workload=api")
	if w := decodeReport(t, body); w.Scope.Level != "WORKLOAD" || w.Breakdown != nil || *w.metric(t, "completed_work").Value != 1 {
		t.Fatalf("workload scope: %s", body)
	}
	code, body = effectivenessGet(t, pe, base+"/api/v1/effectiveness/records?from=2026-10-04&to=2026-10-10&metric=deployment_frequency")
	if code != http.StatusOK {
		t.Fatalf("%d %s", code, body)
	}
	var records struct {
		Records []struct {
			ID, ApplicationName, State, LastRunID string
			Attempts, FailedAttempts              int
			DurationSeconds                       *float64
			Actors                                []struct{ Name, Kind string }
		} `json:"records"`
	}
	if err := json.Unmarshal([]byte(body), &records); err != nil {
		t.Fatal(err)
	}
	if len(records.Records) != 2 || strings.Contains(body, "S3CRET") {
		t.Fatalf("records: %s", body)
	}
	var retried = records.Records[1]
	if retried.ID != "d1" || retried.LastRunID != "d3" || retried.Attempts != 3 || retried.FailedAttempts != 2 || *retried.DurationSeconds != 40*60 || len(retried.Actors) != 2 || retried.Actors[0].Name != "platform-engineer" {
		t.Fatalf("retried item: %+v", retried)
	}
	_, body = effectivenessGet(t, pe, base+"/api/v1/effectiveness/records?from=2026-10-04&to=2026-10-10")
	if !strings.Contains(body, `"state":"RUNNING"`) {
		t.Fatalf("unfiltered records include open work: %s", body)
	}
	for url, field := range map[string]string{
		"/api/v1/effectiveness/records?metric=nope":                  "metric",
		"/api/v1/effectiveness/report?from=2026-02-30":               "from",
		"/api/v1/effectiveness/report?from=2026-10-10&to=2026-10-01": "from",
		"/api/v1/effectiveness/report?workload=api":                  "workload",
	} {
		code, body := effectivenessGet(t, pe, base+url)
		if code != http.StatusBadRequest || !strings.Contains(body, `"field":"`+field+`"`) {
			t.Fatalf("%s: %d %s", url, code, body)
		}
	}
	if code, _ := effectivenessGet(t, pe, base+"/api/v1/effectiveness/report?application=foreign"); code != http.StatusNotFound {
		t.Fatalf("another Organization's Application: %d", code)
	}
	other := bundleClient(t, base, "other-admin")
	_, body = effectivenessGet(t, other, base+"/api/v1/effectiveness/report?from=2026-10-04&to=2026-10-10")
	if r := decodeReport(t, body); *r.metric(t, "deployment_frequency").Value != 1 || strings.Contains(body, "shop") {
		t.Fatalf("other Organization sees acme data: %s", body)
	}
}

func TestEffectivenessBaselineVersions(t *testing.T) {
	base, _ := effectivenessServer(t)
	pe := bundleClient(t, base, "platform-engineer")
	url := base + "/api/v1/effectiveness/baselines"
	first := `{"metric":"work_duration","expectedVersion":0,"valueSeconds":3600,"source":"Đo tay 10 yêu cầu tháng 9","effectiveFrom":"2026-10-01","sampleSize":10,"measuredFrom":"2026-09-01","measuredTo":"2026-09-30"}`
	code, body := effectivenessPost(t, pe, url, first)
	if code != http.StatusCreated || !strings.Contains(body, `"version":1`) || !strings.Contains(body, `"createdBy":"platform-engineer"`) {
		t.Fatalf("first version: %d %s", code, body)
	}
	if code, body := effectivenessPost(t, pe, url, first); code != http.StatusConflict || !strings.Contains(body, "STALE_VERSION") {
		t.Fatalf("stale expected version: %d %s", code, body)
	}
	for body, field := range map[string]string{
		`{"metric":"work_duration","expectedVersion":1,"valueSeconds":0,"source":"x","effectiveFrom":"2026-10-01"}`:                 "valueSeconds",
		`{"metric":"work_duration","expectedVersion":1,"valueSeconds":60,"source":"  ","effectiveFrom":"2026-10-01"}`:               "source",
		`{"metric":"work_duration","expectedVersion":1,"valueSeconds":60,"source":"x","effectiveFrom":"01/10/2026"}`:                "effectiveFrom",
		`{"metric":"work_duration","expectedVersion":1,"retired":true,"valueSeconds":60,"source":"x","effectiveFrom":"2026-10-01"}`: "valueSeconds",
		`{"metric":"work_duration","valueSeconds":60,"source":"x","effectiveFrom":"2026-10-01"}`:                                    "expectedVersion",
	} {
		if code, got := effectivenessPost(t, pe, url, body); code != http.StatusBadRequest || !strings.Contains(got, `"field":"`+field+`"`) {
			t.Fatalf("%s: %d %s", body, code, got)
		}
	}
	if code, _ := effectivenessPost(t, pe, url, `{"metric":"work_duration","expectedVersion":1,"unknown":1}`); code != http.StatusBadRequest {
		t.Fatalf("unknown field: %d", code)
	}
	if code, _ := effectivenessPost(t, pe, url, `{"metric":"work_duration","applicationKey":"foreign","expectedVersion":0,"valueSeconds":60,"source":"x","effectiveFrom":"2026-10-01"}`); code != http.StatusNotFound {
		t.Fatalf("baseline for another Organization's Application: %d", code)
	}
	// Report: median work duration is 30 minutes (40, 10, 60), baseline 60.
	_, body = effectivenessGet(t, pe, base+"/api/v1/effectiveness/report?from=2026-10-04&to=2026-10-10")
	r := decodeReport(t, body)
	diff := r.metric(t, "work_duration_vs_baseline")
	if r.Baseline == nil || r.Baseline.Version != 1 || diff.Status != "COLLECTED" || *diff.Value != 20*60 {
		t.Fatalf("baseline comparison: %+v %+v", r.Baseline, diff)
	}
	// A later version does not change a period before its effective date.
	retire := `{"metric":"work_duration","expectedVersion":1,"retired":true,"source":"Quy trình thủ công đã thay đổi","effectiveFrom":"2026-10-08"}`
	if code, body := effectivenessPost(t, pe, url, retire); code != http.StatusCreated {
		t.Fatalf("retire: %d %s", code, body)
	}
	_, body = effectivenessGet(t, pe, base+"/api/v1/effectiveness/report?from=2026-10-01&to=2026-10-07")
	if old := decodeReport(t, body); old.Baseline == nil || old.Baseline.Version != 1 {
		t.Fatalf("older period lost its baseline: %s", body)
	}
	_, body = effectivenessGet(t, pe, base+"/api/v1/effectiveness/report?from=2026-10-04&to=2026-10-10")
	if now := decodeReport(t, body); now.Baseline != nil || now.metric(t, "work_duration_vs_baseline").Status != "NO_DATA" {
		t.Fatalf("retired baseline still applies: %s", body)
	}
	code, body = effectivenessGet(t, pe, url)
	if code != http.StatusOK || !strings.Contains(body, `"currentVersion":2`) || strings.Index(body, `"version":2`) > strings.Index(body, `"version":1`) {
		t.Fatalf("history newest first: %d %s", code, body)
	}
	if _, body := effectivenessGet(t, pe, url+"?application=shop"); !strings.Contains(body, `"currentVersion":0`) {
		t.Fatalf("application scope is separate: %s", body)
	}
}
