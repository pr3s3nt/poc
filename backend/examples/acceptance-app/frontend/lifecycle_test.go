package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLifecycleErrorsVisibleAndPublicTestValuesEscaped(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/storage" {
			_ = json.NewEncoder(w).Encode(map[string]any{"configured": true, "present": true, "value": "<script>test</script>", "sha256": "digest", "process": "pod-a"})
			return
		}
		w.WriteHeader(503)
	}))
	defer s.Close()
	view := readLifecycle(context.Background(), s.Client(), s.URL)
	if view.StorageError || !view.ScheduleError {
		t.Fatalf("%+v", view)
	}
	var output bytes.Buffer
	if err := page.Execute(&output, pageData{Lifecycle: view}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "<script>test</script>") || !strings.Contains(output.String(), "Scheduled run history unavailable") || !strings.Contains(output.String(), "&lt;script&gt;") {
		t.Fatal(output.String())
	}
}
