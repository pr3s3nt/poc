package http

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"orchestrator/internal/application/pending"
	"orchestrator/internal/ports/persistence"
)

func TestDraftAndPendingErrorsNeverEchoOrLogStoreDetail(t *testing.T) {
	const sentinel = "sentinel-db-password"
	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previous)

	for name, write := range map[string]func(http.ResponseWriter, error){"draft": (&Server{}).writeDraftError, "pending": writePendingError} {
		for _, tc := range []struct {
			err    error
			status int
		}{
			{fmt.Errorf("pq: password=%s", sentinel), http.StatusInternalServerError},
			{fmt.Errorf("row %s: %w", sentinel, persistence.ErrVersionConflict), http.StatusConflict},
			{fmt.Errorf("row %s: %w", sentinel, persistence.ErrNotFound), http.StatusNotFound},
		} {
			rec := httptest.NewRecorder()
			write(rec, tc.err)
			if rec.Code != tc.status || strings.Contains(rec.Body.String(), sentinel) {
				t.Errorf("%s %v -> %d %s", name, tc.err, rec.Code, rec.Body.String())
			}
		}
	}
	rec := httptest.NewRecorder()
	writePendingError(rec, fmt.Errorf("%w", pending.ErrRouteReconcile))
	if rec.Code != http.StatusBadGateway {
		t.Errorf("route failure -> %d", rec.Code)
	}
	if strings.Contains(logs.String(), sentinel) {
		t.Fatalf("raw error logged: %s", logs.String())
	}
}
