package workloadconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning/placeholder"
	"orchestrator/internal/planning/score"
)

var setReference = regexp.MustCompile(`\$\{(externals|shared)\.([A-Za-z0-9_-]+)\.([A-Za-z_][A-Za-z0-9_-]*)\}`)
var configReference = regexp.MustCompile(`\$\{context\.uc12\.([A-Za-z_][A-Za-z0-9_]*)\}`)
var serviceReference = regexp.MustCompile(`\$\{context\.service\.([A-Za-z0-9_-]+)\.([A-Za-z0-9_-]+)\}`)

// ErrReferenceRewriteUnsafe reports a desired-reference change that cannot be
// prepared without losing configuration. Its message is fixed and never quotes
// workload, alias, key or value content.
var ErrReferenceRewriteUnsafe = errors.New("workloadconfig: references cannot be rewritten without losing configuration")

// ReconstructScore exposes only reference-based workload configurations to the
// UC-16 editor. Legacy literals may contain secrets and are never returned.
func ReconstructScore(workload string, module environment.Module, set environment.Document) (map[string]any, error) {
	return reconstruct(workload, module, set, false, nil)
}

// ReconstructScoreProven is ReconstructScore that also returns ordinary
// literals the Deployment Set proves (UC-16 BR-23). Every other literal stays
// hidden: a key name, image or draft never makes an unproven value readable.
func ReconstructScoreProven(workload string, module environment.Module, set environment.Document, proven environment.Prover) (map[string]any, error) {
	return reconstruct(workload, module, set, false, proven)
}

// ReconstructForRedeploy rebuilds a Score for an exact redeploy of an applied
// module (target transitions). Literal bindings are kept verbatim: they are
// already what runs, are never shown to an editor and never leave the backend.
func ReconstructForRedeploy(workload string, module environment.Module, set environment.Document) (map[string]any, error) {
	return reconstruct(workload, module, set, true, nil)
}

// ReconstructVerified rebuilds a reference-based Score and proves it is
// lossless: converting it back must give exactly the deployed module and the
// shared resources it declares, and no shared resource of the set may be
// unreferenced (its owner would be unknowable). Anything else fails with
// ErrReferenceRewriteUnsafe.
func ReconstructVerified(workload string, module environment.Module, set environment.Document, types map[string]resource.Type) (map[string]any, error) {
	return ReconstructVerifiedProven(workload, module, set, types, nil)
}

// ReconstructVerifiedProven is ReconstructVerified for a set that proves some
// ordinary literals, so a rename keeps accepted values and still fails closed
// on an unproven one.
func ReconstructVerifiedProven(workload string, module environment.Module, set environment.Document, types map[string]resource.Type, proven environment.Prover) (map[string]any, error) {
	raw, err := reconstruct(workload, module, set, false, proven)
	if err != nil {
		return nil, ErrReferenceRewriteUnsafe
	}
	plain, err := plainTree(raw)
	if err != nil {
		return nil, ErrReferenceRewriteUnsafe
	}
	if err := verifyRoundTrip(workload, plain, module, set, types, nil); err != nil {
		return nil, err
	}
	return plain, nil
}

// VerifyRewritten proves a rewritten Score converts to the deployed module with
// replace applied to every UC-12 reference, so a rewrite never changes anything
// but the renamed key.
func VerifyRewritten(workload string, rewritten map[string]any, module environment.Module, set environment.Document, types map[string]resource.Type, oldKey, newKey string) error {
	expected := map[string]string{"${context.uc12." + oldKey + "}": "${context.uc12." + newKey + "}"}
	return verifyRoundTrip(workload, rewritten, module, set, types, expected)
}

