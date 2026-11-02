package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/adapters/terraform/registry/registrytest"
	"orchestrator/internal/application/authentication"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/domain/renderbundle"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/platform/password"
	"orchestrator/internal/ports/renderregistry"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

// stubRegistry records verified pins the way the real registry does after a
// successful inspection; errors are injected per URL. HTTP tests only.
type stubRegistry struct {
	store       *store.Store
	unavailable map[string]bool
	fail        map[string]error
	inspections int
}

var stubSchema = map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{
	"team":     map[string]any{"type": "string"},
	"replicas": map[string]any{"type": "integer", "minimum": 1.0, "default": 1.0},
	"strict":   map[string]any{"type": "boolean", "default": true},
}}

func (s *stubRegistry) Inspect(ctx context.Context, org string, req renderregistry.Inspection) (renderbundle.Bundle, error) {
	s.inspections++
	if err := s.fail[req.URL]; err != nil {
		return renderbundle.Bundle{}, err
	}
	revision := strings.Repeat("a", 40)
	if req.Ref != "main" {
		revision = strings.Repeat("b", 40)
	}
	return s.store.InsertRenderBundle(ctx, renderbundle.Bundle{ID: ids.New(), OrganizationKey: org,
		Source:         renderbundle.Source{Kind: "git", URL: req.URL, Path: ".", Revision: revision, ContentSHA256: strings.Repeat(revision[:1], 64)},
		EngineContract: renderbundle.EngineContract, RendererVersion: renderbundle.RendererVersion, RendererBinary: strings.Repeat("f", 64),
		Contract: renderbundle.Contract{Manifest: renderbundle.Manifest{Metadata: struct {
			Name string `yaml:"name" json:"name"`
		}{Name: "standard-workload"}}, InputsSchema: stubSchema, Defaults: map[string]any{}, InputContract: renderbundle.InputContract},
		CreatedAt: time.Now().UTC()})
}

func (s *stubRegistry) Available(_ context.Context, b renderbundle.Bundle) error {
	if s.unavailable[b.ID] {
		return renderregistry.ErrBundleUnavailable
	}
	return nil
}

func gitServer(t *testing.T) (*httptest.Server, *store.Store, *stubRegistry) {
	t.Helper()
	st := store.New()
	opts := seedtest.Defaults()
	if err := seedtest.Apply(context.Background(), st, opts); err != nil {
		t.Fatal(err)
	}
	hash, err := password.Hash(seed.FixedTestPassword)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveUserAccount(context.Background(), identity.UserAccount{ID: "other-admin", OrganizationKey: "other-org", Username: "other-admin", PasswordHash: hash, Role: identity.RoleAdmin, Status: identity.AccountActive}); err != nil {
		t.Fatal(err)
	}
	reg := &stubRegistry{store: st, unavailable: map[string]bool{}, fail: map[string]error{}}
	installed := map[string]resource.RenderBundle{"score-k8s-internal-v1": {ID: "score-k8s-internal-v1", Version: "0.15.0", Digest: "d", BinaryDigest: "b"}}
	s := NewServer(Config{RenderBundles: installed, RenderRegistry: reg, Authentication: authentication.NewService(st), Store: st, ModuleRegistry: registrytest.Open(t, st), SeedOptions: opts})
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)
	return ts, st, reg
}

