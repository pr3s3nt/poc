package http

import (
	"net/http"
	"orchestrator/internal/domain/application"
)

func (s *Server) handleSetupProgress(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	git, err := s.gitAccess.Get(r.Context(), org)
	if err != nil {
		s.writeError(w, err)
		return
	}
	accounts, err := s.store.ListConnections(r.Context(), org)
	if err != nil {
		s.writeError(w, err)
		return
	}
	stores, err := s.store.ListSecretStores(r.Context(), org)
	if err != nil {
		s.writeError(w, err)
		return
	}
	types, err := s.store.ListResourceTypes(r.Context(), org)
	if err != nil {
		s.writeError(w, err)
		return
	}
	defs, err := s.store.ListResourceDefinitions(r.Context(), org)
	if err != nil {
		s.writeError(w, err)
		return
	}
	kube, vault, cd := false, false, false
	for _, a := range accounts {
		if a.Kind == application.ConnectionKubernetes && a.Status == "READY" {
			kube = true
		}
	}
	for _, v := range stores {
		if v.Status == "READY" {
			vault = true
		}
	}
	if s.delivery != nil {
		bindings, err := s.delivery.List(r.Context(), org)
		if err != nil {
			s.writeError(w, err)
			return
		}
		for _, b := range bindings {
			if b.Status == "READY" {
				cd = true
			}
		}
	}
	writeJSON(w, 200, map[string]any{"git": git.URL != "", "kubernetes": kube, "vault": vault, "cd": cd, "types": len(types), "definitions": len(defs)})
}
