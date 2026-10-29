package e2e

// UC-18 Xóa Application over the real HTTP server, sessions, roles and
// PostgreSQL logins. The cleanup engine is the FAKE scriptedEngine of the
// resource cleanup fixture: these tests prove admission, staging, freeze,
// retry and tombstone semantics, not a live destroy (see the kind/AWS
// acceptance runbook for that).

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/domain/tfresource"
)

func (f *rcFixture) appURL() string { return f.server.URL + "/api/v1/applications/" + f.appKey }

func (f *rcFixture) appDeletionPreview(client *http.Client) (int, string, map[string]any) {
	f.t.Helper()
	return f.do(client, http.MethodPost, f.appURL()+"/deletion-preview", `{}`)
}

func (f *rcFixture) appDeletionToken(client *http.Client) string {
	f.t.Helper()
	status, raw, body := f.appDeletionPreview(client)
	if status != http.StatusOK {
		f.t.Fatalf("application deletion preview = %d %s", status, raw)
	}
	token, _ := body["preview"].(map[string]any)["token"].(string)
	if token == "" {
		f.t.Fatalf("preview without token: %s", raw)
	}
	return token
}

func (f *rcFixture) deleteApplication(client *http.Client, token, name string) (int, string, map[string]any) {
	f.t.Helper()
	return f.do(client, http.MethodPost, f.appURL()+"/deletions", map[string]any{"token": token, "confirmationName": name})
}

// waitAppDeletion polls the persisted progress until it is no longer running.
func (f *rcFixture) waitAppDeletion(client *http.Client, operationID string) map[string]any {
	f.t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		status, raw, body := f.do(client, http.MethodGet, f.appURL()+"/deletions/"+operationID, nil)
		if status != http.StatusOK {
			f.t.Fatalf("deletion progress = %d %s", status, raw)
		}
		view := body["deletion"].(map[string]any)
		if running, _ := view["running"].(bool); !running {
			return view
		}
		time.Sleep(50 * time.Millisecond)
	}
	f.t.Fatal("application deletion did not finish")
	return nil
}

func (f *rcFixture) instanceStatus(id string) tfresource.InstanceStatus {
	f.t.Helper()
	return f.refresh(tfresource.Instance{ID: id}).Status
}

func TestApplicationDeletionPreviewScopeAndRoles(t *testing.T) {
	f := newRCFixture(t)
	status, raw, body := f.appDeletionPreview(f.dev)
	if status != http.StatusOK {
		t.Fatalf("developer preview = %d %s", status, raw)
	}
	preview := body["preview"].(map[string]any)
	if preview["confirmationName"] != "Payments" || preview["blocked"] != false || preview["token"] == "" {
		t.Fatalf("preview = %s", raw)
	}
	staging := map[string]any{}
	for _, raw := range preview["environments"].([]any) {
		env := raw.(map[string]any)
		if env["key"] == "staging" {
			staging = env
		}
	}
	listed := map[string]bool{}
	for _, raw := range staging["resources"].([]any) {
		listed[raw.(map[string]any)["id"].(string)] = true
	}
	for _, in := range []tfresource.Instance{f.root, f.a, f.b, f.c, f.x} {
		if !listed[in.ID] {
			t.Errorf("owned resource %s missing from the preview", in.Identity.Descriptor)
		}
	}
	if listed[f.plat.ID] {
		t.Error("platform cluster listed for deletion")
	}
	if kept := preview["kept"].(map[string]any); kept["platformResources"].(float64) != 1 {
		t.Errorf("kept = %v", kept)
	}
	for _, marker := range []string{privateTarget, privateIntent, privateRecov} {
		if strings.Contains(raw, marker) {
			t.Fatalf("preview leaks %s", marker)
		}
	}
	if status, _, _ := f.appDeletionPreview(f.foreign); status != http.StatusNotFound {
		t.Fatalf("foreign preview = %d", status)
	}
	if status, _, _ := f.deleteApplication(f.dev, f.appDeletionToken(f.dev), "payments"); status != http.StatusBadRequest {
		t.Fatalf("case-folded confirmation = %d", status)
	}
	if status, _, _ := f.deleteApplication(f.dev, "ad1.1.0000000000000000."+strings.Repeat("0", 64), "Payments"); status != http.StatusConflict && status != http.StatusBadRequest {
		t.Fatalf("forged token = %d", status)
	}
	if f.engine.Calls() != 0 {
		t.Fatal("a rejected request reached the engine")
	}
}

