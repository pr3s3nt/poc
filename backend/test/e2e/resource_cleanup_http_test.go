package e2e

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

// These tests run on a REAL disposable PostgreSQL (three roles) through the
// real HTTP server. The cleanup engine is the labelled FAKE scriptedEngine.

func TestResourceDetailIsScopedAndSafe(t *testing.T) {
	f := newRCFixture(t)
	for _, tc := range []struct {
		name   string
		client *http.Client
	}{{"developer", f.dev}, {"platform engineer", f.pe}} {
		status, raw, body := f.do(tc.client, http.MethodGet, f.resourceURL(f.root.ID), nil)
		if status != http.StatusOK {
			t.Fatalf("%s detail = %d %s", tc.name, status, raw)
		}
		assertSafe(t, tc.name+" detail", raw)
		res := body["resource"].(map[string]any)
		if res["descriptor"] != f.rootName() || res["status"] != "UNREFERENCED" || res["type"] != "postgres" || res["class"] != "default" {
			t.Fatalf("detail identity: %v", res)
		}
		eligibility := res["eligibility"].(map[string]any)
		if eligibility["eligible"] != true {
			t.Fatalf("root should be eligible: %v", eligibility)
		}
		if module := res["module"].(map[string]any); module["id"] != "fixture-module" {
			t.Fatalf("module: %v", module)
		}
	}
	// A resource with an outside consumer is shown with its consumers and is not eligible.
	_, _, bodyB := f.do(f.dev, http.MethodGet, f.resourceURL(f.b.ID), nil)
	rb := bodyB["resource"].(map[string]any)
	if rb["eligibility"].(map[string]any)["eligible"] != false || len(rb["consumers"].([]any)) != 2 {
		t.Fatalf("provider with consumers: %v", rb)
	}

	// Anonymous 401; foreign Organization, other Environment, unknown and
	// malformed IDs 404 with no hint about existence.
	if status, _, _ := f.do(http.DefaultClient, http.MethodGet, f.resourceURL(f.root.ID), nil); status != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d", status)
	}
	if status, _, _ := f.do(f.foreign, http.MethodGet, f.resourceURL(f.root.ID), nil); status != http.StatusNotFound {
		t.Fatalf("foreign organization = %d", status)
	}
	other := f.server.URL + "/api/v1/applications/" + f.appKey + "/environments/production/resources/" + f.root.ID
	if status, raw, _ := f.do(f.pe, http.MethodGet, other, nil); status != http.StatusNotFound {
		t.Fatalf("other environment = %d %s", status, raw)
	}
	for _, id := range []string{ids.New(), "not-a-uuid", "../../x"} {
		if status, _, _ := f.do(f.pe, http.MethodGet, f.resourceURL(id), nil); status != http.StatusNotFound {
			t.Fatalf("unknown id %q = %d", id, status)
		}
	}
	// The detail read never changed anything.
	if f.engine.Calls() != 0 || f.count(`SELECT count(*) FROM environment_operations WHERE kind='RESOURCE_CLEANUP'`) != 0 {
		t.Fatal("a read started work")
	}
}

func TestDeletionMutationsDenyDeveloperAndForeignBeforeAnyEffect(t *testing.T) {
	f := newRCFixture(t)
	token := f.previewToken(f.pe)
	before := f.count(`SELECT count(*) FROM environment_operations`)
	cases := []struct {
		name   string
		client *http.Client
		want   int
	}{{"developer", f.dev, http.StatusForbidden}, {"foreign", f.foreign, http.StatusNotFound}, {"anonymous", http.DefaultClient, http.StatusUnauthorized}}
	for _, tc := range cases {
		if status, raw, _ := f.preview(tc.client, f.root.ID); status != tc.want {
			t.Errorf("%s preview = %d %s", tc.name, status, raw)
		}
		if status, raw, _ := f.deletion(tc.client, token, f.rootName(), f.root.ID); status != tc.want {
			t.Errorf("%s deletion = %d %s", tc.name, status, raw)
		}
	}
	if f.engine.Calls() != 0 || f.count(`SELECT count(*) FROM environment_operations`) != before {
		t.Fatal("a denied caller reached the engine or persisted an operation")
	}
	// A Developer is also denied for a resource that does not exist (role before lookup is fine) and
	// never learns anything from the denial.
	if status, raw, _ := f.preview(f.dev, ids.New()); status != http.StatusNotFound && status != http.StatusForbidden {
		t.Fatalf("developer unknown preview = %d %s", status, raw)
	}
}

func TestDeletionPreviewShowsRootAndSafeCandidates(t *testing.T) {
	f := newRCFixture(t)
	status, raw, body := f.preview(f.pe, f.root.ID)
	if status != http.StatusOK {
		t.Fatalf("preview = %d %s", status, raw)
	}
	assertSafe(t, "preview", raw)
	pv := body["preview"].(map[string]any)
	if pv["confirmationName"] != f.rootName() {
		t.Fatalf("confirmation name: %v", pv["confirmationName"])
	}
	root := pv["root"].(map[string]any)
	if root["id"] != f.root.ID || root["selectable"] != true {
		t.Fatalf("root: %v", root)
	}
	byID := map[string]map[string]any{}
	for _, c := range pv["candidates"].([]any) {
		m := c.(map[string]any)
		byID[m["id"].(string)] = m
	}
	if len(byID) != 3 {
		t.Fatalf("candidates must be A, B and C only (platform and consumers excluded): %v", byID)
	}
	if _, ok := byID[f.plat.ID]; ok {
		t.Fatal("a platform resource is listed")
	}
	if _, ok := byID[f.x.ID]; ok {
		t.Fatal("a consumer outside the root's providers is listed")
	}
	a, b, c := byID[f.a.ID], byID[f.b.ID], byID[f.c.ID]
	if a["selectable"] != true || c["selectable"] != true {
		t.Fatalf("valid dependencies must be selectable: %v %v", a, c)
	}
	if b["selectable"] != false || !contains(b["reasons"], "HAS_CONSUMERS") {
		t.Fatalf("a dependency with a retained consumer must be locked: %v", b)
	}
	if !contains(a["requiredBy"], f.root.ID) {
		t.Fatalf("A is required by the root: %v", a)
	}

	// The token is an opaque digest binding, not a readable claim set: three
	// dot-separated parts, an expiry and a hex digest; nothing decodes into
	// private data.
	token := pv["token"].(string)
	parts := strings.Split(token, ".")
	if len(parts) != 4 || parts[0] != "rd1" || len(parts[2]) != 16 || len(parts[3]) != 64 {
		t.Fatalf("token shape: %q", token)
	}
	for _, part := range parts {
		if dec, err := base64.RawURLEncoding.DecodeString(part); err == nil {
			assertSafe(t, "decoded token part", string(dec))
		}
	}
	assertSafe(t, "token", token, f.root.StateKey, f.root.ID, f.a.StateKey)
	if status, _, _ := f.do(f.pe, http.MethodPost, f.resourceURL(f.root.ID)+"/deletion-preview", `{"extra":1}`); status != http.StatusBadRequest {
		t.Fatalf("preview with a field = %d", status)
	}
}

