// Package definitionversions resolves immutable authoring and Environment pins.
package definitionversions

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/ports/persistence"
)

var ErrInvalid = errors.New("definition versions: invalid selection")

func ActiveDefinitions(ctx context.Context, st persistence.Store, org string, defs []resource.Definition) ([]resource.Definition, error) {
	out := make([]resource.Definition, 0, len(defs))
	for _, d := range defs {
		h, err := st.GetDefinitionHistory(ctx, org, d.Key)
		if err != nil {
			return nil, err
		}
		if len(h.Versions) > 0 {
			found := false
			for _, v := range h.Versions {
				if v.ID == h.ActiveID {
					d = v.Definition
					found = true
					break
				}
			}
			if !found {
				return nil, persistence.ErrImmutable
			}
		}
		out = append(out, d)
	}
	return out, nil
}

// Resolve keeps Environment policies frozen. A separately selected Catalog release
// may replace its own legacy snapshots; it cannot overwrite authored revision pins.
func Resolve(ctx context.Context, st persistence.Store, app, env string, base, released []resource.Definition, pin string) ([]resource.Definition, error) {
	selected, err := st.GetDefinitionSelection(ctx, app, env)
	if err != nil {
		return nil, err
	}
	if !selected.Frozen {
		return base, nil
	}
	if selected.CatalogPin == pin {
		return legacyRenderPins(ctx, st, app, env, selected.Desired)
	}
	byKey := map[string]resource.Definition{}
	for _, d := range selected.Desired {
		byKey[d.Key] = d
	}
	for _, d := range released {
		if old, ok := byKey[d.Key]; !ok || old.RevisionID == "" {
			byKey[d.Key] = d
		}
	}
	out := []resource.Definition{}
	for _, d := range byKey {
		out = append(out, d)
	}
	resource.SortDefinitions(out)
	return legacyRenderPins(ctx, st, app, env, out)
}

// FreezeOrganization is called before changing an active authoring policy. It
// preserves configuration only and never claims that it was actually deployed.
func FreezeOrganization(ctx context.Context, st persistence.Store, org string, resolve func(context.Context, persistence.Store, string, string, string) ([]resource.Definition, string, error)) error {
	return freezeOrganization(ctx, st, org, resolve, false)
}

// FreezeRunningOrganization captures pre-versioning JSON installations before
// new defaults can affect them. Empty/new Environments remain unselected.
func FreezeRunningOrganization(ctx context.Context, st persistence.Store, org string, resolve func(context.Context, persistence.Store, string, string, string) ([]resource.Definition, string, error)) error {
	return freezeOrganization(ctx, st, org, resolve, true)
}

func freezeOrganization(ctx context.Context, st persistence.Store, org string, resolve func(context.Context, persistence.Store, string, string, string) ([]resource.Definition, string, error), runningOnly bool) error {
	apps, err := st.ListApplications(ctx)
	if err != nil {
		return err
	}
	for _, app := range apps {
		if org != "" && app.OrganizationKey != org {
			continue
		}
		envs, err := st.ListEnvironments(ctx, app.Key)
		if err != nil {
			return err
		}
		for _, env := range envs {
			old, err := st.GetDefinitionSelection(ctx, app.Key, env.Key)
			if err != nil {
				return err
			}
			if old.Frozen {
				continue
			}
			if runningOnly {
				instances, err := st.ListWorkloadInstances(ctx, app.Key+"/"+env.Key)
				if err != nil {
					return err
				}
				running := false
				for _, instance := range instances {
					if instance.LastDeploymentID != "" {
						running = true
						break
					}
				}
				if !running {
					continue
				}
			}
			defs, pin, err := resolve(ctx, st, app.OrganizationKey, app.Key, env.Key)
			if err != nil {
				return err
			}
			if err := st.FreezeDefinitionSelection(ctx, persistence.DefinitionSelection{Frozen: true, ApplicationKey: app.Key, EnvironmentKey: env.Key, CatalogPin: pin, Baseline: defs, Desired: defs}); err != nil {
				return err
			}
		}
	}
	return nil
}

func Record(ctx context.Context, st persistence.Store, org string, def resource.Definition, active bool) (resource.Definition, error) {
	h, err := st.GetDefinitionHistory(ctx, org, def.Key)
	if err != nil {
		return def, err
	}
	expected := h.Generation
	revision := resource.NewDefinitionRevision(def, int64(len(h.Versions)+1))
	h.Versions = append(h.Versions, revision)
	h.Generation++
	h.DefinitionKey = def.Key
	if active || h.ActiveID == "" {
		h.ActiveID = revision.ID
	}
	if err := st.SaveDefinitionHistory(ctx, org, expected, h); err != nil {
		return def, err
	}
	return revision.Definition, nil
}

func SetStatus(ctx context.Context, st persistence.Store, org, key, id string, expected int64, archive bool) error {
	h, err := st.GetDefinitionHistory(ctx, org, key)
	if err != nil {
		return err
	}
	if h.Generation != expected {
		return persistence.ErrVersionConflict
	}
	found := false
	for i, v := range h.Versions {
		if v.ID != id {
			continue
		}
		found = true
		if archive {
			if h.ActiveID == id {
				return ErrInvalid
			}
			h.Versions[i].Archived = true
		} else {
			if v.Archived {
				return ErrInvalid
			}
			h.ActiveID = id
		}
	}
	if !found {
		return persistence.ErrNotFound
	}
	h.Generation++
	return st.SaveDefinitionHistory(ctx, org, expected, h)
}