func call(t *testing.T, c *http.Client, method, url, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func inspectBody(url, ref string) string {
	return `{"url":"` + url + `","path":".","ref":"` + ref + `"}`
}

const gitURL = "https://github.com/pr3s3nt/poc.git"

func workloadDefinition(bundle, inputs string) string {
	ns := `${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}`
	cluster := `${resources['k8s-cluster.internal#environments.@app.@env'].outputs.cluster}`
	extra := ""
	if inputs != "" {
		extra = `,"render_inputs":` + inputs
	}
	return `{"key":"web-git","name":"Mẫu Git","resourceType":"workload","driverType":"score-k8s","driverInputs":{"values":{"variables":{"render_bundle":"` + bundle + `","namespace":"` + ns + `","cluster":"` + cluster + `"` + extra + `}}},"criteria":[{}]}`
}

func TestInspectAndListGitRenderBundles(t *testing.T) {
	ts, st, reg := gitServer(t)
	pe := bundleClient(t, ts.URL, "platform-engineer")
	if code, _ := call(t, bundleClient(t, ts.URL, "developer"), "POST", ts.URL+"/api/v1/render-bundles/inspect", inspectBody(gitURL, "main")); code != http.StatusForbidden {
		t.Fatalf("developer inspect: %d", code)
	}
	if reg.inspections != 0 {
		t.Fatal("a forbidden request reached the registry")
	}
	code, body := call(t, pe, "POST", ts.URL+"/api/v1/render-bundles/inspect", inspectBody(gitURL, "main"))
	if code != http.StatusOK {
		t.Fatalf("inspect: %d %v", code, body)
	}
	bundle := body["renderBundle"].(map[string]any)
	id := bundle["id"].(string)
	if bundle["name"] != "standard-workload" || bundle["version"] != strings.Repeat("a", 12) || bundle["status"] != "AVAILABLE" || bundle["engineContract"] != "git-render-v1" {
		t.Fatalf("metadata: %v", bundle)
	}
	raw, _ := json.Marshal(body)
	for _, leak := range []string{"ffffffff", "rendererBinary", "cache", "organizationKey"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("inspect response leaks %q: %s", leak, raw)
		}
	}
	defs, _ := st.ListResourceDefinitions(context.Background(), seedtest.Defaults().OrganizationKey)
	for _, d := range defs {
		if d.Key == "web-git" {
			t.Fatal("inspection created a Definition")
		}
	}
	code, list := call(t, pe, "GET", ts.URL+"/api/v1/render-bundles", "")
	entries := list["renderBundles"].([]any)
	if code != http.StatusOK || len(entries) != 2 || len(entries[0].(map[string]any)) != 3 || entries[1].(map[string]any)["id"] != id {
		t.Fatalf("list: %d %v", code, list)
	}
	reg.unavailable[id] = true
	if _, list := call(t, pe, "GET", ts.URL+"/api/v1/render-bundles", ""); list["renderBundles"].([]any)[1].(map[string]any)["status"] != "UNAVAILABLE" {
		t.Fatalf("unavailable cache: %v", list)
	}
	if code, detail := call(t, pe, "GET", ts.URL+"/api/v1/render-bundles/"+id, ""); code != http.StatusOK || detail["renderBundle"].(map[string]any)["id"] != id {
		t.Fatalf("detail: %d %v", code, detail)
	}
	// Another Organization cannot read the record.
	if err := st.SaveOrganization(context.Background(), application.Organization{ID: "other-org-id", Key: "other-org", Name: "Other"}); err != nil {
		t.Fatal(err)
	}
	if code, _ := call(t, bundleClient(t, ts.URL, "other-admin"), "GET", ts.URL+"/api/v1/render-bundles/"+id, ""); code != http.StatusNotFound {
		t.Fatalf("foreign detail: %d", code)
	}
}

func TestInspectErrorsAreStableAndSafe(t *testing.T) {
	ts, _, reg := gitServer(t)
	pe := bundleClient(t, ts.URL, "platform-engineer")
	for url, want := range map[string]struct {
		err    error
		status int
		code   string
	}{
		"https://e.com/invalid.git":     {renderregistry.ErrSourceInvalid, 400, "RENDER_SOURCE_INVALID"},
		"https://e.com/bundle.git":      {renderregistry.ErrBundleInvalid, 400, "RENDER_BUNDLE_INVALID"},
		"https://e.com/contract.git":    {renderregistry.ErrContractUnsupported, 400, "RENDER_CONTRACT_UNSUPPORTED"},
		"https://e.com/down.git":        {renderregistry.ErrSourceUnavailable, 503, "RENDER_SOURCE_UNAVAILABLE"},
		"https://e.com/unavailable.git": {renderregistry.ErrBundleUnavailable, 503, "RENDER_BUNDLE_UNAVAILABLE"},
		"https://e.com/drift.git":       {renderregistry.ErrDrift, 409, "RENDER_BUNDLE_DRIFT"},
	} {
		reg.fail[url] = want.err
		code, body := call(t, pe, "POST", ts.URL+"/api/v1/render-bundles/inspect", inspectBody(url, "main"))
		if code != want.status || body["code"] != want.code || strings.Contains(body["error"].(string), "e.com") {
			t.Fatalf("%s: %d %v", url, code, body)
		}
	}
	for name, body := range map[string]string{
		"unknown field": `{"url":"x","path":".","ref":"main","digest":"override"}`,
		"client digest": `{"url":"x","ref":"main","contentSha256":"abc"}`,
		"trailing data": `{"url":"x","ref":"main"}{}`,
		"not an object": `[]`,
	} {
		if code, out := call(t, pe, "POST", ts.URL+"/api/v1/render-bundles/inspect", body); code != 400 || out["code"] != "INVALID_REQUEST" {
			t.Fatalf("%s: %d %v", name, code, out)
		}
	}
	large := `{"url":"` + strings.Repeat("a", 70<<10) + `","ref":"main"}`
	if code, _ := call(t, pe, "POST", ts.URL+"/api/v1/render-bundles/inspect", large); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized inspect: %d", code)
	}
}