func TestPreviewRefusesIneligibleRootsWithAllowlistedReasons(t *testing.T) {
	f := newRCFixture(t)
	reason := func(id string) (int, string) {
		status, raw, body := f.preview(f.pe, id)
		if status == http.StatusConflict {
			if p, ok := body["params"].(map[string]any); ok {
				return status, p["reason"].(string)
			}
		}
		return status, raw
	}
	// Not UNREFERENCED (READY), platform and consumed resources are refused.
	if s, r := reason(f.x.ID); s != http.StatusConflict || r != "NOT_UNREFERENCED" {
		t.Fatalf("READY root = %d %s", s, r)
	}
	if s, r := reason(f.plat.ID); s != http.StatusConflict || (r != "PLATFORM_RESOURCE" && r != "NOT_UNREFERENCED") {
		t.Fatalf("platform root = %d %s", s, r)
	}
	if s, r := reason(f.b.ID); s != http.StatusConflict || r != "HAS_CONSUMERS" {
		t.Fatalf("consumed root = %d %s", s, r)
	}
	// A pending (draft) workload that declares the shared resource is a consumer.
	f.exec(`INSERT INTO workload_drafts(environment_id,workload_id,score_document,state) VALUES($1::uuid,'api',$2::jsonb,'PENDING_UPSERT')`, f.envID, `{"apiVersion":"score.dev/v1b1","metadata":{"name":"api"},"resources":{"db":{"type":"postgres","id":"db"}}}`)
	f.exec(`UPDATE environments SET draft_version=draft_version+1 WHERE id=$1::uuid`, f.envID)
	if s, r := reason(f.root.ID); s != http.StatusConflict || r != "HAS_CONSUMERS" {
		t.Fatalf("root consumed by a pending workload = %d %s", s, r)
	}
	// So is a workload in the current Deployment Set.
	f.exec(`DELETE FROM workload_drafts WHERE environment_id=$1::uuid`, f.envID)
	f.exec(`UPDATE environments SET draft_version=draft_version+1 WHERE id=$1::uuid`, f.envID)
	setID := ids.New()
	f.exec(`INSERT INTO deployment_sets(id,environment_id,document,document_hash,created_at) VALUES($1::uuid,$2::uuid,$3::jsonb,'h',now())`, setID, f.envID,
		`{"modules":{"api":{"profile":"humanitec/default-module","spec":{"containers":{"main":{"image":"x"}}}}},"shared":{"db":{"type":"postgres","class":"default"}}}`)
	f.exec(`UPDATE environments SET current_deployment_set_id=$2::uuid,version=version+1 WHERE id=$1::uuid`, f.envID, setID)
	if s, r := reason(f.root.ID); s != http.StatusConflict || r != "HAS_CONSUMERS" {
		t.Fatalf("root in the current set = %d %s", s, r)
	}
	// An unrelated active operation makes the Environment busy.
	f.exec(`UPDATE environments SET current_deployment_set_id=NULL WHERE id=$1::uuid`, f.envID)
	_, op := f.claim("staging")
	if status, raw, body := f.preview(f.pe, f.root.ID); status != http.StatusConflict || body["code"] != "ENVIRONMENT_BUSY" {
		t.Fatalf("busy preview = %d %s", status, raw)
	}
	f.release(op)
	if f.engine.Calls() != 0 {
		t.Fatal("a refused preview started work")
	}
}

func TestAdmissionRejectsTheWholeInvalidSetBeforeAnyEffect(t *testing.T) {
	f := newRCFixture(t)
	token := f.previewToken(f.pe)
	name := f.rootName()
	foreignEnv := ids.New()
	cases := []struct {
		name     string
		selected []string
		status   int
		code     string
		reason   string
	}{
		{"root missing", []string{f.a.ID}, http.StatusBadRequest, "INVALID_REQUEST", ""},
		{"empty", []string{}, http.StatusBadRequest, "INVALID_REQUEST", ""},
		{"duplicate id", []string{f.root.ID, f.a.ID, f.a.ID}, http.StatusBadRequest, "INVALID_REQUEST", ""},
		{"id outside the preview", []string{f.root.ID, foreignEnv}, http.StatusBadRequest, "INVALID_REQUEST", ""},
		{"consumer outside the set", []string{f.root.ID, f.b.ID}, http.StatusConflict, "CLEANUP_REFUSED", "HAS_CONSUMERS"},
		{"valid plus invalid", []string{f.root.ID, f.a.ID, f.b.ID}, http.StatusConflict, "CLEANUP_REFUSED", "HAS_CONSUMERS"},
		{"platform id", []string{f.root.ID, f.plat.ID}, http.StatusBadRequest, "INVALID_REQUEST", ""},
		{"consumer outside the roots providers", []string{f.root.ID, f.x.ID}, http.StatusBadRequest, "INVALID_REQUEST", ""},
	}
	for _, tc := range cases {
		status, raw, body := f.deletion(f.pe, token, name, tc.selected...)
		if status != tc.status || body["code"] != tc.code {
			t.Errorf("%s = %d %s", tc.name, status, raw)
			continue
		}
		if tc.reason != "" {
			if p, _ := body["params"].(map[string]any); p["reason"] != tc.reason {
				t.Errorf("%s reason: %s", tc.name, raw)
			}
		}
	}
	if f.engine.Calls() != 0 || f.count(`SELECT count(*) FROM environment_operations WHERE kind='RESOURCE_CLEANUP'`) != 0 {
		t.Fatal("an invalid set was partly admitted")
	}
	for _, in := range []tfresource.Instance{f.root, f.a, f.b, f.c} {
		if got := f.refresh(in); got.Status != in.Status || got.Version != in.Version {
			t.Fatalf("%s changed by a rejected request", in.Identity.Descriptor)
		}
	}
}

func TestAdmissionRequiresTheExactRootNameAndNeverEchoesIt(t *testing.T) {
	f := newRCFixture(t)
	token := f.previewToken(f.pe)
	name := f.rootName()
	for _, wrong := range []string{"", " " + name, name + " ", strings.ToUpper(name), "postgres.default#shared.DB", "postgres.default#shared.d", name + "\n", "wrong-" + name} {
		status, raw, body := f.deletion(f.pe, token, wrong, f.root.ID)
		if status != http.StatusBadRequest || body["code"] != "RESOURCE_CONFIRMATION_MISMATCH" {
			t.Errorf("name %q = %d %s", wrong, status, raw)
		}
		if wrong != "" && strings.Contains(raw, strings.TrimSpace(wrong)) && strings.TrimSpace(wrong) != "" {
			t.Errorf("the error echoes the submitted name: %s", raw)
		}
		if strings.Contains(raw, name) {
			t.Errorf("the error reveals the expected name: %s", raw)
		}
	}
	if f.engine.Calls() != 0 || f.count(`SELECT count(*) FROM environment_operations WHERE kind='RESOURCE_CLEANUP'`) != 0 {
		t.Fatal("a wrong name was admitted")
	}
}

func TestConfirmedDeletionRunsOnlyTheSelectedSet(t *testing.T) {
	f := newRCFixture(t)
	token := f.previewToken(f.pe)
	status, raw, body := f.deletion(f.pe, token, f.rootName(), f.root.ID, f.a.ID)
	if status != http.StatusAccepted {
		t.Fatalf("deletion = %d %s", status, raw)
	}
	assertSafe(t, "deletion response", raw)
	adm := body["cleanup"].(map[string]any)
	if adm["stage"] != "REQUESTED" || adm["operationId"] == "" {
		t.Fatalf("admission: %v", adm)
	}
	view := f.waitOperation(f.pe, adm["operationId"].(string))
	assertSafe(t, "operation view", rcJSON(view))
	if view["status"] != "SUCCEEDED" || view["allVerified"] != true || view["terminal"] != "SUCCEEDED" {
		t.Fatalf("operation: %v", view)
	}
	resources := view["resources"].([]any)
	if len(resources) != 2 {
		t.Fatalf("exactly the confirmed set is reported: %v", resources)
	}
	for _, r := range resources {
		m := r.(map[string]any)
		if m["outcome"] != "DESTROYED" || m["verified"] != true {
			t.Fatalf("resource result: %v", m)
		}
	}
	// Untouched resources stay exactly as they were.
	for _, in := range []tfresource.Instance{f.b, f.c, f.x, f.plat} {
		if got := f.refresh(in); got.Status != in.Status || got.Version != in.Version {
			t.Fatalf("%s was changed", in.Identity.Descriptor)
		}
	}
	if f.refresh(f.root).Status != tfresource.InstanceDestroyed || f.refresh(f.a).Status != tfresource.InstanceDestroyed {
		t.Fatal("the confirmed set was not destroyed")
	}
	// Persisted authorization: USER_CONFIRMED, the actor, the exact selection,
	// a token hash that is not the token, and no run authority.
	var mode, actor, scopeIDs, hash, confName string
	var runID *string
	err := f.conn().QueryRow(context.Background(), `SELECT authorization_mode,actor_ref,confirmed_scope->'selectedResourceIds',deletion_request_hash,confirmation_name,run_id FROM environment_operations WHERE id=$1::uuid`, adm["operationId"]).Scan(&mode, &actor, &scopeIDs, &hash, &confName, &runID)
	if err != nil {
		t.Fatal(err)
	}
	if mode != "USER_CONFIRMED" || actor != "platform-engineer" || runID != nil || confName != f.rootName() || hash == token || strings.Contains(hash, token) || len(hash) != 64 {
		t.Fatalf("persisted authorization: %s %s %v %s %s", mode, actor, runID, confName, hash)
	}
	if !strings.Contains(scopeIDs, f.root.ID) || !strings.Contains(scopeIDs, f.a.ID) || strings.Contains(scopeIDs, f.b.ID) || strings.Contains(scopeIDs, f.c.ID) {
		t.Fatalf("confirmed scope: %s", scopeIDs)
	}
	// The caller cannot choose or escalate the authorization mode.
	status, _, _ = f.do(f.pe, http.MethodPost, f.resourceURL(f.root.ID)+"/deletions", map[string]any{"token": token, "confirmationName": f.rootName(), "selectedResourceIds": []string{f.root.ID}, "authorizationMode": "RUN_CLEANUP", "runId": "run-1"})
	if status != http.StatusBadRequest {
		t.Fatalf("a caller-chosen authorization field = %d", status)
	}
}

