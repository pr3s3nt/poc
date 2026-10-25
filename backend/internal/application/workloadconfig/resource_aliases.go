package workloadconfig

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/planning/placeholder"
	"orchestrator/internal/planning/score"
)

// KeepResourceAliases carries display metadata for remaining workloads and
// replaces the deployed workload's metadata from its validated Score. Removed
// workloads and declarations never inherit stale aliases.
func KeepResourceAliases(base environment.ResourceAliases, document environment.Document, after *score.Document) environment.ResourceAliases {
	out := environment.ResourceAliases{}
	for id := range document.Modules {
		if after != nil && id == after.Metadata.Name {
			aliases := environment.WorkloadResourceAliases{Shared: map[string]string{}, References: map[string]string{}}
			for alias, spec := range after.Resources {
				if spec.ID != "" {
					aliases.Shared[alias] = spec.ID
				}
			}
			if len(aliases.Shared) == 0 {
				continue
			}
			raw, _ := json.Marshal(after) // validated typed Score is JSON-serializable
			var tree map[string]any
			_ = json.Unmarshal(raw, &tree)
			walkAliasText(tree, "", func(path, value string) string {
				if aliasBindingPath(path) && canonicalSharedText(value, aliases.Shared) != value {
					aliases.References[path] = value
				}
				return value
			})
			out[id] = aliases
		} else if aliases, ok := base[id]; ok {
			out[id] = aliases
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// canonicalSharedText normalizes only unescaped shared references, so alias
// restoration can prove a saved expression still means exactly the same thing.
func canonicalSharedText(value string, aliases map[string]string) string {
	return placeholder.Pattern.ReplaceAllStringFunc(value, func(raw string) string {
		if strings.HasPrefix(raw, "$$") {
			return raw
		}
		segments := strings.Split(strings.TrimSpace(raw[2:len(raw)-1]), ".")
		if len(segments) < 2 || len(segments) > 3 || segments[0] != "resources" {
			return raw
		}
		if id := aliases[segments[1]]; id != "" {
			return "${shared." + id + strings.TrimPrefix(strings.Join(segments[1:], "."), segments[1]) + "}"
		}
		return raw
	})
}

func restoreResourceAliases(raw map[string]any, aliases environment.WorkloadResourceAliases) error {
	resources, _ := raw["resources"].(map[string]any)
	if len(resources) == 0 || len(aliases.Shared) == 0 {
		return nil
	}
	generated := map[string]string{}
	byID := map[string]map[string]any{}
	for alias, value := range resources {
		spec, _ := value.(map[string]any)
		if id, _ := spec["id"].(string); id != "" {
			generated[alias], byID[id] = id, spec
		}
	}
	names := make([]string, 0, len(aliases.Shared))
	for alias := range aliases.Shared {
		names = append(names, alias)
	}
	sort.Strings(names)
	preferred := map[string]string{}
	for _, alias := range names {
		id := aliases.Shared[alias]
		if byID[id] == nil {
			continue
		}
		if value, exists := resources[alias]; exists {
			spec, _ := value.(map[string]any)
			if spec["id"] != id {
				return ErrReferenceRewriteUnsafe
			}
		}
		if preferred[id] == "" {
			preferred[id] = alias
		}
	}
	for alias, id := range generated {
		if preferred[id] != "" {
			delete(resources, alias)
		}
	}
	for _, alias := range names {
		if spec := byID[aliases.Shared[alias]]; spec != nil {
			copy, err := plainTree(spec)
			if err != nil {
				return ErrReferenceRewriteUnsafe
			}
			resources[alias] = copy
		}
	}
	// First rename generated references everywhere, including nested params.
	walkAliasText(raw, "", func(path string, value string) string {
		if !aliasBindingPath(path) {
			return value
		}
		return placeholder.Pattern.ReplaceAllStringFunc(value, func(ref string) string {
			if strings.HasPrefix(ref, "$$") {
				return ref
			}
			parts := strings.Split(strings.TrimSpace(ref[2:len(ref)-1]), ".")
			if len(parts) >= 2 && parts[0] == "resources" {
				if name := preferred[generated[parts[1]]]; name != "" {
					parts[1] = name
					return "${" + strings.Join(parts, ".") + "}"
				}
			}
			return ref
		})
	})
	current := map[string]string{}
	for alias, value := range resources {
		spec, _ := value.(map[string]any)
		current[alias], _ = spec["id"].(string)
	}
	// Restore a binding's particular alias only when both expressions resolve
	// identically now. This cannot retarget a binding or restore an old literal.
	walkAliasText(raw, "", func(path, value string) string {
		if original, ok := aliases.References[path]; ok && aliasBindingPath(path) && canonicalSharedText(original, current) == canonicalSharedText(value, current) {
			return original
		}
		return value
	})
	return nil
}

// Score expands variables and dependency params, not command/args/image text.
func aliasBindingPath(path string) bool {
	parts := strings.Split(path, "/")
	return len(parts) == 5 && parts[1] == "containers" && parts[3] == "variables" ||
		len(parts) >= 5 && parts[1] == "resources" && parts[3] == "params"
}

// Paths use JSON Pointer escaping, including arbitrary nested param keys.
func walkAliasText(value any, path string, visit func(string, string) string) {
	part := func(key string) string { return strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1") }
	switch tree := value.(type) {
	case map[string]any:
		for key, child := range tree {
			next := path + "/" + part(key)
			if text, ok := child.(string); ok {
				tree[key] = visit(next, text)
			} else {
				walkAliasText(child, next, visit)
			}
		}
	case []any:
		for i, child := range tree {
			next := fmt.Sprintf("%s/%d", path, i)
			if text, ok := child.(string); ok {
				tree[i] = visit(next, text)
			} else {
				walkAliasText(child, next, visit)
			}
		}
	}
}
