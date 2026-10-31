package http

import (
	"errors"
	"net/http"
	catalogapp "orchestrator/internal/application/catalogrelease"
	"orchestrator/internal/domain/catalogrelease"
	"orchestrator/internal/ports/persistence"
)

func catalogSourceView(v persistence.CatalogSource) any {
	return struct {
		catalogrelease.Source
		LastChecked any    `json:"lastChecked"`
		Code        string `json:"code"`
	}{v.Source, v.LastChecked, v.Code}
}
func (s *Server) catalogReleaseService() *catalogapp.Service {
	if s.catalogReleases != nil {
		return s.catalogReleases
	}
	return &catalogapp.Service{Store: s.store}
}
func catalogReleaseError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, catalogrelease.ErrInvalid), errors.Is(err, catalogrelease.ErrIncomplete):
		writeCode(w, codeInvalidRequest, "")
	case errors.Is(err, persistence.ErrVersionConflict):
		writeCode(w, codeStaleVersion, "")
	case errors.Is(err, persistence.ErrEnvironmentBusy):
		writeCode(w, codeEnvironmentBusy, "")
	case errors.Is(err, persistence.ErrNotFound):
		writeCode(w, codeNotFound, "")
	case errors.Is(err, catalogrelease.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]any{"code": "CATALOG_CONFLICT"})
	default:
		writeCode(w, codeServiceUnavailable, "")
	}
}
func (s *Server) handleListCatalogSources(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	sources, err := s.store.ListCatalogSources(r.Context(), org)
	if err != nil {
		catalogReleaseError(w, err)
		return
	}
	out := []any{}
	for _, v := range sources {
		out = append(out, catalogSourceView(v))
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": out})
}
func (s *Server) handleSaveCatalogSource(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var d struct {
		AccountBindings    map[string]string `json:"accountBindings"`
		Repository         string            `json:"repository"`
		Branch             string            `json:"branch"`
		TagPattern         string            `json:"tagPattern"`
		ManifestPath       string            `json:"manifestPath"`
		IncludePrereleases bool              `json:"includePrereleases"`
		ExpectedVersion    int64             `json:"expectedVersion"`
	}
	if !decodePreviewBody(w, r, &d) {
		return
	}
	if d.Branch == "" {
		d.Branch = "main"
	}
	if d.TagPattern == "" {
		d.TagPattern = "v*"
	}
	if d.ManifestPath == "" {
		d.ManifestPath = "catalog.yaml"
	}
	source := catalogrelease.Source{AccountBindings: d.AccountBindings, Organization: org, ID: r.PathValue("source"), Repository: d.Repository, Branch: d.Branch, TagPattern: d.TagPattern, ManifestPath: d.ManifestPath, IncludePrereleases: d.IncludePrereleases, Version: d.ExpectedVersion + 1}
	value, err := s.catalogReleaseService().Save(r.Context(), d.ExpectedVersion, source)
	if err != nil {
		catalogReleaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": catalogSourceView(value)})
}
func (s *Server) handleRefreshCatalogSource(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	value, err := s.catalogReleaseService().Refresh(r.Context(), org, r.PathValue("source"))
	if err != nil {
		catalogReleaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": catalogSourceView(value)})
}
func (s *Server) handleListCatalogReleases(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if _, err := s.store.GetCatalogSource(r.Context(), org, r.PathValue("source")); err != nil {
		catalogReleaseError(w, err)
		return
	}
	values, err := s.store.ListCatalogReleases(r.Context(), org, r.PathValue("source"))
	if err != nil {
		catalogReleaseError(w, err)
		return
	}
	usage, err := s.catalogReleaseService().Usage(r.Context(), org, r.PathValue("source"))
	if err != nil {
		catalogReleaseError(w, err)
		return
	}
	out := []any{}
	for _, v := range values {
		out = append(out, map[string]any{"tag": v.Tag, "commit": v.Commit, "branchTip": v.BranchTip, "manifestSha256": v.ManifestSHA256, "observedAt": v.ObservedAt, "fingerprint": v.Fingerprint(), "usage": usageForTag(usage, v.Tag)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"releases": out})
}

func (s *Server) handleGetEnvironmentCatalog(w http.ResponseWriter, r *http.Request) {
	app, env, ok := s.authorizedConfiguration(w, r)
	if !ok {
		return
	}
	value, err := s.store.GetCatalogEnvironment(r.Context(), app, env)
	if err != nil {
		catalogReleaseError(w, err)
		return
	}
	who, _ := s.sessionIdentity(r)
	sources, err := s.store.ListCatalogSources(r.Context(), who.OrganizationKey)
	if err != nil {
		catalogReleaseError(w, err)
		return
	}
	baseline := catalogrelease.Release{}
	if value.Applied != nil {
		baseline, err = s.store.GetCatalogRelease(r.Context(), who.OrganizationKey, value.Applied.SourceID, value.Applied.Tag)
		if err != nil {
			catalogReleaseError(w, err)
			return
		}
	}
	choices := []any{}
	for _, source := range sources {
		releases, err := s.store.ListCatalogReleases(r.Context(), who.OrganizationKey, source.Source.ID)
		if err != nil {
			catalogReleaseError(w, err)
			return
		}
		for _, release := range releases {
			blockers := []string{}
			selection := catalogrelease.Selection{SourceID: release.SourceID, Tag: release.Tag, Fingerprint: release.Fingerprint(), Accounts: source.Source.AccountBindings}
			if _, err := release.Definitions(selection); err != nil {
				blockers = append(blockers, "ACCOUNT_BINDING_REQUIRED")
			}
			if value.Applied != nil && (value.Applied.SourceID != release.SourceID || catalogrelease.IsDowngrade(value.Applied.Tag, release.Tag)) {
				blockers = append(blockers, "DOWNGRADE_UNSUPPORTED")
			}
			choices = append(choices, map[string]any{"sourceId": release.SourceID, "tag": release.Tag, "fingerprint": release.Fingerprint(), "changes": catalogrelease.Compare(baseline, release), "blockers": blockers})
		}
	}
	current, err := s.store.GetEnvironment(r.Context(), app, env)
	if err != nil {
		catalogReleaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"catalog": value, "environmentVersion": current.Version, "releases": choices})
}
func (s *Server) handleSelectEnvironmentCatalog(w http.ResponseWriter, r *http.Request) {
	app, env, ok := s.authorizedConfiguration(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var d struct {
		SourceID                   string `json:"sourceId"`
		Tag                        string `json:"tag"`
		ExpectedEnvironmentVersion int64  `json:"expectedEnvironmentVersion"`
		ExpectedRevision           int64  `json:"expectedRevision"`
	}
	if !decodePreviewBody(w, r, &d) {
		return
	}
	who, _ := s.sessionIdentity(r)
	value, err := s.catalogReleaseService().Select(r.Context(), who.OrganizationKey, app, env, d.SourceID, d.Tag, d.ExpectedEnvironmentVersion, d.ExpectedRevision)
	if err != nil {
		catalogReleaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"catalog": value})
}

func usageForTag(all []catalogapp.Usage, tag string) []catalogapp.Usage {
	out := []catalogapp.Usage{}
	for _, u := range all {
		if u.Tag == tag {
			out = append(out, u)
		}
	}
	return out
}