func TestApplicationDeletionDestroysOwnedResourcesAndTombstones(t *testing.T) {
	f := newRCFixture(t)
	gate := make(chan struct{})
	f.engine.mu.Lock()
	f.engine.gate = gate
	f.engine.mu.Unlock()

	token := f.appDeletionToken(f.dev)
	status, raw, body := f.deleteApplication(f.dev, token, "Payments")
	if status != http.StatusAccepted {
		t.Fatalf("delete = %d %s", status, raw)
	}
	opID := body["deletion"].(map[string]any)["operationId"].(string)
	if again, _, replay := f.deleteApplication(f.dev, token, "Payments"); again != http.StatusAccepted || replay["deletion"].(map[string]any)["operationId"] != opID {
		t.Fatalf("replay = %d %v", again, replay)
	}

	// Frozen while DELETING: mutations answer 409, reads still work.
	if status, raw, _ := f.do(f.dev, http.MethodPut, f.base()+"/workloads/api", `{"version":0,"score":{}}`); status != http.StatusConflict || !strings.Contains(raw, "APPLICATION_DELETING") {
		t.Fatalf("mutation while deleting = %d %s", status, raw)
	}
	if status, _, app := f.do(f.dev, http.MethodGet, f.appURL(), nil); status != http.StatusOK || app["application"].(map[string]any)["status"] != "DELETING" {
		t.Fatalf("read while deleting = %d %v", status, app)
	}
	close(gate)

	view := f.waitAppDeletion(f.dev, opID)
	if view["status"] != "SUCCEEDED" || view["applicationStatus"] != "DELETED" {
		t.Fatalf("deletion = %v", view)
	}
	for _, in := range []tfresource.Instance{f.root, f.a, f.b, f.c, f.x} {
		if got := f.instanceStatus(in.ID); got != tfresource.InstanceDestroyed {
			t.Errorf("%s = %s", in.Identity.Descriptor, got)
		}
	}
	if got := f.instanceStatus(f.plat.ID); got != tfresource.InstanceReady {
		t.Errorf("platform cluster = %s", got)
	}
	// Tombstone: not listed, not readable, history and state rows retained.
	if status, _, _ := f.do(f.dev, http.MethodGet, f.appURL(), nil); status != http.StatusNotFound {
		t.Fatalf("deleted application read = %d", status)
	}
	if _, raw, _ := f.do(f.dev, http.MethodGet, f.server.URL+"/api/v1/applications", nil); strings.Contains(raw, f.appKey) {
		t.Fatal("deleted application listed")
	}
	if n := f.count(`SELECT count(*) FROM applications WHERE application_key=$1 AND status='DELETED'`, f.appKey); n != 1 {
		t.Fatalf("tombstone rows = %d", n)
	}
	if n := f.count(`SELECT count(*) FROM terraform_resource_instances WHERE id=$1::uuid`, f.root.ID); n != 1 {
		t.Fatal("instance history removed")
	}
	// The name and subdomain are free again.
	if status, raw, _ := f.do(f.pe, http.MethodPost, f.server.URL+"/api/v1/applications", `{"name":"Payments","subdomain":"payments"}`); status != http.StatusCreated {
		t.Fatalf("recreate = %d %s", status, raw)
	}
}

func TestApplicationDeletionBlockedByHeldResource(t *testing.T) {
	f := newRCFixture(t)
	stale := f.appDeletionToken(f.dev)
	f.exec(`UPDATE terraform_resource_instances SET status='NEEDS_ATTENTION' WHERE id=$1::uuid`, f.c.ID)
	status, raw, body := f.appDeletionPreview(f.dev)
	if status != http.StatusOK || body["preview"].(map[string]any)["blocked"] != true || strings.Contains(raw, `"token"`) {
		t.Fatalf("blocked preview = %d %s", status, raw)
	}
	if status, _, _ := f.deleteApplication(f.dev, stale, "Payments"); status != http.StatusConflict {
		t.Fatalf("stale token after block = %d", status)
	}
	if f.engine.Calls() != 0 {
		t.Fatal("blocked deletion reached the engine")
	}
}

func TestApplicationDeletionFailureKeepsDeletingAndRetryCompletes(t *testing.T) {
	f := newRCFixture(t)
	f.engine.mu.Lock()
	f.engine.script[f.b.Identity.Descriptor] = "fail"
	f.engine.mu.Unlock()

	_, _, body := f.deleteApplication(f.pe, f.appDeletionToken(f.pe), "Payments")
	opID := body["deletion"].(map[string]any)["operationId"].(string)
	view := f.waitAppDeletion(f.pe, opID)
	if view["status"] != "FAILED" || view["applicationStatus"] != "DELETING" || view["failureCode"] != "CLEANUP_INCOMPLETE" {
		t.Fatalf("failed deletion = %v", view)
	}
	if got := f.instanceStatus(f.b.ID); got == tfresource.InstanceDestroyed {
		t.Fatal("failed resource reported destroyed")
	}
	// Still frozen and still listed until a Retry succeeds.
	if status, _, _ := f.do(f.pe, http.MethodPost, f.base()+"/preview", `{}`); status != http.StatusConflict {
		t.Fatalf("preview while deleting = %d", status)
	}

	f.engine.mu.Lock()
	delete(f.engine.script, f.b.Identity.Descriptor)
	f.engine.mu.Unlock()
	status, raw, retry := f.deleteApplication(f.pe, f.appDeletionToken(f.pe), "Payments")
	if status != http.StatusAccepted || retry["deletion"].(map[string]any)["operationId"] != opID {
		t.Fatalf("retry = %d %s", status, raw)
	}
	view = f.waitAppDeletion(f.pe, opID)
	if view["status"] != "SUCCEEDED" || view["applicationStatus"] != "DELETED" || view["attempt"].(float64) != 2 {
		t.Fatalf("retried deletion = %v", view)
	}
	if got := f.instanceStatus(f.b.ID); got != tfresource.InstanceDestroyed {
		t.Fatalf("retried resource = %s", got)
	}
}