func TestDeletionReplayIsIdempotentAndABoundSelectionCannotChange(t *testing.T) {
	f := newRCFixture(t)
	f.engine.gate = make(chan struct{})
	token := f.previewToken(f.pe)
	status, raw, body := f.deletion(f.pe, token, f.rootName(), f.root.ID, f.a.ID)
	if status != http.StatusAccepted {
		t.Fatalf("first = %d %s", status, raw)
	}
	first := body["cleanup"].(map[string]any)["operationId"].(string)

	// Same actor, same token, same set: the same operation, no second run.
	// Selection order does not matter.
	status, raw, body = f.deletion(f.pe, token, f.rootName(), f.a.ID, f.root.ID)
	if status != http.StatusAccepted || body["cleanup"].(map[string]any)["operationId"] != first || body["cleanup"].(map[string]any)["replayed"] != true {
		t.Fatalf("replay = %d %s", status, raw)
	}
	// A changed selection after admission is stale, never a second operation.
	status, raw, body = f.deletion(f.pe, token, f.rootName(), f.root.ID)
	if status != http.StatusConflict || body["code"] != "STALE_PREVIEW" {
		t.Fatalf("changed selection = %d %s", status, raw)
	}
	status, raw, body = f.deletion(f.pe, token, f.rootName(), f.root.ID, f.a.ID, f.c.ID)
	if status != http.StatusConflict || body["code"] != "STALE_PREVIEW" {
		t.Fatalf("grown selection = %d %s", status, raw)
	}
	// Another actor cannot use the same token to reach the operation.
	admin := f.signInAdmin()
	status, raw, body = f.deletion(admin, token, f.rootName(), f.root.ID, f.a.ID)
	if status != http.StatusConflict {
		t.Fatalf("another actor with the same token = %d %s", status, raw)
	}
	if f.count(`SELECT count(*) FROM environment_operations WHERE kind='RESOURCE_CLEANUP'`) != 1 {
		t.Fatal("a second operation was created")
	}
	close(f.engine.gate)
	f.waitOperation(f.pe, first)
	if f.engine.Calls() != 1 {
		t.Fatalf("engine runs = %d, want one", f.engine.Calls())
	}
	// Replaying after completion still returns the same operation.
	status, _, body = f.deletion(f.pe, token, f.rootName(), f.root.ID, f.a.ID)
	if status != http.StatusAccepted || body["cleanup"].(map[string]any)["operationId"] != first {
		t.Fatalf("late replay = %d", status)
	}
	if f.engine.Calls() != 1 {
		t.Fatal("a replay re-ran the cleanup")
	}
}

func TestConcurrentIdenticalRequestsAdmitOneOperation(t *testing.T) {
	f := newRCFixture(t)
	f.engine.gate = make(chan struct{})
	token := f.previewToken(f.pe)
	var wg sync.WaitGroup
	results := make([]string, 8)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, _, body := f.deletion(f.pe, token, f.rootName(), f.root.ID, f.a.ID)
			if status == http.StatusAccepted {
				results[i] = body["cleanup"].(map[string]any)["operationId"].(string)
			}
		}()
	}
	wg.Wait()
	close(f.engine.gate)
	first := ""
	for _, id := range results {
		if id == "" {
			t.Fatal("a concurrent identical request was not answered with the admitted operation")
		}
		if first == "" {
			first = id
		}
		if id != first {
			t.Fatalf("two operations were admitted: %v", results)
		}
	}
	f.waitOperation(f.pe, first)
	if f.engine.Calls() != 1 || f.count(`SELECT count(*) FROM environment_operations WHERE kind='RESOURCE_CLEANUP'`) != 1 {
		t.Fatalf("engine runs %d", f.engine.Calls())
	}
}

func TestStaleStateIsRejectedBeforeAnyEffect(t *testing.T) {
	changes := map[string]func(f *rcFixture){
		"instance version": func(f *rcFixture) {
			ctx, op := f.claim("staging")
			cur := f.refresh(f.a)
			if _, err := f.state.SetInstanceStatus(ctx, cur.ID, cur.Version, tfresource.InstanceUnreferenced); err != nil {
				f.t.Fatal(err)
			}
			f.release(op)
		},
		"new consumer edge": func(f *rcFixture) {
			ctx, op := f.claim("staging")
			extra := f.register(ctx, "redis.default#shared.late", "SHARED", tfresource.InstanceReady)
			f.consume(ctx, extra, f.a)
			f.release(op)
		},
		"draft written": func(f *rcFixture) {
			f.exec(`INSERT INTO workload_drafts(environment_id,workload_id,score_document,state) VALUES($1::uuid,'api',$2::jsonb,'PENDING_UPSERT')`, f.envID, `{"resources":{}}`)
			f.exec(`UPDATE environments SET draft_version=draft_version+1 WHERE id=$1::uuid`, f.envID)
		},
		"environment version": func(f *rcFixture) {
			f.exec(`UPDATE environments SET version=version+1 WHERE id=$1::uuid`, f.envID)
		},
		"active resource status": func(f *rcFixture) {
			// A root that stops being UNREFERENCED (referenced again) is stale.
			ctx, op := f.claim("staging")
			cur := f.refresh(f.root)
			if _, err := f.state.SetInstanceStatus(ctx, cur.ID, cur.Version, tfresource.InstanceReady); err != nil {
				f.t.Fatal(err)
			}
			f.release(op)
		},
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			f := newRCFixture(t)
			token := f.previewToken(f.pe)
			change(f)
			status, raw, body := f.deletion(f.pe, token, f.rootName(), f.root.ID, f.a.ID)
			if status != http.StatusConflict {
				t.Fatalf("after %s = %d %s", name, status, raw)
			}
			if code := body["code"]; code != "STALE_PREVIEW" && code != "CLEANUP_REFUSED" && code != "STALE_VERSION" {
				t.Fatalf("code %v", code)
			}
			if f.engine.Calls() != 0 || f.count(`SELECT count(*) FROM environment_operations WHERE kind='RESOURCE_CLEANUP'`) != 0 {
				t.Fatalf("a stale request was admitted after %s", name)
			}
			if got := f.refresh(f.a); got.Status == tfresource.InstanceDestroyed || got.Status == tfresource.InstanceDestroying {
				t.Fatal("a stale request touched a resource")
			}
		})
	}
}

