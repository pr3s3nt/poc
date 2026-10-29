package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"orchestrator/examples/acceptance-app/internal/volume"
)

func TestStorageAPIWritesAndReopensOnlyFixedFile(t *testing.T) {
	dir := t.TempDir()
	mux := http.NewServeMux()
	registerStorage(mux, volume.New(dir, "pod-a"), nil, "")
	request := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	if w := request("POST", "/api/storage", `{"value":"retained"}`); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	mux = http.NewServeMux()
	registerStorage(mux, volume.New(dir, "pod-b"), nil, "")
	w := request("GET", "/api/storage", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"value":"retained"`) || !strings.Contains(w.Body.String(), `"process":"pod-b"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request("POST", "/api/storage", `{"path":"/etc/passwd","value":"bad"}`); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := request("POST", "/api/storage", `{"value":" "}`); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := request("GET", "/api/scheduled-runs", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"configured":false`) {
		t.Fatal(w.Code)
	}
}
