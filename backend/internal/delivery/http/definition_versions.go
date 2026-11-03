package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"orchestrator/internal/planning"
	"sort"

	catalogapp "orchestrator/internal/application/catalogrelease"
	versions "orchestrator/internal/application/definitionversions"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/persistence"
)

func (s *Server) handleDefinitionVersions(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	h, err := s.store.GetDefinitionHistory(r.Context(), org, r.PathValue("key"))
	if err != nil {
		s.writeError(w, err)
		return
	}
	if len(h.Versions) == 0 {
		defs, err := s.store.ListResourceDefinitions(r.Context(), org)
		if err != nil {
			s.writeError(w, err)
			return
		}
		found := false
		for _, d := range defs {
			if d.Key == r.PathValue("key") {
				found = true
			}
		}
		if !found {
			writeCode(w, codeNotFound, "")
			return
		}
	}
	usage := []any{}
	apps, err := s.store.ListApplications(r.Context())
	if err != nil {
		s.writeError(w, err)
		return
	}
	for _, app := range apps {
		if app.OrganizationKey != org {
			continue
		}
		envs, err := s.store.ListEnvironments(r.Context(), app.Key)
		if err != nil {
			s.writeError(w, err)
			return
		}
		for _, env := range envs {
			sel, err := s.store.GetDefinitionSelection(r.Context(), app.Key, env.Key)
			if err != nil {
				s.writeError(w, err)
				return
			}
			desired, applied := "", ""
			pinned := false
			for _, d := range sel.Desired {
				if d.Key == h.DefinitionKey {
					desired = d.RevisionID
					pinned = true
				}
			}
			for _, d := range sel.Applied {
				if d.Key == h.DefinitionKey {
					applied = d.RevisionID
				}
			}
			actual, used, err := appliedDefinitionUsage(r.Context(), s.store, org, app.Key, env.Key)
			if err != nil {
				s.writeError(w, err)
				return
			}
			if pinned || applied != "" || used[h.DefinitionKey] {
				usage = append(usage, map[string]any{"applicationKey": app.Key, "applicationName": app.Name, "environment": env.Key, "desiredRevisionId": desired, "appliedRevisionId": applied, "appliedVerified": sel.AppliedVerified, "pending": sel.Pending(), "inUse": used[h.DefinitionKey], "actualRevisionIds": actual[h.DefinitionKey]})
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"history": h, "usage": usage})
}
func (s *Server) handleDefinitionRevisionAction(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	var body struct {
		ExpectedGeneration int64 `json:"expectedGeneration"`
	}
	if !decodePreviewBody(w, r, &body) {
		return
	}
	err := s.catalog.ChangeRevisionStatus(r.Context(), org, r.PathValue("key"), r.PathValue("revision"), body.ExpectedGeneration, r.PathValue("action") == "archive")
	if err != nil {
		if errors.Is(err, versions.ErrInvalid) || errors.Is(err, persistence.ErrImmutable) {
			writeCode(w, codeInvalidRequest, "")
		} else {
			s.writeError(w, err)
		}
		return
	}
	h, err := s.store.GetDefinitionHistory(r.Context(), org, r.PathValue("key"))
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"history": h})
}