func TestBusyEnvironmentRejectsAdmission(t *testing.T) {
	f := newRCFixture(t)
	token := f.previewToken(f.pe)
	_, op := f.claim("staging")
	status, raw, body := f.deletion(f.pe, token, f.rootName(), f.root.ID)
	if status != http.StatusConflict || body["code"] != "ENVIRONMENT_BUSY" {
		t.Fatalf("busy admission = %d %s", status, raw)
	}
	f.release(op)
	if f.engine.Calls() != 0 {
		t.Fatal("engine ran while the Environment was busy")
	}
}

// TestAdmissionRacesAgainstConcurrentConsumerWrites races the admission with
// a deployment that claims the Environment and records a new consumer of a
// selected dependency. Exactly one side may win; when the consumer edge wins,
// nothing is admitted and the engine never runs.
func TestAdmissionRacesAgainstConcurrentConsumerWrites(t *testing.T) {
	for round := 0; round < 12; round++ {
		f := newRCFixture(t)
		f.engine.gate = make(chan struct{})
		token := f.previewToken(f.pe)
		var wg sync.WaitGroup
		start := make(chan struct{})
		var admitted bool
		var consumerWon bool
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			status, _, _ := f.deletion(f.pe, token, f.rootName(), f.root.ID, f.a.ID)
			admitted = status == http.StatusAccepted
		}()
		go func() {
			defer wg.Done()
			<-start
			op, err := f.app.Store.ClaimEnvironment(context.Background(), persistence.OperationClaim{ApplicationKey: f.appKey, EnvironmentKey: "staging", Owner: "racer", Kind: environment.OpDeploy, Deadline: time.Now().Add(time.Hour)})
			if err != nil {
				return
			}
			ctx := persistence.WithOwner(context.Background(), persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence})
			extra := f.register(ctx, "redis.default#shared.race", "SHARED", tfresource.InstanceReady)
			f.consume(ctx, extra, f.a)
			consumerWon = true
			_ = f.app.Store.ReleaseOperation(context.Background(), persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence}, environment.OpSucceeded, "")
		}()
		close(start)
		wg.Wait()
		close(f.engine.gate)
		f.app.ResourceCleanup.Wait()
		if admitted && consumerWon {
			// Only valid if the cleanup finished before the deployment claimed
			// the Environment; then the edge came after the destroy, not before.
			if f.refresh(f.a).Status != tfresource.InstanceDestroyed {
				t.Fatalf("round %d: a consumer was added to a dependency that was then admitted for deletion", round)
			}
		}
		if !admitted && f.engine.Calls() != 0 {
			t.Fatalf("round %d: the engine ran for a rejected request", round)
		}
		if admitted && f.engine.Calls() != 1 {
			t.Fatalf("round %d: admitted without exactly one run", round)
		}
		if !admitted && f.refresh(f.a).Status == tfresource.InstanceDestroyed {
			t.Fatalf("round %d: a rejected request destroyed a resource", round)
		}
	}
}

func TestEnginePartialFailureKeepsProvidersAndReportsRemainingTruthfully(t *testing.T) {
	f := newRCFixture(t)
	f.engine.script[f.root.Identity.Descriptor] = "fail"
	token := f.previewToken(f.pe)
	_, raw, body := f.deletion(f.pe, token, f.rootName(), f.root.ID, f.a.ID)
	view := f.waitOperation(f.pe, body["cleanup"].(map[string]any)["operationId"].(string))
	assertSafe(t, "failed operation", rcJSON(view))
	if view["status"] != "FAILED" || view["allVerified"] == true || view["terminal"] != "FAILED" {
		t.Fatalf("a failed cleanup was reported as done: %v (%s)", view, raw)
	}
	if len(view["remainingIds"].([]any)) == 0 {
		t.Fatalf("remaining IDs missing: %v", view)
	}
	if strings.Contains(rcJSON(view), "<script>") {
		t.Fatal("an unsafe remaining ID was forwarded")
	}
	for _, r := range view["resources"].([]any) {
		m := r.(map[string]any)
		if m["verified"] == true {
			t.Fatalf("an unverified resource was reported verified: %v", m)
		}
	}
	if got := f.refresh(f.a); got.Status == tfresource.InstanceDestroyed {
		t.Fatal("the provider of a failed consumer was destroyed")
	}
}

func TestRemainingUnknownNeverImpliesAbsence(t *testing.T) {
	f := newRCFixture(t)
	f.engine.script[f.root.Identity.Descriptor] = "unknown"
	token := f.previewToken(f.pe)
	_, _, body := f.deletion(f.pe, token, f.rootName(), f.root.ID)
	view := f.waitOperation(f.pe, body["cleanup"].(map[string]any)["operationId"].(string))
	if view["allVerified"] == true || view["status"] == "SUCCEEDED" {
		t.Fatalf("unknown remaining objects were treated as absent: %v", view)
	}
	rootRes := view["resources"].([]any)[0].(map[string]any)
	if rootRes["remainingUnknown"] != true || rootRes["verified"] == true {
		t.Fatalf("root result: %v", rootRes)
	}
}

func TestUnknownOutcomeHoldsTheEnvironment(t *testing.T) {
	f := newRCFixture(t)
	f.engine.script[f.root.Identity.Descriptor] = "hold"
	token := f.previewToken(f.pe)
	_, _, body := f.deletion(f.pe, token, f.rootName(), f.root.ID, f.a.ID)
	view := f.waitOperation(f.pe, body["cleanup"].(map[string]any)["operationId"].(string))
	if view["status"] != "INTERRUPTED" || view["terminal"] != "NEEDS_ATTENTION" || view["allVerified"] == true {
		t.Fatalf("operation: %v", view)
	}
	// The Environment stays held: nothing else may start, and nothing replays.
	if status, _, b := f.preview(f.pe, f.root.ID); status != http.StatusConflict || b["code"] != "ENVIRONMENT_BUSY" {
		t.Fatalf("held environment preview = %d %v", status, b)
	}
	if f.engine.Calls() != 1 {
		t.Fatalf("engine runs = %d", f.engine.Calls())
	}
}

func TestCleanupProjectionAllowlistsUnknownStageErrorAndReason(t *testing.T) {
	f := newRCFixture(t)
	token := f.previewToken(f.pe)
	_, _, body := f.deletion(f.pe, token, f.rootName(), f.root.ID, f.a.ID)
	id := body["cleanup"].(map[string]any)["operationId"].(string)
	f.waitOperation(f.pe, id)
	hostile := `{"kind":"RESOURCE_CLEANUP","terminal":"SECRET-TERMINAL","current":"` + f.root.ID + `","results":[{"id":"` + f.root.ID + `","destroyed":false,"remaining":["aws_vpc.main=vpc-1","/home/secret/path","token=abc def","` + strings.Repeat("x", 300) + `"],"remainingUnknown":false,"code":"RAW PROVIDER ERROR: password=hunter2"},{"id":"` + ids.New() + `","destroyed":true,"remaining":[],"code":""}]}`
	f.exec(`UPDATE environment_operations SET stage='HOSTILE-STAGE hunter2',status='FAILED',failure='dial tcp 10.0.0.1: password=hunter2',detail=$2::jsonb WHERE id=$1::uuid`, id, hostile)
	// Make the instances look undestroyed again so the stored result is what is shown.
	status, raw, view := f.do(f.dev, http.MethodGet, f.base()+"/resource-cleanups/"+id, nil)
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	for _, bad := range []string{"hunter2", "HOSTILE", "SECRET-TERMINAL", "/home/secret", "RAW PROVIDER", "10.0.0.1", "abc def", strings.Repeat("x", 300)} {
		if strings.Contains(raw, bad) {
			t.Errorf("projection forwards %q: %s", bad, raw)
		}
	}
	cv := view["cleanup"].(map[string]any)
	if cv["stage"] != "UNKNOWN" || cv["failureCode"] != "CLEANUP_FAILED" || cv["terminal"] != nil && cv["terminal"] != "" {
		t.Fatalf("unknown values must map to generic markers: %v", cv)
	}
}

