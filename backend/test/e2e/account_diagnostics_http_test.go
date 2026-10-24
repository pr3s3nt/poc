package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"orchestrator/internal/adapters/credentialmemory"
	"orchestrator/internal/application/accountdiag"
	ct "orchestrator/internal/application/connection/connectiontest"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/domain/application"
	domain "orchestrator/internal/domain/diagnostics"
	probes "orchestrator/internal/ports/diagnostics"
	"orchestrator/internal/seed/seedtest"
)

// The probe boundary is a deterministic test double here: these tests prove the
// HTTP contract of /driver-accounts/{key}/diagnostics (gates, strict bodies,
// safe DTOs, separation from Connection verification, busy/stale/cancel). They
// are not evidence that any cluster or cloud account is ready.

type httpDiagProvider struct {
	mu      sync.Mutex
	release chan struct{}
}

func (p *httpDiagProvider) Plan(domain.Scope, *probes.Requirements) []probes.Check {
	return []probes.Check{
		{ID: "k8s.api", Group: "access"},
		{ID: "k8s.nodes", Group: "cluster", DependsOn: []string{"k8s.api"}},
		{ID: "k8s.storage.classes", Group: "storage", DependsOn: []string{"k8s.nodes"}},
	}
}
func (p *httpDiagProvider) Open(context.Context, application.Connection, domain.Scope) probes.Session {
	return &httpDiagSession{p: p}
}

type httpDiagSession struct{ p *httpDiagProvider }

const httpDiagSecret = "kubectl: Unauthorized token=SENTINEL-DO-NOT-LEAK"

func (s *httpDiagSession) Run(ctx context.Context, c probes.Check, _ map[string]domain.Finding, _ *probes.Requirements, _ probes.Recorder) domain.Finding {
	if s.p.release != nil {
		select {
		case <-s.p.release:
		case <-ctx.Done():
			return domain.Finding{}
		}
	}
	switch c.ID {
	case "k8s.nodes":
		return domain.Finding{Verdict: domain.VerdictFail, Reason: "NO_READY_NODE", Remediation: "restore-nodes", Facts: map[string]string{"nodes": "1", "nodesReady": "0", "stderr": httpDiagSecret}}
	}
	return domain.Finding{Verdict: domain.VerdictPass, Reason: "API_ANSWERED", Facts: map[string]string{"serverVersion": "v1.31.0"}}
}
func (s *httpDiagSession) Cleanup(_ context.Context, r []domain.ProbeResource, _ string) []domain.ProbeResource {
	return r
}
func (s *httpDiagSession) Close() {}

