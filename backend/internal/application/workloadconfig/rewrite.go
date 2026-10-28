package workloadconfig

import (
	"strings"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/planning/placeholder"
)

// EnvironmentAliases lists the aliases of virtual `environment` resources of a
// raw Score tree. Only these aliases can reference UC-12 keys.
func EnvironmentAliases(raw map[string]any) map[string]bool {
	aliases := map[string]bool{}
	resources, _ := raw["resources"].(map[string]any)
	for alias, spec := range resources {
		if body, ok := spec.(map[string]any); ok && body["type"] == "environment" {
			aliases[alias] = true
		}
	}
	return aliases
}

// ReferencesEnvironmentKey reports whether a Score tree binds key through an
// environment resource, in container variables or resource params.
func ReferencesEnvironmentKey(raw map[string]any, key string) bool {
	found := false
	scanEnvironmentReferences(raw, func(value string, aliases map[string]bool) string {
		rewriteReferences(value, aliases, key, "", &found)
		return value
	}, false)
	return found
}

// RewriteEnvironmentKey returns a copy of a Score tree in which every exact
// `${resources.<environment alias>.<oldKey>}` is renamed to newKey. Aliases,
// container names, variable names and every other field are kept. The input is
// never modified; changed reports whether any reference was renamed.
func RewriteEnvironmentKey(raw map[string]any, oldKey, newKey string) (map[string]any, bool, error) {
	out, err := plainTree(raw)
	if err != nil {
		return nil, false, ErrReferenceRewriteUnsafe
	}
	changed := false
	scanEnvironmentReferences(out, func(value string, aliases map[string]bool) string {
		return rewriteReferences(value, aliases, oldKey, newKey, &changed)
	}, true)
	if !changed {
		return raw, false, nil
	}
	return out, true, nil
}

// scanEnvironmentReferences visits every string of container variables and
// resource params; when write is set the visit result replaces the string.
func scanEnvironmentReferences(raw map[string]any, visit func(string, map[string]bool) string, write bool) {
	aliases := EnvironmentAliases(raw)
	if len(aliases) == 0 {
		return
	}
	var walk func(any) any
	walk = func(value any) any {
		switch t := value.(type) {
		case string:
			return visit(t, aliases)
		case map[string]any:
			for k, child := range t {
				if replaced := walk(child); write {
					t[k] = replaced
				}
			}
		case []any:
			for i, child := range t {
				if replaced := walk(child); write {
					t[i] = replaced
				}
			}
		}
		return value
	}
	if containers, ok := raw["containers"].(map[string]any); ok {
		for _, container := range containers {
			if body, ok := container.(map[string]any); ok {
				if variables, ok := body["variables"].(map[string]any); ok {
					walk(variables)
				}
			}
		}
	}
	if execution, ok := raw["workloadExecution"].(map[string]any); ok {
		if files, ok := execution["secretFiles"].([]any); ok {
			walk(files)
		}
	}
	if resources, ok := raw["resources"].(map[string]any); ok {
		for _, spec := range resources {
			if body, ok := spec.(map[string]any); ok && body["params"] != nil {
				walk(body["params"])
			}
		}
	}
}

// rewriteReferences renames exact environment-key tokens inside one string.
// With an empty newKey it only reports a match. `$${...}` stays literal.
func rewriteReferences(value string, aliases map[string]bool, oldKey, newKey string, changed *bool) string {
	matches := placeholder.Pattern.FindAllStringSubmatchIndex(value, -1)
	if len(matches) == 0 {
		return value
	}
	var out strings.Builder
	last := 0
	for _, m := range matches {
		out.WriteString(value[last:m[0]])
		last = m[1]
		raw := value[m[0]:m[1]]
		segments := strings.Split(strings.TrimSpace(value[m[2]:m[3]]), ".")
		if strings.HasPrefix(raw, "$$") || len(segments) != 3 || segments[0] != "resources" || !aliases[segments[1]] || segments[2] != oldKey {
			out.WriteString(raw)
			continue
		}
		*changed = true
		if newKey == "" {
			out.WriteString(raw)
			continue
		}
		at := strings.LastIndex(raw, oldKey)
		out.WriteString(raw[:at] + newKey + raw[at+len(oldKey):])
	}
	out.WriteString(value[last:])
	return out.String()
}

// ModuleReferencesKey reports whether a deployed module binds the UC-12 key
// through `${context.uc12.<key>}` in a container variable or, directly or via
// a shared resource it references, in a resource param.
func ModuleReferencesKey(module environment.Module, set environment.Document, key string) bool {
	visitedShared := map[string]bool{}
	var found bool
	var visit func(string)
	visit = func(value string) {
		for _, match := range placeholder.Pattern.FindAllStringSubmatch(value, -1) {
			if strings.HasPrefix(match[0], "$$") {
				continue
			}
			segments := strings.Split(strings.TrimSpace(match[1]), ".")
			switch {
			case len(segments) == 3 && segments[0] == "context" && segments[1] == "uc12" && segments[2] == key:
				found = true
			case len(segments) >= 2 && segments[0] == placeholder.ScopeShared && !visitedShared[segments[1]]:
				visitedShared[segments[1]] = true
				if entry, ok := set.Shared[segments[1]]; ok {
					walkStrings(entry.Params, visit)
				}
			}
		}
	}
	for _, container := range module.Spec.Containers {
		for _, value := range container.Variables {
			visit(value)
		}
	}
	for _, entry := range module.Externals {
		walkStrings(entry.Params, visit)
	}
	return found
}