func TestCleanupOperationIsScoped(t *testing.T) {
	f := newRCFixture(t)
	token := f.previewToken(f.pe)
	_, _, body := f.deletion(f.pe, token, f.rootName(), f.root.ID)
	id := body["cleanup"].(map[string]any)["operationId"].(string)
	f.waitOperation(f.pe, id)
	read := func(client *http.Client, env string) int {
		status, _, _ := f.do(client, http.MethodGet, f.server.URL+"/api/v1/applications/"+f.appKey+"/environments/"+env+"/resource-cleanups/"+id, nil)
		return status
	}
	if read(f.dev, "staging") != http.StatusOK {
		t.Fatal("a Developer may read safe progress")
	}
	if read(f.foreign, "staging") != http.StatusNotFound || read(f.pe, "production") != http.StatusNotFound {
		t.Fatal("foreign scope must be 404")
	}
	if status, _, _ := f.do(f.pe, http.MethodGet, f.base()+"/resource-cleanups/"+ids.New(), nil); status != http.StatusNotFound {
		t.Fatalf("unknown operation = %d", status)
	}
	if status, _, _ := f.do(http.DefaultClient, http.MethodGet, f.base()+"/resource-cleanups/"+id, nil); status != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d", status)
	}
}

func TestActiveResourceFollowsTheStateMachine(t *testing.T) {
	f := newRCFixture(t)
	root := f.linkActive(f.root, "UNREFERENCED")
	f.linkActive(f.a, "UNREFERENCED")
	token := f.previewToken(f.pe)
	_, _, body := f.deletion(f.pe, token, f.rootName(), f.root.ID, f.a.ID)
	view := f.waitOperation(f.pe, body["cleanup"].(map[string]any)["operationId"].(string))
	if view["status"] != "SUCCEEDED" {
		t.Fatalf("%v", view)
	}
	if got := f.activeStatus(root); got != "DESTROYED" {
		t.Fatalf("active resource after verified absence = %s", got)
	}
	// A failed cleanup returns the resource to UNREFERENCED.
	f2 := newRCFixture(t)
	f2.engine.script[f2.root.Identity.Descriptor] = "fail"
	active := f2.linkActive(f2.root, "UNREFERENCED")
	token = f2.previewToken(f2.pe)
	_, _, body = f2.deletion(f2.pe, token, f2.rootName(), f2.root.ID)
	f2.waitOperation(f2.pe, body["cleanup"].(map[string]any)["operationId"].(string))
	if got := f2.activeStatus(active); got != "UNREFERENCED" {
		t.Fatalf("active resource after a failed cleanup = %s", got)
	}
	// An Active Resource that is still READY keeps the instance ineligible.
	f3 := newRCFixture(t)
	f3.linkActive(f3.root, "READY")
	if status, _, b := f3.preview(f3.pe, f3.root.ID); status != http.StatusConflict || b["params"].(map[string]any)["reason"] != "NOT_UNREFERENCED" {
		t.Fatalf("READY active resource preview = %d %v", status, b)
	}
}

func contains(list any, want string) bool {
	items, _ := list.([]any)
	for _, v := range items {
		if v == want {
			return true
		}
	}
	return false
}

// TestConsoleLoginCannotReachPrivateColumnsAndOperationRecordHoldsNone proves
// at the database, with the real Console login, that private instance columns
// are denied and that the admitted operation record the Console can read
// carries only digests, never an original target reference.
func TestConsoleLoginCannotReachPrivateColumnsAndOperationRecordHoldsNone(t *testing.T) {
	f := newRCFixture(t)
	token := f.previewToken(f.pe)
	_, _, body := f.deletion(f.pe, token, f.rootName(), f.root.ID, f.a.ID)
	id := body["cleanup"].(map[string]any)["operationId"].(string)
	f.waitOperation(f.pe, id)

	console, err := pgx.Connect(context.Background(), f.db.Console)
	if err != nil {
		t.Fatal(err)
	}
	defer console.Close(context.Background())
	for _, column := range []string{"target_ref", "physical_intent"} {
		var v string
		if err := console.QueryRow(context.Background(), `SELECT `+column+`::text FROM terraform_resource_instances LIMIT 1`).Scan(&v); err == nil {
			t.Errorf("the Console login read %s", column)
		}
	}
	for _, column := range []string{"module_snapshot", "target_snapshot", "observed_physical_ids", "recovery_handle"} {
		var v string
		if err := console.QueryRow(context.Background(), `SELECT `+column+`::text FROM terraform_resource_attempts LIMIT 1`).Scan(&v); err == nil {
			t.Errorf("the Console login read attempt column %s", column)
		}
	}
	if err := console.QueryRow(context.Background(), `SELECT ciphertext::text FROM terraform_states LIMIT 1`).Scan(new(string)); err == nil {
		t.Error("the Console login read state ciphertext")
	}
	var record string
	if err := console.QueryRow(context.Background(), `SELECT confirmed_scope::text||scope_snapshot::text||detail::text||failure||stage FROM environment_operations WHERE id=$1::uuid`, id).Scan(&record); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{privateTarget, privateIntent, privateRecov, "targetRef"} {
		if strings.Contains(record, bad) {
			t.Errorf("the Console-readable operation record holds %q: %.300s", bad, record)
		}
	}
	if !strings.Contains(record, "targetDigest") {
		t.Fatalf("the pinned original target is not bound by a digest: %.300s", record)
	}
}

// TestAdmissionRacesAgainstConcurrentDraftWrites races the admission with a
// pending-workload write that declares the root's shared resource. A draft can
// only be saved while no operation holds the Environment, so it cannot slip in
// between the admission check and the claim: exactly one side wins.
func TestAdmissionRacesAgainstConcurrentDraftWrites(t *testing.T) {
	for round := 0; round < 12; round++ {
		f := newRCFixture(t)
		f.engine.gate = make(chan struct{})
		token := f.previewToken(f.pe)
		env, err := f.app.Store.GetEnvironment(context.Background(), f.appKey, "staging")
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		start := make(chan struct{})
		var admitted, saved bool
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			status, _, _ := f.deletion(f.pe, token, f.rootName(), f.root.ID, f.a.ID)
			admitted = status == http.StatusAccepted
		}()
		go func() {
			defer wg.Done()
			<-start
			err := f.app.Store.SaveWorkloadDraft(context.Background(), env.DraftVersion, environment.WorkloadDraft{ApplicationKey: f.appKey, EnvironmentKey: "staging", WorkloadID: "api", State: environment.DraftUpsert,
				Score: map[string]any{"resources": map[string]any{"db": map[string]any{"type": "postgres", "id": "db"}}}})
			saved = err == nil
		}()
		close(start)
		wg.Wait()
		close(f.engine.gate)
		f.app.ResourceCleanup.Wait()
		if admitted && saved {
			t.Fatalf("round %d: a pending consumer was saved while the deletion held the Environment", round)
		}
		if !admitted && f.engine.Calls() != 0 {
			t.Fatalf("round %d: the engine ran for a rejected request", round)
		}
		if saved && f.refresh(f.root).Status == tfresource.InstanceDestroyed {
			t.Fatalf("round %d: the shared resource was destroyed after a workload started to need it", round)
		}
	}
}

func TestResourceListIsScopedSafeAndOmitsPlatform(t *testing.T) {
	f := newRCFixture(t)
	list := f.base() + "/resources"
	for _, client := range []*http.Client{f.dev, f.pe} {
		status, raw, body := f.do(client, http.MethodGet, list, nil)
		if status != http.StatusOK {
			t.Fatalf("list = %d %s", status, raw)
		}
		assertSafe(t, "resource list", raw)
		got := map[string]string{}
		for _, r := range body["resources"].([]any) {
			m := r.(map[string]any)
			got[m["id"].(string)] = m["status"].(string)
		}
		if len(got) != 5 || got[f.root.ID] != "UNREFERENCED" || got[f.x.ID] != "READY" {
			t.Fatalf("unexpected list: %v", got)
		}
		if _, ok := got[f.plat.ID]; ok {
			t.Fatal("a platform resource is listed")
		}
	}
	if status, _, _ := f.do(f.foreign, http.MethodGet, list, nil); status != http.StatusNotFound {
		t.Fatalf("foreign list = %d", status)
	}
	if status, _, _ := f.do(http.DefaultClient, http.MethodGet, list, nil); status != http.StatusUnauthorized {
		t.Fatalf("anonymous list = %d", status)
	}
	other := f.server.URL + "/api/v1/applications/" + f.appKey + "/environments/production/resources"
	_, _, body := f.do(f.pe, http.MethodGet, other, nil)
	if len(body["resources"].([]any)) != 0 {
		t.Fatal("another Environment's list is not empty")
	}
}

