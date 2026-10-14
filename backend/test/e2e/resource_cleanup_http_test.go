package e2e

import (
	"context"
	"encoding/base64"
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
	if len(parts) != 3 || parts[0] != "rd1" || len(parts[2]) != 64 {
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