func verifyRoundTrip(workload string, raw map[string]any, module environment.Module, set environment.Document, types map[string]resource.Type, replace map[string]string) error {
	if orphanShared(set) {
		return fmt.Errorf("%w: shared resource has no known owner", ErrReferenceRewriteUnsafe)
	}
	doc, err := score.FromMap(raw)
	if err != nil {
		return ErrReferenceRewriteUnsafe
	}
	if doc.Metadata.Name != workload {
		return ErrReferenceRewriteUnsafe
	}
	fragment, err := doc.Fragment(types)
	if err != nil {
		return ErrReferenceRewriteUnsafe
	}
	want, err := normalizedModule(module, replace)
	if err != nil {
		return ErrReferenceRewriteUnsafe
	}
	got, err := normalizedModule(fragment.Module, nil)
	if err != nil || !bytes.Equal(want, got) {
		return ErrReferenceRewriteUnsafe
	}
	for id, entry := range fragment.Shared {
		deployed, ok := set.Shared[id]
		if !ok {
			return ErrReferenceRewriteUnsafe
		}
		a, errA := normalizedEntry(deployed, replace)
		b, errB := normalizedEntry(entry, nil)
		if errA != nil || errB != nil || !bytes.Equal(a, b) {
			return ErrReferenceRewriteUnsafe
		}
	}
	return nil
}

// orphanShared reports a shared resource that no module or shared resource
// references: the Deployment Set does not record which workload declared it.
func orphanShared(set environment.Document) bool {
	referenced := map[string]bool{}
	mark := func(value string) {
		for _, match := range placeholder.Pattern.FindAllStringSubmatch(value, -1) {
			if strings.HasPrefix(match[0], "$$") {
				continue
			}
			segments := strings.Split(strings.TrimSpace(match[1]), ".")
			if len(segments) >= 2 && segments[0] == placeholder.ScopeShared {
				referenced[segments[1]] = true
			}
		}
	}
	for _, module := range set.Modules {
		for _, container := range module.Spec.Containers {
			for _, value := range container.Variables {
				mark(value)
			}
		}
		for _, entry := range module.Externals {
			walkStrings(entry.Params, mark)
		}
	}
	for _, entry := range set.Shared {
		walkStrings(entry.Params, mark)
	}
	for id := range set.Shared {
		if !referenced[id] {
			return true
		}
	}
	return false
}

func walkStrings(value any, visit func(string)) {
	switch t := value.(type) {
	case string:
		visit(t)
	case map[string]any:
		for _, child := range t {
			walkStrings(child, visit)
		}
	case []any:
		for _, child := range t {
			walkStrings(child, visit)
		}
	}
}

func normalizedModule(module environment.Module, replace map[string]string) ([]byte, error) {
	copy := module
	copy.Spec.Containers = make(map[string]environment.Container, len(module.Spec.Containers))
	for name, container := range module.Spec.Containers {
		container.ID = name
		copy.Spec.Containers[name] = container
	}
	b, err := json.Marshal(copy)
	if err != nil {
		return nil, err
	}
	return normalizeEntries(b, replace)
}

func normalizedEntry(entry environment.ResourceEntry, replace map[string]string) ([]byte, error) {
	if entry.Class == "" {
		entry.Class = "default"
	}
	b, err := json.Marshal(entry)
	if err != nil {
		return nil, err
	}
	return normalizeEntries(b, replace)
}

// normalizeEntries re-encodes through a plain tree (stable key order) and
// applies the exact-token replacements to every string.
func normalizeEntries(b []byte, replace map[string]string) ([]byte, error) {
	var tree any
	if err := json.Unmarshal(b, &tree); err != nil {
		return nil, err
	}
	if len(replace) > 0 {
		tree = replaceTokens(tree, replace)
	}
	return json.Marshal(tree)
}