// TestTokenExpiryIsBoundToTheDigest: the expiry is an input of the opaque
// digest, so a client that rewrites it to extend a token gets a stale preview,
// never an admission.
func TestTokenExpiryIsBoundToTheDigest(t *testing.T) {
	f := newRCFixture(t)
	token := f.previewToken(f.pe)
	parts := strings.Split(token, ".")
	if len(parts) != 4 {
		t.Fatalf("token %q", token)
	}
	for _, rewritten := range []string{"rd1.4999999999." + parts[2] + "." + parts[3], "rd1.1." + parts[2] + "." + parts[3], "rd1.4102444800." + parts[2] + "." + parts[3]} {
		status, raw, body := f.deletion(f.pe, rewritten, f.rootName(), f.root.ID, f.a.ID)
		if status != http.StatusConflict || body["code"] != "STALE_PREVIEW" {
			t.Fatalf("a token with a rewritten expiry = %d %s", status, raw)
		}
	}
	if f.engine.Calls() != 0 || f.count(`SELECT count(*) FROM environment_operations WHERE kind='RESOURCE_CLEANUP'`) != 0 {
		t.Fatal("a rewritten token was admitted")
	}
	// The untouched token is still accepted.
	if status, raw, _ := f.deletion(f.pe, token, f.rootName(), f.root.ID, f.a.ID); status != http.StatusAccepted {
		t.Fatalf("the issued token = %d %s", status, raw)
	}
	f.app.ResourceCleanup.Wait()
}

// TestDeveloperIsDeniedBeforeBodyOrPlanning: the role gate follows the session
// scope immediately, so malformed bodies and unknown roots are 403, not 400/404.
func TestDeveloperIsDeniedBeforeBodyOrPlanning(t *testing.T) {
	f := newRCFixture(t)
	for name, body := range map[string]any{"malformed": "{not json", "empty": "", "unknown field": `{"extra":1}`, "array": "[]"} {
		for _, suffix := range []string{"/deletion-preview", "/deletions"} {
			if status, raw, _ := f.do(f.dev, http.MethodPost, f.resourceURL(f.root.ID)+suffix, body); status != http.StatusForbidden {
				t.Errorf("developer %s %s = %d %s", name, suffix, status, raw)
			}
			if status, raw, _ := f.do(f.dev, http.MethodPost, f.resourceURL(ids.New())+suffix, body); status != http.StatusForbidden {
				t.Errorf("developer %s %s on an unknown root = %d %s", name, suffix, status, raw)
			}
		}
	}
	if f.engine.Calls() != 0 {
		t.Fatal("a Developer reached the engine")
	}
}

// TestOldFailedOperationKeepsItsOwnResultAfterALaterCleanup: the progress of a
// historical operation is its own persisted result; a later operation that
// destroyed the same resource must not erase its remaining objects.
func TestOldFailedOperationKeepsItsOwnResultAfterALaterCleanup(t *testing.T) {
	f := newRCFixture(t)
	f.engine.script[f.root.Identity.Descriptor] = "fail"
	token := f.previewToken(f.pe)
	_, _, body := f.deletion(f.pe, token, f.rootName(), f.root.ID)
	oldID := body["cleanup"].(map[string]any)["operationId"].(string)
	oldView := f.waitOperation(f.pe, oldID)
	if oldView["status"] != "FAILED" || len(oldView["remainingIds"].([]any)) == 0 {
		t.Fatalf("setup: %v", oldView)
	}
	delete(f.engine.script, f.root.Identity.Descriptor)
	token2 := f.previewToken(f.pe)
	_, _, body = f.deletion(f.pe, token2, f.rootName(), f.root.ID)
	newID := body["cleanup"].(map[string]any)["operationId"].(string)
	newView := f.waitOperation(f.pe, newID)
	if newView["status"] != "SUCCEEDED" || newView["allVerified"] != true {
		t.Fatalf("the later cleanup: %v", newView)
	}
	if f.refresh(f.root).Status != tfresource.InstanceDestroyed {
		t.Fatal("setup: the resource is not destroyed now")
	}
	_, _, again := f.do(f.pe, http.MethodGet, f.base()+"/resource-cleanups/"+oldID, nil)
	old := again["cleanup"].(map[string]any)
	if old["status"] != "FAILED" || old["terminal"] != "FAILED" || old["allVerified"] == true {
		t.Fatalf("the old operation was rewritten by the later one: %v", old)
	}
	resources := old["resources"].([]any)
	if len(resources) != 1 {
		t.Fatalf("resources: %v", resources)
	}
	item := resources[0].(map[string]any)
	if item["verified"] == true || item["outcome"] != "REMAINING" || item["currentStatus"] != "DESTROYED" {
		t.Fatalf("the old result must stay unverified while the current status is DESTROYED: %v", item)
	}
	if len(item["remaining"].([]any)) == 0 || len(old["remainingIds"].([]any)) == 0 {
		t.Fatalf("the old remaining IDs were erased: %v", old)
	}
	if old["stage"] == "UNKNOWN" || old["stage"] == "" {
		t.Fatalf("the old stage was lost: %v", old)
	}
	// And the new one is verified by its own proof.
	if n := newView["resources"].([]any)[0].(map[string]any); n["verified"] != true {
		t.Fatalf("new result: %v", n)
	}
}

func TestMissingInfrastructureIsIneligibleAndUnavailableNotEligible(t *testing.T) {
	f := newRCFixture(t, withoutEngine)
	_, raw, body := f.do(f.pe, http.MethodGet, f.resourceURL(f.root.ID), nil)
	res := body["resource"].(map[string]any)
	elig := res["eligibility"].(map[string]any)
	if elig["eligible"] != false || !contains(elig["reasons"], "INFRASTRUCTURE_UNAVAILABLE") {
		t.Fatalf("detail without the means to delete is eligible: %s", raw)
	}
	// Reads keep working; the mutations are unavailable (503), after the role gate.
	if status, _, _ := f.preview(f.pe, f.root.ID); status != http.StatusServiceUnavailable {
		t.Fatalf("preview = %d", status)
	}
	if status, _, _ := f.deletion(f.pe, "rd1.4102444800."+strings.Repeat("b", 16)+"."+strings.Repeat("a", 64), f.rootName(), f.root.ID); status != http.StatusServiceUnavailable {
		t.Fatalf("deletion = %d", status)
	}
	if status, _, _ := f.preview(f.dev, f.root.ID); status != http.StatusForbidden {
		t.Fatalf("developer preview = %d", status)
	}
	if status, _, _ := f.do(f.dev, http.MethodGet, f.base()+"/resources", nil); status != http.StatusOK {
		t.Fatalf("list = %d", status)
	}
}