func Select(ctx context.Context, st persistence.Store, org, app, env, key, id string, expectedEnv, expected int64, defs []resource.Definition, pin string, discard bool) error {
	a, err := st.GetApplication(ctx, app)
	if err != nil {
		return err
	}
	if a.OrganizationKey != org || a.Lifecycle() != "ACTIVE" {
		return persistence.ErrNotFound
	}
	current, err := st.GetDefinitionSelection(ctx, app, env)
	if err != nil {
		return err
	}
	if !current.Frozen {
		current = persistence.DefinitionSelection{Frozen: true, ApplicationKey: app, EnvironmentKey: env, CatalogPin: pin, Baseline: defs, Desired: defs}
	}
	if current.Revision != expected {
		return persistence.ErrVersionConflict
	}
	if discard {
		current.Desired = current.Applied
		if !current.AppliedVerified {
			current.Desired = current.Baseline
		}
	} else {
		h, err := st.GetDefinitionHistory(ctx, org, key)
		if err != nil {
			return err
		}
		var chosen *resource.Definition
		for _, v := range h.Versions {
			if v.ID == id && !v.Archived {
				d := v.Definition
				chosen = &d
				break
			}
		}
		if chosen == nil {
			return ErrInvalid
		}
		next := []resource.Definition{}
		for _, d := range current.Desired {
			if d.Key != key {
				next = append(next, d)
			}
		}
		next = append(next, *chosen)
		resource.SortDefinitions(next)
		current.Desired = next
	}
	current.Revision++
	current.CatalogPin = pin
	return st.SelectDefinitionSelection(ctx, org, expectedEnv, expected, current)
}

// ChangedPaths is a stable, value-free comparison. Values remain in the explicit
// revision detail disclosure; the summary never echoes secret-like literals.
func ChangedPaths(before, after any) []string {
	a, _ := json.Marshal(before)
	b, _ := json.Marshal(after)
	var x, y map[string]any
	json.Unmarshal(a, &x)
	json.Unmarshal(b, &y)
	for _, key := range []string{"version", "revisionId", "sourceFingerprint"} {
		delete(x, key)
		delete(y, key)
	}
	out := []string{}
	var walk func(string, any, any)
	walk = func(prefix string, a, b any) {
		aa, okA := a.(map[string]any)
		bb, okB := b.(map[string]any)
		if okA && okB {
			keys := map[string]bool{}
			for k := range aa {
				keys[k] = true
			}
			for k := range bb {
				keys[k] = true
			}
			for k := range keys {
				p := k
				if prefix != "" {
					p = prefix + "." + k
				}
				walk(p, aa[k], bb[k])
			}
			return
		}
		ra, _ := json.Marshal(a)
		rb, _ := json.Marshal(b)
		if string(ra) != string(rb) {
			out = append(out, prefix)
		}
	}
	walk("", x, y)
	sort.Strings(out)
	return out
}

func legacyRenderPins(ctx context.Context, st persistence.Store, app, env string, defs []resource.Definition) ([]resource.Definition, error) {
	needs := false
	for _, d := range defs {
		if d.DriverType == resource.DriverScoreK8s && d.RenderPin == nil {
			needs = true
			break
		}
	}
	if !needs {
		return defs, nil
	}
	instances, err := st.ListWorkloadInstances(ctx, app+"/"+env)
	if err != nil {
		return nil, err
	}
	pins := map[string]resource.RenderBundle{}
	missingEvidence := false
	for _, instance := range instances {
		if instance.LastDeploymentID == "" {
			continue
		}
		raw, err := st.GetPlan(ctx, instance.LastDeploymentID)
		if errors.Is(err, persistence.ErrNotFound) {
			missingEvidence = true
			continue
		}
		if err != nil {
			return nil, err
		}
		data, _ := json.Marshal(raw)
		var p planning.Plan
		if json.Unmarshal(data, &p) != nil || len(p.Rendering) == 0 {
			missingEvidence = true
			continue
		}
		for _, render := range p.Rendering {
			if old, ok := pins[render.DefinitionKey]; ok {
				a, _ := json.Marshal(old)
				b, _ := json.Marshal(render.Bundle)
				if string(a) != string(b) {
					pins[render.DefinitionKey] = resource.RenderBundle{ID: "unverified"}
					continue
				}
			}
			pins[render.DefinitionKey] = render.Bundle
		}
	}
	out := append([]resource.Definition{}, defs...)
	for i, d := range out {
		if d.DriverType != resource.DriverScoreK8s || d.RenderPin != nil {
			continue
		}
		if missingEvidence {
			out[i].RenderPin = &resource.RenderBundle{ID: "unverified"}
		} else if pin, ok := pins[d.Key]; ok {
			out[i].RenderPin = &pin
		}
	}
	return out, nil
}