func replaceTokens(value any, replace map[string]string) any {
	switch t := value.(type) {
	case string:
		return replaceUnescaped(t, replace)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, v := range t {
			out[k] = replaceTokens(v, replace)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, v := range t {
			out[i] = replaceTokens(v, replace)
		}
		return out
	}
	return value
}

func plainTree(raw map[string]any) (map[string]any, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

type rebuilder struct {
	module        environment.Module
	set           environment.Document
	resources     map[string]any
	sharedAliases map[string]string
}

// declare adds one generated resource alias. A different resource already
// holding the alias (a declared dependency named like a generated alias) is a
// collision and never overwritten.
func (b *rebuilder) declare(alias string, value map[string]any) error {
	if _, external := b.module.Externals[alias]; external {
		return fmt.Errorf("%w: generated resource alias collides with a declared dependency", ErrReferenceRewriteUnsafe)
	}
	if existing, ok := b.resources[alias]; ok {
		if reflect.DeepEqual(existing, value) {
			return nil
		}
		return fmt.Errorf("%w: generated resource alias collides", ErrReferenceRewriteUnsafe)
	}
	b.resources[alias] = value
	return nil
}

// mapString maps the Deployment Set placeholder form back to Score resources.
// mapString maps container variables strictly: any placeholder it cannot map
// is an error. Resource params are mapped leniently (see mapParams).
func (b *rebuilder) mapString(value string) (string, error) { return b.mapText(value, true) }

func (b *rebuilder) mapText(value string, strict bool) (string, error) {
	matches := placeholder.Pattern.FindAllStringSubmatchIndex(value, -1)
	if len(matches) == 0 {
		return value, nil
	}
	var out strings.Builder
	last := 0
	for _, m := range matches {
		out.WriteString(value[last:m[0]])
		last = m[1]
		raw := value[m[0]:m[1]]
		if strings.HasPrefix(raw, "$$") {
			out.WriteString(raw)
			continue
		}
		segments := strings.Split(strings.TrimSpace(value[m[2]:m[3]]), ".")
		replacement, err := b.mapReference(segments, raw, strict)
		if err != nil {
			return "", err
		}
		out.WriteString(replacement)
	}
	out.WriteString(value[last:])
	return out.String(), nil
}

func (b *rebuilder) mapReference(segments []string, raw string, strict bool) (string, error) {
	unsupported := func() (string, error) {
		if strict {
			return "", fmt.Errorf("workloadconfig: unsupported deployed binding")
		}
		return raw, nil // params keep ordinary placeholders (for example ${context.env.namespace}) verbatim, as Score conversion does
	}
	switch segments[0] {
	case placeholder.ScopeExternals:
		if len(segments) < 2 || len(segments) > 3 || segments[1] == "" {
			return unsupported()
		}
		if _, ok := b.module.Externals[segments[1]]; !ok {
			return unsupported()
		}
		return "${resources." + strings.Join(segments[1:], ".") + "}", nil
	case placeholder.ScopeShared:
		if len(segments) < 2 || len(segments) > 3 || segments[1] == "" {
			return unsupported()
		}
		if _, known := b.set.Shared[segments[1]]; !known {
			return unsupported()
		}
		alias, err := b.sharedAlias(segments[1])
		if err != nil {
			return "", err
		}
		return "${resources." + alias + strings.TrimPrefix(strings.Join(segments[1:], "."), segments[1]) + "}", nil
	case "context":
		if len(segments) == 3 && segments[1] == "uc12" {
			if err := b.declare("env", map[string]any{"type": "environment"}); err != nil {
				return "", err
			}
			return "${resources.env." + segments[2] + "}", nil
		}
		if len(segments) == 4 && segments[1] == "service" {
			alias := "svc_" + segments[2] + "_" + segments[3]
			if err := b.declare(alias, map[string]any{"type": "service", "params": map[string]any{"workload": segments[2], "port": segments[3]}}); err != nil {
				return "", err
			}
			return "${resources." + alias + ".url}", nil
		}
		return unsupported()
	case "resources":
		return unsupported()
	}
	return raw, nil // other placeholders pass through Score conversion unchanged
}

func (b *rebuilder) mapTree(value any) (any, error) {
	switch t := value.(type) {
	case string:
		return b.mapText(t, false)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, child := range t {
			mapped, err := b.mapTree(child)
			if err != nil {
				return nil, err
			}
			out[k] = mapped
		}
		return out, nil
	case []any:
		out := make([]any, len(t))
		for i, child := range t {
			mapped, err := b.mapTree(child)
			if err != nil {
				return nil, err
			}
			out[i] = mapped
		}
		return out, nil
	}
	return value, nil
}

func (b *rebuilder) sharedAlias(id string) (string, error) {
	alias := b.sharedAliases[id]
	if alias == "" {
		return "", fmt.Errorf("workloadconfig: unsupported deployed binding")
	}
	if _, done := b.resources[alias]; done {
		return alias, nil
	}
	entry := b.set.Shared[id]
	b.resources[alias] = nil // reserve before mapping params (cycles)
	out, err := b.resourceMap(entry, id)
	if err != nil {
		return "", err
	}
	b.resources[alias] = out
	return alias, nil
}

func (b *rebuilder) resourceMap(entry environment.ResourceEntry, id string) (map[string]any, error) {
	out := map[string]any{"type": entry.Type}
	if entry.Class != "" && entry.Class != "default" {
		out["class"] = entry.Class
	}
	if id != "" {
		out["id"] = id
	}
	if len(entry.Params) > 0 {
		params, err := b.mapTree(entry.Params)
		if err != nil {
			return nil, err
		}
		out["params"] = params
	}
	return out, nil
}

func reconstruct(workload string, module environment.Module, set environment.Document, literals bool, proven environment.Prover) (map[string]any, error) {
	if module.Profile != environment.ModuleProfile {
		return nil, fmt.Errorf("workloadconfig: unsupported deployed workload profile")
	}
	b := &rebuilder{module: module, set: set, resources: map[string]any{}, sharedAliases: map[string]string{}}
	externalAliases := make([]string, 0, len(module.Externals))
	for alias := range module.Externals {
		externalAliases = append(externalAliases, alias)
		b.resources[alias] = nil
	}
	sort.Strings(externalAliases)
	sharedIDs := make([]string, 0, len(set.Shared))
	for id := range set.Shared {
		sharedIDs = append(sharedIDs, id)
	}
	sort.Strings(sharedIDs)
	for i, id := range sharedIDs {
		alias := fmt.Sprintf("shared_%d", i+1)
		for {
			if _, taken := module.Externals[alias]; !taken {
				break
			}
			alias += "_"
		}
		b.sharedAliases[id] = alias
	}
	for _, alias := range externalAliases {
		entry, err := b.resourceMap(module.Externals[alias], "")
		if err != nil {
			return nil, err
		}
		b.resources[alias] = entry
	}
	containers := map[string]any{}
	for name, container := range module.Spec.Containers {
		encoded, err := json.Marshal(container)
		if err != nil {
			return nil, err
		}
		var body map[string]any
		if err := json.Unmarshal(encoded, &body); err != nil {
			return nil, err
		}
		delete(body, "id")
		variables := map[string]any{}
		for key, value := range container.Variables {
			isReference := setReference.FindString(value) == value || configReference.FindString(value) == value || serviceReference.FindString(value) == value
			if !literals && !isReference && !(proven != nil && !hasUnescapedPlaceholder(value) && proven(name, key, value)) {
				return nil, fmt.Errorf("workloadconfig: legacy literal binding cannot be shown in editor")
			}
			mapped, err := b.mapString(value)
			if err != nil {
				return nil, err
			}
			if strings.Contains(mapped, "${externals.") || strings.Contains(mapped, "${shared.") || strings.Contains(mapped, "${context.") {
				return nil, fmt.Errorf("workloadconfig: unsupported deployed binding")
			}
			variables[key] = mapped
		}
		if len(variables) > 0 {
			body["variables"] = variables
		}
		containers[name] = body
	}
	metadata := map[string]any{"name": workload}
	if module.Class != "" {
		metadata["class"] = module.Class // explicit class survives rebuild/export unchanged (ADR-016)
	}
	out := map[string]any{"apiVersion": "score.dev/v1b1", "metadata": metadata, "containers": containers}
	if len(b.resources) > 0 {
		out["resources"] = b.resources
	}
	if module.Spec.Service != nil {
		out["service"] = module.Spec.Service
	}
	if module.Spec.Replicas != nil {
		out["replicas"] = *module.Spec.Replicas
	}
	return out, nil
}

// replaceUnescaped applies exact-token replacements like the real rewrite: a
// placeholder escaped as $${...} stays literal.
func replaceUnescaped(value string, replace map[string]string) string {
	return placeholder.Pattern.ReplaceAllStringFunc(value, func(raw string) string {
		if strings.HasPrefix(raw, "$$") {
			return raw
		}
		if to, ok := replace[raw]; ok {
			return to
		}
		return raw
	})
}