func diagnosticsServer(t *testing.T, provider probes.Provider) *httptest.Server {
	t.Helper()
	app, err := bootstrap.Build(context.Background(), bootstrap.Options{
		Seed: seedtest.Defaults(), Adapters: bootstrap.AdapterFake, KubeconfigVerifierOverride: &uploadVerifier{}, ConnectionCredentialsOverride: credentialmemory.New(),
		DiagnosticProvidersOverride: map[string]probes.Provider{"INTERNAL": provider}, DiagnosticConfigOverride: &accountdiag.Config{SweepEvery: time.Hour},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	server := httptest.NewServer(app.Server)
	t.Cleanup(server.Close)
	return server
}

func decodeMap(t *testing.T, body string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("not JSON: %s", body)
	}
	return out
}

func TestAccountDiagnosticsHTTPContract(t *testing.T) {
	provider := &httpDiagProvider{release: make(chan struct{})}
	server := diagnosticsServer(t, provider)
	pe := authenticatedClientAs(t, server.URL, "platform-engineer")
	dev := authenticatedClient(t, server.URL)
	anon := newJarClient(t)
	base := server.URL + "/api/v1/driver-accounts"

	doc := ct.Single("kind-lab", "https://lab.example:6443", accToken)
	resp, body := call(t, pe, http.MethodPost, base+"/internal", jsonBody(t, map[string]string{"name": "Lab cluster", "kubeconfig": doc, "context": "kind-lab"}))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create account: %d %s", resp.StatusCode, body)
	}
	account := decodeMap(t, body)
	key := account["key"].(string)
	version := int64(account["version"].(float64))
	if account["latestDiagnostic"] != nil {
		t.Fatalf("a new account has no diagnostic: %s", body)
	}
	diag := base + "/" + key + "/diagnostics"

	// Gates run before any body parsing or provider work.
	for name, tc := range map[string]struct{ method, path, body string }{
		"start":  {http.MethodPost, "", `{"version":1,"purpose":"GENERAL","mode":"CONDITIONS"}`},
		"list":   {http.MethodGet, "", ""},
		"report": {http.MethodGet, "/0b1c2d3e-0000-4000-8000-000000000000", ""},
		"cancel": {http.MethodPost, "/0b1c2d3e-0000-4000-8000-000000000000/cancel", "{}"},
	} {
		if r, _ := call(t, anon, tc.method, diag+tc.path, tc.body); r.StatusCode != http.StatusUnauthorized {
			t.Errorf("anonymous %s: %d", name, r.StatusCode)
		}
		if r, b := call(t, dev, tc.method, diag+tc.path, tc.body); r.StatusCode != http.StatusForbidden {
			t.Errorf("developer %s: %d %s", name, r.StatusCode, b)
		}
	}

	// Strict bodies and scope validation.
	valid := func(mutate func(map[string]any)) string {
		m := map[string]any{"version": version, "purpose": "GENERAL", "mode": "CONDITIONS"}
		if mutate != nil {
			mutate(m)
		}
		return jsonBody(t, m)
	}
	for name, tc := range map[string]struct {
		body   string
		status int
		field  string
	}{
		"unknown member":    {valid(func(m map[string]any) { m["namespace"] = "x" }), http.StatusBadRequest, ""},
		"credential member": {valid(func(m map[string]any) { m["kubeconfig"] = "x" }), http.StatusBadRequest, ""},
		"duplicate member":  {`{"version":1,"version":1,"purpose":"GENERAL","mode":"CONDITIONS"}`, http.StatusBadRequest, ""},
		"wrong type":        {valid(func(m map[string]any) { m["useCases"] = "deploy" }), http.StatusBadRequest, ""},
		"trailing":          {valid(nil) + `{}`, http.StatusBadRequest, ""},
		"bad purpose":       {valid(func(m map[string]any) { m["purpose"] = "ALL" }), http.StatusUnprocessableEntity, "purpose"},
		"general execution": {valid(func(m map[string]any) { m["mode"] = "EXECUTION" }), http.StatusUnprocessableEntity, "mode"},
		"region on k8s":     {valid(func(m map[string]any) { m["region"] = "us-east-1" }), http.StatusUnprocessableEntity, "region"},
		"general use cases": {valid(func(m map[string]any) { m["useCases"] = []string{"deploy"} }), http.StatusUnprocessableEntity, "useCases"},
		"use case empty":    {valid(func(m map[string]any) { m["purpose"] = "USE_CASE" }), http.StatusUnprocessableEntity, "useCases"},
		"use case unknown":  {valid(func(m map[string]any) { m["purpose"], m["useCases"] = "USE_CASE", []string{"teleport"} }), http.StatusUnprocessableEntity, "useCases"},
		"environment no app": {valid(func(m map[string]any) {
			m["purpose"], m["useCases"], m["environment"] = "USE_CASE", []string{"deploy"}, "staging"
		}), http.StatusUnprocessableEntity, "environment"},
		"unknown application": {valid(func(m map[string]any) {
			m["purpose"], m["useCases"], m["application"] = "USE_CASE", []string{"deploy"}, "ghost"
		}), http.StatusNotFound, ""},
		"stale version": {valid(func(m map[string]any) { m["version"] = version + 5 }), http.StatusConflict, ""},
	} {
		r, b := call(t, pe, http.MethodPost, diag, tc.body)
		if r.StatusCode != tc.status {
			t.Errorf("%s: %d %s", name, r.StatusCode, b)
		}
		if tc.field != "" && !strings.Contains(b, `"field":"`+tc.field+`"`) {
			t.Errorf("%s: field %q missing: %s", name, tc.field, b)
		}
		if strings.Contains(b, "teleport") || strings.Contains(b, "ghost") {
			t.Errorf("%s echoes the request: %s", name, b)
		}
	}
	if r, _ := call(t, pe, http.MethodPost, diag, `{"version":1,"purpose":"`+strings.Repeat("x", 17<<10)+`"}`); r.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized body: %d", r.StatusCode)
	}
	if r, _ := call(t, pe, http.MethodPost, base+"/ghost/diagnostics", valid(nil)); r.StatusCode != http.StatusNotFound {
		t.Errorf("unknown account: %d", r.StatusCode)
	}

	// A run is accepted with a persisted report and is visibly in flight.
	r, b := call(t, pe, http.MethodPost, diag, valid(nil))
	if r.StatusCode != http.StatusAccepted {
		t.Fatalf("start: %d %s", r.StatusCode, b)
	}
	started := decodeMap(t, b)
	id := started["id"].(string)
	if started["status"] != "QUEUED" && started["status"] != "RUNNING" {
		t.Fatalf("start status: %s", b)
	}
	if r, b := call(t, pe, http.MethodPost, diag, valid(nil)); r.StatusCode != http.StatusConflict || !strings.Contains(b, `"code":"BUSY"`) {
		t.Errorf("second active run: %d %s", r.StatusCode, b)
	}
	_, inflight := call(t, pe, http.MethodGet, diag+"/"+id, "")
	if m := decodeMap(t, inflight); m["ready"] != false || m["progress"].(map[string]any)["total"].(float64) != 3 {
		t.Errorf("in flight report: %s", inflight)
	}
	close(provider.release)

	var final map[string]any
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(25 * time.Millisecond) {
		_, b := call(t, pe, http.MethodGet, diag+"/"+id, "")
		final = decodeMap(t, b)
		if final["status"] == "COMPLETED" {
			break
		}
	}
	if final["status"] != "COMPLETED" {
		t.Fatalf("report never completed: %v", final)
	}
	raw, _ := json.Marshal(final)
	for _, leaked := range []string{"SENTINEL-DO-NOT-LEAK", "stderr", accToken, "owner", "lease", "heartbeat", "secretRef", "probeResources", "revision", "organization", "createdBy", "kv2://", "memory://"} {
		if strings.Contains(string(raw), leaked) {
			t.Errorf("report exposes %q: %s", leaked, raw)
		}
	}
	counts := final["counts"].(map[string]any)
	if counts["pass"].(float64) != 1 || counts["fail"].(float64) != 1 || counts["blocked"].(float64) != 1 || final["ready"] != false {
		t.Errorf("counts: %v", counts)
	}
	scope := final["scope"].(map[string]any)
	if scope["purpose"] != "GENERAL" || scope["accountKey"] != key || scope["accountVersion"].(float64) != float64(version) {
		t.Errorf("scope: %v", scope)
	}

	// Diagnostics never rewrite the Connection: same version, status and verification.
	_, detail := call(t, pe, http.MethodGet, base+"/"+key, "")
	got := decodeMap(t, detail)
	if got["version"].(float64) != float64(version) || got["status"] != account["status"] {
		t.Errorf("connection changed by a diagnostic: %s", detail)
	}
	if got["diagnosticsUnavailable"] != false {
		t.Errorf("history was readable: %s", detail)
	}
	summary, _ := got["latestDiagnostic"].(map[string]any)
	if summary == nil || summary["reportId"] != id || summary["ready"] != false || summary["stale"] != false {
		t.Errorf("latest summary: %s", detail)
	}
	_, listed := call(t, pe, http.MethodGet, base, "")
	if !strings.Contains(listed, `"latestDiagnostic":{"reportId":"`+id+`"`) {
		t.Errorf("account list lacks the latest summary: %s", listed)
	}

	// History paging and filters.
	if r, b := call(t, pe, http.MethodGet, diag+"?limit=0", ""); r.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("limit 0: %d %s", r.StatusCode, b)
	}
	if r, b := call(t, pe, http.MethodGet, diag+"?purpose=ALL", ""); r.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("bad purpose filter: %d %s", r.StatusCode, b)
	}
	if r, b := call(t, pe, http.MethodGet, diag+"?cursor=garbage", ""); r.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("bad cursor: %d %s", r.StatusCode, b)
	}
	_, page := call(t, pe, http.MethodGet, diag+"?purpose=GENERAL&limit=5", "")
	if !strings.Contains(page, id) || strings.Contains(page, `"findings"`) {
		t.Errorf("history page: %s", page)
	}
	if r, _ := call(t, pe, http.MethodGet, diag+"?purpose=USE_CASE", ""); r.StatusCode != http.StatusOK {
		t.Errorf("empty filter: %d", r.StatusCode)
	}

	// Renaming the account (a real configuration change) marks history stale.
	if r, b := call(t, pe, http.MethodPut, base+"/"+key, jsonBody(t, map[string]any{"name": "Lab renamed", "version": version})); r.StatusCode != http.StatusOK {
		t.Fatalf("rename: %d %s", r.StatusCode, b)
	}
	_, after := call(t, pe, http.MethodGet, diag+"/"+id, "")
	if m := decodeMap(t, after); m["stale"] != true || !strings.Contains(after, "ACCOUNT_CHANGED") || m["status"] != "COMPLETED" {
		t.Errorf("stale history: %s", after)
	}
	if r, b := call(t, pe, http.MethodPost, diag, valid(nil)); r.StatusCode != http.StatusConflict || !strings.Contains(b, "STALE_VERSION") {
		t.Errorf("start at the old version: %d %s", r.StatusCode, b)
	}

	// Cancel is idempotent and terminal reports ignore it.
	if r, b := call(t, pe, http.MethodPost, diag+"/"+id+"/cancel", "{}"); r.StatusCode != http.StatusOK || decodeMap(t, b)["status"] != "COMPLETED" {
		t.Errorf("cancel of a finished report: %d %s", r.StatusCode, b)
	}
	if r, _ := call(t, pe, http.MethodGet, diag+"/0b1c2d3e-0000-4000-8000-000000000000", ""); r.StatusCode != http.StatusNotFound {
		t.Errorf("unknown report: %d", r.StatusCode)
	}
	if r, _ := call(t, pe, http.MethodGet, diag+"/not-a-uuid", ""); r.StatusCode != http.StatusNotFound {
		t.Errorf("malformed report id: %d", r.StatusCode)
	}
}
