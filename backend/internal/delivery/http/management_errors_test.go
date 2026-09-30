package http

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"orchestrator/internal/application/catalog"
	connectionapp "orchestrator/internal/application/connection"
)

func TestWriteManagementError_ProjectsSafely(t *testing.T) {
	const sentinel = "sentinel-store-or-module-secret"
	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previous)
	for _, tc := range []struct {
		err    error
		status int
		body   string
	}{
		{fmt.Errorf("pq: password=%s", sentinel), http.StatusInternalServerError, "the request failed; retry"},
		{fmt.Errorf("terraform: read %s", sentinel), http.StatusInternalServerError, "the request failed; retry"},
		{fmt.Errorf("%w: resource definition %q", catalog.ErrDuplicate, "pg"), http.StatusConflict, "duplicate"},
		{fmt.Errorf("%w: driver requires an explicit connection", catalog.ErrInvalid), http.StatusBadRequest, "explicit connection"},
		{fmt.Errorf("%w: the cluster API could not be reached with this context", connectionapp.ErrVerification), http.StatusUnprocessableEntity, "could not be reached"},
	} {
		rec := httptest.NewRecorder()
		writeManagementError(rec, tc.err)
		if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.body) || strings.Contains(rec.Body.String(), sentinel) {
			t.Errorf("%v -> %d %s", tc.err, rec.Code, rec.Body.String())
		}
	}
	if strings.Contains(logs.String(), sentinel) {
		t.Fatalf("raw cause logged: %s", logs.String())
	}
}