// TestDetailShowsTheOriginalTargetNotTheCurrentOne: the safe target comes from
// the persisted run that last executed the resource, in the resource's own
// generation, never from the Environment's current binding.
func TestDetailShowsTheOriginalTargetNotTheCurrentOne(t *testing.T) {
	f := newRCFixture(t)
	// The Environment is bound to a different Connection now.
	active := f.linkActive(f.root, "UNREFERENCED")
	dep := f.linkDeployment(active, "aws-original", "Original AWS", 0)
	// The Connection allows several regions and its configured one is only the
	// default (us-east-1); the run was planned for eu-west-1.
	f.savePlan(dep, "aws-original", "eu-west-1", 0)
	read := func(id string) map[string]any {
		_, raw, body := f.do(f.dev, http.MethodGet, f.resourceURL(id), nil)
		assertSafe(t, "detail", raw)
		return body["resource"].(map[string]any)["target"].(map[string]any)
	}
	target := read(f.root.ID)
	if target["known"] != true || target["connectionKey"] != "aws-original" || target["connectionName"] != "Original AWS" || target["profile"] != "aws-eks" || target["accountId"] != "123456789012" || target["region"] != "eu-west-1" {
		t.Fatalf("original target: %v", target)
	}
	if _, leaked := target["privateNote"]; leaked || strings.Contains(rcJSON(target), privateTarget) {
		t.Fatalf("a private Connection value leaked: %v", target)
	}
	// The Environment moves to other regions after the run: the original region stays.
	f.exec(`UPDATE environments SET connection_id=(SELECT id FROM connections WHERE connection_key='aws-original'),execution_profile='aws-eks',runtime_status='READY',region='ap-southeast-1' WHERE id=$1::uuid`, f.envID)
	if again := read(f.root.ID); again["region"] != "eu-west-1" {
		t.Fatalf("a later Environment region changed the original region: %v", again)
	}
	f.exec(`UPDATE environments SET connection_id=(SELECT id FROM connections WHERE connection_key='aws-original'),execution_profile='aws-eks',runtime_status='READY',region='us-east-1' WHERE id=$1::uuid`, f.envID)
	if again := read(f.root.ID); again["region"] != "eu-west-1" {
		t.Fatalf("the Connection default region replaced the planned one: %v", again)
	}
	// Plan evidence that does not match the run (other Connection or other
	// generation) or is missing leaves the region unknown; the rest stays known.
	for name, setup := range map[string]func(){
		"another connection": func() { f.savePlan(dep, "other-connection", "eu-west-1", 0) },
		"another generation": func() { f.savePlan(dep, "aws-original", "eu-west-1", 3) },
		"an invalid region":  func() { f.savePlan(dep, "aws-original", "../etc", 0) },
	} {
		setup()
		got := read(f.root.ID)
		if got["known"] != true || got["connectionKey"] != "aws-original" {
			t.Fatalf("%s: the target must stay known: %v", name, got)
		}
		if region, present := got["region"]; present && region != "" {
			t.Fatalf("%s: the region must be unknown, got %v", name, region)
		}
	}
	// Malformed stored generations fail closed against a run of generation 0: a
	// fraction that truncates to 0 (or any other malformed value) never matches by cast.
	for name, generation := range map[string]string{"fractional": `0.5`, "tiny fraction": `1e-300`, "negative fraction": `-0.5`, "negative": `-1`, "huge": `1e30`, "string": `"0"`, "boolean": `false`, "object": `{}`, "array": `[0]`} {
		f.savePlanRaw(dep, `{"connectionKey":"aws-original","region":"eu-west-1","generation":`+generation+`}`)
		got := read(f.root.ID)
		if got["known"] != true {
			t.Fatalf("%s: the target must stay known: %v", name, got)
		}
		if region, present := got["region"]; present && region != "" {
			t.Fatalf("a %s stored generation must leave the region unknown, got %v", name, region)
		}
	}
	f.savePlanRaw(dep, `{"connectionKey":"aws-original","region":"eu-west-1","generation":0}`)
	if got := read(f.root.ID); got["region"] != "eu-west-1" {
		t.Fatalf("a well-formed generation 0 must match: %v", got)
	}
	f.savePlan(dep, "aws-original", "eu-west-1", 0)
	// No Active Resource, or a run of another generation: unknown, never the Environment's.
	if t0 := read(f.a.ID); t0["known"] != false || t0["connectionKey"] != nil {
		t.Fatalf("a resource without a persisted run must have an unknown target: %v", t0)
	}
	f.exec(`UPDATE deployments SET target_generation=7 WHERE connection_key='aws-original'`)
	if t1 := read(f.root.ID); t1["known"] != false || t1["connectionKey"] != nil {
		t.Fatalf("a run of another generation must not identify the target: %v", t1)
	}
}

