package http

import (
	"net/http"
	"orchestrator/internal/seed/seedtest"
	"strings"
	"testing"
)

func TestRollbackRoutesRejectForeignScopeAndMalformedConfirmation(t *testing.T) {
	_, server, _ := bundleTestServer(t, nil)
	opts := seedtest.Defaults()
	developer := bundleClient(t, server.URL, "developer")
	foreign := bundleClient(t, server.URL, "other-admin")
	base := server.URL + "/api/v1/applications/" + opts.ApplicationKey + "/environments/staging"
	for _, route := range []string{"/rollback-preview", "/rollback"} {
		status, body := request(t, foreign, http.MethodPost, base+route, `{}`)
		if status != 404 && status != 403 {
			t.Fatalf("foreign rollback allowed: %d %s", status, body)
		}
		status, body = request(t, developer, http.MethodPost, base+route, `{"unrecognized":"private-provider-text"}`)
		if status != 400 || strings.Contains(body, "private-provider-text") {
			t.Fatalf("invalid rollback body accepted or leaked: %d %s", status, body)
		}
	}
	status, body := request(t, developer, http.MethodPost, base+"/rollback", `{"token":"token","confirmation":"wrong"}`)
	if status != 400 || strings.Contains(body, "private-provider-text") {
		t.Fatalf("bad confirmation: %d %s", status, body)
	}
}
