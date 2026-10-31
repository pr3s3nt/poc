package registry

import (
	"context"
	"orchestrator/internal/domain/catalogrelease"
	"orchestrator/internal/domain/renderbundle"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/catalogsource"
	"orchestrator/internal/ports/gitaccess"
	"orchestrator/internal/ports/moduleregistry"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/renderregistry"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// CatalogInspector prepares caches using the existing hardened inspectors. No
// global Definition is registered and no provider is invoked.
type CatalogDefinitionValidator interface {
	PrepareReleaseDefinition(context.Context, string, resource.Definition) (resource.Definition, error)
}

type CatalogInspector struct {
	Store       persistence.Store
	Definitions CatalogDefinitionValidator
	Git         *GitCLI
	Modules     moduleregistry.GitInspector
	Rendering   renderregistry.Registry
}

func (i CatalogInspector) InspectRelease(ctx context.Context, source catalogrelease.Source, tag catalogsource.Tag, observation catalogsource.Observation) (catalogrelease.Release, error) {
	if source.Validate() != nil || !tag.InBranch || !catalogrelease.ValidCommit(tag.Commit) || !catalogrelease.ValidCommit(observation.BranchTip) || observation.ObservedAt.IsZero() {
		return catalogrelease.Release{}, catalogrelease.ErrInvalid
	}
	if i.Git == nil || i.Modules == nil || i.Definitions == nil {
		return catalogrelease.Release{}, catalogrelease.ErrUnavailable
	}
	ctx = gitaccess.WithOrganization(ctx, source.Organization)
	snapshot, err := i.Git.Open(ctx, GitSource{URL: source.Repository}, tag.Commit)
	if err != nil {
		return catalogrelease.Release{}, catalogrelease.ErrUnavailable
	}
	defer snapshot.Close()
	snap, ok := snapshot.(*gitSnapshot)
	if !ok {
		return catalogrelease.Release{}, catalogrelease.ErrUnavailable
	}
	snap.outputLimit = catalogrelease.MaxManifestBytes + 4096
	raw, err := snap.catalogFile(source.ManifestPath)
	if err != nil {
		return catalogrelease.Release{}, catalogrelease.ErrInvalid
	}
	manifest, err := catalogrelease.ParseManifest(raw)
	if err != nil {
		return catalogrelease.Release{}, err
	}
	result := catalogrelease.Release{Organization: source.Organization, SourceID: source.ID, Tag: tag.Name, Commit: tag.Commit, BranchTip: observation.BranchTip, Manifest: raw, ManifestSHA256: catalogrelease.Digest(raw), ObservedAt: observation.ObservedAt}
	previous := []catalogrelease.Release{}
	if i.Store != nil {
		previous, err = i.Store.ListCatalogReleases(ctx, source.Organization, source.ID)
		if err != nil {
			return catalogrelease.Release{}, catalogrelease.ErrUnavailable
		}
	}
	sort.Slice(previous, func(a, b int) bool { return previous[a].Tag < previous[b].Tag })
	modules := map[string]tfmodule.Bundle{}
	renders := map[string]renderbundle.Bundle{}
	for _, c := range manifest.Components {
		v := catalogrelease.VerifiedComponent{Component: c}
		switch c.Kind {
		case catalogrelease.TerraformModule:
			b, err := i.Modules.InspectGit(ctx, moduleregistry.GitInspection{URL: source.Repository, Path: c.Path, Ref: tag.Commit, ResourceType: c.ResourceType})
			if err != nil {
				return catalogrelease.Release{}, catalogrelease.ErrInvalid
			}
			if i.Store != nil {
				for _, release := range previous {
					for _, component := range release.Components {
						if component.ID == c.ID && component.Kind == c.Kind && component.SHA256 == b.Ref.ContentSHA256 {
							old, err := i.Store.GetModuleBundleByID(ctx, component.BundleID)
							if err != nil {
								return catalogrelease.Release{}, catalogrelease.ErrUnavailable
							}
							if reuseCatalogModule(old, b) {
								b = old
							}
						}
					}
				}
			}
			modules[c.ID] = b
			v.BundleID = b.ID
			v.SHA256 = b.Ref.ContentSHA256
		case catalogrelease.RenderBundle:
			if i.Rendering == nil {
				return catalogrelease.Release{}, catalogrelease.ErrUnavailable
			}
			b, err := i.Rendering.Inspect(ctx, source.Organization, renderregistry.Inspection{URL: source.Repository, Path: c.Path, Ref: tag.Commit})
			if err != nil {
				return catalogrelease.Release{}, catalogrelease.ErrInvalid
			}
			if repo, ok := i.Store.(persistence.RenderBundleRepository); ok {
				for _, release := range previous {
					for _, component := range release.Components {
						if component.ID == c.ID && component.Kind == c.Kind && component.SHA256 == b.Source.ContentSHA256 {
							old, err := repo.GetRenderBundle(ctx, source.Organization, component.BundleID)
							if err != nil {
								return catalogrelease.Release{}, catalogrelease.ErrUnavailable
							}
							normalized := b
							normalized.Source.Revision = old.Source.Revision
							if old.SameVerifiedContent(normalized) {
								b = old
							}
						}
					}
				}
			}
			renders[c.ID] = b
			v.BundleID = b.ID
			v.SHA256 = b.Source.ContentSHA256
		default:
			continue
		}
		result.Components = append(result.Components, v)
	}
	keys := map[string]bool{}
	for _, c := range manifest.Components {
		if c.Kind != catalogrelease.Definition {
			continue
		}
		raw, err := snap.catalogFile(c.Path)
		if err != nil {
			return catalogrelease.Release{}, catalogrelease.ErrInvalid
		}
		doc, err := catalogrelease.ParseDefinition(raw)
		if err != nil || keys[doc.Definition.Key] {
			return catalogrelease.Release{}, catalogrelease.ErrInvalid
		}
		keys[doc.Definition.Key] = true
		d := doc.Definition
		switch d.DriverType {
		case resource.DriverTerraform:
			m, ok := modules[doc.Component]
			if !ok || !slices.Contains(m.Contract.ResourceTypes, d.ResourceTypeKey) {
				return catalogrelease.Release{}, catalogrelease.ErrInvalid
			}
			if d.DriverInputs == nil {
				d.DriverInputs = map[string]any{}
			}
			if d.DriverInputs["values"] == nil {
				d.DriverInputs["values"] = map[string]any{}
			}
			d.DriverValues()["source"] = m.Ref.Map()
			if _, err := tfmodule.ParseDriverInputs(d.DriverValues()); err != nil {
				return catalogrelease.Release{}, catalogrelease.ErrInvalid
			}
		case resource.DriverScoreK8s:
			b, ok := renders[doc.Component]
			if !ok {
				return catalogrelease.Release{}, catalogrelease.ErrInvalid
			}
			values := d.DriverValues()
			if len(values) == 0 {
				return catalogrelease.Release{}, catalogrelease.ErrInvalid
			}
			variables, ok := values["variables"].(map[string]any)
			if !ok {
				return catalogrelease.Release{}, catalogrelease.ErrInvalid
			}
			variables[resource.VariableRenderBundle] = b.ID
		}
		if doc.Binding != "" {
			d.DriverAccount = source.AccountBindings[doc.Binding]
			if d.DriverAccount == "" {
				return catalogrelease.Release{}, catalogrelease.ErrInvalid
			}
		}
		prepared, err := i.Definitions.PrepareReleaseDefinition(ctx, source.Organization, d)
		if err != nil {
			return catalogrelease.Release{}, catalogrelease.ErrInvalid
		}
		prepared.DriverAccount = ""  // platform binding remains outside the immutable snapshot
		prepared.SourceFingerpr = "" // materialized validation evidence is recomputed by planner
		d = prepared
		result.Components = append(result.Components, catalogrelease.VerifiedComponent{Component: c, SHA256: catalogrelease.Digest(raw), Snapshot: &d, Binding: doc.Binding})
	}
	if err := result.Validate(); err != nil {
		return catalogrelease.Release{}, err
	}
	return result, nil
}
func (s *gitSnapshot) catalogFile(name string) ([]byte, error) {
	if !catalogrelease.ValidPath(name) {
		return nil, catalogrelease.ErrInvalid
	}
	out, err := s.git("ls-tree", "-z", "refs/pinned", "--", name)
	if err != nil {
		return nil, err
	}
	entry := strings.TrimSuffix(string(out), "\x00")
	parts := strings.SplitN(entry, "\t", 2)
	if len(parts) != 2 || parts[1] != name {
		return nil, catalogrelease.ErrInvalid
	}
	meta := strings.Fields(parts[0])
	if len(meta) != 3 || meta[0] != "100644" || meta[1] != "blob" || !catalogrelease.ValidCommit(meta[2]) {
		return nil, catalogrelease.ErrInvalid
	}
	size, err := s.git("cat-file", "-s", meta[2])
	if err != nil {
		return nil, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(size)))
	if err != nil || n > catalogrelease.MaxManifestBytes {
		return nil, catalogrelease.ErrInvalid
	}
	return s.git("cat-file", "blob", meta[2])
}

var _ catalogsource.Inspector = CatalogInspector{}

// reuseModule keeps the old verified execution identity when content and every
// transitive/release contract are unchanged. Observation still records the new
// catalog commit; this avoids updating Redis merely because PostgreSQL changed.
func reuseCatalogModule(old, new tfmodule.Bundle) bool {
	if old.Ref.SourceID != new.Ref.SourceID || old.Ref.ModuleID != new.Ref.ModuleID || old.Ref.Subdirectory != new.Ref.Subdirectory || old.Ref.ContentSHA256 != new.Ref.ContentSHA256 {
		return false
	}
	normalized := new.Clone()
	normalized.Ref = old.Ref
	for j := range normalized.Contract.Transitive {
		t := &normalized.Contract.Transitive[j]
		if t.SourceID == new.Ref.SourceID && t.Revision == new.Ref.Revision {
			t.Revision = old.Ref.Revision
		}
	}
	return old.SameVerifiedContent(normalized)
}