func TestGitDefinitionValidationAndCAS(t *testing.T) {
	ts, st, reg := gitServer(t)
	pe := bundleClient(t, ts.URL, "platform-engineer")
	_, a := call(t, pe, "POST", ts.URL+"/api/v1/render-bundles/inspect", inspectBody(gitURL, "main"))
	_, b := call(t, pe, "POST", ts.URL+"/api/v1/render-bundles/inspect", inspectBody(gitURL, "v2"))
	idA := a["renderBundle"].(map[string]any)["id"].(string)
	idB := b["renderBundle"].(map[string]any)["id"].(string)
	for name, c := range map[string]struct {
		body, code string
	}{
		"wrong type":         {workloadDefinition(idA, `{"replicas":"2"}`), "RENDER_INPUT_INVALID"},
		"unknown input":      {workloadDefinition(idA, `{"secret-sentinel":"x"}`), "RENDER_INPUT_INVALID"},
		"reference in input": {workloadDefinition(idA, `{"team":"${resources.db.outputs.host}"}`), "RENDER_INPUT_INVALID"},
		"not an object":      {workloadDefinition(idA, `[1]`), "RENDER_INPUT_INVALID"},
		"installed + inputs": {workloadDefinition("score-k8s-internal-v1", `{}`), "RENDER_INPUT_INVALID"},
		"unknown UUID":       {workloadDefinition("33333333-3333-4333-8333-333333333333", ""), "INVALID_REQUEST"},
	} {
		code, out := call(t, pe, "POST", ts.URL+"/api/v1/resource-definitions", c.body)
		if code != 400 || out["code"] != c.code || strings.Contains(out["error"].(string), "sentinel") {
			t.Fatalf("%s: %d %v", name, code, out)
		}
	}
	reg.unavailable[idA] = true
	if code, out := call(t, pe, "POST", ts.URL+"/api/v1/resource-definitions", workloadDefinition(idA, "")); code != 503 || out["code"] != "RENDER_BUNDLE_UNAVAILABLE" {
		t.Fatalf("unavailable pin: %d %v", code, out)
	}
	delete(reg.unavailable, idA)
	code, created := call(t, pe, "POST", ts.URL+"/api/v1/resource-definitions", workloadDefinition(idA, `{"team":"payments","strict":false}`))
	if code != 201 || created["sourceFingerprint"] == "" {
		t.Fatalf("create: %d %v", code, created)
	}
	// Explicit false is kept as supplied; defaults are not persisted.
	defs, _ := st.ListResourceDefinitions(context.Background(), seedtest.Defaults().OrganizationKey)
	var saved resource.Definition
	for _, d := range defs {
		if d.Key == "web-git" {
			saved = d
		}
	}
	if got := saved.Variables()[resource.VariableRenderInputs]; !jsonEqual(got, map[string]any{"team": "payments", "strict": false}) {
		t.Fatalf("persisted inputs: %v", got)
	}
	// Re-select another verified pin with CAS; a stale version is rejected.
	update := strings.Replace(workloadDefinition(idB, `{"team":"payments"}`), `"criteria":[{}]}`, `"criteria":[{}],"version":1}`, 1)
	if code, out := call(t, pe, "PUT", ts.URL+"/api/v1/resource-definitions/web-git", update); code != 200 || out["version"] != 2.0 {
		t.Fatalf("update: %d %v", code, out)
	}
	if code, out := call(t, pe, "PUT", ts.URL+"/api/v1/resource-definitions/web-git", update); code != 409 || out["code"] != "STALE_VERSION" {
		t.Fatalf("stale update: %d %v", code, out)
	}
	// A foreign Organization's pin is not selectable.
	if err := st.SaveOrganization(context.Background(), application.Organization{ID: "other-org-id", Key: "other-org", Name: "Other"}); err != nil {
		t.Fatal(err)
	}
	foreign, _ := reg.Inspect(context.Background(), "other-org", renderregistry.Inspection{URL: gitURL, Ref: "main"})
	stale := strings.Replace(workloadDefinition(foreign.ID, ""), `"criteria":[{}]}`, `"criteria":[{}],"version":2}`, 1)
	if code, out := call(t, pe, "PUT", ts.URL+"/api/v1/resource-definitions/web-git", stale); code != 400 || out["field"] != "definition.driverInputs.values.variables.render_bundle" {
		t.Fatalf("foreign pin: %d %v", code, out)
	}
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
