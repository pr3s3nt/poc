package http

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"orchestrator/internal/application/preview"
	"orchestrator/internal/ports/persistence"
)

func TestWriteScorePreviewError_NeverEchoesOrLogsInternalDetail(t *testing.T) {
	const sentinel = "sentinel-store-password"
	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previous)

	cases := []struct {
		err    error
		status int
		body   string
	}{
		{fmt.Errorf("store: dial password=%s", sentinel), http.StatusInternalServerError, "preview failed; retry"},
		{fmt.Errorf("lookup %s: %w", sentinel, persistence.ErrNotFound), http.StatusNotFound, "not found"},
		{&preview.PublicError{Kind: preview.ErrPlanningRejected, Message: "fixed sentence"}, http.StatusBadRequest, "fixed sentence"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		writeScorePreviewError(rec, tc.err)
		if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.body) || strings.Contains(rec.Body.String(), sentinel) {
			t.Fatalf("%v -> %d %s", tc.err, rec.Code, rec.Body.String())
		}
	}
	if strings.Contains(logs.String(), sentinel) {
		t.Fatalf("raw error logged: %s", logs.String())
	}
	if !strings.Contains(logs.String(), "score preview failed: internal error") {
		t.Fatalf("internal failure not logged generically: %q", logs.String())
	}
	if errors.Is(cases[2].err, preview.ErrInvalidScore) {
		t.Fatal("public error kind leaked across kinds")
	}
}
