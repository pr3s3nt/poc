package http

import (
	"context"
	"net/http"
	"orchestrator/internal/application/catalog"
	"time"
)

// SetupDiscovery reads bounded, exact Git snapshots using platform Git access.
type SetupDiscovery interface {
	DiscoverSetup(context.Context, string, string, string, string) (any, error)
}

func (s *Server) handleDiscoverSetup(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	reader, ok := s.gitTester.(SetupDiscovery)
	if !ok {
		writeCode(w, codeServiceUnavailable, "")
		return
	}
	var d struct {
		URL  string `json:"url"`
		Ref  string `json:"ref"`
		Path string `json:"path"`
	}
	if !decodeStrict(w, r, 16<<10, &d) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	result, err := reader.DiscoverSetup(ctx, org, d.URL, d.Ref, d.Path)
	if err != nil {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) handlePreviewSetup(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	var d struct {
		Documents []catalog.SetupDocument `json:"documents"`
	}
	if !decodeStrict(w, r, 8<<20, &d) {
		return
	}
	if len(d.Documents) == 0 || len(d.Documents) > 100 {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	result, err := s.catalog.PreviewSetup(ctx, org, d.Documents)
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"documents": result})
}