func definitionByKey(defs []resource.Definition, key string) *resource.Definition {
	for _, d := range defs {
		if d.Key == key {
			return &d
		}
	}
	return nil
}
func (s *Server) handleEnvironmentDefinitionVersions(w http.ResponseWriter, r *http.Request) {
	app, env, ok := s.authorizedConfiguration(w, r)
	if !ok {
		return
	}
	who, _ := s.sessionIdentity(r)
	w.Header().Set("Cache-Control", "no-store")
	var response map[string]any
	err := s.store.ReadSnapshot(r.Context(), func(ctx context.Context, view persistence.Store) error {
		current, err := view.GetEnvironment(ctx, app, env)
		if err != nil {
			return err
		}
		sel, err := view.GetDefinitionSelection(ctx, app, env)
		if err != nil {
			return err
		}
		desired, _, err := catalogapp.ResolveDefinitions(ctx, view, who.OrganizationKey, app, env)
		if err != nil {
			return err
		}
		heads, err := view.ListResourceDefinitions(ctx, who.OrganizationKey)
		if err != nil {
			return err
		}
		keys := map[string]bool{}
		for _, d := range desired {
			keys[d.Key] = true
		}
		for _, d := range heads {
			keys[d.Key] = true
		}
		ordered := []string{}
		for k := range keys {
			ordered = append(ordered, k)
		}
		sort.Strings(ordered)
		actual, used, err := appliedDefinitionUsage(ctx, view, who.OrganizationKey, app, env)
		if err != nil {
			return err
		}
		rows := []any{}
		for _, key := range ordered {
			h, err := view.GetDefinitionHistory(ctx, who.OrganizationKey, key)
			if err != nil {
				return err
			}
			d := definitionByKey(desired, key)
			a := definitionByKey(sel.Applied, key)
			choices := []any{}
			desiredID, appliedID, typ := "", "", ""
			if d != nil {
				desiredID = d.RevisionID
				typ = d.ResourceTypeKey
			}
			if a != nil {
				appliedID = a.RevisionID
			}
			for _, v := range h.Versions {
				if typ == "" {
					typ = v.Definition.ResourceTypeKey
				}
				choices = append(choices, map[string]any{"id": v.ID, "number": v.Number, "active": v.ID == h.ActiveID, "archived": v.Archived, "changes": versions.ChangedPaths(d, v.Definition)})
			}
			rows = append(rows, map[string]any{"key": key, "resourceType": typ, "desiredRevisionId": desiredID, "appliedRevisionId": appliedID, "versions": choices, "inUse": used[key], "actualRevisionIds": actual[key]})
		}
		// Environment API returns policy references and value-free diffs, never global driver inputs.
		response = map[string]any{"revision": sel.Revision, "appliedRevision": sel.AppliedRevision, "pending": sel.Pending(), "frozen": sel.Frozen, "appliedVerified": sel.AppliedVerified, "environmentVersion": current.Version, "definitions": rows}
		return nil
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}
func (s *Server) handleSelectDefinitionVersions(w http.ResponseWriter, r *http.Request) {
	app, env, ok := s.authorizedConfiguration(w, r)
	if !ok {
		return
	}
	who, _ := s.sessionIdentity(r)
	var body struct {
		DefinitionKey              string `json:"definitionKey"`
		RevisionID                 string `json:"revisionId"`
		ExpectedRevision           int64  `json:"expectedRevision"`
		ExpectedEnvironmentVersion int64  `json:"expectedEnvironmentVersion"`
	}
	if !decodePreviewBody(w, r, &body) {
		return
	}
	err := s.store.Transact(r.Context(), func(ctx context.Context) error {
		defs, pin, err := catalogapp.ResolveDefinitions(ctx, s.store, who.OrganizationKey, app, env)
		if err != nil {
			return err
		}
		return versions.Select(ctx, s.store, who.OrganizationKey, app, env, body.DefinitionKey, body.RevisionID, body.ExpectedEnvironmentVersion, body.ExpectedRevision, defs, pin, r.PathValue("action") == "discard")
	})
	if err != nil {
		if errors.Is(err, versions.ErrInvalid) {
			writeCode(w, codeInvalidRequest, "")
		} else {
			catalogReleaseError(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": true})
}

// Applied revision evidence comes from the last acknowledged Plans, not from a
// policy that merely lists possible Definitions for an Environment.
func appliedDefinitionUsage(ctx context.Context, st persistence.Store, org, app, env string) (map[string][]string, map[string]bool, error) {
	ids := map[string]map[string]bool{}
	used := map[string]bool{}
	include := func(key, id string) {
		used[key] = true
		if id != "" {
			if ids[key] == nil {
				ids[key] = map[string]bool{}
			}
			ids[key][id] = true
		}
	}
	plans := map[string]*planning.Plan{}
	read := func(id string) (*planning.Plan, error) {
		if p, ok := plans[id]; ok {
			return p, nil
		}
		raw, err := st.GetPlan(ctx, id)
		if errors.Is(err, persistence.ErrNotFound) {
			plans[id] = nil
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		data, _ := json.Marshal(raw)
		var p planning.Plan
		if json.Unmarshal(data, &p) != nil {
			plans[id] = nil
			return nil, nil
		}
		plans[id] = &p
		return &p, nil
	}
	active, err := st.ListActiveResources(ctx, org)
	if err != nil {
		return nil, nil, err
	}
	for _, a := range active {
		if a.Status == resource.StatusDestroyed || a.LastDeploymentID == "" {
			continue
		}
		d, err := st.GetDeployment(ctx, a.LastDeploymentID)
		if errors.Is(err, persistence.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		if d.ApplicationKey != app || d.EnvironmentKey != env {
			continue
		}
		p, err := read(a.LastDeploymentID)
		if err != nil {
			return nil, nil, err
		}
		revision := ""
		if p != nil {
			for _, def := range p.DefinitionSnapshots {
				if def.Key == a.DefinitionKey {
					revision = def.RevisionID
					break
				}
			}
		}
		include(a.DefinitionKey, revision)
	}
	instances, err := st.ListWorkloadInstances(ctx, app+"/"+env)
	if err != nil {
		return nil, nil, err
	}
	for _, instance := range instances {
		if instance.LastDeploymentID == "" || string(instance.Status) == "REMOVED" {
			continue
		}
		p, err := read(instance.LastDeploymentID)
		if err != nil {
			return nil, nil, err
		}
		if p == nil {
			continue
		}
		for _, render := range p.Rendering {
			revision := ""
			for _, def := range p.DefinitionSnapshots {
				if def.Key == render.DefinitionKey {
					revision = def.RevisionID
					break
				}
			}
			include(render.DefinitionKey, revision)
		}
	}
	out := map[string][]string{}
	for key, versions := range ids {
		for id := range versions {
			out[key] = append(out[key], id)
		}
		sort.Strings(out[key])
	}
	return out, used, nil
}