// UC-03 V02-09: an AWS node ran under the Driver Account and region its
// Definition pinned in that run's immutable plan; the safe original target shows
// that account, not the Environment's Connection of the same Deployment.
func TestDetailShowsTheNodeDriverAccountPinFromTheRunPlan(t *testing.T) {
	f := newRCFixture(t)
	active := f.linkActive(f.root, "UNREFERENCED")
	dep := f.linkDeployment(active, "aws-original", "Original AWS", 0)
	f.exec(`INSERT INTO connections(id,organization_id,connection_key,name,kind,authentication_type,config,secret_ref,status,verification) SELECT gen_random_uuid(),o.id,'aws-node','Node AWS','AWS','AWS_ACCESS_KEY','{"accountId":"210987654321","region":"eu-west-1"}'::jsonb,'vault:private/ref','READY','{}'::jsonb FROM organizations o WHERE o.organization_key=$1 ON CONFLICT DO NOTHING`, f.orgKey)
	doc, err := json.Marshal(map[string]any{
		"target": map[string]any{"connectionKey": "aws-original", "executionProfile": "aws-eks", "region": "us-east-1"},
		"terraform": []any{
			map[string]any{"descriptor": "other.default#shared.x", "providerBindings": []any{map[string]any{"kind": "aws", "connectionKey": "aws-original", "region": "us-east-1"}}},
			map[string]any{"descriptor": f.root.Identity.Descriptor, "providerBindings": []any{map[string]any{"kind": "aws", "target": "account", "connectionKey": "aws-node", "region": "ap-southeast-1", "accountVersion": 3}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.exec(`INSERT INTO deployment_plans(deployment_id,document,plan_hash) VALUES($1::uuid,$2::jsonb,'plan-hash') ON CONFLICT(deployment_id) DO UPDATE SET document=EXCLUDED.document`, dep, string(doc))
	_, raw, body := f.do(f.dev, http.MethodGet, f.resourceURL(f.root.ID), nil)
	assertSafe(t, "detail", raw)
	target := body["resource"].(map[string]any)["target"].(map[string]any)
	if target["known"] != true || target["connectionKey"] != "aws-node" || target["connectionName"] != "Node AWS" || target["accountId"] != "210987654321" || target["region"] != "ap-southeast-1" {
		t.Fatalf("the node pin must identify the original target: %v", target)
	}
}

// UC-03 V02-09: an AWS node whose per-node pin is missing or malformed in the
// run plan has an unknown original target; the Environment's Connection and
// region of the same Deployment are never guessed for it.
func TestDetailDoesNotGuessTheEnvironmentAccountForAnUnprovenAWSNodePin(t *testing.T) {
	f := newRCFixture(t)
	active := f.linkActive(f.root, "UNREFERENCED")
	dep := f.linkDeployment(active, "aws-original", "Original AWS", 0)
	f.exec(`INSERT INTO connections(id,organization_id,connection_key,name,kind,authentication_type,config,secret_ref,status,verification) SELECT gen_random_uuid(),o.id,'aws-node','Node AWS','AWS','AWS_ACCESS_KEY','{"accountId":"210987654321","region":"eu-west-1"}'::jsonb,'vault:private/ref','READY','{}'::jsonb FROM organizations o WHERE o.organization_key=$1 ON CONFLICT DO NOTHING`, f.orgKey)
	target := map[string]any{"connectionKey": "aws-original", "executionProfile": "aws-eks", "region": "us-east-1"}
	descriptor := f.root.Identity.Descriptor
	// The whole original plan is missing, unreadable or empty: AWS-vs-legacy cannot be proven, so the
	// Environment's account (aws-original) is never named for the node; an invalid pinned region is not a known target either.
	for name, setup := range map[string]func(){
		"no plan row": func() { f.exec(`DELETE FROM deployment_plans WHERE deployment_id=$1::uuid`, dep) },
		"unparseable plan": func() {
			f.exec(`INSERT INTO deployment_plans(deployment_id,document,plan_hash) VALUES($1::uuid,'[]'::jsonb,'plan-hash') ON CONFLICT(deployment_id) DO UPDATE SET document=EXCLUDED.document`, dep)
		},
		"empty plan": func() {
			f.exec(`INSERT INTO deployment_plans(deployment_id,document,plan_hash) VALUES($1::uuid,'{}'::jsonb,'plan-hash') ON CONFLICT(deployment_id) DO UPDATE SET document=EXCLUDED.document`, dep)
		},
		"no target section": func() {
			f.exec(`INSERT INTO deployment_plans(deployment_id,document,plan_hash) VALUES($1::uuid,'{"matches":{}}'::jsonb,'plan-hash') ON CONFLICT(deployment_id) DO UPDATE SET document=EXCLUDED.document`, dep)
		},
		"invalid pin region": func() {
			raw, _ := json.Marshal(map[string]any{"target": target, "terraform": []any{map[string]any{"descriptor": descriptor, "providerBindings": []any{map[string]any{"kind": "aws", "target": "account", "connectionKey": "aws-node", "region": "../etc"}}}}})
			f.exec(`INSERT INTO deployment_plans(deployment_id,document,plan_hash) VALUES($1::uuid,$2::jsonb,'plan-hash') ON CONFLICT(deployment_id) DO UPDATE SET document=EXCLUDED.document`, dep, string(raw))
		},
	} {
		setup()
		_, body, parsed := f.do(f.dev, http.MethodGet, f.resourceURL(f.root.ID), nil)
		assertSafe(t, name, body)
		got := parsed["resource"].(map[string]any)["target"].(map[string]any)
		if got["known"] != false || got["connectionKey"] != nil || got["accountId"] != nil || got["region"] != nil {
			t.Fatalf("%s: an unprovable original target must be unknown: %v", name, got)
		}
	}
	for name, doc := range map[string]map[string]any{
		"malformed pin": {"target": target, "terraform": []any{map[string]any{"descriptor": descriptor, "providerBindings": []any{map[string]any{"kind": "aws", "target": "account", "connectionKey": "", "region": "ap-southeast-1"}}}}},
		"missing pin":   {"target": target, "matches": map[string]any{descriptor: map[string]any{"driverAccount": "aws-node"}}, "terraform": []any{map[string]any{"descriptor": descriptor}}},
	} {
		raw, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		f.exec(`INSERT INTO deployment_plans(deployment_id,document,plan_hash) VALUES($1::uuid,$2::jsonb,'plan-hash') ON CONFLICT(deployment_id) DO UPDATE SET document=EXCLUDED.document`, dep, string(raw))
		_, body, parsed := f.do(f.dev, http.MethodGet, f.resourceURL(f.root.ID), nil)
		assertSafe(t, name, body)
		got := parsed["resource"].(map[string]any)["target"].(map[string]any)
		if got["known"] != false || got["connectionKey"] != nil || got["accountId"] != nil || got["region"] != nil {
			t.Fatalf("%s: an unproven AWS node must have an unknown original target: %v", name, got)
		}
	}
}

// TestAdmissionCommitsRequestedBeforeTheEngineRuns: REQUESTED is durable with
// the claim and the confirmed scope, so the 202, the persisted operation, the
// stored column and a re-read all agree while the engine has not even started.
func TestAdmissionCommitsRequestedBeforeTheEngineRuns(t *testing.T) {
	f := newRCFixture(t)
	f.engine.holdStart = make(chan struct{})
	token := f.previewToken(f.pe)
	status, raw, body := f.deletion(f.pe, token, f.rootName(), f.root.ID, f.a.ID)
	if status != http.StatusAccepted {
		t.Fatalf("deletion = %d %s", status, raw)
	}
	adm := body["cleanup"].(map[string]any)
	id := adm["operationId"].(string)
	if adm["stage"] != "REQUESTED" || adm["status"] != "ACTIVE" {
		t.Fatalf("admission: %v", adm)
	}
	var stage, mode string
	if err := f.conn().QueryRow(context.Background(), `SELECT stage,authorization_mode FROM environment_operations WHERE id=$1::uuid`, id).Scan(&stage, &mode); err != nil {
		t.Fatal(err)
	}
	if stage != "REQUESTED" || mode != "USER_CONFIRMED" || f.engine.Calls() != 1 {
		t.Fatalf("persisted stage %q mode %q engine calls %d", stage, mode, f.engine.Calls())
	}
	// The query endpoint reports the committed stage, not an overridden one.
	_, _, got := f.do(f.dev, http.MethodGet, f.base()+"/resource-cleanups/"+id, nil)
	view := got["cleanup"].(map[string]any)
	if view["stage"] != "REQUESTED" || view["status"] != "ACTIVE" || view["allVerified"] == true {
		t.Fatalf("persisted view: %v", view)
	}
	// A replay reads the same committed stage.
	_, _, again := f.deletion(f.pe, token, f.rootName(), f.root.ID, f.a.ID)
	if r := again["cleanup"].(map[string]any); r["stage"] != "REQUESTED" || r["operationId"] != id || r["replayed"] != true {
		t.Fatalf("replay: %v", r)
	}
	// Reopening the server (a new application over the same database) still sees REQUESTED.
	if err := f.conn().QueryRow(context.Background(), `SELECT stage FROM environment_operations WHERE id=$1::uuid`, id).Scan(&stage); err != nil || stage != "REQUESTED" {
		t.Fatalf("stage after re-read: %q %v", stage, err)
	}
	close(f.engine.holdStart)
	f.waitOperation(f.pe, id)
}

// TestUnconfirmedStateMirrorRefusesDeletionWithoutFalseProof: the committed
// Active Resource says UNREFERENCED but the State projection (the instance
// status, written by the separate State login after the Console transaction)
// was not confirmed, e.g. because the mirror failed. The resource is shown with
// the safe METADATA_SYNC_PENDING refusal; preview and deletion are refused
// before any engine call or operation, nothing is reported ready or verified,
// and the committed Active Resource history is untouched. No repair is attempted.
func TestUnconfirmedStateMirrorRefusesDeletionWithoutFalseProof(t *testing.T) {
	f := newRCFixture(t)
	active := f.linkActive(f.x, "UNREFERENCED") // instance f.x is still READY: the mirror did not land
	_, raw, body := f.do(f.pe, http.MethodGet, f.resourceURL(f.x.ID), nil)
	assertSafe(t, "detail", raw)
	view := body["resource"].(map[string]any)
	elig := view["eligibility"].(map[string]any)
	if elig["eligible"] != false || !contains(elig["reasons"], "METADATA_SYNC_PENDING") || contains(elig["reasons"], "NOT_UNREFERENCED") {
		t.Fatalf("eligibility must name the pending metadata sync: %v", elig)
	}
	if view["status"] == "UNREFERENCED" {
		t.Fatalf("the READY instance must not be reported as UNREFERENCED: %v", view["status"])
	}
	status, _, b := f.preview(f.pe, f.x.ID)
	if status != http.StatusConflict || b["params"].(map[string]any)["reason"] != "METADATA_SYNC_PENDING" {
		t.Fatalf("preview = %d %v", status, b)
	}
	forged := "rd1.4102444800." + strings.Repeat("b", 16) + "." + strings.Repeat("a", 64)
	if status, _, _ := f.deletion(f.pe, forged, f.x.Identity.Descriptor, f.x.ID); status == http.StatusAccepted {
		t.Fatal("a deletion must not be admitted while the State mirror is unconfirmed")
	}
	var operations int
	if err := f.conn().QueryRow(context.Background(), `SELECT count(*) FROM environment_operations WHERE kind='RESOURCE_CLEANUP'`).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if operations != 0 || f.engine.Calls() != 0 {
		t.Fatalf("no operation or engine call may follow: operations=%d engine=%d", operations, f.engine.Calls())
	}
	if got := f.activeStatus(active); got != "UNREFERENCED" {
		t.Fatalf("the committed Active Resource history must be preserved, got %s", got)
	}
}
